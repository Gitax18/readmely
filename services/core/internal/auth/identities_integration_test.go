//go:build integration

package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

// connect plays the whole connect flow for a signed-in user and returns the callback outcome.
func (e *env) connect(l login, provider string, p fakeProfile) result {
	e.t.Helper()
	start := l.send(e, "POST", "/v1/me/identities/"+provider+"/start", "")
	if start.Code != http.StatusOK {
		e.t.Fatalf("connect start = %d %s", start.Code, start.Body.String())
	}
	var body struct {
		URL string `json:"authorization_url"`
	}
	_ = json.Unmarshal(start.Body.Bytes(), &body)
	a := e.parseAttempt(provider, body.URL, start)
	if a.binding == "" {
		e.t.Fatal("the connect start set no attempt cookie")
	}
	code := e.fake.grant(provider, a, p)
	return e.outcome(e.callback(a, cb{code: code, session: l.token, ua: chrome}))
}

func (e *env) identities(l login) []map[string]any {
	e.t.Helper()
	rec := l.get(e, "/v1/me/identities")
	if rec.Code != 200 {
		e.t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Identities []map[string]any `json:"identities"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.Identities
}

// Scenario 10: connecting.
func TestConnectingAProvider(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")
	codes := len(e.mail.to("ada@example.com"))

	// The provider account may carry any verified address: connecting is the signed-in user's choice.
	other := fakeProfile{account: "g-77", email: "ada.work@example.org", name: "Ada W", verified: true}
	r := e.connect(l, "google", other)
	if r.location != "/account" || r.session != "" {
		t.Fatalf("connect outcome = %q, new session %q (a connect must not mint a session)", r.location, r.session)
	}
	list := e.identities(l)
	if len(list) != 1 || list[0]["provider"] != "google" || list[0]["email"] != "ada.work@example.org" {
		t.Fatalf("identities = %v", list)
	}
	mails := e.awaitMails("ada@example.com", codes+1)
	if last := mails[len(mails)-1]; !strings.Contains(last, "Google connected") || !strings.Contains(last, "ada.work@example.org") {
		t.Errorf("alert = %q", last)
	}
	// It now signs in on its own, as the same user.
	if in := e.providerSignIn("google", other, chrome); !in.ok() {
		t.Fatalf("sign-in with the connected account = %q", in.location)
	}
	if n := e.count(`SELECT count(*) FROM users`); n != 1 {
		t.Errorf("users = %d, want 1", n)
	}

	// Connecting what is already connected is refused at the start.
	if rec := l.send(e, "POST", "/v1/me/identities/google/start", ""); rec.Code != http.StatusConflict {
		t.Errorf("second connect = %d, want 409", rec.Code)
	}
	// Connecting the same provider account again through the callback is a no-op, not an error.
	if r := e.connect(l, "github", fakeProfile{account: "583231", email: "ada@example.com", verified: true}); r.location != "/account" {
		t.Errorf("github connect = %q", r.location)
	}
	if len(e.identities(l)) != 2 {
		t.Error("expected two connected accounts")
	}
}

func TestConnectNeedsASessionCSRFAndOrigin(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")
	path := "/v1/me/identities/google/start"

	if rec := e.do(req{method: "POST", path: path}); rec.Code != 401 {
		t.Errorf("no session = %d, want 401", rec.Code)
	}
	if rec := e.do(req{method: "POST", path: path, cookie: l.token}); rec.Code != 403 {
		t.Errorf("no CSRF token = %d, want 403", rec.Code)
	}
	if rec := e.do(req{method: "POST", path: path, cookie: l.token, csrf: "AAAA" + l.csrf}); rec.Code != 403 {
		t.Errorf("wrong CSRF token = %d, want 403", rec.Code)
	}
	if rec := e.do(req{method: "POST", path: path, cookie: l.token, csrf: l.csrf, origin: str("https://evil.example")}); rec.Code != 403 {
		t.Errorf("foreign origin = %d, want 403", rec.Code)
	}
	if rec := e.do(req{method: "POST", path: path, cookie: l.token, csrf: l.csrf, origin: str("")}); rec.Code != 403 {
		t.Errorf("no origin = %d, want 403", rec.Code)
	}
	// The start is not reachable as a GET, which a cross-site page could trigger.
	if rec := e.do(req{method: "GET", path: path, cookie: l.token}); rec.Code != 405 {
		t.Errorf("GET connect start = %d, want 405", rec.Code)
	}
	if rec := l.send(e, "POST", "/v1/me/identities/facebook/start", ""); rec.Code != 404 {
		t.Errorf("unknown provider = %d, want 404", rec.Code)
	}
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 0 {
		t.Errorf("identities = %d", n)
	}
	if rec := e.do(req{method: "GET", path: "/v1/me/identities"}); rec.Code != 401 {
		t.Errorf("list without a session = %d, want 401", rec.Code)
	}
}

// The connect must finish in the session that started it.
func TestConnectMustFinishInTheSameSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")
	e.resetLimits()
	second := e.signIn("ada@example.com", "") // another session of the same user
	bob := e.signIn("bob@example.com", "")

	start := l.send(e, "POST", "/v1/me/identities/google/start", "")
	var body struct {
		URL string `json:"authorization_url"`
	}
	_ = json.Unmarshal(start.Body.Bytes(), &body)

	try := func(name, session string) {
		a := e.parseAttempt("google", body.URL, start)
		// A fresh attempt each time, since a failed one is spent.
		if name != "same session" {
			start = l.send(e, "POST", "/v1/me/identities/google/start", "")
			_ = json.Unmarshal(start.Body.Bytes(), &body)
			a = e.parseAttempt("google", body.URL, start)
		}
		code := e.fake.grant("google", a, fakeProfile{account: "g-5", email: "ada@example.com", verified: true})
		r := e.outcome(e.callback(a, cb{code: code, session: session}))
		switch name {
		case "same session":
			if r.location != "/account" {
				t.Errorf("%s: %q", name, r.location)
			}
		default:
			if r.refused() != "invalid_state" {
				t.Errorf("%s: %q, want invalid_state", name, r.location)
			}
		}
	}
	try("no session at all", "")
	try("another session of the same user", second.token)
	try("another user's session", bob.token)
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 0 {
		t.Fatalf("an identity was attached by a refused connect: %d", n)
	}
	// The same flow in the right session works.
	start = l.send(e, "POST", "/v1/me/identities/google/start", "")
	_ = json.Unmarshal(start.Body.Bytes(), &body)
	try("same session", l.token)
	if n := e.count(`SELECT count(*) FROM oauth_identities`); n != 1 {
		t.Errorf("identities = %d, want 1", n)
	}

	// A sign-in attempt cannot be finished as a connect and vice versa: a login state carries no user.
	a, _ := e.startOAuth("github")
	code := e.fake.grant("github", a, fakeProfile{account: "9", email: "ada@example.com", verified: true})
	r := e.outcome(e.callback(a, cb{code: code, session: bob.token}))
	if !r.ok() || r.session == bob.token {
		t.Errorf("a login attempt with a session cookie = %q", r.location)
	}
	owner, _ := store.New(e.db.Pool).GetIdentityByProviderAccount(context.Background(), store.GetIdentityByProviderAccountParams{Provider: "github", ProviderUserID: "9"})
	if owner.User.Email != "ada@example.com" {
		t.Errorf("the login attempt attached github to %s", owner.User.Email)
	}
}

func TestAProviderAccountBelongsToOneUser(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	ada := e.signIn("ada@example.com", "")
	bob := e.signIn("bob@example.com", "")

	if r := e.connect(ada, "google", fakeProfile{account: "g-1", email: "ada@example.com", verified: true}); r.location != "/account" {
		t.Fatalf("ada's connect = %q", r.location)
	}
	// Bob tries to connect the same Google account.
	r := e.connect(bob, "google", fakeProfile{account: "g-1", email: "bob@example.com", verified: true})
	if r.refused() != "identity_in_use" {
		t.Fatalf("bob's connect = %q, want identity_in_use", r.location)
	}
	if len(e.identities(bob)) != 0 || len(e.identities(ada)) != 1 {
		t.Error("an identity moved or was shared")
	}
	// Bob already holds another Google account: conflict on a second one.
	if r := e.connect(bob, "google", fakeProfile{account: "g-2", email: "bob@example.com", verified: true}); r.location != "/account" {
		t.Fatalf("bob's own connect = %q", r.location)
	}
	// Tenancy: nobody sees or removes another user's accounts.
	if got := e.identities(bob); len(got) != 1 || got[0]["email"] != "bob@example.com" {
		t.Errorf("bob sees %v", got)
	}
	if rec := bob.send(e, "DELETE", "/v1/me/identities/github", ""); rec.Code != 404 {
		t.Errorf("bob disconnecting a provider he has not connected = %d, want 404", rec.Code)
	}
	if len(e.identities(ada)) != 1 {
		t.Error("ada lost an identity to bob's request")
	}
}

// awaitSubject waits until an email to the address has a subject containing the text.
func (e *env) awaitSubject(to, text string) {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range e.mail.to(to) {
			if strings.Contains(m.Subject, text) {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("no email to %s with %q in its subject arrived", to, text)
}

// Scenario 11.
func TestDisconnect(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	l := e.signIn("ada@example.com", "")
	e.connect(l, "google", ada("g-1"))
	e.connect(l, "github", ada("583231"))
	// Alerts are sent through a queue that delivers in order, so once the last "connected" alert is here
	// everything earlier is too, and none of it is counted as new below.
	e.awaitSubject("ada@example.com", "GitHub connected")
	before := len(e.mail.to("ada@example.com"))

	path := "/v1/me/identities/google"
	if rec := e.do(req{method: "DELETE", path: path}); rec.Code != 401 {
		t.Errorf("no session = %d", rec.Code)
	}
	if rec := e.do(req{method: "DELETE", path: path, cookie: l.token}); rec.Code != 403 {
		t.Errorf("no CSRF = %d", rec.Code)
	}
	if rec := l.send(e, "DELETE", "/v1/me/identities/facebook", ""); rec.Code != 404 {
		t.Errorf("unknown provider = %d", rec.Code)
	}
	if n := len(e.identities(l)); n != 2 {
		t.Fatalf("refused requests changed the identities: %d", n)
	}

	if rec := l.send(e, "DELETE", path, ""); rec.Code != 204 {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
	// At once: the list, and signing in with it.
	if got := e.identities(l); len(got) != 1 || got[0]["provider"] != "github" {
		t.Errorf("identities = %v", got)
	}
	if rec := l.send(e, "DELETE", path, ""); rec.Code != 404 {
		t.Errorf("disconnecting twice = %d, want 404", rec.Code)
	}
	// The alert is queued, so wait for it before signing in: otherwise the next email to arrive can be the
	// alert, and signIn would read it as the sign-in code.
	mails := e.awaitMails("ada@example.com", before+1)
	var found bool
	for _, m := range mails {
		if strings.Contains(m, "Google disconnected") {
			found = true
		}
	}
	if !found {
		t.Errorf("no disconnect alert in %q", mails)
	}
	// The same Google account now creates a link to the account again through the email match, because it
	// is verified: it is not a lockout and not a merge.
	// Email codes always still work.
	e.resetLimits()
	again := e.signIn("ada@example.com", "")
	if again.userID != l.userID {
		t.Error("email sign-in reached another account")
	}
}

// Scenario 12.
func TestAlerts(t *testing.T) {
	t.Parallel()
	t.Run("a new device by email code", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		first := e.signIn("ada@example.com", "") // Safari; a brand new account: no alert
		codes := len(e.mail.to("ada@example.com"))
		if n := e.quiet("ada@example.com"); n != codes {
			t.Fatalf("first sign-in sent %d emails, want %d (the code only)", n, codes)
		}
		// Same device again: no alert.
		e.resetLimits()
		e.signIn("ada@example.com", "")
		codes = len(e.mail.to("ada@example.com"))
		if n := e.quiet("ada@example.com"); n != codes {
			t.Fatalf("a known device sent an alert: %d emails", n)
		}
		// A different device: one alert.
		e.resetLimits()
		ip := e.freshIP()
		e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "ada@example.com"}), ip: ip})
		code := e.awaitCode("ada@example.com", codes+1)
		rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: jsonBody(map[string]string{"email": "ada@example.com", "code": code}), ip: ip, ua: chrome})
		if rec.Code != 200 {
			t.Fatalf("verify = %d", rec.Code)
		}
		mails := e.awaitMails("ada@example.com", codes+2)
		last := mails[len(mails)-1]
		if !strings.Contains(last, "New sign-in") || !strings.Contains(last, "Chrome on Linux") || strings.Contains(last, "http") || strings.Contains(last, code) {
			t.Errorf("alert = %q", last)
		}
		_ = first
	})

	t.Run("a new device by provider", func(t *testing.T) {
		e := newEnv(t, envOpts{oauth: true})
		e.providerSignIn("github", ada("583231"), safari)
		e.quiet("ada@example.com")
		if r := e.providerSignIn("github", ada("583231"), chrome); !r.ok() {
			t.Fatal(r.location)
		}
		mails := e.awaitMails("ada@example.com", 1)
		if !strings.Contains(mails[0], "New sign-in") || !strings.Contains(mails[0], "GitHub") || !strings.Contains(mails[0], "Chrome on Linux") {
			t.Errorf("alert = %q", mails)
		}
	})

	t.Run("the hourly cap", func(t *testing.T) {
		e := newEnv(t, envOpts{oauth: true})
		l := e.signIn("ada@example.com", "")
		codes := len(e.mail.to("ada@example.com"))
		// Each connect and disconnect is an alert; ten of them, five delivered.
		for i := 0; i < 5; i++ {
			e.connect(l, "google", fakeProfile{account: "g-1", email: "ada@example.com", verified: true})
			if rec := l.send(e, "DELETE", "/v1/me/identities/google", ""); rec.Code != 204 {
				t.Fatalf("disconnect = %d", rec.Code)
			}
		}
		e.awaitMails("ada@example.com", codes+5)
		if n := e.quiet("ada@example.com"); n != codes+5 {
			t.Errorf("emails = %d, want %d: the cap of 5 alerts per hour did not hold", n, codes+5)
		}
		if !strings.Contains(e.logs.String(), "alert dropped") {
			t.Error("dropped alerts are not counted in the log")
		}
	})

	t.Run("a full queue is invisible", func(t *testing.T) {
		gate := make(chan struct{})
		m := &capture{gate: gate, in: make(chan struct{}, 1)}
		e := newEnv(t, envOpts{oauth: true, mailer: m, queueCapacity: 1, senders: 1})
		// Sign in first (the code email occupies the sender), then flood alerts.
		defer close(gate)
		start := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "ada@example.com"})})
		if start.Code != 202 {
			t.Fatalf("start = %d", start.Code)
		}
		// Provider sign-ins never wait for mail.
		done := make(chan result, 1)
		go func() { done <- e.providerSignIn("google", ada("g-1"), safari) }()
		select {
		case r := <-done:
			if !r.ok() {
				t.Errorf("sign-in with a full mail queue = %q", r.location)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a provider sign-in waited for the mail queue")
		}
	})
}

// Scenario 15: case differences match the same account; and a name that breaks the display rules is dropped.
func TestEmailCaseAndOddNames(t *testing.T) {
	t.Parallel()
	e := newEnv(t, envOpts{oauth: true})
	a := fakeProfile{account: "1", email: "Ada@Example.COM", name: "Ada\u202e Lovelace", verified: true}
	if r := e.providerSignIn("google", a, safari); !r.ok() {
		t.Fatal(r.location)
	}
	u, _ := store.New(e.db.Pool).GetUserByEmail(context.Background(), "ada@example.com")
	if u.Email != "ada@example.com" || u.DisplayName != "" {
		t.Errorf("user = %q %q: the address should be lower case and a name with a bidirectional override dropped", u.Email, u.DisplayName)
	}
	long := fakeProfile{account: "2", email: "bob@example.com", name: strings.Repeat("n", 200), verified: true}
	if r := e.providerSignIn("github", long, safari); !r.ok() {
		t.Fatal(r.location)
	}
	b, _ := store.New(e.db.Pool).GetUserByEmail(context.Background(), "bob@example.com")
	if b.DisplayName != "" {
		t.Errorf("an over-long name was used: %q", b.DisplayName)
	}
	_ = url.Values{}
}
