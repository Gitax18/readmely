package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Provider names, as they appear in URLs, the database and the API.
const (
	Google = "google"
	GitHub = "github"
)

const (
	requestTimeout = 5 * time.Second
	maxBodyBytes   = 64 << 10
	maxAccountLen  = 255
	maxTokenLen    = 4096
)

// Profile is what a sign-in needs to know about the person, as the provider reports it.
type Profile struct {
	Account       string // the provider's stable id for this person
	Email         string // as the provider gave it; empty when there is none to trust
	EmailVerified bool   // the provider itself vouches for the address
	Name          string
}

// Provider is one sign-in provider.
type Provider interface {
	Name() string
	// AuthorizationURL is where the browser goes first.
	AuthorizationURL(state, challenge, redirectURI string) string
	// Exchange trades the code for an access token, reads the profile with it, and forgets the
	// token. Errors never contain anything the provider sent.
	Exchange(ctx context.Context, code, verifier, redirectURI string) (Profile, error)
}

// Error is a failed call to a provider. It names the step and the HTTP status, nothing else:
// bodies can carry codes and tokens, and must not reach a log.
type Error struct {
	Provider string
	Step     string
	Status   int
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("%s %s failed with status %d", e.Provider, e.Step, e.Status)
	}
	return fmt.Sprintf("%s %s failed", e.Provider, e.Step)
}

// client talks to providers: fixed hosts, short timeouts, no redirects (a redirect would send the
// credentials to a place the code never named), capped bodies.
type client struct {
	name string
	http *http.Client
}

func newClient(name string) *client {
	return &client{name: name, http: &http.Client{
		Timeout:       requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// do sends the request and returns the status and a body of at most maxBodyBytes.
func (c *client) do(req *http.Request, step string) (int, []byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, &Error{Provider: c.name, Step: step}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil || len(body) > maxBodyBytes {
		return resp.StatusCode, nil, &Error{Provider: c.name, Step: step, Status: resp.StatusCode}
	}
	return resp.StatusCode, body, nil
}

// token exchanges the code at the token endpoint and returns the access token.
func (c *client) token(ctx context.Context, endpoint string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", &Error{Provider: c.name, Step: "token"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	status, body, err := c.do(req, "token")
	if err != nil {
		return "", err
	}
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &out) != nil || out.Error != "" ||
		out.AccessToken == "" || len(out.AccessToken) > maxTokenLen {
		return "", &Error{Provider: c.name, Step: "token", Status: status}
	}
	return out.AccessToken, nil
}

// getJSON reads one JSON document with the access token.
func (c *client) getJSON(ctx context.Context, endpoint, accessToken, step string, headers map[string]string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return &Error{Provider: c.name, Step: step}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	status, body, err := c.do(req, step)
	if err != nil {
		return err
	}
	if status != http.StatusOK || json.Unmarshal(body, v) != nil {
		return &Error{Provider: c.name, Step: step, Status: status}
	}
	return nil
}

func query(base string, params [][2]string) string {
	v := url.Values{}
	for _, p := range params {
		v.Set(p[0], p[1])
	}
	return base + "?" + v.Encode()
}

// flexBool reads a JSON boolean, or the strings "true" and "false" some Google responses use.
type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", `"true"`:
		*b = true
	case "false", `"false"`, "null":
		*b = false
	default:
		return errors.New("not a boolean")
	}
	return nil
}

func validAccount(id string) bool { return id != "" && len(id) <= maxAccountLen }

// ---- Google

// GoogleEndpoints are Google's URLs. Tests point them at a fake; nothing in the configuration can.
type GoogleEndpoints struct{ Auth, Token, UserInfo string }

// DefaultGoogleEndpoints are the real ones.
var DefaultGoogleEndpoints = GoogleEndpoints{ //nolint:gosec // G101: public endpoint URLs, not credentials
	Auth:     "https://accounts.google.com/o/oauth2/v2/auth",
	Token:    "https://oauth2.googleapis.com/token",
	UserInfo: "https://openidconnect.googleapis.com/v1/userinfo",
}

type google struct {
	id, secret string
	ep         GoogleEndpoints
	c          *client
}

// NewGoogle returns the Google provider. The profile comes from the userinfo endpoint with the
// access token handed over by the token endpoint on a direct TLS call, so no ID token is parsed
// and no signing key has to be followed.
func NewGoogle(clientID, clientSecret string, ep *GoogleEndpoints) Provider {
	g := &google{id: clientID, secret: clientSecret, ep: DefaultGoogleEndpoints, c: newClient(Google)}
	if ep != nil {
		g.ep = *ep
	}
	return g
}

func (g *google) Name() string { return Google }

func (g *google) AuthorizationURL(state, challenge, redirectURI string) string {
	return query(g.ep.Auth, [][2]string{
		{"client_id", g.id}, {"redirect_uri", redirectURI}, {"response_type", "code"},
		{"scope", "openid email profile"}, {"state", state},
		{"code_challenge", challenge}, {"code_challenge_method", "S256"},
		{"prompt", "select_account"},
	})
}

func (g *google) Exchange(ctx context.Context, code, verifier, redirectURI string) (Profile, error) {
	token, err := g.c.token(ctx, g.ep.Token, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier},
		"redirect_uri": {redirectURI}, "client_id": {g.id}, "client_secret": {g.secret},
	})
	if err != nil {
		return Profile{}, err
	}
	var info struct {
		Sub           string   `json:"sub"`
		Email         string   `json:"email"`
		EmailVerified flexBool `json:"email_verified"`
		Name          string   `json:"name"`
	}
	if err := g.c.getJSON(ctx, g.ep.UserInfo, token, "profile", nil, &info); err != nil {
		return Profile{}, err
	}
	if !validAccount(info.Sub) {
		return Profile{}, &Error{Provider: Google, Step: "profile"}
	}
	return Profile{Account: info.Sub, Email: info.Email, EmailVerified: bool(info.EmailVerified), Name: info.Name}, nil
}

// ---- GitHub

// GitHubEndpoints are GitHub's URLs. Tests point them at a fake; nothing in the configuration can.
type GitHubEndpoints struct{ Auth, Token, User, Emails string }

// DefaultGitHubEndpoints are the real ones.
var DefaultGitHubEndpoints = GitHubEndpoints{ //nolint:gosec // G101: public endpoint URLs, not credentials
	Auth:   "https://github.com/login/oauth/authorize",
	Token:  "https://github.com/login/oauth/access_token",
	User:   "https://api.github.com/user",
	Emails: "https://api.github.com/user/emails",
}

type github struct {
	id, secret string
	ep         GitHubEndpoints
	c          *client
}

// NewGitHub returns the GitHub provider (an OAuth App with read:user and user:email). GitHub's
// profile email can be public, unverified or hidden, so the address is taken only from the
// primary and verified entry of /user/emails.
func NewGitHub(clientID, clientSecret string, ep *GitHubEndpoints) Provider {
	g := &github{id: clientID, secret: clientSecret, ep: DefaultGitHubEndpoints, c: newClient(GitHub)}
	if ep != nil {
		g.ep = *ep
	}
	return g
}

func (g *github) Name() string { return GitHub }

func (g *github) AuthorizationURL(state, challenge, redirectURI string) string {
	return query(g.ep.Auth, [][2]string{
		{"client_id", g.id}, {"redirect_uri", redirectURI}, {"scope", "read:user user:email"},
		{"state", state}, {"code_challenge", challenge}, {"code_challenge_method", "S256"},
	})
}

func (g *github) Exchange(ctx context.Context, code, verifier, redirectURI string) (Profile, error) {
	token, err := g.c.token(ctx, g.ep.Token, url.Values{
		"code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirectURI},
		"client_id": {g.id}, "client_secret": {g.secret},
	})
	if err != nil {
		return Profile{}, err
	}
	headers := map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28", "User-Agent": "readmely"}
	var user struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	}
	if err := g.c.getJSON(ctx, g.ep.User, token, "profile", headers, &user); err != nil {
		return Profile{}, err
	}
	n, err := strconv.ParseInt(user.ID.String(), 10, 64)
	if err != nil || n <= 0 {
		return Profile{}, &Error{Provider: GitHub, Step: "profile"}
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := g.c.getJSON(ctx, g.ep.Emails, token, "emails", headers, &emails); err != nil {
		return Profile{}, err
	}
	p := Profile{Account: strconv.FormatInt(n, 10), Name: user.Name}
	for _, e := range emails {
		if e.Primary && e.Verified {
			p.Email, p.EmailVerified = e.Email, true
			break
		}
	}
	return p, nil
}
