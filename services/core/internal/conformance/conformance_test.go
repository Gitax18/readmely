// Package conformance holds tests that check the real servers against the
// OpenAPI specs in contracts/. It has no non-test code.
package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/clientip"
	"github.com/Gitax18/readmely/services/core/internal/contract/coreinternal"
	"github.com/Gitax18/readmely/services/core/internal/health"
	"github.com/Gitax18/readmely/services/core/internal/httpx"
	"github.com/Gitax18/readmely/services/core/internal/server"
	"github.com/Gitax18/readmely/services/core/internal/status"
)

const testToken = "conformance-token-conformance-token-0123"

func logger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

type spec struct {
	doc    *openapi3.T
	router routers.Router
}

func loadSpec(t *testing.T, name string) spec {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../../contracts/" + name + ".yaml")
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("%s is not valid OpenAPI: %v", name, err)
	}
	doc.Servers = nil // match on path only; the servers list is documentation
	router, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return spec{doc, router}
}

// conforms sends the request to h and checks the response against the spec:
// status, headers and body. It returns the recorded response.
func (s spec) conforms(t *testing.T, h http.Handler, method, path string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	return s.conformsAs(t, h, method, http.MethodGet, path, wantStatus)
}

// conformsAs is conforms for a request whose method differs from the documented
// one (a 405 is documented under the path's real operation, so the route is
// looked up with documentedMethod while the request is sent with method).
func (s spec) conformsAs(t *testing.T, h http.Handler, method, documentedMethod, path string, wantStatus int) *httptest.ResponseRecorder {
	t.Helper()
	routing := httptest.NewRequest(documentedMethod, path, http.NoBody)
	route, params, err := s.router.FindRoute(routing)
	if err != nil {
		t.Fatalf("%s %s is not in the spec: %v", documentedMethod, path, err)
	}
	req := httptest.NewRequest(method, path, http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s = %d, want %d (body %s)", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	err = openapi3filter.ValidateResponse(context.Background(), &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: routing, PathParams: params, Route: route},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Body:                   io.NopCloser(bytes.NewReader(rec.Body.Bytes())),
	})
	if err != nil {
		t.Fatalf("%s %s response does not match the spec: %v\nbody: %s", method, path, err, rec.Body.String())
	}
	return rec
}

// matchesSchema checks a JSON body against a named component schema.
func (s spec) matchesSchema(t *testing.T, schema string, body []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("body is not JSON: %v: %s", err, body)
	}
	if err := s.doc.Components.Schemas[schema].Value.VisitJSON(v); err != nil {
		t.Fatalf("body does not match schema %s: %v\nbody: %s", schema, err, body)
	}
}

// example returns the spec's example value for a response.
func (s spec) example(t *testing.T, path string, code int, name string) any {
	t.Helper()
	media := s.doc.Paths.Find(path).Get.Responses.Status(code).Value.Content["application/json"]
	if name == "" {
		return media.Example
	}
	ex, ok := media.Examples[name]
	if !ok {
		t.Fatalf("spec has no example %q for %s %d", name, path, code)
	}
	return ex.Value.Value
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func decode(t *testing.T, body []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func apiChecks(postgres, redis error) *health.Handler {
	return health.New(logger(), time.Second,
		health.Check{Name: "postgres", Pinger: pinger{postgres}},
		health.Check{Name: "redis", Pinger: pinger{redis}},
	)
}

func internalRouter(checks *health.Handler, ai *status.AIClient) http.Handler {
	return server.NewInternalRouter(logger(), checks, status.NewHandler(checks, ai).Get)
}

// ---- AI service doubles

func aiServer(t *testing.T, code int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func aiOK(t *testing.T) *status.AIClient {
	return status.NewAIClient(logger(), aiServer(t, 200, `{"status":"ready","checks":{"postgres":"ok"}}`).URL, testToken)
}

func aiDown(t *testing.T) *status.AIClient {
	return status.NewAIClient(logger(), aiServer(t, 503, `{"status":"unavailable","checks":{"postgres":"down"}}`).URL, testToken)
}

func aiUnreachable(t *testing.T) *status.AIClient {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	return status.NewAIClient(logger(), url, testToken)
}

func aiUnconfigured(*testing.T) *status.AIClient {
	return status.NewAIClient(logger(), "http://127.0.0.1:1", "")
}

// ---- tests

var errDown = errors.New("down")

func TestPublicListenerConformsToItsSpec(t *testing.T) {
	s := loadSpec(t, "core-public")
	h := publicRouter(apiChecks(nil, nil))

	rec := s.conforms(t, h, http.MethodGet, "/healthz", http.StatusOK)
	if got, want := decode(t, rec.Body.Bytes()), s.example(t, "/healthz", 200, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("/healthz = %v, spec example %v", got, want)
	}
	s.conformsAs(t, h, http.MethodPost, http.MethodGet, "/healthz", http.StatusMethodNotAllowed)
}

func TestInternalHealthAndReadiness(t *testing.T) {
	s := loadSpec(t, "core-internal")

	ready := internalRouter(apiChecks(nil, nil), aiUnconfigured(t))
	s.conforms(t, ready, http.MethodGet, "/healthz", http.StatusOK)
	rec := s.conforms(t, ready, http.MethodGet, "/readyz", http.StatusOK)
	if got, want := decode(t, rec.Body.Bytes()), s.example(t, "/readyz", 200, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("/readyz 200 = %v, spec example %v", got, want)
	}

	down := internalRouter(apiChecks(errDown, nil), aiUnconfigured(t))
	rec = s.conforms(t, down, http.MethodGet, "/readyz", http.StatusServiceUnavailable)
	if got, want := decode(t, rec.Body.Bytes()), s.example(t, "/readyz", 503, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("/readyz 503 = %v, spec example %v", got, want)
	}

	s.conformsAs(t, ready, http.MethodPost, http.MethodGet, "/readyz", http.StatusMethodNotAllowed)
	s.conformsAs(t, ready, http.MethodDelete, http.MethodGet, "/api/v1/status", http.StatusMethodNotAllowed)
}

// Every documented state of /api/v1/status is produced by the real handler and
// must equal the example the spec shows for it.
func TestStatusEveryDocumentedStateMatchesItsExample(t *testing.T) {
	s := loadSpec(t, "core-internal")
	cases := []struct {
		example         string
		postgres, redis error
		ai              func(*testing.T) *status.AIClient
	}{
		{"operational", nil, nil, aiOK},
		{"ai_unreachable", nil, nil, aiUnreachable},
		{"ai_down", nil, nil, aiDown},
		{"ai_unconfigured", nil, nil, aiUnconfigured},
		{"redis_down", nil, errDown, aiOK},
		{"unavailable", errDown, nil, aiUnreachable},
	}
	documented := s.doc.Paths.Find("/api/v1/status").Get.Responses.Status(200).Value.Content["application/json"].Examples
	if len(documented) != len(cases) {
		t.Fatalf("spec documents %d status examples, test covers %d: add the missing case", len(documented), len(cases))
	}
	for _, c := range cases {
		t.Run(c.example, func(t *testing.T) {
			h := internalRouter(apiChecks(c.postgres, c.redis), c.ai(t))
			rec := s.conforms(t, h, http.MethodGet, "/api/v1/status", http.StatusOK)
			if got, want := decode(t, rec.Body.Bytes()), s.example(t, "/api/v1/status", 200, c.example); !reflect.DeepEqual(got, want) {
				t.Errorf("got %v\nspec example %v", got, want)
			}
		})
	}
}

func TestErrorBodiesMatchTheErrorSchema(t *testing.T) {
	s := loadSpec(t, "core-public")
	h := publicRouter(apiChecks(nil, nil))

	// An unknown route cannot be a path in the spec, so check it against the schema directly.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", http.NoBody))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
	s.matchesSchema(t, "Error", rec.Body.Bytes())

	// A crashing handler becomes the generic 500.
	boom := httpx.RequestID(httpx.Recover(logger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })))
	rec = httptest.NewRecorder()
	boom.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	s.matchesSchema(t, "Error", rec.Body.Bytes())
	if strings.Contains(rec.Body.String(), "boom") {
		t.Error("the panic value leaked into the response")
	}
}

// The generated types must be able to carry every example without loss or unknown fields.
func TestSpecExamplesRoundTripThroughGeneratedTypes(t *testing.T) {
	s := loadSpec(t, "core-internal")
	roundTrip := func(name string, example any, into any) {
		raw, err := json.Marshal(example)
		if err != nil {
			t.Fatal(err)
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(into); err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		back, _ := json.Marshal(into)
		if !reflect.DeepEqual(decode(t, back), example) {
			t.Errorf("%s: round trip changed the value: %s", name, back)
		}
	}
	for name, ex := range s.doc.Paths.Find("/api/v1/status").Get.Responses.Status(200).Value.Content["application/json"].Examples {
		roundTrip("status/"+name, ex.Value.Value, new(coreinternal.StatusReport))
	}
	roundTrip("readyz 200", s.example(t, "/readyz", 200, ""), new(coreinternal.Readiness))
	roundTrip("readyz 503", s.example(t, "/readyz", 503, ""), new(coreinternal.Readiness))
}

// ---- route parity: every registered route is documented, every documented path is registered

func registered(t *testing.T, h http.Handler) []string {
	t.Helper()
	r, ok := h.(chi.Routes)
	if !ok {
		t.Fatalf("%T is not a chi router", h)
	}
	var out []string
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		out = append(out, method+" "+route)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func documented(doc *openapi3.T) []string {
	var out []string
	for path, item := range doc.Paths.Map() {
		for method := range item.Operations() {
			out = append(out, method+" "+path)
		}
	}
	sort.Strings(out)
	return out
}

// parity returns routes missing from the spec and spec operations missing from the code.
func parity(code, spec []string) (undocumented, unimplemented []string) {
	for _, c := range code {
		if !slices.Contains(spec, c) {
			undocumented = append(undocumented, c)
		}
	}
	for _, s := range spec {
		if !slices.Contains(code, s) {
			unimplemented = append(unimplemented, s)
		}
	}
	return undocumented, unimplemented
}

// planned lists operations that are in a spec (contracts come first) before their handler exists.
// Implementing one means deleting it from this list; the test below fails until you do, and fails
// if an entry is not in the spec, so the list cannot rot. It is empty now that the provider routes exist (Phase 2b step 4); the next feature to add a contract first uses it again.
var planned = map[string][]string{}

// publicRouter is the public listener as main wires it. The account routes only need to be
// registered here: nothing in these tests reaches a handler that touches a database.
func publicRouter(checks *health.Handler) http.Handler {
	return server.NewPublicRouter(logger(), checks, accountRoutes)
}

// accountRoutes registers the /v1 account endpoints on a router.
func accountRoutes(r chi.Router) {
	svc := auth.NewService(nil, nil, nil, auth.NewKeys(testToken), logger())
	auth.NewHandler(svc, clientip.New(nil), "http://127.0.0.1:3000", logger()).Mount(r)
}

func withoutPlanned(spec, skip []string) []string {
	var out []string
	for _, r := range spec {
		if !slices.Contains(skip, r) {
			out = append(out, r)
		}
	}
	return out
}

func TestRoutesMatchTheSpecs(t *testing.T) {
	checks := apiChecks(nil, nil)
	for name, h := range map[string]http.Handler{
		"core-public":   publicRouter(checks),
		"core-internal": internalRouter(checks, aiUnconfigured(t)),
	} {
		t.Run(name, func(t *testing.T) {
			spec, code := documented(loadSpec(t, name).doc), registered(t, h)
			for _, p := range planned[name] {
				if !slices.Contains(spec, p) {
					t.Errorf("planned route %q is not in the spec; remove it from planned", p)
				}
				if slices.Contains(code, p) {
					t.Errorf("planned route %q is implemented now; remove it from planned", p)
				}
			}
			undocumented, unimplemented := parity(code, withoutPlanned(spec, planned[name]))
			if len(undocumented) > 0 || len(unimplemented) > 0 {
				t.Errorf("routes in code but not in the spec: %v\nin the spec but not in code: %v", undocumented, unimplemented)
			}
		})
	}
}

// The parity check itself must notice both kinds of drift.
func TestParityCheckDetectsDrift(t *testing.T) {
	s := loadSpec(t, "core-public")

	extra := chi.NewRouter()
	extra.Get("/healthz", func(http.ResponseWriter, *http.Request) {})
	extra.Get("/secret", func(http.ResponseWriter, *http.Request) {})
	accountRoutes(extra)
	undocumented, unimplemented := parity(registered(t, extra), withoutPlanned(documented(s.doc), planned["core-public"]))
	if !reflect.DeepEqual(undocumented, []string{"GET /secret"}) || len(unimplemented) != 0 {
		t.Errorf("planted extra route: undocumented=%v unimplemented=%v", undocumented, unimplemented)
	}

	missing := chi.NewRouter()
	accountRoutes(missing)
	undocumented, unimplemented = parity(registered(t, missing), withoutPlanned(documented(s.doc), planned["core-public"]))
	if len(undocumented) != 0 || !reflect.DeepEqual(unimplemented, []string{"GET /healthz"}) {
		t.Errorf("planted missing route: undocumented=%v unimplemented=%v", undocumented, unimplemented)
	}
}
