package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authStore interface {
	FindOrCreateClientByIdentity(ctx context.Context, id ClientIdentity) (userID uuid.UUID, created bool, err error)
	FillUserDisplayNameIfEmpty(ctx context.Context, userID uuid.UUID, displayName string) (bool, error)
	SetUserPrimaryEmailIfEmpty(ctx context.Context, userID uuid.UUID, email string) (bool, error)
	InsertRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) error
	RotateRefreshSession(ctx context.Context, oldHash, newHash []byte, newExpiresAt time.Time) (uuid.UUID, string, error)
	RevokeRefreshSession(ctx context.Context, tokenHash []byte) error
	RevokeAllUserRefreshSessions(ctx context.Context, userID uuid.UUID) error
	UserProfile(ctx context.Context, userID uuid.UUID) (UserProfile, error)
	UpdateUserDisplayName(ctx context.Context, userID uuid.UUID, displayName string) error
	ActivateTrainer(ctx context.Context, userID uuid.UUID, termsVersion string) error
	LinkTelegram(ctx context.Context, appUserID uuid.UUID, telegramUserID string) (uuid.UUID, error)
	DeleteClientAccount(ctx context.Context, userID uuid.UUID, revokedAppleSubject string) (telegramUserID string, err error)
	TrainerHasBusinessData(ctx context.Context, userID uuid.UUID) (bool, error)
	UserRoles(ctx context.Context, userID uuid.UUID) ([]string, error)
	UserAppleSubjects(ctx context.Context, userID uuid.UUID) ([]string, error)
}

// linkCodeTaker redeems a one-time Telegram link code. Implemented by *LinkCodeStore.
type linkCodeTaker interface {
	Take(ctx context.Context, code string) (telegramUserID string, ok bool, err error)
}

type Service struct {
	store      authStore
	jwtSecret  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration

	googleVerifier IDTokenVerifier
	appleVerifier  IDTokenVerifier
	appleRevoker   AppleRevoker
	linkCodes      linkCodeTaker

	log *slog.Logger
	// telegramCleanup runs after an account with a Telegram sign-in is deleted.
	telegramCleanup func(ctx context.Context, telegramUserID string) error
}

type IssuedAuth struct {
	AccessToken    string
	AccessExpires  time.Time
	RefreshExpires time.Time
	RefreshToken   string
	UserID         uuid.UUID
	Email          string
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

// WithAppleRevoker enables DeleteAccount for accounts with an Apple sign-in. Without it
// such an account is not deleted.
func WithAppleRevoker(r AppleRevoker) ServiceOption {
	return func(s *Service) { s.appleRevoker = r }
}

// WithLinkCodes enables LinkTelegram.
func WithLinkCodes(c *LinkCodeStore) ServiceOption {
	return func(s *Service) { s.linkCodes = c }
}

// WithTelegramCleanup sets the function DeleteAccount calls after the commit with
// the Telegram user id of the deleted account. It removes what the bot keeps for
// that Telegram user outside the database. A failure is logged and does not fail
// the deletion.
func WithTelegramCleanup(fn func(ctx context.Context, telegramUserID string) error) ServiceOption {
	return func(s *Service) { s.telegramCleanup = fn }
}

// WithLogger sets the logger of the service; nil keeps the current one.
func WithLogger(log *slog.Logger) ServiceOption {
	return func(s *Service) {
		if log != nil {
			s.log = log
		}
	}
}

func NewService(pool *pgxpool.Pool, jwtSecret string, accessTTL, refreshTTL time.Duration, opts ...ServiceOption) *Service {
	s := &Service{
		store:      NewStore(pool),
		jwtSecret:  []byte(jwtSecret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		log:        slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func normalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

// NormalizeEmail trims and lowercases an email address for storage and lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

const maxDisplayNameLen = 100

// ValidateDisplayName rejects names longer than maxDisplayNameLen runes after trimming.
func ValidateDisplayName(name string) error {
	if utf8.RuneCountInString(normalizeDisplayName(name)) > maxDisplayNameLen {
		return fmt.Errorf("name must be at most %d characters", maxDisplayNameLen)
	}
	return nil
}

// SignInWithGoogle verifies a Google ID token and signs in the matching client,
// creating the account on the first sign-in. The nonce is optional; a non-empty
// one must match the token's nonce claim, which carries its SHA-256 as lowercase hex.
func (s *Service) SignInWithGoogle(ctx context.Context, rawIDToken, nonce string) (IssuedAuth, error) {
	return s.signInWithProvider(ctx, ProviderGoogle, s.googleVerifier, rawIDToken, "", nonce, false)
}

// SignInWithApple verifies an Apple ID token and signs in the matching client,
// creating the account on the first sign-in. Apple puts no name in the token,
// so name from the request is used when the account is created or its name is empty.
// The nonce is required: an empty one is refused with ErrNonceRequired before the
// token is verified, but after the provider check, so an unconfigured Apple answers
// ErrProviderNotConfigured first. That is this method's own order; the HTTP handler
// refuses an empty nonce before it calls the service. A nonce must match the token's
// nonce claim, which carries its SHA-256 as lowercase hex.
func (s *Service) SignInWithApple(ctx context.Context, rawIDToken, name, nonce string) (IssuedAuth, error) {
	return s.signInWithProvider(ctx, ProviderApple, s.appleVerifier, rawIDToken, name, nonce, true)
}

// signInWithProvider uses fallbackName only when the token carries no name. The
// name goes into a new account; an existing account gets it only while its own
// name is empty, so a name that is set is never overwritten. The same holds for a
// verified token email and the account's primary email. With nonceRequired
// an empty nonce is refused after the provider check and before the verifier runs;
// that is the service's own order, the HTTP handler checks the nonce before it calls here.
func (s *Service) signInWithProvider(ctx context.Context, provider string, verifier IDTokenVerifier, rawIDToken, fallbackName, nonce string, nonceRequired bool) (IssuedAuth, error) {
	var out IssuedAuth
	if verifier == nil {
		return out, ErrProviderNotConfigured
	}
	if nonceRequired && nonce == "" {
		return out, ErrNonceRequired
	}
	claims, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return out, err
	}
	if nonce != "" {
		sum := sha256.Sum256([]byte(nonce))
		want := hex.EncodeToString(sum[:])
		// The error names neither value: the cause reaches the request log.
		if subtle.ConstantTimeCompare([]byte(want), []byte(claims.Nonce)) != 1 {
			return out, fmt.Errorf("%w: nonce mismatch", ErrInvalidIDToken)
		}
	}
	name := cleanProviderName(claims.Name)
	if name == "" {
		name = cleanProviderName(fallbackName)
	}
	name = truncateRunes(name, maxDisplayNameLen)
	id := ClientIdentity{
		Provider:    provider,
		Subject:     claims.Subject,
		DisplayName: name,
	}
	if claims.EmailVerified {
		id.Email = NormalizeEmail(claims.Email)
	}
	userID, created, err := s.store.FindOrCreateClientByIdentity(ctx, id)
	if err != nil {
		return out, fmt.Errorf("find or create client: %w", err)
	}
	if !created && name != "" {
		if _, err := s.store.FillUserDisplayNameIfEmpty(ctx, userID, name); err != nil {
			return out, fmt.Errorf("fill display name: %w", err)
		}
	}
	if !created && id.Email != "" {
		if _, err := s.store.SetUserPrimaryEmailIfEmpty(ctx, userID, id.Email); err != nil {
			return out, fmt.Errorf("fill primary email: %w", err)
		}
	}
	profile, err := s.store.UserProfile(ctx, userID)
	if err != nil {
		return out, fmt.Errorf("load profile: %w", err)
	}
	return s.issueSession(ctx, userID, profile.Email)
}

// LinkTelegram redeems a code the bot gave to a Telegram user and links that
// Telegram to appUserID, merging into the Telegram account when one exists. The
// session returned belongs to the account that remains. The code is spent even
// when the link is then refused.
func (s *Service) LinkTelegram(ctx context.Context, appUserID uuid.UUID, code string) (IssuedAuth, error) {
	var out IssuedAuth
	if strings.TrimSpace(code) == "" {
		return out, ErrInvalidLinkCode
	}
	if s.linkCodes == nil {
		return out, ErrLinkCodesUnavailable
	}
	telegramUserID, ok, err := s.linkCodes.Take(ctx, code)
	if err != nil {
		return out, fmt.Errorf("take link code: %w", err)
	}
	if !ok {
		return out, ErrInvalidLinkCode
	}
	remainingID, err := s.store.LinkTelegram(ctx, appUserID, telegramUserID)
	if err != nil {
		return out, fmt.Errorf("link telegram: %w", err)
	}
	profile, err := s.store.UserProfile(ctx, remainingID)
	if err != nil {
		return out, fmt.Errorf("load profile: %w", err)
	}
	return s.issueSession(ctx, remainingID, profile.Email)
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
		AccessToken:    token,
		AccessExpires:  exp,
		RefreshExpires: expiresAt,
		RefreshToken:   plain,
		UserID:         userID,
		Email:          email,
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
	out.RefreshExpires = newExpires
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
