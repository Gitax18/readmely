package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/mail"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

// WelcomeCredits is the one-time grant every new account receives.
const WelcomeCredits int64 = 100

// The lifetimes and limits from the approved plan. They are constants, not settings: changing
// one is a security decision that should go through review.
const (
	CodeTTL         = 10 * time.Minute
	MaxCodeAttempts = 5
	SessionIdle     = 7 * 24 * time.Hour
	SessionAbsolute = 30 * 24 * time.Hour
	// touchEvery bounds how often a busy session writes to the database.
	touchEvery       = time.Minute
	challengeKeepFor = 30 * 24 * time.Hour
	maxUserAgent     = 200
)

// The rate limits (plans/phase-2a-email-login.md, "Mail bombing and cost abuse").
var (
	ruleStartEmailMinute = ratelimit.Rule{Name: "login_start_email_minute", Max: 1, Window: time.Minute}
	ruleStartEmailHour   = ratelimit.Rule{Name: "login_start_email_hour", Max: 5, Window: time.Hour}
	ruleStartIPHour      = ratelimit.Rule{Name: "login_start_ip_hour", Max: 20, Window: time.Hour}
	ruleVerifyIP         = ratelimit.Rule{Name: "login_verify_ip", Max: 30, Window: 15 * time.Minute}
)

// Errors the handlers translate into the documented responses.
var (
	ErrInvalidCode     = errors.New("invalid code") // wrong, expired, used, superseded, unknown address or suspended: deliberately one error
	ErrUnauthenticated = errors.New("not signed in")
	ErrNotFound        = errors.New("not found")
)

// RateLimitedError carries how long the caller should wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Service is the sign-in and session logic. It has no HTTP in it.
type Service struct {
	pool    *pgxpool.Pool
	q       *store.Queries
	limiter *ratelimit.Limiter
	mail    *mail.Queue
	brand   mail.Brand // builds the emails; without a web origin they carry no logo or links
	keys    Keys
	logger  *slog.Logger
	oauth   *OAuthConfig // nil until EnableOAuth: no provider sign-in

	// onIdentityMiss is a test seam: it runs between "this provider account is not known" and the
	// lookup by address, the one moment a concurrent first sign-in can slip in. Nil in production.
	onIdentityMiss func()
}

// NewService wires the service. The mail queue may be shared with other features.
func NewService(pool *pgxpool.Pool, limiter *ratelimit.Limiter, queue *mail.Queue, keys Keys, logger *slog.Logger) *Service {
	return &Service{pool: pool, q: store.New(pool), limiter: limiter, mail: queue, keys: keys, logger: logger}
}

// SetWebOrigin gives the emails the web app's address, for the logo and the links in them.
func (s *Service) SetWebOrigin(origin string) { s.brand = mail.NewBrand(origin) }

// Keys returns the key set, for the HTTP layer's CSRF checks.
func (s *Service) Keys() Keys { return s.keys }

func addrPtr(a netip.Addr) *netip.Addr {
	if !a.IsValid() {
		return nil
	}
	return &a
}

func truncateUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if utf8.RuneCountInString(ua) > maxUserAgent {
		ua = string([]rune(ua)[:maxUserAgent])
	}
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, ua)
}

// StartLogin issues a code for the address and queues the email. It does the same work, in the
// same order, whether or not the address has an account: it never looks the account up.
func (s *Service) StartLogin(ctx context.Context, rawEmail string, ip netip.Addr) error {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	ipKey := clientip.RateKey(ip)
	d, err := s.limiter.Allow(ctx,
		ratelimit.Check{Rule: ruleStartEmailMinute, Subject: email},
		ratelimit.Check{Rule: ruleStartEmailHour, Subject: email},
		ratelimit.Check{Rule: ruleStartIPHour, Subject: ipKey},
	)
	if err != nil {
		return fmt.Errorf("start login: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}

	code, err := NewCode()
	if err != nil {
		return err
	}
	if _, err := s.q.CreateLoginChallenge(ctx, store.CreateLoginChallengeParams{
		Email:       email,
		CodeHash:    s.keys.CodeHash(email, code),
		TtlSeconds:  int32(CodeTTL / time.Second),
		MaxAttempts: MaxCodeAttempts,
		RequestedIp: addrPtr(ip),
	}); err != nil {
		return fmt.Errorf("start login: %w", err)
	}

	// Never awaited, and a full queue changes nothing the caller can see.
	s.mail.Enqueue(s.brand.SignInCode(email, code, int(CodeTTL/time.Minute)))
	return nil
}

// LoggedIn is the result of a successful sign-in.
type LoggedIn struct {
	Token   string // the cookie value; shown once, never stored
	Session store.Session
	User    store.User
}

// VerifyLogin checks a code and, if it is right, signs the person in. Every way to fail
// except being rate limited or an unexpected error is ErrInvalidCode.
func (s *Service) VerifyLogin(ctx context.Context, rawEmail, code string, ip netip.Addr, userAgent string) (*LoggedIn, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return nil, err
	}
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: ruleVerifyIP, Subject: clientip.RateKey(ip)})
	if err != nil {
		return nil, fmt.Errorf("verify login: %w", err)
	}
	if !d.Allowed {
		return nil, &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	if !ValidCodeFormat(code) {
		return nil, ErrInvalidCode
	}

	// One guess, counted whether or not it is right. Committed on its own so a later failure
	// cannot give the guess back.
	attempt, err := s.q.AttemptLoginChallenge(ctx, store.AttemptLoginChallengeParams{
		Email: email, CodeHash: s.keys.CodeHash(email, code),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCode
	}
	if err != nil {
		return nil, fmt.Errorf("verify login: %w", err)
	}
	if attempt.ClosedAt == nil {
		return nil, ErrInvalidCode
	}

	var out *LoggedIn
	newDevice, created := false, false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		_, lookup := q.GetUserByEmail(ctx, email)
		if lookup != nil && !errors.Is(lookup, pgx.ErrNoRows) {
			return lookup
		}
		existed := lookup == nil
		created = !existed
		user, err := q.UpsertVerifiedUser(ctx, email)
		if err != nil {
			return err
		}
		if !existed {
			if err := q.GrantWelcomeCredits(ctx, store.GrantWelcomeCreditsParams{UserID: user.ID, Amount: WelcomeCredits}); err != nil {
				return err
			}
		}
		if user.Status != "active" {
			return ErrInvalidCode
		}
		out, newDevice, err = s.startSession(ctx, q, user, existed, ip, userAgent)
		return err
	})
	if errors.Is(err, ErrInvalidCode) {
		return nil, ErrInvalidCode
	}
	if err != nil {
		return nil, fmt.Errorf("verify login: %w", err)
	}
	switch {
	case created:
		s.welcome(out.User)
	case newDevice:
		s.alertNewDevice(ctx, out.User, "an emailed code", userAgent)
	}
	return out, nil
}

// Principal is who a request is acting as.
type Principal struct {
	Session store.Session
	User    store.User
}

// Authenticate resolves a cookie value to a live session. Idle and absolute expiry and the
// user's status are enforced in the query, not trusted from the cookie.
func (s *Service) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if token == "" || len(token) > 128 {
		return nil, ErrUnauthenticated
	}
	row, err := s.q.GetSessionByTokenHash(ctx, HashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	// Sliding idle expiry. A failure here must not sign the person out.
	if _, err := s.q.TouchSession(ctx, store.TouchSessionParams{
		ID: row.Session.ID, IdleSeconds: int32(SessionIdle / time.Second), MinIntervalSeconds: int32(touchEvery / time.Second),
	}); err != nil {
		s.logger.WarnContext(ctx, "session touch failed", "error", err)
	}
	return &Principal{Session: row.Session, User: row.User}, nil
}

// Logout ends the session the request came in on.
func (s *Service) Logout(ctx context.Context, p *Principal) error {
	if _, err := s.q.DeleteUserSession(ctx, store.DeleteUserSessionParams{ID: p.Session.ID, UserID: p.User.ID}); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// ListSessions returns the user's live sessions, newest first.
func (s *Service) ListSessions(ctx context.Context, p *Principal) ([]store.Session, error) {
	rows, err := s.q.ListUserSessions(ctx, p.User.ID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	return rows, nil
}

// RevokeSession ends one of the user's own sessions. Someone else's id is ErrNotFound.
func (s *Service) RevokeSession(ctx context.Context, p *Principal, id uuid.UUID) error {
	n, err := s.q.DeleteUserSession(ctx, store.DeleteUserSessionParams{ID: id, UserID: p.User.ID})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDisplayName validates and saves a new display name.
func (s *Service) UpdateDisplayName(ctx context.Context, p *Principal, raw string) (store.User, error) {
	name, err := CleanDisplayName(raw)
	if err != nil {
		return store.User{}, err
	}
	u, err := s.q.UpdateUserDisplayName(ctx, store.UpdateUserDisplayNameParams{ID: p.User.ID, DisplayName: name})
	if err != nil {
		return store.User{}, fmt.Errorf("update display name: %w", err)
	}
	return u, nil
}

// Cleanup removes ended sessions and challenges past their retention. It touches nothing else.
func (s *Service) Cleanup(ctx context.Context) (sessions, challenges int64, err error) {
	if sessions, err = s.q.DeleteEndedSessions(ctx); err != nil {
		return 0, 0, fmt.Errorf("cleanup sessions: %w", err)
	}
	if challenges, err = s.q.DeleteOldLoginChallenges(ctx, int32(challengeKeepFor/(24*time.Hour))); err != nil {
		return sessions, 0, fmt.Errorf("cleanup challenges: %w", err)
	}
	return sessions, challenges, nil
}

// RunCleanup runs Cleanup now and then every interval until ctx ends.
func (s *Service) RunCleanup(ctx context.Context, interval time.Duration) {
	run := func() {
		sess, ch, err := s.Cleanup(ctx)
		if err != nil {
			if ctx.Err() == nil {
				s.logger.Error("auth cleanup failed", "error", err)
			}
			return
		}
		if sess > 0 || ch > 0 {
			s.logger.Info("auth cleanup", "sessions_removed", sess, "challenges_removed", ch)
		}
	}
	run()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
