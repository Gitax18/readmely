package auth

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/mail"
	"github.com/Gitax18/readmely/services/core/internal/oauth"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

// The error codes on /login?error=. Fixed, so nothing a provider says ever reaches the page.
const (
	ErrCodeAccessDenied       = "access_denied"
	ErrCodeInvalidState       = "invalid_state"
	ErrCodeNoVerifiedEmail    = "no_verified_email"
	ErrCodeProviderConflict   = "provider_conflict"
	ErrCodeIdentityInUse      = "identity_in_use"
	ErrCodeUnsupportedEmail   = "unsupported_email"
	ErrCodeProviderError      = "provider_error"
	ErrCodeAccountUnavailable = "account_unavailable"
)

const (
	maxCallbackCode  = 2048
	exchangeDeadline = 20 * time.Second
	maxAlertsPerHour = 5
)

var (
	ruleOAuthStartIP    = ratelimit.Rule{Name: "oauth_start_ip", Max: 30, Window: 15 * time.Minute}
	ruleOAuthCallbackIP = ratelimit.Rule{Name: "oauth_callback_ip", Max: 30, Window: 15 * time.Minute}
	ruleAlertUser       = ratelimit.Rule{Name: "alert_user_hour", Max: maxAlertsPerHour, Window: time.Hour}
)

// ErrConflict is the request being right but the state not: the provider is already connected.
var ErrConflict = errors.New("conflict")

// OAuthConfig wires provider sign-in. Providers holds only the configured ones.
type OAuthConfig struct {
	Providers map[string]oauth.Provider
	Store     *oauth.Store
	WebOrigin string // the redirect URIs are derived from it
}

// EnableOAuth turns provider sign-in on. Without it, no provider is available.
func (s *Service) EnableOAuth(c OAuthConfig) { s.oauth = &c }

// Providers lists the configured providers in a stable order.
func (s *Service) Providers() []string {
	var out []string
	if s.oauth == nil {
		return out
	}
	for _, name := range []string{oauth.Google, oauth.GitHub} {
		if _, ok := s.oauth.Providers[name]; ok {
			out = append(out, name)
		}
	}
	return out
}

func (s *Service) provider(name string) (oauth.Provider, bool) {
	if s.oauth == nil {
		return nil, false
	}
	p, ok := s.oauth.Providers[name]
	return p, ok
}

func (s *Service) redirectURI(provider string) string {
	return strings.TrimSuffix(s.oauth.WebOrigin, "/") + "/api/v1/auth/" + provider + "/callback"
}

// ValidProvider reports whether name is a provider Readmely knows (configured or not).
func ValidProvider(name string) bool { return name == oauth.Google || name == oauth.GitHub }

func providerLabel(name string) string {
	if name == oauth.GitHub {
		return "GitHub"
	}
	return "Google"
}

// begin stores a new attempt and returns where to send the browser and the binding for its cookie.
func (s *Service) begin(ctx context.Context, prov oauth.Provider, a oauth.Attempt) (authURL, binding string, err error) {
	if a.Verifier, err = oauth.NewVerifier(); err != nil {
		return "", "", err
	}
	state, binding, err := s.oauth.Store.Begin(ctx, a)
	if err != nil {
		return "", "", err
	}
	return prov.AuthorizationURL(state, oauth.Challenge(a.Verifier), s.redirectURI(prov.Name())), binding, nil
}

func (s *Service) allow(ctx context.Context, rule ratelimit.Rule, ip netip.Addr) error {
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: rule, Subject: clientip.RateKey(ip)})
	if err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	return nil
}

// StartOAuth begins a sign-in. Unknown or unconfigured providers are ErrNotFound.
func (s *Service) StartOAuth(ctx context.Context, provider string, ip netip.Addr) (authURL, binding string, err error) {
	prov, ok := s.provider(provider)
	if !ok {
		return "", "", ErrNotFound
	}
	if err := s.allow(ctx, ruleOAuthStartIP, ip); err != nil {
		return "", "", err
	}
	return s.begin(ctx, prov, oauth.Attempt{Provider: provider, Intent: oauth.IntentLogin})
}

// StartConnect begins connecting a provider to the signed-in user. The attempt remembers the user
// and the session, and the callback insists on both.
func (s *Service) StartConnect(ctx context.Context, p *Principal, provider string, ip netip.Addr) (authURL, binding string, err error) {
	prov, ok := s.provider(provider)
	if !ok {
		return "", "", ErrNotFound
	}
	if err := s.allow(ctx, ruleOAuthStartIP, ip); err != nil {
		return "", "", err
	}
	if _, err := s.q.GetUserIdentity(ctx, store.GetUserIdentityParams{UserID: p.User.ID, Provider: provider}); err == nil {
		return "", "", ErrConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("start connect: %w", err)
	}
	return s.begin(ctx, prov, oauth.Attempt{
		Provider: provider, Intent: oauth.IntentLink, UserID: p.User.ID.String(), SessionID: p.Session.ID.String(),
	})
}

// Callback is what arrives when the provider sends the browser back.
type Callback struct {
	Provider      string
	Code, State   string
	ProviderError string // the "error" query parameter; only compared, never shown
	Binding       string // the attempt cookie
	SessionToken  string // the session cookie; used only to finish a connect
	IP            netip.Addr
	UserAgent     string
}

// CallbackResult is where the browser goes next: a session and /account, or an error code.
type CallbackResult struct {
	ErrCode  string
	LoggedIn *LoggedIn // set only when a new session was created
}

func refuse(code string) (CallbackResult, error) { return CallbackResult{ErrCode: code}, nil }

// FinishOAuth completes a sign-in or a connect. It returns an error only for an unknown provider
// and for rate limiting; every other outcome is a CallbackResult with a fixed code.
func (s *Service) FinishOAuth(ctx context.Context, c Callback) (CallbackResult, error) {
	prov, ok := s.provider(c.Provider)
	if !ok {
		return CallbackResult{}, ErrNotFound
	}
	if err := s.allow(ctx, ruleOAuthCallbackIP, c.IP); err != nil {
		return CallbackResult{}, err
	}

	// The attempt is spent here, whatever happens next.
	attempt, err := s.oauth.Store.Take(ctx, c.State, c.Binding)
	if errors.Is(err, oauth.ErrInvalidState) {
		return refuse(ErrCodeInvalidState)
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "oauth state store failed", "provider", c.Provider, "error", err)
		return refuse(ErrCodeProviderError)
	}
	if attempt.Provider != c.Provider {
		return refuse(ErrCodeInvalidState)
	}
	if c.ProviderError != "" {
		if c.ProviderError == "access_denied" {
			return refuse(ErrCodeAccessDenied)
		}
		return refuse(ErrCodeProviderError)
	}
	if c.Code == "" || len(c.Code) > maxCallbackCode {
		return refuse(ErrCodeProviderError)
	}

	xctx, cancel := context.WithTimeout(ctx, exchangeDeadline)
	defer cancel()
	profile, err := prov.Exchange(xctx, c.Code, attempt.Verifier, s.redirectURI(c.Provider))
	if err != nil {
		// The error names the step and status only; nothing the provider sent is in it.
		s.logger.WarnContext(ctx, "oauth exchange failed", "provider", c.Provider, "error", err)
		return refuse(ErrCodeProviderError)
	}
	if !profile.EmailVerified || strings.TrimSpace(profile.Email) == "" {
		return refuse(ErrCodeNoVerifiedEmail)
	}
	email, err := NormalizeEmail(profile.Email)
	if err != nil {
		return refuse(ErrCodeUnsupportedEmail)
	}
	name, _ := CleanDisplayName(profile.Name) // a name that fails the rules is simply not used

	switch attempt.Intent {
	case oauth.IntentLogin:
		return s.finishLogin(ctx, c, profile.Account, email, name)
	case oauth.IntentLink:
		return s.finishLink(ctx, c, attempt, profile.Account, email)
	}
	return refuse(ErrCodeInvalidState)
}

// refusal rolls a transaction back and carries the code out of it.
type refusal struct{ code string }

func (r *refusal) Error() string { return "refused: " + r.code }

func isUnique(err error, constraint string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == constraint
}

type loginOutcome struct {
	in        *LoggedIn
	attached  bool // an existing account just gained this provider
	newDevice bool
}

func (s *Service) finishLogin(ctx context.Context, c Callback, account, email, name string) (CallbackResult, error) {
	// Two first sign-ins for one provider account race on the unique key; the loser retries and
	// finds the winner's identity.
	for range 2 {
		out, err := s.loginTx(ctx, c, account, email, name)
		var ref *refusal
		switch {
		case err == nil:
			label := providerLabel(c.Provider)
			if out.attached {
				s.alertConnected(ctx, out.in.User, label, email, c.UserAgent)
			} else if out.newDevice {
				s.alertNewDevice(ctx, out.in.User, label, c.UserAgent)
			}
			return CallbackResult{LoggedIn: out.in}, nil
		case errors.As(err, &ref):
			return refuse(ref.code)
		case isUnique(err, "oauth_identities_provider_account_key"):
			continue
		case isUnique(err, "oauth_identities_user_provider_key"):
			return refuse(ErrCodeProviderConflict)
		default:
			s.logger.ErrorContext(ctx, "oauth sign-in failed", "provider", c.Provider, "error", err)
			return refuse(ErrCodeProviderError)
		}
	}
	return refuse(ErrCodeProviderError)
}

func (s *Service) loginTx(ctx context.Context, c Callback, account, email, name string) (*loginOutcome, error) {
	var out loginOutcome
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		var user store.User
		existed := true

		ident, err := q.GetIdentityByProviderAccount(ctx, store.GetIdentityByProviderAccountParams{Provider: c.Provider, ProviderUserID: account})
		switch {
		case err == nil:
			// Known provider account: this user, whatever the email says today.
			user = ident.User
			if user.Status != "active" {
				return &refusal{ErrCodeAccountUnavailable}
			}
			if err := q.TouchIdentityLogin(ctx, store.TouchIdentityLoginParams{ID: ident.OauthIdentity.ID, Email: email}); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			found, err := q.GetUserByEmail(ctx, email)
			switch {
			case err == nil:
				if found.Status != "active" {
					return &refusal{ErrCodeAccountUnavailable}
				}
				if _, err := q.GetUserIdentity(ctx, store.GetUserIdentityParams{UserID: found.ID, Provider: c.Provider}); err == nil {
					return &refusal{ErrCodeProviderConflict} // a different account of this provider is already attached
				} else if !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
			case errors.Is(err, pgx.ErrNoRows):
				existed = false
			default:
				return err
			}
			if user, err = q.UpsertVerifiedUserWithName(ctx, store.UpsertVerifiedUserWithNameParams{Email: email, DisplayName: name}); err != nil {
				return err
			}
			if user.Status != "active" {
				return &refusal{ErrCodeAccountUnavailable}
			}
			if _, err := q.CreateIdentity(ctx, store.CreateIdentityParams{UserID: user.ID, Provider: c.Provider, ProviderUserID: account, Email: email}); err != nil {
				return err
			}
			out.attached = existed
		default:
			return err
		}

		in, newDevice, err := s.startSession(ctx, q, user, existed, c.IP, c.UserAgent)
		if err != nil {
			return err
		}
		out.in, out.newDevice = in, newDevice
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) finishLink(ctx context.Context, c Callback, a *oauth.Attempt, account, email string) (CallbackResult, error) {
	// The connect must finish in the session that started it, as the user who started it.
	if c.SessionToken == "" {
		return refuse(ErrCodeInvalidState)
	}
	p, err := s.Authenticate(ctx, c.SessionToken)
	if errors.Is(err, ErrUnauthenticated) {
		return refuse(ErrCodeInvalidState)
	}
	if err != nil {
		s.logger.ErrorContext(ctx, "oauth connect failed", "provider", c.Provider, "error", err)
		return refuse(ErrCodeProviderError)
	}
	if a.UserID != p.User.ID.String() || a.SessionID != p.Session.ID.String() {
		return refuse(ErrCodeInvalidState)
	}

	created := false
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		ident, err := q.GetIdentityByProviderAccount(ctx, store.GetIdentityByProviderAccountParams{Provider: c.Provider, ProviderUserID: account})
		switch {
		case err == nil && ident.User.ID == p.User.ID:
			return nil // already connected to this user: nothing to do
		case err == nil:
			return &refusal{ErrCodeIdentityInUse}
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if _, err := q.GetUserIdentity(ctx, store.GetUserIdentityParams{UserID: p.User.ID, Provider: c.Provider}); err == nil {
			return &refusal{ErrCodeProviderConflict}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := q.CreateIdentity(ctx, store.CreateIdentityParams{UserID: p.User.ID, Provider: c.Provider, ProviderUserID: account, Email: email}); err != nil {
			return err
		}
		created = true
		return nil
	})
	var ref *refusal
	switch {
	case err == nil:
		if created {
			s.alertConnected(ctx, p.User, providerLabel(c.Provider), email, c.UserAgent)
		}
		return CallbackResult{}, nil
	case errors.As(err, &ref):
		return refuse(ref.code)
	case isUnique(err, "oauth_identities_provider_account_key"):
		return refuse(ErrCodeIdentityInUse)
	case isUnique(err, "oauth_identities_user_provider_key"):
		return refuse(ErrCodeProviderConflict)
	}
	s.logger.ErrorContext(ctx, "oauth connect failed", "provider", c.Provider, "error", err)
	return refuse(ErrCodeProviderError)
}

// ListIdentities returns the user's connected provider accounts.
func (s *Service) ListIdentities(ctx context.Context, p *Principal) ([]store.OauthIdentity, error) {
	rows, err := s.q.ListUserIdentities(ctx, p.User.ID)
	if err != nil {
		return nil, fmt.Errorf("list identities: %w", err)
	}
	return rows, nil
}

// DisconnectIdentity removes the user's own account for a provider. It is always allowed: email
// codes keep working. Not connected is ErrNotFound.
func (s *Service) DisconnectIdentity(ctx context.Context, p *Principal, provider, userAgent string) error {
	n, err := s.q.DeleteUserIdentity(ctx, store.DeleteUserIdentityParams{UserID: p.User.ID, Provider: provider})
	if err != nil {
		return fmt.Errorf("disconnect identity: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	s.alert(ctx, p.User, providerLabel(provider)+" disconnected from your Readmely account",
		fmt.Sprintf("%s was disconnected from your Readmely account on %s, from %s.\n\n"+
			"You can still sign in with an emailed code. If this was not you, sign in, review your sessions under Account, and sign out of any you do not recognise.\n",
			providerLabel(provider), alertTime(), deviceOrUnknown(userAgent)))
	return nil
}

// startSession creates a session for the user inside the caller's transaction and reports whether
// it comes from a device that none of the user's live sessions share. A brand new account has no
// "usual device", so it never counts as new.
func (s *Service) startSession(ctx context.Context, q *store.Queries, user store.User, existed bool, ip netip.Addr, userAgent string) (*LoggedIn, bool, error) {
	newDevice := false
	if existed {
		live, err := q.ListUserSessions(ctx, user.ID)
		if err != nil {
			return nil, false, err
		}
		label := DeviceLabel(truncateUserAgent(userAgent))
		newDevice = true
		for _, sess := range live {
			if DeviceLabel(sess.UserAgent) == label {
				newDevice = false
				break
			}
		}
	}
	token, hash, err := NewSessionToken()
	if err != nil {
		return nil, false, err
	}
	sess, err := q.CreateSession(ctx, store.CreateSessionParams{
		UserID:          user.ID,
		TokenHash:       hash,
		AbsoluteSeconds: int32(SessionAbsolute / time.Second),
		IdleSeconds:     int32(SessionIdle / time.Second),
		UserAgent:       truncateUserAgent(userAgent),
		Ip:              addrPtr(ip),
	})
	if err != nil {
		return nil, false, err
	}
	return &LoggedIn{Token: token, Session: sess, User: user}, newDevice, nil
}

// ---- Alerts: an email to the account owner when something about sign-in changes. They carry the
// provider, the device and the time, never a link that signs anyone in, and at most
// maxAlertsPerHour go out per user per hour so they cannot become a mail-bombing channel.

func alertTime() string { return time.Now().UTC().Format("2006-01-02 15:04 UTC") }

func deviceOrUnknown(userAgent string) string {
	if d := DeviceLabel(truncateUserAgent(userAgent)); d != "" {
		return d
	}
	return "an unknown device"
}

func (s *Service) alert(ctx context.Context, user store.User, subject, text string) {
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: ruleAlertUser, Subject: user.ID.String()})
	if err != nil {
		s.logger.WarnContext(ctx, "alert skipped", "reason", "rate limiter unavailable", "error", err)
		return
	}
	if !d.Allowed {
		s.logger.InfoContext(ctx, "alert dropped", "reason", "per-user hourly cap", "user_id", user.ID)
		return
	}
	s.mail.Enqueue(mail.Message{To: user.Email, Subject: subject, Text: text})
}

func (s *Service) alertConnected(ctx context.Context, user store.User, label, providerEmail, userAgent string) {
	s.alert(ctx, user, label+" connected to your Readmely account",
		fmt.Sprintf("%s (%s) was connected to your Readmely account on %s, from %s.\n\n"+
			"If this was not you, sign in with an emailed code, disconnect it under Account, and sign out of any session you do not recognise.\n",
			label, providerEmail, alertTime(), deviceOrUnknown(userAgent)))
}

func (s *Service) alertNewDevice(ctx context.Context, user store.User, method, userAgent string) {
	s.alert(ctx, user, "New sign-in to your Readmely account",
		fmt.Sprintf("Your Readmely account was signed in from %s on %s, using %s.\n\n"+
			"If this was you, there is nothing to do. If not, sign in with an emailed code and sign out of any session you do not recognise under Account.\n",
			deviceOrUnknown(userAgent), alertTime(), method))
}
