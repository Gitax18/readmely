//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/redis/go-redis/v9"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/dbtest"
	"github.com/Gitax18/readmely/services/core/internal/health"
	"github.com/Gitax18/readmely/services/core/internal/mail"
	"github.com/Gitax18/readmely/services/core/internal/oauth"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
	"github.com/Gitax18/readmely/services/core/internal/server"
)

const (
	webOrigin = "http://127.0.0.1:3000"
	secret    = "test-secret-test-secret-test-secret-0123"
)

// capture is a Mailer that keeps what it was asked to send.
type capture struct {
	mu   sync.Mutex
	msgs []mail.Message
	gate chan struct{} // when set, Send blocks until it is closed
	in   chan struct{} // signalled when a Send starts
}

func (c *capture) Send(ctx context.Context, m mail.Message) error {
	if c.in != nil {
		select {
		case c.in <- struct{}{}:
		default:
		}
	}
	if c.gate != nil {
		select {
		case <-c.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
	return nil
}

func (c *capture) to(email string) []mail.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []mail.Message
	for _, m := range c.msgs {
		if m.To == email {
			out = append(out, m)
		}
	}
	return out
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type envOpts struct {
	trusted       []netip.Prefix
	queueCapacity int
	senders       int
	mailer        *capture
	oauth         bool // enable Google and GitHub against a fake provider
}

type env struct {
	t      *testing.T
	db     *dbtest.DB
	svc    *auth.Service
	h      http.Handler
	mail   *capture
	queue  *mail.Queue
	rdb    *redis.Client
	prefix string
	logs   *syncBuf
	spec   *openapi3.T
	router routers.Router
	keys   auth.Keys
	nextIP int
	fake   *fakeProvider
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	url := os.Getenv("REDIS_URL")
	if url == "" {
		t.Fatal("REDIS_URL is not set: run through `make test-integration`")
	}
	ropts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatal("REDIS_URL could not be parsed")
	}
	rdb := redis.NewClient(ropts)
	prefix := "auth-test:" + rand.Text() + ":"
	t.Cleanup(func() {
		keys, _ := rdb.Keys(context.Background(), prefix+"*").Result()
		if len(keys) > 0 {
			_ = rdb.Del(context.Background(), keys...).Err()
		}
		_ = rdb.Close()
	})

	db := dbtest.New(t)
	logs := &syncBuf{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cap := o.mailer
	if cap == nil {
		cap = &capture{}
	}
	if o.queueCapacity == 0 {
		o.queueCapacity = 64
	}
	if o.senders == 0 {
		o.senders = 2
	}
	queue := mail.NewQueue(cap, logger, o.senders, o.queueCapacity, 5*time.Second)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = queue.Close(ctx)
	})

	keys := auth.NewKeys(secret)
	svc := auth.NewService(db.Pool, ratelimit.New(rdb, prefix), queue, keys, logger)
	var fake *fakeProvider
	if o.oauth {
		fake = newFakeProvider(t)
		svc.EnableOAuth(auth.OAuthConfig{Providers: fake.providers(), Store: oauth.NewStore(rdb, prefix), WebOrigin: webOrigin})
	}
	handler := auth.NewHandler(svc, clientip.New(o.trusted), webOrigin, logger)
	router := server.NewPublicRouter(logger, health.New(logger, time.Second), handler.Mount)

	doc, err := openapi3.NewLoader().LoadFromFile("../../../../contracts/core-public.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc.Servers = nil
	rt, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, db: db, svc: svc, h: router, mail: cap, queue: queue, rdb: rdb, prefix: prefix, logs: logs, spec: doc, router: rt, keys: keys, fake: fake}
}

// req describes one request. The zero value is a well-formed browser request from a fresh client.
type req struct {
	method, path string
	body         string
	cookie       string // the session token
	csrf         string
	origin       *string // nil means the real web origin; pointer to "" means no Origin header
	contentType  string  // "" means application/json when there is a body
	ip           string  // socket address; "" means a fresh one
	xff          string
	ua           string
	oauthCookie  string // the sign-in attempt cookie
}

func str(s string) *string { return &s }

func (e *env) freshIP() string {
	e.nextIP++
	return "192.0.2." + itoa(e.nextIP%250+1)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// do sends the request through the real router and checks the response against the OpenAPI spec
// whenever the spec documents the route and status.
func (e *env) do(r req) *httptest.ResponseRecorder {
	e.t.Helper()
	var body io.Reader = http.NoBody
	if r.body != "" {
		body = strings.NewReader(r.body)
	}
	hr := httptest.NewRequest(r.method, r.path, body)
	ip := r.ip
	if ip == "" {
		ip = e.freshIP()
	}
	hr.RemoteAddr = ip + ":40000"
	if r.origin == nil {
		hr.Header.Set("Origin", webOrigin)
	} else if *r.origin != "" {
		hr.Header.Set("Origin", *r.origin)
	}
	if r.body != "" {
		ct := r.contentType
		if ct == "" {
			ct = "application/json"
		}
		hr.Header.Set("Content-Type", ct)
	}
	if r.cookie != "" {
		hr.AddCookie(&http.Cookie{Name: auth.CookieName, Value: r.cookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // G124 wants the attributes on a request cookie, which carries only name and value
	}
	if r.oauthCookie != "" {
		hr.AddCookie(&http.Cookie{Name: auth.OAuthCookieName, Value: r.oauthCookie, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}) //nolint:gosec // G124 wants the attributes on a request cookie, which carries only name and value
	}
	if r.csrf != "" {
		hr.Header.Set("X-CSRF-Token", r.csrf)
	}
	if r.xff != "" {
		hr.Header.Set("X-Forwarded-For", r.xff)
	}
	if r.ua != "" {
		hr.Header.Set("User-Agent", r.ua)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, hr)
	e.validate(hr, rec)
	return rec
}

func (e *env) validate(hr *http.Request, rec *httptest.ResponseRecorder) {
	e.t.Helper()
	route, params, err := e.router.FindRoute(hr)
	if err != nil {
		return // an unknown route or method is not in the spec
	}
	op := route.PathItem.GetOperation(hr.Method)
	if op == nil {
		return
	}
	if op.Responses.Status(rec.Code) == nil && op.Responses.Default() == nil {
		e.t.Errorf("%s %s answered %d, which the spec does not document", hr.Method, hr.URL.Path, rec.Code)
		return
	}
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: hr, PathParams: params, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	})
	if err != nil {
		e.t.Errorf("%s %s (%d) does not match the spec: %v\nbody: %s", hr.Method, hr.URL.Path, rec.Code, err, rec.Body.String())
	}
}

var codeRe = regexp.MustCompile(`\b([0-9]{6})\b`)

// awaitCode waits for the n-th email to the address and returns the code in it.
func (e *env) awaitCode(email string, n int) string {
	e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if msgs := e.mail.to(email); len(msgs) >= n {
			m := codeRe.FindStringSubmatch(msgs[n-1].Text)
			if m == nil {
				e.t.Fatalf("no code in the email: %q", msgs[n-1].Text)
			}
			return m[1]
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatalf("no email number %d to %s arrived", n, email)
	return ""
}

func (e *env) resetLimits() {
	e.t.Helper()
	ctx := context.Background()
	keys, err := e.rdb.Keys(ctx, e.prefix+"*").Result()
	if err != nil {
		e.t.Fatal(err)
	}
	if len(keys) > 0 {
		if err := e.rdb.Del(ctx, keys...).Err(); err != nil {
			e.t.Fatal(err)
		}
	}
}

func jsonBody(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %v: %s", err, rec.Body.String())
	}
	return m
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	s, _ := decodeMap(t, rec)["error"].(string)
	return s
}

// login is a convenience for tests that need a signed-in user: start, read the code, verify.
type login struct {
	token, csrf, email string
	userID, sessionID  string
	rec                *httptest.ResponseRecorder
}

func (e *env) signIn(email string, ip string) login {
	e.t.Helper()
	if ip == "" {
		ip = e.freshIP()
	}
	norm := strings.ToLower(strings.TrimSpace(email))
	n := len(e.mail.to(norm)) + 1
	start := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": email}), ip: ip})
	if start.Code != http.StatusAccepted {
		e.t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	code := e.awaitCode(norm, n)
	rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: jsonBody(map[string]string{"email": email, "code": code}), ip: ip, ua: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15"})
	if rec.Code != http.StatusOK {
		e.t.Fatalf("verify = %d %s", rec.Code, rec.Body.String())
	}
	var token string
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			token = c.Value
		}
	}
	me := decodeMap(e.t, rec)
	user := me["user"].(map[string]any)
	l := login{token: token, csrf: me["csrf_token"].(string), email: user["email"].(string), userID: user["id"].(string), rec: rec}
	// The session id is not in the response; find it from the stored hash.
	q := store.New(e.db.Pool)
	row, err := q.GetSessionByTokenHash(context.Background(), auth.HashToken(token))
	if err != nil {
		e.t.Fatal(err)
	}
	l.sessionID = row.Session.ID.String()
	return l
}

func (l login) get(e *env, path string) *httptest.ResponseRecorder {
	return e.do(req{method: "GET", path: path, cookie: l.token})
}

func (l login) send(e *env, method, path, body string) *httptest.ResponseRecorder {
	return e.do(req{method: method, path: path, body: body, cookie: l.token, csrf: l.csrf})
}
