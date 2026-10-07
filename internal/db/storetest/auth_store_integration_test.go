//go:build integration

package storetest

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"mentorix-backend/internal/auth"
	"mentorix-backend/internal/db/pgconv"
	"mentorix-backend/internal/db/sqlc"
)

func TestAuthStore_RegisterTrainerEmailPassword(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	email := "trainer-integration@test.com"
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	userID, err := store.RegisterTrainerEmailPassword(ctx, email, hash, "Viktor")
	if err != nil {
		t.Fatalf("RegisterTrainerEmailPassword() error = %v", err)
	}
	if userID == uuid.Nil {
		t.Fatal("user id is nil")
	}

	profile, err := store.UserProfile(ctx, userID)
	if err != nil {
		t.Fatalf("UserProfile() error = %v", err)
	}
	if profile.Email != email {
		t.Errorf("email = %q, want %q", profile.Email, email)
	}
	if len(profile.Roles) != 1 || profile.Roles[0] != auth.RoleTrainer {
		t.Errorf("roles = %v, want [%q]", profile.Roles, auth.RoleTrainer)
	}

	primary, err := store.UserPrimaryEmail(ctx, userID)
	if err != nil {
		t.Fatalf("UserPrimaryEmail() error = %v", err)
	}
	if primary != email {
		t.Errorf("primary email = %q, want %q", primary, email)
	}

	if profile.Name != "Viktor" {
		t.Errorf("name = %q, want %q", profile.Name, "Viktor")
	}

	_, err = store.RegisterTrainerEmailPassword(ctx, email, hash, "")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Errorf("duplicate register error = %v, want ErrEmailTaken", err)
	}
}

func TestAuthStore_RotateRefreshSession(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(ctx, "rotate@test.com", hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	oldHash := randomTokenHash(t)
	expires := time.Now().UTC().Add(24 * time.Hour)
	if err := store.InsertRefreshSession(ctx, userID, oldHash, expires); err != nil {
		t.Fatalf("InsertRefreshSession() error = %v", err)
	}

	newHash := randomTokenHash(t)
	newExpires := time.Now().UTC().Add(30 * 24 * time.Hour)

	gotUserID, err := store.RotateRefreshSession(ctx, oldHash, newHash, newExpires, auth.DefaultRefreshReuseGrace)
	if err != nil {
		t.Fatalf("RotateRefreshSession() error = %v", err)
	}
	if gotUserID != userID {
		t.Errorf("user id = %v, want %v", gotUserID, userID)
	}

	// Inside the grace window a spent token can be retried once more.
	retryHash := randomTokenHash(t)
	if _, err := store.RotateRefreshSession(ctx, oldHash, retryHash, newExpires, auth.DefaultRefreshReuseGrace); err != nil {
		t.Errorf("retry inside the window error = %v, want nil", err)
	}

	// After the window the same token is a reuse.
	backdateRotation(t, pool, oldHash, 31*time.Second)
	_, err = store.RotateRefreshSession(ctx, oldHash, randomTokenHash(t), newExpires, auth.DefaultRefreshReuseGrace)
	if !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Errorf("rotate with spent hash after the window error = %v, want ErrRefreshTokenReused", err)
	}
}

type refreshSessionRow struct {
	FamilyID  uuid.UUID
	RotatedAt *time.Time
	RevokedAt *time.Time
}

func readRefreshSession(t *testing.T, pool *pgxpool.Pool, tokenHash []byte) refreshSessionRow {
	t.Helper()
	var row refreshSessionRow
	err := pool.QueryRow(context.Background(),
		`SELECT family_id, rotated_at, revoked_at FROM mentorix.auth_refresh_sessions WHERE token_hash = $1`,
		tokenHash).Scan(&row.FamilyID, &row.RotatedAt, &row.RevokedAt)
	if err != nil {
		t.Fatalf("read refresh session: %v", err)
	}
	return row
}

func countRefreshSessions(t *testing.T, pool *pgxpool.Pool, familyID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.auth_refresh_sessions WHERE family_id = $1`, familyID).Scan(&n)
	if err != nil {
		t.Fatalf("count refresh sessions: %v", err)
	}
	return n
}

// snapshotRefreshSessions describes every session row so a test can assert that
// a refused rotation changed nothing without printing token material.
func snapshotRefreshSessions(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT encode(token_hash, 'hex'), family_id::text, coalesce(revoked_at::text, ''), coalesce(rotated_at::text, '')
		 FROM mentorix.auth_refresh_sessions ORDER BY token_hash`)
	if err != nil {
		t.Fatalf("snapshot refresh sessions: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var h, fam, revoked, rotated string
		if err := rows.Scan(&h, &fam, &revoked, &rotated); err != nil {
			t.Fatalf("scan snapshot: %v", err)
		}
		out = append(out, h+"|"+fam+"|"+revoked+"|"+rotated)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("snapshot rows: %v", err)
	}
	return strings.Join(out, "\n")
}

func newRefreshTestUser(t *testing.T, store *auth.Store, email string) uuid.UUID {
	t.Helper()
	hash, err := auth.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	userID, err := store.RegisterTrainerEmailPassword(context.Background(), email, hash, "")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	return userID
}

func insertRefreshSession(t *testing.T, store *auth.Store, userID uuid.UUID) []byte {
	t.Helper()
	h := randomTokenHash(t)
	if err := store.InsertRefreshSession(context.Background(), userID, h, time.Now().UTC().Add(24*time.Hour)); err != nil {
		t.Fatalf("InsertRefreshSession() error = %v", err)
	}
	return h
}

func rotateWithDefaultWindow(store *auth.Store, oldHash, newHash []byte) (uuid.UUID, error) {
	return store.RotateRefreshSession(context.Background(), oldHash, newHash, time.Now().UTC().Add(24*time.Hour), auth.DefaultRefreshReuseGrace)
}

func backdateRotation(t *testing.T, pool *pgxpool.Pool, tokenHash []byte, by time.Duration) {
	t.Helper()
	tag, err := pool.Exec(context.Background(),
		`UPDATE mentorix.auth_refresh_sessions SET rotated_at = rotated_at - make_interval(secs => $2::float8)
		 WHERE token_hash = $1 AND rotated_at IS NOT NULL`, tokenHash, by.Seconds())
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("backdate rotation: err = %v, rows = %d", err, tag.RowsAffected())
	}
}

func activeRefreshSessions(t *testing.T, pool *pgxpool.Pool, familyID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.auth_refresh_sessions
		 WHERE family_id = $1 AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > now()`,
		familyID).Scan(&n)
	if err != nil {
		t.Fatalf("count active refresh sessions: %v", err)
	}
	return n
}

func assertSpent(t *testing.T, pool *pgxpool.Pool, name string, tokenHash []byte) {
	t.Helper()
	if row := readRefreshSession(t, pool, tokenHash); row.RotatedAt == nil || row.RevokedAt == nil {
		t.Errorf("%s: rotated_at = %v, revoked_at = %v, want both set", name, row.RotatedAt, row.RevokedAt)
	}
}

func TestAuthStore_RotateKeepsFamilyAndMarksSpentRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-rotate@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	newHash := randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, newHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	spent := readRefreshSession(t, pool, oldHash)
	successor := readRefreshSession(t, pool, newHash)
	assertSpent(t, pool, "spent row", oldHash)
	if successor.FamilyID != spent.FamilyID {
		t.Errorf("successor family = %v, want %v", successor.FamilyID, spent.FamilyID)
	}
	if successor.RotatedAt != nil || successor.RevokedAt != nil {
		t.Errorf("successor rotated_at = %v, revoked_at = %v, want both nil", successor.RotatedAt, successor.RevokedAt)
	}
}

func TestAuthStore_SignInsGetDifferentFamilies(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-signins@test.com")

	first := readRefreshSession(t, pool, insertRefreshSession(t, store, userID))
	second := readRefreshSession(t, pool, insertRefreshSession(t, store, userID))
	if first.FamilyID == second.FamilyID {
		t.Errorf("two sign-ins share family %v", first.FamilyID)
	}
}

func TestAuthStore_RetryInsideWindowGetsSuccessorInSameFamily(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-retry@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	firstHash := randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, firstHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	spentBefore := readRefreshSession(t, pool, oldHash)

	retryHash := randomTokenHash(t)
	gotUserID, err := rotateWithDefaultWindow(store, oldHash, retryHash)
	if err != nil {
		t.Fatalf("retry inside the window error = %v, want nil", err)
	}
	if gotUserID != userID {
		t.Errorf("user id = %v, want %v", gotUserID, userID)
	}

	retry := readRefreshSession(t, pool, retryHash)
	if retry.FamilyID != spentBefore.FamilyID {
		t.Errorf("retry family = %v, want %v", retry.FamilyID, spentBefore.FamilyID)
	}
	if retry.RevokedAt != nil || retry.RotatedAt != nil {
		t.Errorf("retry successor is not active: revoked_at = %v, rotated_at = %v", retry.RevokedAt, retry.RotatedAt)
	}
	if spentAfter := readRefreshSession(t, pool, oldHash); !reflect.DeepEqual(spentAfter, spentBefore) {
		t.Error("the retried row changed during a retry")
	}
}

func TestAuthStore_GraceRetryLeavesOneLiveToken(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-one-live@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, oldHash).FamilyID
	firstHash, retryHash := randomTokenHash(t), randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, firstHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := rotateWithDefaultWindow(store, oldHash, retryHash); err != nil {
		t.Fatalf("retry: %v", err)
	}

	if n := activeRefreshSessions(t, pool, family); n != 1 {
		t.Errorf("family has %d active sessions, want 1", n)
	}
	assertSpent(t, pool, "earlier successor", firstHash)
	if row := readRefreshSession(t, pool, retryHash); row.RevokedAt != nil || row.RotatedAt != nil {
		t.Error("the retry successor is not the live token")
	}
}

func TestAuthStore_RetryInsideWindowAfterLogoutIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-retry-logout@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	firstHash := randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, firstHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if err := store.RevokeRefreshSession(context.Background(), firstHash); err != nil {
		t.Fatalf("logout: %v", err)
	}
	family := readRefreshSession(t, pool, oldHash).FamilyID
	before := snapshotRefreshSessions(t, pool)

	_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("retry after logout error = %v, want ErrInvalidRefresh", err)
	}
	if errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Error("a retry inside the window after logout was reported as reuse")
	}
	if n := countRefreshSessions(t, pool, family); n != 2 {
		t.Errorf("family has %d rows, want 2", n)
	}
	if snapshotRefreshSessions(t, pool) != before {
		t.Error("a refused retry changed session rows")
	}
}

func TestAuthStore_LogoutRevokesTheWholeFamily(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-logout@test.com")

	t.Run("after a grace retry", func(t *testing.T) {
		t1 := insertRefreshSession(t, store, userID)
		family := readRefreshSession(t, pool, t1).FamilyID
		t2, t3 := randomTokenHash(t), randomTokenHash(t)
		if _, err := rotateWithDefaultWindow(store, t1, t2); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		if _, err := rotateWithDefaultWindow(store, t1, t3); err != nil {
			t.Fatalf("retry: %v", err)
		}
		if err := store.RevokeRefreshSession(ctx, t3); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if n := activeRefreshSessions(t, pool, family); n != 0 {
			t.Errorf("%d active sessions after logout, want 0", n)
		}
		for name, h := range map[string][]byte{"first token": t1, "second token": t2} {
			if _, err := rotateWithDefaultWindow(store, h, randomTokenHash(t)); !errors.Is(err, auth.ErrInvalidRefresh) {
				t.Errorf("retry of the %s after logout error = %v, want ErrInvalidRefresh", name, err)
			}
		}
		if n := activeRefreshSessions(t, pool, family); n != 0 {
			t.Errorf("%d active sessions after refused retries, want 0", n)
		}
	})

	t.Run("with a spent token of a live family", func(t *testing.T) {
		t1 := insertRefreshSession(t, store, userID)
		family := readRefreshSession(t, pool, t1).FamilyID
		if _, err := rotateWithDefaultWindow(store, t1, randomTokenHash(t)); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		if err := store.RevokeRefreshSession(ctx, t1); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if n := activeRefreshSessions(t, pool, family); n != 0 {
			t.Errorf("%d active sessions after logout with a spent token, want 0", n)
		}
	})

	t.Run("leaves other families alone", func(t *testing.T) {
		mine := insertRefreshSession(t, store, userID)
		other := insertRefreshSession(t, store, userID)
		if err := store.RevokeRefreshSession(ctx, mine); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if row := readRefreshSession(t, pool, other); row.RevokedAt != nil {
			t.Error("logout revoked another family")
		}
	})

	t.Run("unknown token is a no-op", func(t *testing.T) {
		before := snapshotRefreshSessions(t, pool)
		if err := store.RevokeRefreshSession(ctx, randomTokenHash(t)); err != nil {
			t.Fatalf("logout with unknown token error = %v, want nil", err)
		}
		if snapshotRefreshSessions(t, pool) != before {
			t.Error("logout with an unknown token changed rows")
		}
	})
}

func TestAuthStore_GraceBoundary(t *testing.T) {
	tests := []struct {
		name      string
		age       time.Duration
		wantReuse bool
	}{
		{"29 seconds old retries", 29 * time.Second, false},
		{"31 seconds old is reuse", 31 * time.Second, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-boundary@test.com")
			oldHash := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, oldHash).FamilyID
			if _, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t)); err != nil {
				t.Fatalf("rotate: %v", err)
			}
			backdateRotation(t, pool, oldHash, tt.age)

			_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
			if tt.wantReuse {
				if !errors.Is(err, auth.ErrRefreshTokenReused) {
					t.Fatalf("error = %v, want ErrRefreshTokenReused", err)
				}
				if n := activeRefreshSessions(t, pool, family); n != 0 {
					t.Errorf("%d active sessions after reuse, want 0", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if n := activeRefreshSessions(t, pool, family); n != 1 {
				t.Errorf("%d active sessions after the retry, want 1", n)
			}
		})
	}
}

func TestAuthStore_ZeroGraceMakesAnImmediateRetryReuse(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-zero-grace@test.com")
	oldHash := insertRefreshSession(t, store, userID)
	expires := time.Now().UTC().Add(time.Hour)
	if _, err := store.RotateRefreshSession(ctx, oldHash, randomTokenHash(t), expires, 0); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	_, err := store.RotateRefreshSession(ctx, oldHash, randomTokenHash(t), expires, 0)
	if !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Errorf("immediate retry with grace 0 error = %v, want ErrRefreshTokenReused", err)
	}
}

func TestAuthStore_ReuseAfterWindowRevokesTheFamily(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-reuse@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	otherFamilyHash := insertRefreshSession(t, store, userID)
	successorHash := randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, successorHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	family := readRefreshSession(t, pool, oldHash).FamilyID
	backdateRotation(t, pool, oldHash, 31*time.Second)

	_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
	if !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Fatalf("reuse error = %v, want ErrRefreshTokenReused", err)
	}
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Errorf("reuse error = %v, want it to satisfy ErrInvalidRefresh", err)
	}
	var reuse *auth.RefreshReuseError
	if !errors.As(err, &reuse) {
		t.Fatalf("reuse error %T is not *RefreshReuseError", err)
	}
	if reuse.FamilyID != family || reuse.UserID != userID || reuse.Revoked != 1 {
		t.Errorf("reuse error = %+v, want family %v, user %v, revoked 1", reuse, family, userID)
	}

	// The revoke was committed, not rolled back with the failed rotation.
	if got := readRefreshSession(t, pool, successorHash); got.RevokedAt == nil {
		t.Error("live successor of the reused family is still active")
	}
	if n := activeRefreshSessions(t, pool, family); n != 0 {
		t.Errorf("%d sessions of the reused family are still active", n)
	}
	if n := countRefreshSessions(t, pool, family); n != 2 {
		t.Errorf("family has %d rows, want 2 (a refused reuse inserts nothing)", n)
	}
	if other := readRefreshSession(t, pool, otherFamilyHash); other.RevokedAt != nil {
		t.Error("another family of the same user was revoked")
	}

	// A second reuse finds the family already dead and says so.
	_, err = rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
	if !errors.As(err, &reuse) || reuse.Revoked != 0 {
		t.Errorf("second reuse error = %v, want revoked=0", err)
	}
}

func TestAuthStore_SuccessorAfterReuseRevokeIsRefused(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-after-reuse@test.com")

	oldHash := insertRefreshSession(t, store, userID)
	successorHash := randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, oldHash, successorHash); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	backdateRotation(t, pool, oldHash, 31*time.Second)
	if _, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t)); !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Fatalf("reuse error = %v, want ErrRefreshTokenReused", err)
	}
	before := snapshotRefreshSessions(t, pool)

	// The successor is now revoked without being rotated, so it is a plain 401
	// and not a second reuse report.
	_, err := rotateWithDefaultWindow(store, successorHash, randomTokenHash(t))
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("successor error = %v, want ErrInvalidRefresh", err)
	}
	if errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Error("revoked successor was reported as reuse")
	}
	if snapshotRefreshSessions(t, pool) != before {
		t.Error("refusing the revoked successor changed session rows")
	}
}

// stolenTokenFixture builds the timeline: the victim rotates T1 to T2, then a
// second party retries T1 inside the window and gets T3, which spends T2.
type stolenTokenFixture struct {
	pool       *pgxpool.Pool
	store      *auth.Store
	family     uuid.UUID
	t1, t2, t3 []byte
}

func newStolenTokenFixture(t *testing.T, email string) stolenTokenFixture {
	t.Helper()
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, email)
	f := stolenTokenFixture{pool: pool, store: store, t2: randomTokenHash(t), t3: randomTokenHash(t)}
	f.t1 = insertRefreshSession(t, store, userID)
	f.family = readRefreshSession(t, pool, f.t1).FamilyID
	if _, err := rotateWithDefaultWindow(store, f.t1, f.t2); err != nil {
		t.Fatalf("victim rotation: %v", err)
	}
	if _, err := rotateWithDefaultWindow(store, f.t1, f.t3); err != nil {
		t.Fatalf("second party retry: %v", err)
	}
	return f
}

func TestAuthStore_StolenTokenTimeline(t *testing.T) {
	t.Run("T2 after the window revokes the family and T3 is refused", func(t *testing.T) {
		f := newStolenTokenFixture(t, "family-theft-a@test.com")
		assertSpent(t, f.pool, "T2", f.t2)
		if n := activeRefreshSessions(t, f.pool, f.family); n != 1 {
			t.Fatalf("%d active sessions before T2 returns, want 1", n)
		}
		backdateRotation(t, f.pool, f.t2, 31*time.Second)

		if _, err := rotateWithDefaultWindow(f.store, f.t2, randomTokenHash(t)); !errors.Is(err, auth.ErrRefreshTokenReused) {
			t.Fatalf("T2 error = %v, want ErrRefreshTokenReused", err)
		}
		if n := activeRefreshSessions(t, f.pool, f.family); n != 0 {
			t.Errorf("%d active sessions after reuse, want 0", n)
		}
		if _, err := rotateWithDefaultWindow(f.store, f.t3, randomTokenHash(t)); !errors.Is(err, auth.ErrInvalidRefresh) {
			t.Errorf("T3 error = %v, want ErrInvalidRefresh", err)
		}
	})

	t.Run("T2 inside the window is a retry that spends T3", func(t *testing.T) {
		f := newStolenTokenFixture(t, "family-theft-b@test.com")
		t4 := randomTokenHash(t)
		if _, err := rotateWithDefaultWindow(f.store, f.t2, t4); err != nil {
			t.Fatalf("T2 inside the window error = %v, want nil", err)
		}
		assertSpent(t, f.pool, "T3", f.t3)
		if n := activeRefreshSessions(t, f.pool, f.family); n != 1 {
			t.Errorf("%d active sessions, want 1", n)
		}
		if row := readRefreshSession(t, f.pool, t4); row.RevokedAt != nil || row.RotatedAt != nil {
			t.Error("T4 is not the live token")
		}

		backdateRotation(t, f.pool, f.t3, 31*time.Second)
		if _, err := rotateWithDefaultWindow(f.store, f.t3, randomTokenHash(t)); !errors.Is(err, auth.ErrRefreshTokenReused) {
			t.Fatalf("T3 after the window error = %v, want ErrRefreshTokenReused", err)
		}
		if n := activeRefreshSessions(t, f.pool, f.family); n != 0 {
			t.Errorf("%d active sessions after reuse, want 0", n)
		}
	})
}

func TestAuthStore_LostResponseThenNormalRotation(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-lost-response@test.com")

	t1 := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, t1).FamilyID
	t2, t3, t4 := randomTokenHash(t), randomTokenHash(t), randomTokenHash(t)
	if _, err := rotateWithDefaultWindow(store, t1, t2); err != nil {
		t.Fatalf("first rotation: %v", err)
	}
	if _, err := rotateWithDefaultWindow(store, t1, t3); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := rotateWithDefaultWindow(store, t3, t4); err != nil {
		t.Fatalf("T3 rotation: %v", err)
	}
	if n := activeRefreshSessions(t, pool, family); n != 1 {
		t.Errorf("%d active sessions, want 1", n)
	}

	backdateRotation(t, pool, t2, 31*time.Second)
	if _, err := rotateWithDefaultWindow(store, t2, randomTokenHash(t)); !errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Errorf("T2 after the window error = %v, want ErrRefreshTokenReused", err)
	}
}

func TestAuthStore_RefusedTokensChangeNothing(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-refused@test.com")

	live := insertRefreshSession(t, store, userID)
	loggedOut := insertRefreshSession(t, store, userID)
	if err := store.RevokeRefreshSession(ctx, loggedOut); err != nil {
		t.Fatalf("logout: %v", err)
	}
	expired := randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, expired, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	// A token that was rotated and then expired is refused without a reuse revoke.
	expiredRotated := randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, expiredRotated, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE mentorix.auth_refresh_sessions SET rotated_at = now() - interval '2 hours', revoked_at = now() - interval '2 hours' WHERE token_hash = $1`,
		expiredRotated); err != nil {
		t.Fatalf("mark rotated: %v", err)
	}
	before := snapshotRefreshSessions(t, pool)

	cases := []struct {
		name string
		hash []byte
	}{
		{"unknown", randomTokenHash(t)},
		{"expired", expired},
		{"expired and rotated", expiredRotated},
		{"logged out", loggedOut},
	}
	for _, tc := range cases {
		_, err := rotateWithDefaultWindow(store, tc.hash, randomTokenHash(t))
		if !errors.Is(err, auth.ErrInvalidRefresh) {
			t.Errorf("%s: error = %v, want ErrInvalidRefresh", tc.name, err)
		}
		if errors.Is(err, auth.ErrRefreshTokenReused) {
			t.Errorf("%s: reported as reuse", tc.name)
		}
	}
	if snapshotRefreshSessions(t, pool) != before {
		t.Error("a refused rotation changed session rows")
	}
	if got := readRefreshSession(t, pool, live); got.RevokedAt != nil {
		t.Error("an unrelated live session was revoked")
	}
}

func TestAuthStore_RowRevokedByPreviousBinaryIsRefusedWithoutFamilyRevoke(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-prev-binary@test.com")

	// The previous binary rotated by setting revoked_at only and inserting a successor.
	oldHash := insertRefreshSession(t, store, userID)
	successorHash := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, oldHash).FamilyID
	if _, err := pool.Exec(ctx,
		`UPDATE mentorix.auth_refresh_sessions SET revoked_at = now() WHERE token_hash = $1`, oldHash); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE mentorix.auth_refresh_sessions SET family_id = $1 WHERE token_hash = $2`, family, successorHash); err != nil {
		t.Fatalf("join family: %v", err)
	}
	before := snapshotRefreshSessions(t, pool)

	_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
	if !errors.Is(err, auth.ErrInvalidRefresh) {
		t.Fatalf("error = %v, want ErrInvalidRefresh", err)
	}
	if errors.Is(err, auth.ErrRefreshTokenReused) {
		t.Error("a row in the previous binary's shape was reported as reuse")
	}
	if snapshotRefreshSessions(t, pool) != before {
		t.Error("the family changed")
	}
	if got := readRefreshSession(t, pool, successorHash); got.RevokedAt != nil {
		t.Error("the successor was revoked")
	}
}

// familyLock holds the advisory lock that serializes one family, on its own
// connection, so a test controls when queued operations may proceed.
type familyLock struct {
	conn   *pgx.Conn
	tx     pgx.Tx
	family uuid.UUID
}

func holdFamilyLock(t *testing.T, pool *pgxpool.Pool, family uuid.UUID) *familyLock {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.ConnectConfig(ctx, pool.Config().ConnConfig)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	l := &familyLock{conn: conn, tx: tx, family: family}
	t.Cleanup(func() {
		l.release(t)
		_ = conn.Close(ctx)
	})
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, family.String()); err != nil {
		t.Fatalf("take family lock: %v", err)
	}
	return l
}

func (l *familyLock) release(t *testing.T) {
	t.Helper()
	if l.tx == nil {
		return
	}
	tx := l.tx
	l.tx = nil
	if err := tx.Rollback(context.Background()); err != nil {
		t.Errorf("release family lock: %v", err)
	}
}

// waitForLockWaiters polls until n backends wait for this family's advisory lock.
// pg_locks stores a bigint key as classid (high 32 bits) and objid (low 32 bits),
// so the match is exact and a parallel test cannot satisfy it.
func (l *familyLock) waitForLockWaiters(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		err := l.tx.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_locks
			 WHERE locktype = 'advisory' AND NOT granted AND objsubid = 1
			   AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			   AND ((classid::bigint << 32) | objid::bigint) = hashtextextended($1::text, 0)`,
			l.family.String()).Scan(&waiting)
		if err != nil {
			t.Fatalf("poll pg_locks: %v", err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d backends wait on the family lock, want %d: the operation does not take it", waiting, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// raceBehindFamilyLock holds the family lock, queues first and then second behind
// it, releases the lock and returns both results.
func raceBehindFamilyLock(t *testing.T, pool *pgxpool.Pool, family uuid.UUID, first, second func() error) (error, error) {
	t.Helper()
	lock := holdFamilyLock(t, pool, family)
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- first() }()
	lock.waitForLockWaiters(t, 1)
	go func() { secondDone <- second() }()
	lock.waitForLockWaiters(t, 2)
	lock.release(t)
	select {
	case err1 := <-firstDone:
		return err1, <-secondDone
	case <-time.After(15 * time.Second):
		t.Fatal("queued operations did not finish after the lock was released")
		return nil, nil
	}
}

func activeUserSessions(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM mentorix.auth_refresh_sessions
		 WHERE user_id = $1 AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > now()`,
		userID).Scan(&n)
	if err != nil {
		t.Fatalf("count active user sessions: %v", err)
	}
	return n
}

func TestAuthStore_RotationWaitsForTheFamilyLock(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-lock@test.com")
	oldHash := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, oldHash).FamilyID

	lock := holdFamilyLock(t, pool, family)
	done := make(chan error, 1)
	go func() {
		_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
		done <- err
	}()

	lock.waitForLockWaiters(t, 1)
	select {
	case err := <-done:
		t.Fatalf("rotation finished while the family lock was held, error = %v", err)
	default:
	}
	lock.release(t)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("rotation after the lock was released error = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("rotation did not finish after the lock was released")
	}
}

func TestAuthStore_LogoutWaitsForTheFamilyLock(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-lock-logout@test.com")
	hash := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, hash).FamilyID

	lock := holdFamilyLock(t, pool, family)
	done := make(chan error, 1)
	go func() { done <- store.RevokeRefreshSession(context.Background(), hash) }()

	lock.waitForLockWaiters(t, 1)
	lock.release(t)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("logout error = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("logout did not finish after the lock was released")
	}
	if n := activeRefreshSessions(t, pool, family); n != 0 {
		t.Errorf("%d active sessions after logout, want 0", n)
	}
}

func TestAuthStore_ReuseRacingRotationOfTheLiveToken(t *testing.T) {
	for _, reuseFirst := range []bool{true, false} {
		name := "rotation queued first"
		if reuseFirst {
			name = "reuse queued first"
		}
		t.Run(name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-race@test.com")
			t1 := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, t1).FamilyID
			t2 := randomTokenHash(t)
			if _, err := rotateWithDefaultWindow(store, t1, t2); err != nil {
				t.Fatalf("rotate: %v", err)
			}
			backdateRotation(t, pool, t1, 31*time.Second)

			reuseHash, rotateHash := randomTokenHash(t), randomTokenHash(t)
			reuse := func() error {
				_, err := rotateWithDefaultWindow(store, t1, reuseHash)
				return err
			}
			rotate := func() error {
				_, err := rotateWithDefaultWindow(store, t2, rotateHash)
				return err
			}
			lock := holdFamilyLock(t, pool, family)
			reuseDone, rotateDone := make(chan error, 1), make(chan error, 1)
			start := func(fn func() error, ch chan error, waiters int) {
				go func() { ch <- fn() }()
				lock.waitForLockWaiters(t, waiters)
			}
			if reuseFirst {
				start(reuse, reuseDone, 1)
				start(rotate, rotateDone, 2)
			} else {
				start(rotate, rotateDone, 1)
				start(reuse, reuseDone, 2)
			}
			lock.release(t)

			if err := <-reuseDone; !errors.Is(err, auth.ErrRefreshTokenReused) {
				t.Errorf("reuse error = %v, want ErrRefreshTokenReused", err)
			}
			if err := <-rotateDone; err != nil && !errors.Is(err, auth.ErrInvalidRefresh) {
				t.Errorf("rotation error = %v, want nil or ErrInvalidRefresh", err)
			}
			if n := activeRefreshSessions(t, pool, family); n != 0 {
				t.Errorf("%d active sessions remain after the reuse revoke, want 0", n)
			}
		})
	}
}

// issuedTokens is the set of tokens of one family that concurrent workers present.
type issuedTokens struct {
	mu     sync.Mutex
	tokens [][]byte
	next   int
}

func (it *issuedTokens) pick() []byte {
	it.mu.Lock()
	defer it.mu.Unlock()
	tok := it.tokens[it.next%len(it.tokens)]
	it.next++
	return tok
}

func (it *issuedTokens) add(tok []byte) {
	it.mu.Lock()
	defer it.mu.Unlock()
	it.tokens = append(it.tokens, tok)
}

func TestAuthStore_ConcurrentOperationsOnOneFamily(t *testing.T) {
	for _, withReuse := range []bool{false, true} {
		name := "rotations and retries"
		if withReuse {
			name = "rotations, retries and one reuse"
		}
		t.Run(name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-concurrent@test.com")

			// first and second are spent, third is live. Workers present different tokens
			// of the family, so the row lock of one token cannot serialize them.
			first := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, first).FamilyID
			second, third := randomTokenHash(t), randomTokenHash(t)
			if _, err := rotateWithDefaultWindow(store, first, second); err != nil {
				t.Fatalf("rotate: %v", err)
			}
			if _, err := rotateWithDefaultWindow(store, second, third); err != nil {
				t.Fatalf("rotate: %v", err)
			}
			if withReuse {
				backdateRotation(t, pool, first, 31*time.Second)
			}
			issued := &issuedTokens{tokens: [][]byte{second, third}}
			if !withReuse {
				issued.add(first)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			const workers, rounds = 6, 4
			results := make(chan error, workers*rounds+1)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for range workers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for range rounds {
						newHash := make([]byte, 32)
						if _, err := rand.Read(newHash); err != nil {
							results <- err
							return
						}
						_, err := store.RotateRefreshSession(ctx, issued.pick(), newHash, time.Now().UTC().Add(time.Hour), auth.DefaultRefreshReuseGrace)
						if err == nil {
							issued.add(newHash)
						}
						results <- err
					}
				}()
			}
			if withReuse {
				reuseHash := randomTokenHash(t)
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					_, err := store.RotateRefreshSession(ctx, first, reuseHash, time.Now().UTC().Add(time.Hour), auth.DefaultRefreshReuseGrace)
					if !errors.Is(err, auth.ErrRefreshTokenReused) {
						err = fmt.Errorf("reuse goroutine: want ErrRefreshTokenReused, got %v", err)
					} else {
						err = nil
					}
					results <- err
				}()
			}
			close(start)
			wg.Wait()
			close(results)

			for err := range results {
				if err != nil && !errors.Is(err, auth.ErrInvalidRefresh) {
					t.Errorf("operation error = %v", err)
				}
			}
			active := activeRefreshSessions(t, pool, family)
			if withReuse && active != 0 {
				t.Errorf("%d active sessions after a reuse, want 0", active)
			}
			if active > 1 {
				t.Errorf("%d active sessions, want at most 1", active)
			}
		})
	}
}

func TestAuthStore_LogoutAllWaitsForTheFamilyLock(t *testing.T) {
	for _, rotationFirst := range []bool{true, false} {
		name := "logout-all queued first"
		if rotationFirst {
			name = "rotation queued first"
		}
		t.Run(name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-logout-all@test.com")
			live := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, live).FamilyID
			newHash := randomTokenHash(t)

			rotate := func() error { _, err := rotateWithDefaultWindow(store, live, newHash); return err }
			logoutAll := func() error { return store.RevokeAllUserRefreshSessions(context.Background(), userID) }
			first, second := logoutAll, rotate
			if rotationFirst {
				first, second = rotate, logoutAll
			}
			err1, err2 := raceBehindFamilyLock(t, pool, family, first, second)
			for _, err := range []error{err1, err2} {
				if err != nil && !errors.Is(err, auth.ErrInvalidRefresh) {
					t.Errorf("operation error = %v, want nil or ErrInvalidRefresh", err)
				}
			}
			if n := activeUserSessions(t, pool, userID); n != 0 {
				t.Errorf("%d active sessions remain after logout-all, want 0", n)
			}
		})
	}
}

func TestAuthStore_LogoutRacingRotationOfTheLiveToken(t *testing.T) {
	for _, rotationFirst := range []bool{true, false} {
		name := "logout queued first"
		if rotationFirst {
			name = "rotation queued first"
		}
		t.Run(name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-logout-race@test.com")
			live := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, live).FamilyID
			newHash := randomTokenHash(t)

			rotate := func() error { _, err := rotateWithDefaultWindow(store, live, newHash); return err }
			logout := func() error { return store.RevokeRefreshSession(context.Background(), live) }
			first, second := logout, rotate
			if rotationFirst {
				first, second = rotate, logout
			}
			err1, err2 := raceBehindFamilyLock(t, pool, family, first, second)
			for _, err := range []error{err1, err2} {
				if err != nil && !errors.Is(err, auth.ErrInvalidRefresh) {
					t.Errorf("operation error = %v, want nil or ErrInvalidRefresh", err)
				}
			}
			if n := activeRefreshSessions(t, pool, family); n != 0 {
				t.Errorf("%d active sessions remain after logout, want 0", n)
			}
		})
	}
}

func TestAuthStore_GraceRetryRacingRotationOfTheLiveToken(t *testing.T) {
	for _, retryFirst := range []bool{true, false} {
		name := "rotation queued first"
		if retryFirst {
			name = "retry queued first"
		}
		t.Run(name, func(t *testing.T) {
			pool := NewPool(t)
			store := auth.NewStore(pool)
			userID := newRefreshTestUser(t, store, "family-retry-race@test.com")
			t1 := insertRefreshSession(t, store, userID)
			family := readRefreshSession(t, pool, t1).FamilyID
			t2 := randomTokenHash(t)
			if _, err := rotateWithDefaultWindow(store, t1, t2); err != nil {
				t.Fatalf("rotate: %v", err)
			}
			retryHash, rotateHash := randomTokenHash(t), randomTokenHash(t)

			retry := func() error { _, err := rotateWithDefaultWindow(store, t1, retryHash); return err }
			rotate := func() error { _, err := rotateWithDefaultWindow(store, t2, rotateHash); return err }
			first, second := rotate, retry
			if retryFirst {
				first, second = retry, rotate
			}
			err1, err2 := raceBehindFamilyLock(t, pool, family, first, second)
			if err1 != nil || err2 != nil {
				t.Errorf("errors = %v, %v, want both nil", err1, err2)
			}
			if n := activeRefreshSessions(t, pool, family); n != 1 {
				t.Errorf("%d active sessions, want exactly 1", n)
			}
		})
	}
}

func TestAuthStore_RotationJudgesTimeAfterTheLockWait(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	userID := newRefreshTestUser(t, store, "family-clock@test.com")
	oldHash := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, oldHash).FamilyID

	lock := holdFamilyLock(t, pool, family)
	done := make(chan error, 1)
	go func() {
		_, err := rotateWithDefaultWindow(store, oldHash, randomTokenHash(t))
		done <- err
	}()
	lock.waitForLockWaiters(t, 1)
	time.Sleep(2 * time.Second)
	var releasedAt time.Time
	if err := lock.tx.QueryRow(context.Background(), `SELECT clock_timestamp()`).Scan(&releasedAt); err != nil {
		t.Fatalf("read database time: %v", err)
	}
	lock.release(t)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("rotation error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("rotation did not finish after the lock was released")
	}

	spent := readRefreshSession(t, pool, oldHash)
	if spent.RotatedAt == nil || spent.RevokedAt == nil {
		t.Fatal("spent row is not marked")
	}
	if spent.RotatedAt.Before(releasedAt) || spent.RevokedAt.Before(releasedAt) {
		t.Errorf("rotated_at = %v, revoked_at = %v, earlier than the lock release at %v: the clock is the transaction start",
			spent.RotatedAt, spent.RevokedAt, releasedAt)
	}
}

func TestAuthStore_FamilyHoldsOneUnrevokedRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-unique@test.com")
	first := insertRefreshSession(t, store, userID)
	family := readRefreshSession(t, pool, first).FamilyID

	insertSecond := func() error {
		_, err := pool.Exec(ctx,
			`INSERT INTO mentorix.auth_refresh_sessions (user_id, family_id, token_hash, expires_at)
			 VALUES ($1, $2, $3, now() + interval '1 day')`, userID, family, randomTokenHash(t))
		return err
	}
	err := insertSecond()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "auth_refresh_sessions_family_id_live_uniq" {
		t.Fatalf("second unrevoked row in a family error = %v, want unique violation of auth_refresh_sessions_family_id_live_uniq", err)
	}
	if err := store.RevokeRefreshSession(ctx, first); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := insertSecond(); err != nil {
		t.Errorf("row next to a revoked one error = %v, want nil", err)
	}
}

func TestAuthStore_LogoutWithExpiredTokenLeavesTheFamily(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-logout-expired@test.com")

	t.Run("spent and expired token", func(t *testing.T) {
		t1 := insertRefreshSession(t, store, userID)
		family := readRefreshSession(t, pool, t1).FamilyID
		if _, err := rotateWithDefaultWindow(store, t1, randomTokenHash(t)); err != nil {
			t.Fatalf("rotate: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE mentorix.auth_refresh_sessions SET expires_at = now() - interval '1 hour' WHERE token_hash = $1`, t1); err != nil {
			t.Fatalf("expire: %v", err)
		}
		if err := store.RevokeRefreshSession(ctx, t1); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if n := activeRefreshSessions(t, pool, family); n != 1 {
			t.Errorf("%d active sessions after logout with an expired token, want 1", n)
		}
	})

	t.Run("unrevoked expired token", func(t *testing.T) {
		expired := randomTokenHash(t)
		if err := store.InsertRefreshSession(ctx, userID, expired, time.Now().UTC().Add(-time.Hour)); err != nil {
			t.Fatalf("insert expired: %v", err)
		}
		other := insertRefreshSession(t, store, userID)
		if err := store.RevokeRefreshSession(ctx, expired); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if row := readRefreshSession(t, pool, expired); row.RevokedAt == nil {
			t.Error("the expired row itself was not revoked")
		}
		if row := readRefreshSession(t, pool, other); row.RevokedAt != nil {
			t.Error("another family was revoked")
		}
	})
}

// The store never calls MarkRefreshFamilyRotated on a family whose only unrevoked
// row is expired, but a row can expire between the active check and the mark, so the
// query is called directly to show it spends that row too.
func TestAuthStore_MarkFamilyRotatedSpendsAnExpiredRow(t *testing.T) {
	pool := NewPool(t)
	store := auth.NewStore(pool)
	ctx := context.Background()
	userID := newRefreshTestUser(t, store, "family-mark-expired@test.com")
	expired := randomTokenHash(t)
	if err := store.InsertRefreshSession(ctx, userID, expired, time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	family := readRefreshSession(t, pool, expired).FamilyID

	if err := sqlc.New(pool).MarkRefreshFamilyRotated(ctx, pgconv.ToPGUUID(family)); err != nil {
		t.Fatalf("MarkRefreshFamilyRotated: %v", err)
	}
	var unrevoked int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM mentorix.auth_refresh_sessions WHERE family_id = $1 AND revoked_at IS NULL`,
		family).Scan(&unrevoked); err != nil {
		t.Fatalf("count unrevoked: %v", err)
	}
	if unrevoked != 0 {
		t.Errorf("%d unrevoked rows remain after the mark, want 0", unrevoked)
	}
	assertSpent(t, pool, "expired row", expired)
}

func randomTokenHash(t *testing.T) []byte {
	t.Helper()
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}
