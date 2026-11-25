//go:build integration

package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

const safari = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15"
const chrome = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

func ada(account string) fakeProfile {
	return fakeProfile{account: account, email: "ada@example.com", name: "Ada Lovelace", verified: true}
}

func (e *env) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) noAccountOrSession() {
	e.t.Helper()
	if n := e.count(`SELECT count(*) FROM users`); n != 0 {
		e.t.Errorf("users = %d, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 0 {
		e.t.Errorf("identities = %d, want 0", n)
	}
	if n := e.count(`SELECT count(*) FROM sessions`); n != 0 {
		e.t.Errorf("sessions = %d, want 0", n)
	}
}

// awaitMails waits until the address has received n emails and returns them.
func (e *env) awaitMails(to string, n int) []string {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if msgs := e.mail.to(to); len(msgs) >= n {
			var out []string
			for _, m := range msgs {
				out = append(out, m.Subject+"\n"+m.Text)
			}
			return out
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("%s received %d emails, want at least %d", to, len(e.mail.to(to)), n)
	return nil
}

// quiet gives any email that is going to be sent time to arrive, then returns how many the
// address has. Only for asserting that nothing was sent.
func (e *env) quiet(to string) int {
	time.Sleep(150 * time.Millisecond)
	return len(e.mail.to(to))
}

func TestProvidersListsOnlyConfigured(t *testing.T) {
	t.Parallel()
	off := newEnv(t, envOpts{})
	rec := off.do(req{method: "GET", path: "/v1/auth/providers"})
	if rec.Code != 200 || rec.Body.String() != "{\"providers\":[]}\n" && rec.Body.String() != "{\"providers\":[]}" {
		t.Fatalf("no providers configured: %d %s", rec.Code, rec.Body.String())
	}
	for _, p := range []string{"google", "github", "facebook"} {
		if rec := off.do(req{method: "GET", path: "/v1/auth/" + p + "/start"}); rec.Code != http.StatusNotFound {
			t.Errorf("start %s with none configured = %d, want 404", p, rec.Code)
		}
		if rec := off.do(req{method: "GET", path: "/v1/auth/" + p + "/callback?code=x&state=y"}); rec.Code != http.StatusNotFound {
			t.Errorf("callback %s with none configured = %d, want 404", p, rec.Code)
		}
	}
	on := newEnv(t, envOpts{oauth: true})
	rec = on.do(req{method: "GET", path: "/v1/auth/providers"})
	m := decodeMap(t, rec)
	if got := m["providers"].([]any); len(got) != 2 || got[0] != "google" || got[1] != "github" {
		t.Fatalf("providers = %v", got)
	}
	if rec := on.do(req{method: "GET", path: "/v1/auth/facebook/start"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown provider = %d, want 404", rec.Code)
	}
}

// Scenario 1.
func TestFirstProviderSignInCreatesTheAccount(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ provider, account string }{{"google", "1234567890"}, {"github", "583231"}} {
		t.Run(tt.provider, func(t *testing.T) {
			e := newEnv(t, envOpts{oauth: true})
			r := e.providerSignIn(tt.provider, ada(tt.account), safari)
			if !r.ok() {
				t.Fatalf("outcome = %q", r.location)
			}
			var cookie *http.Cookie
			var attemptCleared bool
			for _, c := range r.rec.Result().Cookies() {
				switch c.Name {
				case "__Host-rm_session":
					cookie = c
				case "__Host-rm_oauth":
					attemptCleared = c.MaxAge < 0
				}
			}
			if cookie == nil || !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
				t.Errorf("session cookie = %+v", cookie)
			}
			if !attemptCleared {
				t.Error("the attempt cookie was not cleared")
			}
			q := store.New(e.db.Pool)
			u, err := q.GetUserByEmail(context.Background(), "ada@example.com")
			if err != nil || u.EmailVerifiedAt == nil || u.DisplayName != "Ada Lovelace" {
				t.Fatalf("user = %+v, %v", u, err)
			}
			ident, err := q.GetUserIdentity(context.Background(), store.GetUserIdentityParams{UserID: u.ID, Provider: tt.provider})
			if err != nil || ident.ProviderUserID != tt.account || ident.Email != "ada@example.com" {
				t.Fatalf("identity = %+v, %v", ident, err)
			}
			// The session works like any other.
			me := e.do(req{method: "GET", path: "/v1/me", cookie: r.session})
			if me.Code != 200 {
				t.Fatalf("/v1/me with the new session = %d", me.Code)
			}
			if n := e.quiet("ada@example.com"); n != 0 {
				t.Errorf("a brand new account was sent %d alert emails", n)
			}
			if n := e.mail.welcomed("ada@example.com"); n != 1 {
				t.Errorf("a brand new account was sent %d welcome emails, want 1", n)
			}
		})
	}
}

// Scenario 2.
func TestReturningUserIsTheSameUser(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	first := e.providerSignIn("google", ada("g-1"), safari)
	if !first.ok() {
		t.Fatal(first.location)
	}
	q := store.New(e.db.Pool)
	u, _ := q.GetUserByEmail(context.Background(), "ada@example.com")
	before, _ := q.GetUserIdentity(context.Background(), store.GetUserIdentityParams{UserID: u.ID, Provider: "google"})

	// Google now reports another address for the same person: still the same user.
	moved := ada("g-1")
	moved.email = "ada.new@example.com"
	time.Sleep(20 * time.Millisecond)
	second := e.providerSignIn("google", moved, safari)
	if !second.ok() {
		t.Fatal(second.location)
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	after, _ := q.GetUserIdentity(context.Background(), store.GetUserIdentityParams{UserID: u.ID, Provider: "google"})
	if !after.LastLoginAt.After(before.LastLoginAt) || after.Email != "ada.new@example.com" || after.UserID != u.ID {
		t.Errorf("identity after = %+v, before = %+v", after, before)
	}
	if got, _ := q.GetUser(context.Background(), u.ID); got.Email != "ada@example.com" {
		t.Errorf("the account email changed to %q", got.Email)
	}
	if n := e.count(`SELECT count(*) FROM sessions`); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
	if n := e.quiet("ada@example.com"); n != 0 {
		t.Errorf("a known device was sent %d emails", n)
	}
	if n := e.mail.welcomed("ada@example.com"); n != 1 {
		t.Errorf("a returning user was welcomed again (%d welcome emails in all)", n)
	}
}

// Scenario 3 (and 15, case).
func TestProviderLinksToTheExistingAccountWithTheSameVerifiedEmail(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")
	codes := len(e.mail.to("ada@example.com"))

	p := ada("g-1")
	p.email = "Ada@Example.COM"
	r := e.providerSignIn("google", p, safari)
	if !r.ok() {
		t.Fatal(r.location)
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 1 {
		t.Fatalf("users = %d, want 1: a second account was created", n)
	}
	row, err := store.New(e.db.Pool).GetIdentityByProviderAccount(context.Background(), store.GetIdentityByProviderAccountParams{Provider: "google", ProviderUserID: "g-1"})
	if err != nil || row.User.ID.String() != l.userID {
		t.Fatalf("the identity belongs to %v, want %s (%v)", row.User.ID, l.userID, err)
	}
	mails := e.awaitMails("ada@example.com", codes+1)
	if len(mails) != codes+1 || !strings.Contains(mails[codes], "Google connected") {
		t.Fatalf("mails = %q", mails)
	}
	if !strings.Contains(mails[codes], "Safari on macOS") || strings.Contains(mails[codes], "http") {
		t.Errorf("the alert should name the device and carry no link: %q", mails[codes])
	}
	if n := e.quiet("ada@example.com"); n != codes+1 {
		t.Errorf("emails = %d, want %d: connect and new-device alerts doubled up", n, codes+1)
	}
}

// Scenario 4.
func TestUnverifiedOrMissingEmailsAreRefused(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		provider string
		profile  fakeProfile
		want     string
	}{
		"google unverified": {"google", fakeProfile{account: "1", email: "ada@example.com", verified: false}, "no_verified_email"},
		"google no email":   {"google", fakeProfile{account: "1", verified: true}, "no_verified_email"},
		"github unverified": {"github", fakeProfile{account: "1", email: "ada@example.com", verified: false}, "no_verified_email"},
		"github no email":   {"github", fakeProfile{account: "1"}, "no_verified_email"},
		"non-ascii":         {"google", fakeProfile{account: "1", email: "ada@exämple.com", verified: true}, "unsupported_email"},
		"malformed":         {"google", fakeProfile{account: "1", email: "not-an-email", verified: true}, "unsupported_email"},
		"control character": {"google", fakeProfile{account: "1", email: "ada\n@example.com", verified: true}, "unsupported_email"},
		"two at signs":      {"github", fakeProfile{account: "1", email: "a@b@example.com", verified: true}, "unsupported_email"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, envOpts{oauth: true})
			r := e.providerSignIn(tt.provider, tt.profile, safari)
			if r.session != "" || r.refused() != tt.want {
				t.Fatalf("outcome = %q, session %q; want %s", r.location, r.session, tt.want)
			}
			e.noAccountOrSession()
		})
	}
}

// An unverified address must not take over an account, even when it matches one.
func TestAnUnverifiedEmailCannotTakeOverAnAccount(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	e.signIn("ada@example.com", "")
	p := ada("attacker")
	p.verified = false
	r := e.providerSignIn("google", p, chrome)
	if r.session != "" || r.refused() != "no_verified_email" {
		t.Fatalf("outcome = %q", r.location)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 0 {
		t.Errorf("identities = %d", n)
	}
}

// Scenario 5.
func TestAnotherAccountOfTheSameProviderIsAConflict(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	if r := e.providerSignIn("google", ada("g-1"), safari); !r.ok() {
		t.Fatal(r.location)
	}
	// A different Google account whose verified address is Ada's.
	r := e.providerSignIn("google", ada("g-2"), chrome)
	if r.session != "" || r.refused() != "provider_conflict" {
		t.Fatalf("outcome = %q", r.location)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
}

// Scenario 6.
func TestStateRefusals(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	good := func() (attempt, string) {
		a, _ := e.startOAuth("google")
		return a, e.fake.grant("google", a, ada("g-1"))
	}
	empty := ""
	cases := []struct {
		name string
		want string
		do   func() *httptest.ResponseRecorder
	}{
		{"no state", "invalid_state", func() *httptest.ResponseRecorder {
			a, code := good()
			rec := e.callback(a, cb{code: code, state: &empty})
			return rec
		}},
		{"unknown state", "invalid_state", func() *httptest.ResponseRecorder {
			a, code := good()
			bogus := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			rec := e.callback(a, cb{code: code, state: &bogus})
			return rec
		}},
		{"no binding cookie", "invalid_state", func() *httptest.ResponseRecorder {
			a, code := good()
			rec := e.callback(a, cb{code: code, binding: &empty})
			return rec
		}},
		{"another browser's binding", "invalid_state", func() *httptest.ResponseRecorder {
			a, code := good()
			other, _ := e.startOAuth("google")
			rec := e.callback(a, cb{code: code, binding: &other.binding})
			return rec
		}},
		{"the wrong provider path", "invalid_state", func() *httptest.ResponseRecorder {
			a, code := good()
			rec := e.callback(a, cb{code: code, provider: "github"})
			return rec
		}},
		{"provider says access_denied", "access_denied", func() *httptest.ResponseRecorder {
			a, _ := good()
			rec := e.callback(a, cb{errParam: "access_denied"})
			return rec
		}},
		{"provider says something else", "provider_error", func() *httptest.ResponseRecorder {
			a, _ := good()
			rec := e.callback(a, cb{errParam: "server_error"})
			return rec
		}},
		{"no code", "provider_error", func() *httptest.ResponseRecorder {
			a, _ := good()
			rec := e.callback(a, cb{})
			return rec
		}},
		{"a code that was never granted", "provider_error", func() *httptest.ResponseRecorder {
			a, _ := good()
			rec := e.callback(a, cb{code: "made-up"})
			return rec
		}},
		{"an enormous code", "provider_error", func() *httptest.ResponseRecorder {
			a, _ := good()
			rec := e.callback(a, cb{code: strings.Repeat("c", 5000)})
			return rec
		}},
	}
	for _, tt := range cases {
		rec := tt.do()
		if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || loc != "/login?error="+tt.want {
			t.Errorf("%s: %d %q, want /login?error=%s", tt.name, rec.Code, loc, tt.want)
		}
		if r := e.outcome(rec); r.session != "" {
			t.Errorf("%s: a session was issued", tt.name)
		}
	}
	e.noAccountOrSession()

	// A state works once, and a failed attempt spends it: the right code and cookie afterwards are too late.
	a, code := good()
	wrong := "wrong"
	e.callback(a, cb{code: code, binding: &wrong})
	if r := e.outcome(e.callback(a, cb{code: code})); r.session != "" || r.refused() != "invalid_state" {
		t.Errorf("a state survived a failed attempt: %q", r.location)
	}
	a, code = good()
	if r := e.outcome(e.callback(a, cb{code: code})); !r.ok() {
		t.Fatalf("first use = %q", r.location)
	}
	if r := e.outcome(e.callback(a, cb{code: e.fake.grant("google", a, ada("g-1"))})); r.session != "" || r.refused() != "invalid_state" {
		t.Errorf("a state was used twice: %q", r.location)
	}

	// An expired attempt.
	a, code = good()
	keys, _ := e.rdb.Keys(context.Background(), e.prefix+"oauth:*").Result()
	for _, k := range keys {
		_ = e.rdb.Del(context.Background(), k).Err()
	}
	if r := e.outcome(e.callback(a, cb{code: code})); r.session != "" || r.refused() != "invalid_state" {
		t.Errorf("an expired attempt was accepted: %q", r.location)
	}
}

// Scenario 7.
func TestThePKCEVerifierMatchesTheChallenge(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	a, _ := e.startOAuth("github")
	code := e.fake.grant("github", a, ada("583231"))
	if r := e.outcome(e.callback(a, cb{code: code})); !r.ok() {
		t.Fatalf("outcome = %q", r.location)
	}
	form := e.fake.tokenForms[len(e.fake.tokenForms)-1]
	if form.Get("code_verifier") == "" || form.Get("redirect_uri") != webOrigin+"/api/v1/auth/github/callback" {
		t.Errorf("token request = %v", form)
	}
	if form.Get("code_verifier") == a.challenge || form.Get("code_verifier") == a.state {
		t.Error("the verifier is not independent of what the browser saw")
	}

	// A code granted for another attempt's challenge is refused by the provider, so no session.
	a1, _ := e.startOAuth("google")
	a2, _ := e.startOAuth("google")
	stolen := e.fake.grant("google", a2, ada("g-1")) // issued against a2's challenge
	if r := e.outcome(e.callback(a1, cb{code: stolen})); r.session != "" || r.refused() != "provider_error" {
		t.Errorf("a code for another challenge was accepted: %q", r.location)
	}
}

// Scenario 8.
func TestProviderFailuresGiveProviderErrorAndLeakNothing(t *testing.T) {
	t.Parallel()
	for name, set := range map[string]func(f *fakeProvider){
		"token endpoint 500": func(f *fakeProvider) { f.tokenStatus = 500 },
		"token endpoint 400": func(f *fakeProvider) { f.tokenStatus = 400 },
		"profile malformed":  func(f *fakeProvider) { f.profileMode = "malformed" },
		"profile redirects":  func(f *fakeProvider) { f.profileMode = "redirect" },
		"profile oversized":  func(f *fakeProvider) { f.profileMode = "huge" },
		"profile status 500": func(f *fakeProvider) { f.profileMode = "status500" },
	} {
		for _, provider := range []string{"google", "github"} {
			t.Run(name+" "+provider, func(t *testing.T) {
				e := newEnv(t, envOpts{oauth: true})
				a, _ := e.startOAuth(provider)
				code := e.fake.grant(provider, a, ada("583231"))
				e.fake.set(func() { set(e.fake) })
				r := e.outcome(e.callback(a, cb{code: code}))
				if r.session != "" || r.refused() != "provider_error" {
					t.Fatalf("outcome = %q", r.location)
				}
				e.noAccountOrSession()
				logs := e.logs.String()
				for _, secret := range []string{code, "at-", googleSecret, githubSecret, a.state, a.binding, "ada@example.com", "elsewhere"} {
					if strings.Contains(logs, secret) {
						t.Errorf("the log contains %q:\n%s", secret, logs)
					}
				}
			})
		}
	}
}

// Scenario 9.
func TestSuspendedUsersGetAGenericFailure(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	q := store.New(e.db.Pool)

	// Through a linked identity.
	if r := e.providerSignIn("google", ada("g-1"), safari); !r.ok() {
		t.Fatal(r.location)
	}
	u, _ := q.GetUserByEmail(context.Background(), "ada@example.com")
	if _, err := q.SetUserStatus(context.Background(), store.SetUserStatusParams{ID: u.ID, Status: "suspended"}); err != nil {
		t.Fatal(err)
	}
	sessions := e.count(`SELECT count(*) FROM sessions`)
	for _, ua := range []string{safari, chrome} {
		if r := e.providerSignIn("google", ada("g-1"), ua); r.session != "" || r.refused() != "account_unavailable" {
			t.Errorf("linked identity: %q", r.location)
		}
	}
	// Through the email address, with a different provider.
	if r := e.providerSignIn("github", ada("583231"), safari); r.session != "" || r.refused() != "account_unavailable" {
		t.Errorf("by email: %q", r.location)
	}
	if n := e.count(`SELECT count(*) FROM sessions`); n != sessions {
		t.Errorf("sessions grew from %d to %d", sessions, n)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
}

// Scenario 13.
func TestConcurrentCallbacksAndFirstSignIns(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})

	// 20 callbacks with one state: exactly one session.
	a, _ := e.startOAuth("google")
	codes := make([]string, 20)
	ips := make([]string, 20)
	for i := range codes {
		codes[i] = e.fake.grant("google", a, ada("g-1"))
		ips[i] = e.freshIP()
	}
	var wg sync.WaitGroup
	results := make(chan result, 20)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- e.outcome(e.callback(a, cb{code: codes[i], ip: ips[i]}))
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for r := range results {
		if r.ok() {
			wins++
		} else if r.refused() != "invalid_state" {
			t.Errorf("a losing callback got %q", r.location)
		}
	}
	if wins != 1 || e.count(`SELECT count(*) FROM sessions`) != 1 {
		t.Fatalf("%d callbacks signed in, %d sessions; want 1 and 1", wins, e.count(`SELECT count(*) FROM sessions`))
	}

	// Simultaneous first sign-ins for one provider account (different states): one user, one
	// identity, every request signed in. Repeated, because the window is narrow.
	e2 := newEnv(t, envOpts{oauth: true})
	for round := range 12 {
		account := "58" + itoa(round)
		profile := fakeProfile{account: account, email: "round" + itoa(round) + "@example.com", verified: true}
		provider := []string{"github", "google"}[round%2]
		attempts := make([]attempt, 8)
		cs := make([]string, 8)
		for i := range attempts {
			attempts[i], _ = e2.startOAuth(provider)
			cs[i] = e2.fake.grant(provider, attempts[i], profile)
			ips[i] = e2.freshIP()
		}
		results = make(chan result, 8)
		for i := range attempts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results <- e2.outcome(e2.callback(attempts[i], cb{code: cs[i], ip: ips[i]}))
			}()
		}
		wg.Wait()
		close(results)
		for r := range results {
			if !r.ok() {
				t.Errorf("round %d: a first sign-in failed: %q", round, r.location)
			}
		}
		if n := e2.count(`SELECT count(*) FROM users WHERE email = $1`, profile.email); n != 1 {
			t.Errorf("round %d: users = %d, want 1", round, n)
		}
		if n := e2.count(`SELECT count(*) FROM oauth_identities WHERE provider_user_id = $1`, account); n != 1 {
			t.Errorf("round %d: identities = %d, want 1", round, n)
		}
	}
}

// Scenario 16: where the API sends the browser.
func TestRedirectsAreFixedAndNeverTakenFromTheRequest(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	allowed := map[string]bool{"/account": true}
	for _, c := range []string{"access_denied", "invalid_state", "no_verified_email", "provider_conflict", "identity_in_use", "unsupported_email", "provider_error", "account_unavailable"} {
		allowed["/login?error="+c] = true
	}

	for _, provider := range []string{"google", "github"} {
		// The start sends the browser to the provider, with the redirect URI built from configuration.
		hr := req{method: "GET", path: "/v1/auth/" + provider + "/start?next=https://evil.example&redirect_uri=https://evil.example&return=//evil.example"}
		rec := e.do(hr)
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusFound || loc == nil || !strings.HasPrefix(loc.String(), e.fake.srv.URL+"/"+provider+"/auth?") {
			t.Fatalf("start Location = %q", rec.Header().Get("Location"))
		}
		if got := loc.Query().Get("redirect_uri"); got != webOrigin+"/api/v1/auth/"+provider+"/callback" {
			t.Errorf("redirect_uri = %q", got)
		}
		if strings.Contains(loc.String(), "evil") || strings.Contains(loc.String(), "secret") {
			t.Errorf("the request reached the provider URL: %s", loc)
		}
	}

	// Every callback outcome, with hostile query parameters and headers, lands on a fixed path.
	for _, extra := range []string{"&next=https://evil.example", "&redirect=//evil.example", "&return_to=%2F%2Fevil.example", "&error_description=https://evil.example"} {
		a, _ := e.startOAuth("google")
		code := e.fake.grant("google", a, ada("g-1"))
		rec := e.do(req{method: "GET", path: "/v1/auth/google/callback?code=" + code + "&state=" + a.state + extra, oauthCookie: a.binding, ua: safari})
		if !allowed[rec.Header().Get("Location")] {
			t.Errorf("Location = %q", rec.Header().Get("Location"))
		}
		rec = e.do(req{method: "GET", path: "/v1/auth/google/callback?error=" + url.QueryEscape("https://evil.example") + "&state=zzz" + extra})
		if !allowed[rec.Header().Get("Location")] {
			t.Errorf("Location = %q", rec.Header().Get("Location"))
		}
	}
	// Redirect responses are never cached and never leak the callback URL as a referrer.
	a, _ := e.startOAuth("google")
	rec := e.callback(a, cb{errParam: "access_denied"})
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", rec.Header())
	}
}

func TestOAuthEndpointsAreRateLimited(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	ip := "198.51.100.9"
	for i := 0; i < 30; i++ {
		if rec := e.do(req{method: "GET", path: "/v1/auth/google/start", ip: ip}); rec.Code != http.StatusFound {
			t.Fatalf("start %d = %d", i+1, rec.Code)
		}
	}
	rec := e.do(req{method: "GET", path: "/v1/auth/google/start", ip: ip})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("31st start = %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := e.do(req{method: "GET", path: "/v1/auth/google/start", ip: "198.51.100.10"}); rec.Code != http.StatusFound {
		t.Errorf("another client was limited: %d", rec.Code)
	}
	for i := 0; i < 30; i++ {
		if rec := e.do(req{method: "GET", path: "/v1/auth/google/callback?state=x", ip: ip}); rec.Code != http.StatusFound {
			t.Fatalf("callback %d = %d", i+1, rec.Code)
		}
	}
	rec = e.do(req{method: "GET", path: "/v1/auth/google/callback?state=x", ip: ip})
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("31st callback = %d", rec.Code)
	}
}

// The attempt cookie is what ties a callback to the browser that started it, so it gets the same
// protections as the session cookie, and lives only as long as the attempt does.
func TestTheAttemptCookieIsLockedDown(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")

	check := func(name string, cookies []*http.Cookie) {
		t.Helper()
		var c *http.Cookie
		for _, k := range cookies {
			if k.Name == "__Host-rm_oauth" {
				c = k
			}
		}
		if c == nil {
			t.Fatalf("%s: no attempt cookie", name)
		}
		if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.Domain != "" || c.MaxAge != 600 || len(c.Value) < 43 {
			t.Errorf("%s: cookie = %+v", name, c)
		}
	}
	rec := e.do(req{method: "GET", path: "/v1/auth/google/start"})
	check("login start", rec.Result().Cookies())
	rec = l.send(e, "POST", "/v1/me/identities/github/start", "")
	check("connect start", rec.Result().Cookies())
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("connect start Cache-Control = %q", cc)
	}
}

// The race that random concurrency rarely hits, made certain: a second sign-in for the same
// provider account finishes between the first one's "unknown account" lookup and its lookup by
// address. The first must then sign in as the user the second created, not report a conflict.
func TestFirstSignInRacedByTheSameAccount(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	p := ada("g-race")

	a, _ := e.startOAuth("google")
	code := e.fake.grant("google", a, p)

	var fired atomic.Bool
	var inner result
	e.svc.OnIdentityMiss(func() {
		if fired.Swap(true) {
			return // only the outer sign-in plays the race
		}
		inner = e.providerSignIn("google", p, safari)
	})
	outer := e.outcome(e.callback(a, cb{code: code, ua: chrome}))

	if !fired.Load() {
		t.Fatal("the seam never ran")
	}
	if !inner.ok() {
		t.Fatalf("the sign-in that got there first = %q", inner.location)
	}
	if !outer.ok() {
		t.Fatalf("the sign-in that lost the race = %q, want /account", outer.location)
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}
	if n := e.count(`SELECT count(*) FROM sessions`); n != 2 {
		t.Errorf("sessions = %d, want 2", n)
	}
}
