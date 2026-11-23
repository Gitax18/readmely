//go:build integration

package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Gitax18/readmely/services/core/internal/oauth"
)

const (
	googleID, googleSecret = "gid-12345", "gsecret-12345"
	githubID, githubSecret = "hid-12345", "hsecret-12345"
)

// fakeProfile is who the fake provider says the person is.
type fakeProfile struct {
	account  string // Google "sub" or the GitHub numeric id
	email    string
	name     string
	verified bool
}

type fakeGrant struct {
	provider    string
	challenge   string
	redirectURI string
	profile     fakeProfile
}

// fakeProvider stands in for Google's and GitHub's token, profile and email endpoints. It checks
// what a real provider checks (the client secret, the redirect URI, and that the PKCE verifier
// matches the challenge from the authorization request), so a flow that skips any of them fails.
type fakeProvider struct {
	t   *testing.T
	srv *httptest.Server

	mu          sync.Mutex
	grants      map[string]fakeGrant   // authorization code -> grant (single use)
	tokens      map[string]fakeProfile // access token -> profile
	n           int
	tokenForms  []url.Values
	tokenStatus int    // when set, the token endpoint answers with it
	profileMode string // "", "malformed", "redirect", "huge", "status500"
}

func newFakeProvider(t *testing.T) *fakeProvider {
	f := &fakeProvider{t: t, grants: map[string]fakeGrant{}, tokens: map[string]fakeProfile{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/google/token", func(w http.ResponseWriter, r *http.Request) { f.token(w, r, oauth.Google, googleID, googleSecret) })
	mux.HandleFunc("/github/token", func(w http.ResponseWriter, r *http.Request) { f.token(w, r, oauth.GitHub, githubID, githubSecret) })
	mux.HandleFunc("/google/userinfo", f.userinfo)
	mux.HandleFunc("/github/user", f.githubUser)
	mux.HandleFunc("/github/emails", f.githubEmails)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeProvider) providers() map[string]oauth.Provider {
	return map[string]oauth.Provider{
		oauth.Google: oauth.NewGoogle(googleID, googleSecret, &oauth.GoogleEndpoints{
			Auth: f.srv.URL + "/google/auth", Token: f.srv.URL + "/google/token", UserInfo: f.srv.URL + "/google/userinfo"}),
		oauth.GitHub: oauth.NewGitHub(githubID, githubSecret, &oauth.GitHubEndpoints{
			Auth: f.srv.URL + "/github/auth", Token: f.srv.URL + "/github/token", User: f.srv.URL + "/github/user", Emails: f.srv.URL + "/github/emails"}),
	}
}

// grant is the person approving at the provider: it returns the code the provider would put in
// the callback URL.
func (f *fakeProvider) grant(provider string, a attempt, p fakeProfile) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	code := fmt.Sprintf("code-%s-%d", provider, f.n)
	f.grants[code] = fakeGrant{provider: provider, challenge: a.challenge, redirectURI: a.redirectURI, profile: p}
	return code
}

func (f *fakeProvider) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *fakeProvider) token(w http.ResponseWriter, r *http.Request, provider, id, secret string) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenForms = append(f.tokenForms, r.PostForm)
	if f.tokenStatus != 0 {
		w.WriteHeader(f.tokenStatus)
		return
	}
	g, ok := f.grants[r.PostForm.Get("code")]
	delete(f.grants, r.PostForm.Get("code")) // a code works once
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case !ok, g.provider != provider,
		r.PostForm.Get("client_id") != id, r.PostForm.Get("client_secret") != secret,
		r.PostForm.Get("redirect_uri") != g.redirectURI,
		base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	f.n++
	tok := "at-" + strconv.Itoa(f.n)
	f.tokens[tok] = g.profile
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"bearer"}`, tok)
}

// profile finds the person for a bearer token, or answers the failure mode in force.
func (f *fakeProvider) profile(w http.ResponseWriter, r *http.Request) (fakeProfile, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch f.profileMode {
	case "malformed":
		_, _ = w.Write([]byte(`{"sub": nope`))
		return fakeProfile{}, false
	case "redirect":
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
		return fakeProfile{}, false
	case "huge":
		_, _ = w.Write([]byte(`{"sub":"1","pad":"` + strings.Repeat("x", 100<<10) + `"}`))
		return fakeProfile{}, false
	case "status500":
		w.WriteHeader(http.StatusInternalServerError)
		return fakeProfile{}, false
	}
	p, ok := f.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
	}
	return p, ok
}

func (f *fakeProvider) userinfo(w http.ResponseWriter, r *http.Request) {
	p, ok := f.profile(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(map[string]any{"sub": p.account, "email": p.email, "email_verified": p.verified, "name": p.name})
	_, _ = w.Write(b)
}

func (f *fakeProvider) githubUser(w http.ResponseWriter, r *http.Request) {
	p, ok := f.profile(w, r)
	if !ok {
		return
	}
	id, _ := strconv.ParseInt(p.account, 10, 64)
	w.Header().Set("Content-Type", "application/json")
	// The public profile email is never the one that counts.
	b, _ := json.Marshal(map[string]any{"id": id, "name": p.name, "email": "public-decoy@example.invalid"})
	_, _ = w.Write(b)
}

func (f *fakeProvider) githubEmails(w http.ResponseWriter, r *http.Request) {
	p, ok := f.profile(w, r)
	if !ok {
		return
	}
	list := []map[string]any{{"email": "old-decoy@example.invalid", "primary": false, "verified": true}}
	if p.email != "" {
		list = append(list, map[string]any{"email": p.email, "primary": true, "verified": p.verified})
	}
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(list)
	_, _ = w.Write(b)
}

// ---- The browser's side.

// attempt is one trip to the provider as the browser sees it.
type attempt struct {
	provider    string
	state       string
	binding     string // the __Host-rm_oauth cookie value
	challenge   string
	redirectURI string
	location    string
}

// startOAuth plays the browser following "Continue with ..." and returns what it was given.
func (e *env) startOAuth(provider string) (attempt, *httptest.ResponseRecorder) {
	e.t.Helper()
	rec := e.do(req{method: "GET", path: "/v1/auth/" + provider + "/start"})
	if rec.Code != http.StatusFound {
		return attempt{}, rec
	}
	return e.parseAttempt(provider, rec.Header().Get("Location"), rec), rec
}

func (e *env) parseAttempt(provider, location string, rec *httptest.ResponseRecorder) attempt {
	e.t.Helper()
	u, err := url.Parse(location)
	if err != nil {
		e.t.Fatalf("bad Location %q", location)
	}
	q := u.Query()
	a := attempt{provider: provider, state: q.Get("state"), challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri"), location: location}
	if q.Get("code_challenge_method") != "S256" {
		e.t.Errorf("code_challenge_method = %q", q.Get("code_challenge_method"))
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-rm_oauth" {
			a.binding = c.Value
		}
	}
	return a
}

// cb is the callback the provider sends the browser to.
type cb struct {
	provider string // path provider; "" means the attempt's
	code     string
	state    *string // nil means the attempt's
	binding  *string // nil means the attempt's
	session  string  // the session cookie, for a connect
	errParam string
	ip       string
	ua       string
}

func (e *env) callback(a attempt, c cb) *httptest.ResponseRecorder {
	e.t.Helper()
	provider := c.provider
	if provider == "" {
		provider = a.provider
	}
	state := a.state
	if c.state != nil {
		state = *c.state
	}
	binding := a.binding
	if c.binding != nil {
		binding = *c.binding
	}
	v := url.Values{}
	if c.code != "" {
		v.Set("code", c.code)
	}
	if state != "" {
		v.Set("state", state)
	}
	if c.errParam != "" {
		v.Set("error", c.errParam)
	}
	return e.do(req{method: "GET", path: "/v1/auth/" + provider + "/callback?" + v.Encode(), oauthCookie: binding, cookie: c.session, ip: c.ip, ua: c.ua})
}

// result is the outcome of a callback.
type result struct {
	rec      *httptest.ResponseRecorder
	location string
	session  string // the new session token, if one was set
}

func (e *env) outcome(rec *httptest.ResponseRecorder) result {
	e.t.Helper()
	r := result{rec: rec, location: rec.Header().Get("Location")}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-rm_session" && c.MaxAge > 0 {
			r.session = c.Value
		}
	}
	return r
}

// providerSignIn runs a whole sign-in: start, approve at the provider, callback.
func (e *env) providerSignIn(provider string, p fakeProfile, ua string) result {
	e.t.Helper()
	a, rec := e.startOAuth(provider)
	if rec.Code != http.StatusFound {
		e.t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	code := e.fake.grant(provider, a, p)
	return e.outcome(e.callback(a, cb{code: code, ua: ua}))
}

func (r result) ok() bool        { return r.location == "/account" && r.session != "" }
func (r result) refused() string { return strings.TrimPrefix(r.location, "/login?error=") }
