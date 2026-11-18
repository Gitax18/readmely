//go:build integration

package store_test

import (
	"crypto/sha256"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

// h stands in for the HMAC of a code or the SHA-256 of a token: 32 bytes derived from s.
func h(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func (e *env) challenge(t *testing.T, email, code string) store.LoginChallenge {
	t.Helper()
	ip := netip.MustParseAddr("203.0.113.7")
	c, err := e.q.CreateLoginChallenge(ctxT(t), store.CreateLoginChallengeParams{
		Email: email, CodeHash: h(code), TtlSeconds: 600, MaxAttempts: 5, RequestedIp: &ip,
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	return c
}

// attempt returns "ok" (right code, now closed), "wrong" (counted) or "none" (no usable code).
func (e *env) attempt(t *testing.T, email, code string) string {
	t.Helper()
	row, err := e.q.AttemptLoginChallenge(ctxT(t), store.AttemptLoginChallengeParams{Email: email, CodeHash: h(code)})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "none"
	case err != nil:
		t.Fatalf("attempt: %v", err)
	case row.ClosedAt != nil:
		return "ok"
	}
	return "wrong"
}

func (e *env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.Pool.QueryRow(ctxT(t), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func wantCode(t *testing.T, err error, code, constraint string) {
	t.Helper()
	var pe *pgconn.PgError
	if !errors.As(err, &pe) || pe.Code != code || (constraint != "" && pe.ConstraintName != constraint) {
		t.Fatalf("want %s %s, got %v", code, constraint, err)
	}
}

func TestACodeWorksOnceAndCountsEveryGuess(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.challenge(t, "ada@example.invalid", "013842")
	if c.Attempts != 0 || c.ClosedAt != nil || c.RequestedIp == nil || c.RequestedIp.String() != "203.0.113.7" {
		t.Fatalf("unexpected new challenge: %+v", c)
	}
	if got := e.attempt(t, "ada@example.invalid", "999999"); got != "wrong" {
		t.Fatalf("wrong code: %s", got)
	}
	if got := e.attempt(t, "ada@example.invalid", "013842"); got != "ok" {
		t.Fatalf("right code: %s", got)
	}
	if got := e.attempt(t, "ada@example.invalid", "013842"); got != "none" {
		t.Fatalf("a used code must not work again: %s", got)
	}
	if n := e.count(t, `SELECT attempts FROM login_challenges WHERE id = $1`, c.ID); n != 2 {
		t.Fatalf("attempts = %d, want 2", n)
	}
}

func TestFiveWrongGuessesLockTheCode(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.challenge(t, "ada@example.invalid", "013842")
	for i := range 5 {
		if got := e.attempt(t, "ada@example.invalid", "000000"); got != "wrong" {
			t.Fatalf("guess %d: %s", i+1, got)
		}
	}
	if got := e.attempt(t, "ada@example.invalid", "013842"); got != "none" {
		t.Fatalf("the right code after five wrong guesses must fail: %s", got)
	}
}

func TestExpiredAndReplacedCodesDoNotWork(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)

	old := e.challenge(t, "ada@example.invalid", "111111")
	e.challenge(t, "ada@example.invalid", "222222")
	if got := e.attempt(t, "ada@example.invalid", "111111"); got != "wrong" {
		t.Fatalf("a replaced code must not sign in: %s", got)
	}
	if n := e.count(t, `SELECT count(*) FROM login_challenges WHERE id = $1 AND closed_at IS NOT NULL`, old.ID); n != 1 {
		t.Fatal("the replaced challenge was not closed")
	}
	if got := e.attempt(t, "ada@example.invalid", "222222"); got != "ok" {
		t.Fatalf("the newest code must work: %s", got)
	}

	e.challenge(t, "bob@example.invalid", "333333")
	if _, err := e.db.Pool.Exec(ctx, `UPDATE login_challenges SET created_at = now() - interval '11 minutes', expires_at = now() - interval '1 minute' WHERE email = 'bob@example.invalid'`); err != nil {
		t.Fatal(err)
	}
	if got := e.attempt(t, "bob@example.invalid", "333333"); got != "none" {
		t.Fatalf("an expired code must not work: %s", got)
	}
	if got := e.attempt(t, "nobody@example.invalid", "333333"); got != "none" {
		t.Fatalf("an unknown address has no code: %s", got)
	}
}

// Concurrent requests for one address leave exactly one usable code, and concurrent right
// guesses sign in exactly once.
func TestCodesUnderConcurrency(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		wg.Go(func() {
			_, err := e.q.CreateLoginChallenge(ctxT(t), store.CreateLoginChallengeParams{
				Email: "ada@example.invalid", CodeHash: h("424242"), TtlSeconds: 600, MaxAttempts: 5,
			})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM login_challenges WHERE email = 'ada@example.invalid' AND closed_at IS NULL`); n != 1 {
		t.Fatalf("%d open codes for one address, want 1", n)
	}

	results := make(chan string, 20)
	for range 20 {
		wg.Go(func() { results <- e.attempt(t, "ada@example.invalid", "424242") })
	}
	wg.Wait()
	close(results)
	wins := 0
	for r := range results {
		if r == "ok" {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("one code signed in %d times, want exactly once", wins)
	}
}

func TestChallengeRules(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	create := func(email string, hash []byte, ttl, maxAttempts int32) error {
		_, err := e.q.CreateLoginChallenge(ctx, store.CreateLoginChallengeParams{Email: email, CodeHash: hash, TtlSeconds: ttl, MaxAttempts: maxAttempts})
		return err
	}
	wantCode(t, create("Ada@example.invalid", h("x"), 600, 5), "23514", "login_challenges_email_check")
	wantCode(t, create(" ada@example.invalid", h("x"), 600, 5), "23514", "login_challenges_email_check")
	wantCode(t, create("ada@example.invalid", []byte("short"), 600, 5), "23514", "login_challenges_code_hash_check")
	wantCode(t, create("ada@example.invalid", h("x"), 7200, 5), "23514", "login_challenges_expires_at_check")
	wantCode(t, create("ada@example.invalid", h("x"), 600, 50), "23514", "login_challenges_max_attempts_check")
	// The partial unique index holds even for a writer that bypasses the function.
	e.challenge(t, "ada@example.invalid", "123456")
	_, err := e.db.Pool.Exec(ctx, `INSERT INTO login_challenges (email, code_hash, expires_at) VALUES ('ada@example.invalid', $1, now() + interval '5 minutes')`, h("y"))
	wantCode(t, err, "23505", "login_challenges_email_open_key")
}

func TestSignInCreatesOrVerifiesTheAccount(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)

	fresh, err := e.q.UpsertVerifiedUser(ctx, "new@example.invalid")
	if err != nil || fresh.EmailVerifiedAt == nil {
		t.Fatalf("first sign-in: %+v %v", fresh, err)
	}
	existing := e.user(t, "Bob@Example.invalid") // created earlier, not verified, different case
	got, err := e.q.UpsertVerifiedUser(ctx, "bob@example.invalid")
	if err != nil || got.ID != existing.ID || got.EmailVerifiedAt == nil {
		t.Fatalf("existing account: got %+v (want id %s) %v", got, existing.ID, err)
	}
	again, err := e.q.UpsertVerifiedUser(ctx, "bob@example.invalid")
	if err != nil || again.ID != existing.ID || !again.EmailVerifiedAt.Equal(*got.EmailVerifiedAt) {
		t.Fatalf("a second sign-in must keep the account and its first verification time: %+v %v", again, err)
	}
	if n := e.count(t, `SELECT count(*) FROM users WHERE lower(email) = 'bob@example.invalid'`); n != 1 {
		t.Fatalf("%d accounts for one address", n)
	}
}

func (e *env) session(t *testing.T, user store.User, token string) store.Session {
	t.Helper()
	ip := netip.MustParseAddr("2001:db8::1")
	s, err := e.q.CreateSession(ctxT(t), store.CreateSessionParams{
		UserID: user.ID, TokenHash: h(token), AbsoluteSeconds: 30 * 86400, IdleSeconds: 7 * 86400,
		UserAgent: "Safari on macOS", Ip: &ip,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return s
}

func (e *env) lookup(t *testing.T, token string) (store.GetSessionByTokenHashRow, bool) {
	t.Helper()
	row, err := e.q.GetSessionByTokenHash(ctxT(t), h(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return row, true
}

func TestSessionLifecycle(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada := e.user(t, "ada@example.invalid")

	s := e.session(t, ada, "token-a")
	if s.ExpiresAt.Sub(s.CreatedAt) < 29*24*time.Hour || s.IdleExpiresAt.Sub(s.CreatedAt) > 8*24*time.Hour {
		t.Fatalf("unexpected lifetimes: %+v", s)
	}
	row, ok := e.lookup(t, "token-a")
	if !ok || row.Session.ID != s.ID || row.User.ID != ada.ID {
		t.Fatalf("lookup: %+v %v", row, ok)
	}
	if _, ok := e.lookup(t, "token-unknown"); ok {
		t.Fatal("an unknown token found a session")
	}

	t.Run("touch is throttled and never passes the absolute expiry", func(t *testing.T) {
		n, err := e.q.TouchSession(ctx, store.TouchSessionParams{ID: s.ID, IdleSeconds: 7 * 86400, MinIntervalSeconds: 300})
		if err != nil || n != 0 {
			t.Fatalf("a touch within the interval changed %d rows (%v)", n, err)
		}
		if _, err := e.db.Pool.Exec(ctx, `UPDATE sessions SET last_seen_at = now() - interval '10 minutes', expires_at = created_at + interval '1 day', idle_expires_at = created_at + interval '1 day' WHERE id = $1`, s.ID); err != nil {
			t.Fatal(err)
		}
		n, err = e.q.TouchSession(ctx, store.TouchSessionParams{ID: s.ID, IdleSeconds: 7 * 86400, MinIntervalSeconds: 300})
		if err != nil || n != 1 {
			t.Fatalf("touch: %d %v", n, err)
		}
		if bad := e.count(t, `SELECT count(*) FROM sessions WHERE id = $1 AND idle_expires_at > expires_at`, s.ID); bad != 0 {
			t.Fatal("idle expiry slid past the absolute expiry")
		}
	})

	t.Run("an idle session ends even with the cookie", func(t *testing.T) {
		e.session(t, ada, "token-idle")
		if _, err := e.db.Pool.Exec(ctx, `UPDATE sessions SET idle_expires_at = now() - interval '1 second' WHERE token_hash = $1`, h("token-idle")); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.lookup(t, "token-idle"); ok {
			t.Fatal("an idle-expired session still works")
		}
	})

	t.Run("a suspended user's sessions stop working", func(t *testing.T) {
		carol := e.user(t, "carol@example.invalid")
		e.session(t, carol, "token-carol")
		if _, err := e.q.SetUserStatus(ctx, store.SetUserStatusParams{ID: carol.ID, Status: "suspended"}); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.lookup(t, "token-carol"); ok {
			t.Fatal("a suspended user's session still works")
		}
	})

	t.Run("logout ends exactly that session", func(t *testing.T) {
		e.session(t, ada, "token-b")
		n, err := e.q.DeleteSessionByTokenHash(ctx, h("token-b"))
		if err != nil || n != 1 {
			t.Fatalf("logout: %d %v", n, err)
		}
		if _, ok := e.lookup(t, "token-b"); ok {
			t.Fatal("a logged-out session still works")
		}
		if _, ok := e.lookup(t, "token-a"); !ok {
			t.Fatal("logout ended another session")
		}
	})

	t.Run("cleanup removes ended sessions and old challenges only", func(t *testing.T) {
		e.challenge(t, "ada@example.invalid", "555555")
		if _, err := e.db.Pool.Exec(ctx, `INSERT INTO login_challenges (email, code_hash, created_at, expires_at, closed_at) VALUES ('old@example.invalid', $1, now() - interval '31 days', now() - interval '31 days' + interval '10 minutes', now() - interval '31 days')`, h("old")); err != nil {
			t.Fatal(err)
		}
		before := e.count(t, `SELECT count(*) FROM sessions WHERE idle_expires_at > now()`)
		if _, err := e.q.DeleteEndedSessions(ctx); err != nil {
			t.Fatal(err)
		}
		if n := e.count(t, `SELECT count(*) FROM sessions WHERE idle_expires_at <= now()`); n != 0 {
			t.Fatalf("%d ended sessions left", n)
		}
		if after := e.count(t, `SELECT count(*) FROM sessions`); after != before {
			t.Fatalf("live sessions %d -> %d", before, after)
		}
		n, err := e.q.DeleteOldLoginChallenges(ctx, 30)
		if err != nil || n != 1 {
			t.Fatalf("challenge cleanup removed %d (%v), want 1", n, err)
		}
	})
}

func TestOneUserCannotSeeOrEndAnothersSessions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada, bob := e.user(t, "ada@example.invalid"), e.user(t, "bob@example.invalid")
	adaSession := e.session(t, ada, "token-ada")
	e.session(t, bob, "token-bob")

	list, err := e.q.ListUserSessions(ctx, bob.ID)
	if err != nil || len(list) != 1 || list[0].UserID != bob.ID {
		t.Fatalf("bob's list: %+v %v", list, err)
	}
	n, err := e.q.DeleteUserSession(ctx, store.DeleteUserSessionParams{ID: adaSession.ID, UserID: bob.ID})
	if err != nil || n != 0 {
		t.Fatalf("bob revoked ada's session: %d %v", n, err)
	}
	if _, ok := e.lookup(t, "token-ada"); !ok {
		t.Fatal("ada's session was ended by bob")
	}
	if n, err := e.q.DeleteUserSession(ctx, store.DeleteUserSessionParams{ID: adaSession.ID, UserID: ada.ID}); err != nil || n != 1 {
		t.Fatalf("ada revoking her own session: %d %v", n, err)
	}
	if _, err := e.q.DeleteUser(ctx, bob.ID); err != nil {
		t.Fatal(err)
	}
	if n := e.count(t, `SELECT count(*) FROM sessions WHERE user_id = $1`, bob.ID); n != 0 {
		t.Fatal("deleting an account left its sessions")
	}
}

func TestSessionRules(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ada := e.user(t, "ada@example.invalid")
	e.session(t, ada, "dup")
	_, err := e.q.CreateSession(ctx, store.CreateSessionParams{UserID: ada.ID, TokenHash: h("dup"), AbsoluteSeconds: 86400, IdleSeconds: 3600})
	wantCode(t, err, "23505", "sessions_token_hash_key")
	_, err = e.q.CreateSession(ctx, store.CreateSessionParams{UserID: ada.ID, TokenHash: []byte("short"), AbsoluteSeconds: 86400, IdleSeconds: 3600})
	wantCode(t, err, "23514", "sessions_token_hash_check")
	_, err = e.q.CreateSession(ctx, store.CreateSessionParams{UserID: ada.ID, TokenHash: h("long"), AbsoluteSeconds: 365 * 86400, IdleSeconds: 3600})
	wantCode(t, err, "23514", "sessions_expires_at_check")
	_, err = e.db.Pool.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at, idle_expires_at) VALUES ($1, $2, now() + interval '1 day', now() + interval '2 days')`, ada.ID, h("idle"))
	wantCode(t, err, "23514", "sessions_idle_expires_at_check")
}

// The AI service never sees credentials; the API may issue codes but not alter the schema.
func TestAuthTablePrivileges(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := ctxT(t)
	ai := e.db.AsRole(t, "readmely_ai")
	for _, q := range []string{`SELECT 1 FROM sessions`, `SELECT 1 FROM login_challenges`,
		`SELECT login_challenge_create('a@example.invalid', '\x00'::bytea, 60, 5, NULL)`} {
		if _, err := ai.Exec(ctx, q); err == nil {
			t.Errorf("readmely_ai may run %q", q)
		} else {
			wantCode(t, err, "42501", "")
		}
	}
	app := e.db.AsRole(t, "readmely_app")
	if _, err := store.New(app).CreateLoginChallenge(ctx, store.CreateLoginChallengeParams{
		Email: "ada@example.invalid", CodeHash: h("c"), TtlSeconds: 600, MaxAttempts: 5,
	}); err != nil {
		t.Fatalf("readmely_app cannot issue a code: %v", err)
	}
	if _, err := app.Exec(ctx, `ALTER TABLE sessions ADD COLUMN x int`); err == nil {
		t.Error("readmely_app may alter the sessions table")
	}
}
