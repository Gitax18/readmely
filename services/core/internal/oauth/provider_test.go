package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fake stands in for a provider's three or four endpoints and records what it was sent.
type fake struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	forms   []url.Values
	headers map[string]http.Header // path -> headers of the last request
	paths   []string

	token   func(w http.ResponseWriter, r *http.Request)
	handler map[string]func(w http.ResponseWriter, r *http.Request)
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, headers: map[string]http.Header{}, handler: map[string]func(http.ResponseWriter, *http.Request){}}
	f.token = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"access_token":"tok-123","token_type":"bearer"}`))
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.paths = append(f.paths, r.URL.Path)
		f.headers[r.URL.Path] = r.Header.Clone()
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			f.forms = append(f.forms, r.PostForm)
		}
		f.mu.Unlock()
		if r.URL.Path == "/token" {
			f.token(w, r)
			return
		}
		if h, ok := f.handler[r.URL.Path]; ok {
			h(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func json200(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}
}

func (f *fake) google() Provider {
	return NewGoogle("gid-12345", "gsecret-12345", &GoogleEndpoints{Auth: f.srv.URL + "/auth", Token: f.srv.URL + "/token", UserInfo: f.srv.URL + "/userinfo"})
}

func (f *fake) github() Provider {
	return NewGitHub("hid-12345", "hsecret-12345", &GitHubEndpoints{Auth: f.srv.URL + "/auth", Token: f.srv.URL + "/token", User: f.srv.URL + "/user", Emails: f.srv.URL + "/emails"})
}

const redirect = "http://127.0.0.1:3000/api/v1/auth/google/callback"

func TestAuthorizationURLs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		p      Provider
		host   string
		scope  string
		prompt string
	}{
		{NewGoogle("gid-12345", "s", nil), "accounts.google.com", "openid email profile", "select_account"},
		{NewGitHub("hid-12345", "s", nil), "github.com", "read:user user:email", ""},
	} {
		raw := tt.p.AuthorizationURL("st", "ch", redirect)
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != tt.host {
			t.Fatalf("%s: url %q", tt.p.Name(), raw)
		}
		q := u.Query()
		for k, want := range map[string]string{"state": "st", "code_challenge": "ch", "code_challenge_method": "S256", "redirect_uri": redirect, "scope": tt.scope} {
			if q.Get(k) != want {
				t.Errorf("%s: %s = %q, want %q", tt.p.Name(), k, q.Get(k), want)
			}
		}
		if q.Get("client_secret") != "" || strings.Contains(raw, "secret") {
			t.Errorf("%s: the client secret is in the browser URL", tt.p.Name())
		}
		if q.Get("prompt") != tt.prompt {
			t.Errorf("%s: prompt = %q", tt.p.Name(), q.Get("prompt"))
		}
	}
}

func TestGoogleExchange(t *testing.T) {
	t.Parallel()
	f := newFake(t)
	f.handler["/userinfo"] = json200(`{"sub":"1234567890","email":"Ada@Example.com","email_verified":true,"name":"Ada Lovelace","picture":"x"}`)
	p, err := f.google().Exchange(context.Background(), "the-code", "the-verifier", redirect)
	if err != nil {
		t.Fatal(err)
	}
	if p != (Profile{Account: "1234567890", Email: "Ada@Example.com", EmailVerified: true, Name: "Ada Lovelace"}) {
		t.Fatalf("profile = %+v", p)
	}
	form := f.forms[0]
	for k, want := range map[string]string{
		"grant_type": "authorization_code", "code": "the-code", "code_verifier": "the-verifier",
		"redirect_uri": redirect, "client_id": "gid-12345", "client_secret": "gsecret-12345",
	} {
		if form.Get(k) != want {
			t.Errorf("token request %s = %q, want %q", k, form.Get(k), want)
		}
	}
	if got := f.headers["/userinfo"].Get("Authorization"); got != "Bearer tok-123" {
		t.Errorf("userinfo Authorization = %q", got)
	}
}

func TestGoogleEmailVerifiedFlavours(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]bool{
		`"email_verified":true`: true, `"email_verified":"true"`: true,
		`"email_verified":false`: false, `"email_verified":"false"`: false, `"email_verified":null`: false, ``: false,
	} {
		f := newFake(t)
		sep := ","
		if body == "" {
			sep = ""
		}
		f.handler["/userinfo"] = json200(`{"sub":"1","email":"a@example.com"` + sep + body + `}`)
		p, err := f.google().Exchange(context.Background(), "c", "v", redirect)
		if err != nil || p.EmailVerified != want {
			t.Errorf("%s: verified = %v, %v; want %v", body, p.EmailVerified, err, want)
		}
	}
	f := newFake(t)
	f.handler["/userinfo"] = json200(`{"sub":"1","email":"a@example.com","email_verified":"yes"}`)
	if _, err := f.google().Exchange(context.Background(), "c", "v", redirect); err == nil {
		t.Error("an unreadable email_verified was accepted")
	}
}

func TestGitHubExchange(t *testing.T) {
	t.Parallel()
	f := newFake(t)
	f.handler["/user"] = json200(`{"id":583231,"login":"octocat","name":"The Octocat","email":"public@example.com"}`)
	f.handler["/emails"] = json200(`[
		{"email":"old@example.com","primary":false,"verified":true},
		{"email":"unverified@example.com","primary":true,"verified":false},
		{"email":"Octo@Example.com","primary":true,"verified":true}]`)
	p, err := f.github().Exchange(context.Background(), "the-code", "the-verifier", redirect)
	if err != nil {
		t.Fatal(err)
	}
	if p != (Profile{Account: "583231", Email: "Octo@Example.com", EmailVerified: true, Name: "The Octocat"}) {
		t.Fatalf("profile = %+v", p)
	}
	form := f.forms[0]
	for k, want := range map[string]string{"code": "the-code", "code_verifier": "the-verifier", "client_id": "hid-12345", "client_secret": "hsecret-12345"} {
		if form.Get(k) != want {
			t.Errorf("token request %s = %q, want %q", k, form.Get(k), want)
		}
	}
	for _, path := range []string{"/user", "/emails"} {
		h := f.headers[path]
		if h.Get("Authorization") != "Bearer tok-123" || h.Get("User-Agent") != "readmely" {
			t.Errorf("%s headers = %v", path, h)
		}
	}
}

func TestGitHubNeverTrustsAnUnverifiedOrNonPrimaryAddress(t *testing.T) {
	t.Parallel()
	for name, emails := range map[string]string{
		"primary but unverified":   `[{"email":"a@example.com","primary":true,"verified":false}]`,
		"verified but not primary": `[{"email":"a@example.com","primary":false,"verified":true}]`,
		"no addresses":             `[]`,
	} {
		f := newFake(t)
		// The public profile email must never be used instead.
		f.handler["/user"] = json200(`{"id":1,"email":"public@example.com"}`)
		f.handler["/emails"] = json200(emails)
		p, err := f.github().Exchange(context.Background(), "c", "v", redirect)
		if err != nil || p.EmailVerified || p.Email != "" || p.Account != "1" {
			t.Errorf("%s: %+v, %v", name, p, err)
		}
	}
}

func TestBadAccountIDs(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"id":0}`, `{"id":-4}`, `{"id":"abc"}`, `{"id":1.5}`, `{}`, `{"id":null}`} {
		f := newFake(t)
		f.handler["/user"] = json200(body)
		f.handler["/emails"] = json200(`[]`)
		if p, err := f.github().Exchange(context.Background(), "c", "v", redirect); err == nil {
			t.Errorf("GitHub %s accepted: %+v", body, p)
		}
	}
	for _, body := range []string{`{}`, `{"sub":""}`, `{"sub":"` + strings.Repeat("x", 300) + `"}`} {
		f := newFake(t)
		f.handler["/userinfo"] = json200(body)
		if p, err := f.google().Exchange(context.Background(), "c", "v", redirect); err == nil {
			t.Errorf("Google %.30s accepted: %+v", body, p)
		}
	}
}

func TestProviderFailuresGiveGenericErrors(t *testing.T) {
	t.Parallel()
	good := json200(`{"sub":"1","email":"a@example.com","email_verified":true}`)
	secretish := "SECRET-BODY-must-not-leak-sk_live_123"
	cases := map[string]func(f *fake){
		"token endpoint 400": func(f *fake) {
			f.token = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(400); _, _ = w.Write([]byte(secretish)) }
		},
		"token endpoint 500": func(f *fake) { f.token = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) } },
		"token error in a 200": func(f *fake) {
			f.token = json200(`{"error":"bad_verification_code","error_description":"` + secretish + `"}`)
		},
		"token not JSON":         func(f *fake) { f.token = json200(secretish) },
		"token missing":          func(f *fake) { f.token = json200(`{"token_type":"bearer"}`) },
		"token absurdly long":    func(f *fake) { f.token = json200(`{"access_token":"` + strings.Repeat("a", 5000) + `"}`) },
		"token response too big": func(f *fake) { f.token = json200(`{"access_token":"a","pad":"` + strings.Repeat("x", 70<<10) + `"}`) },
		"profile 401": func(f *fake) {
			f.handler["/userinfo"] = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(401); _, _ = w.Write([]byte(secretish)) }
		},
		"profile not JSON": func(f *fake) { f.handler["/userinfo"] = json200(secretish) },
		"profile padded past the cap": func(f *fake) {
			f.handler["/userinfo"] = json200(`{"sub":"1","email":"a@example.com","email_verified":true}` + strings.Repeat(" ", 70<<10))
		},
		"token padded past the cap":  func(f *fake) { f.token = json200(`{"access_token":"tok-123"}` + strings.Repeat(" ", 70<<10)) },
		"token error beside a token": func(f *fake) { f.token = json200(`{"access_token":"tok-123","error":"incorrect_client_credentials"}`) },
		"profile too big": func(f *fake) {
			f.handler["/userinfo"] = json200(`{"sub":"1","pad":"` + strings.Repeat("x", 70<<10) + `"}`)
		},
		"token redirects": func(f *fake) {
			f.token = func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/userinfo", http.StatusFound) }
		},
		"profile redirects": func(f *fake) {
			f.handler["/userinfo"] = func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/elsewhere", http.StatusFound) }
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			f.handler["/userinfo"] = good
			setup(f)
			_, err := f.google().Exchange(context.Background(), "the-code", "the-verifier", redirect)
			if err == nil {
				t.Fatal("no error")
			}
			var pe *Error
			if !errors.As(err, &pe) {
				t.Fatalf("error is %T, want *oauth.Error", err)
			}
			for _, leak := range []string{secretish, "tok-123", "gsecret-12345", "the-code", "the-verifier"} {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("error leaks %q: %v", leak, err)
				}
			}
			// A redirect must not have been followed.
			f.mu.Lock()
			defer f.mu.Unlock()
			for _, p := range f.paths {
				if p == "/elsewhere" {
					t.Fatal("a provider redirect was followed")
				}
			}
		})
	}
}

func TestATokenRedirectIsNotFollowedWithTheSecret(t *testing.T) {
	t.Parallel()
	f := newFake(t)
	f.token = func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/userinfo", http.StatusTemporaryRedirect)
	}
	f.handler["/userinfo"] = json200(`{"sub":"1"}`)
	if _, err := f.google().Exchange(context.Background(), "c", "v", redirect); err == nil {
		t.Fatal("no error")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.paths {
		if p == "/userinfo" {
			t.Fatal("the redirect was followed and the client secret went with it")
		}
	}
}

func TestASlowProviderIsGivenUpOn(t *testing.T) {
	t.Parallel()
	f := newFake(t)
	release := make(chan struct{})
	f.token = func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := f.google().Exchange(ctx, "c", "v", redirect); err == nil {
		t.Fatal("no error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestErrorTextNamesOnlyTheStep(t *testing.T) {
	t.Parallel()
	e := &Error{Provider: "google", Step: "token", Status: 400}
	if e.Error() != "google token failed with status 400" {
		t.Fatal(e.Error())
	}
}

func TestTheClientNeverWaitsLong(t *testing.T) {
	t.Parallel()
	c := newClient("x")
	if c.http.Timeout <= 0 || c.http.Timeout > 10*time.Second {
		t.Fatalf("client timeout = %v, want 1ns..10s", c.http.Timeout)
	}
	if c.http.CheckRedirect == nil {
		t.Fatal("redirects are followed")
	}
}
