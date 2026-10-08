package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authStore interface {
	RegisterTrainerEmailPassword(ctx context.Context, email, passwordHash, displayName string) (uuid.UUID, error)
	getEmailPasswordIdentity(ctx context.Context, email string) (emailIdentityRow, error)
	FindOrCreateClientByIdentity(ctx context.Context, id ClientIdentity) (uuid.UUID, error)
	InsertRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error
	RotateRefreshSession(ctx context.Context, oldHash, newHash []byte, newExpiresAt time.Time) (uuid.UUID, string, error)
	RevokeRefreshSession(ctx context.Context, tokenHash []byte) error
	RevokeAllUserRefreshSessions(ctx context.Context, userID uuid.UUID) error
	UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error)
	UpdateUserDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error
}

type Service struct {
	store      authStore
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration

	googleVerifier IDTokenVerifier
	appleVerifier  IDTokenVerifier
}

type IssuedAuth struct {
	AccessToken   string
	AccessExpires time.Time
	RefreshToken  string
	UserID        uuid.UUID
	Email         string
}

// ServiceOption configures optional Service features.
type ServiceOption func(*Service)

// WithGoogleVerifier enables SignInWithGoogle.
func WithGoogleVerifier(v IDTokenVerifier) ServiceOption {
	return func(s *Service) { s.googleVerifier = v }
}

// WithAppleVerifier enables SignInWithApple.
func WithAppleVerifier(v IDTokenVerifier) ServiceOption {
	return func(s *Service) { s.appleVerifier = v }
}

func NewService(pool *pgxpool.Pool, jwtSecret string, accessTTL, refreshTTL time.Duration, opts ...ServiceOption) *Service {
	s := &Service{
		store:      NewStore(pool),
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func normalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

const (
	maxDisplayNameLen = 100
	maxEmailLen       = 254
)

// ValidateDisplayName rejects names longer than maxDisplayNameLen runes after trimming.
func ValidateDisplayName(name string) error {
	if utf8.RuneCountInString(normalizeDisplayName(name)) > maxDisplayNameLen {
		return fmt.Errorf("name must be at most %d characters", maxDisplayNameLen)
	}
	return nil
}

func (s *Service) RegisterTrainer(ctx context.Context, email, password, name string) (IssuedAuth, error) {
	var out IssuedAuth
	if err := ValidatePassword(password); err != nil {
		return out, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return out, fmt.Errorf("hash password: %w", err)
	}
	userID, err := s.store.RegisterTrainerEmailPassword(ctx, email, hash, normalizeDisplayName(name))
	if err != nil {
		return out, err
	}
	plain, h, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	if err := s.store.InsertRefreshSession(ctx, userID, h, expiresAt); err != nil {
		return out, fmt.Errorf("session: %w", err)
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = plain
	out.UserID = userID
	out.Email = email
	return out, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (IssuedAuth, error) {
	var out IssuedAuth
	row, err := s.store.getEmailPasswordIdentity(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, ErrInvalidCredentials
		}
		return out, fmt.Errorf("load identity: %w", err)
	}
	if row.PasswordHash == "" {
		return out, ErrInvalidCredentials
	}
	ok, err := PasswordMatches(row.PasswordHash, password)
	if err != nil {
		return out, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return out, ErrInvalidCredentials
	}
	plain, h, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	if err := s.store.InsertRefreshSession(ctx, row.UserID, h, expiresAt); err != nil {
		return out, fmt.Errorf("session: %w", err)
	}
	token, exp, err := signAccessToken(row.UserID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = plain
	out.UserID = row.UserID
	out.Email = email
	return out, nil
}

// SignInWithGoogle verifies a Google ID token and signs in the matching client,
// creating the account on the first sign-in.
func (s *Service) SignInWithGoogle(ctx context.Context, rawIDToken string) (IssuedAuth, error) {
	return s.signInWithProvider(ctx, ProviderGoogle, s.googleVerifier, rawIDToken, "")
}

// SignInWithApple verifies an Apple ID token and signs in the matching client,
// creating the account on the first sign-in. Apple puts no name in the token,
// so name from the request is used when the account is created.
func (s *Service) SignInWithApple(ctx context.Context, rawIDToken, name string) (IssuedAuth, error) {
	return s.signInWithProvider(ctx, ProviderApple, s.appleVerifier, rawIDToken, name)
}

// signInWithProvider uses fallbackName only when the token carries no name. The
// store applies the name on creation only, so an existing account keeps its own.
func (s *Service) signInWithProvider(ctx context.Context, provider string, verifier IDTokenVerifier, rawIDToken, fallbackName string) (IssuedAuth, error) {
	var out IssuedAuth
	if verifier == nil {
		return out, ErrProviderNotConfigured
	}
	claims, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return out, err
	}
	name := cleanProviderName(claims.Name)
	if name == "" {
		name = cleanProviderName(fallbackName)
	}
	id := ClientIdentity{
		Provider:    provider,
		Subject:     claims.Subject,
		DisplayName: truncateRunes(name, maxDisplayNameLen),
	}
	if claims.EmailVerified {
		id.Email = NormalizeEmail(claims.Email)
	}
	userID, err := s.store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		return out, fmt.Errorf("find or create client: %w", err)
	}
	profile, err := s.store.UserProfile(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("load profile: %w", err)
	}
	return s.issueSession(ctx, userID, profile.Email)
}

func (s *Service) issueSession(ctx context.Context, userID uuid.UUID, email string) (IssuedAuth, error) {
	var out IssuedAuth
	plain, h, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.refreshTTL)
	if err := s.store.InsertRefreshSession(ctx, userID, h, expiresAt); err != nil {
		return out, fmt.Errorf("session: %w", err)
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	return IssuedAuth{
		AccessToken:   token,
		AccessExpires: exp,
		RefreshToken:  plain,
		UserID:        userID,
		Email:         email,
	}, nil
}

// cleanProviderName drops NUL characters, which Postgres rejects in text, then trims.
func cleanProviderName(name string) string {
	return normalizeDisplayName(strings.ReplaceAll(name, "\x00", ""))
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func (s *Service) Refresh(ctx context.Context, refreshPlain string) (IssuedAuth, error) {
	var out IssuedAuth
	if refreshPlain == "" {
		return out, ErrInvalidRefresh
	}
	oldHash := hashRefreshToken(refreshPlain)
	newPlain, newHash, err := newRefreshToken()
	if err != nil {
		return out, fmt.Errorf("refresh token: %w", err)
	}
	newExpires := time.Now().UTC().Add(s.refreshTTL)
	userID, email, err := s.store.RotateRefreshSession(ctx, oldHash, newHash, newExpires)
	if err != nil {
		return out, err
	}
	token, exp, err := signAccessToken(userID, s.jwtSecret, s.accessTTL)
	if err != nil {
		return out, fmt.Errorf("sign token: %w", err)
	}
	out.AccessToken = token
	out.AccessExpires = exp
	out.RefreshToken = newPlain
	out.UserID = userID
	out.Email = email
	return out, nil
}

func (s *Service) Logout(ctx context.Context, refreshPlain string) error {
	if refreshPlain == "" {
		return nil
	}
	return s.store.RevokeRefreshSession(ctx, hashRefreshToken(refreshPlain))
}

func (s *Service) LogoutAll(ctx context.Context, userID uuid.UUID) error {
	return s.store.RevokeAllUserRefreshSessions(ctx, userID)
}

func (s *Service) UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error) {
	return s.store.UserProfile(ctx, userID)
}

func (s *Service) UpdateProfileName(ctx context.Context, userID uuid.UUID, name string) (UserProfile, error) {
	if err := s.store.UpdateUserDisplayName(ctx, userID, normalizeDisplayName(name)); err != nil {
		return UserProfile{}, err
	}
	return s.store.UserProfile(ctx, userID)
}
