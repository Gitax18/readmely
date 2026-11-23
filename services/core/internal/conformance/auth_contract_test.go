package conformance

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The security rules for the account API (plans/phase-2a-email-login.md) are checked on the spec
// itself, so a later endpoint cannot be added without them. Handlers are tested against the same
// spec as they are built.
func TestAccountAPIDocumentsItsSecurityRules(t *testing.T) {
	s := loadSpec(t, "core-public")

	mutating := func(m string) bool { return m != http.MethodGet && m != http.MethodHead }
	hasParam := func(op *openapi3.Operation, name string) bool {
		for _, p := range op.Parameters {
			if p.Value != nil && p.Value.Name == name && p.Value.In == "header" && p.Value.Required {
				return true
			}
		}
		return false
	}
	has := func(op *openapi3.Operation, code string) bool { return op.Responses.Status(atoi(code)) != nil }

	var seen []string
	for path, item := range s.doc.Paths.Map() {
		if !strings.HasPrefix(path, "/v1/") {
			continue
		}
		for method, op := range item.Operations() {
			seen = append(seen, method+" "+path)
			emailStep := strings.HasPrefix(path, "/v1/auth/email/")
			// Provider sign-in steps are browser navigations (GET, redirects), protected by the
			// attempt cookie and state rather than Origin or CSRF; they are rate limited.
			providerStep := strings.HasPrefix(path, "/v1/auth/{provider}/")
			providerList := path == "/v1/auth/providers"
			public := emailStep || providerStep || providerList
			signedIn := op.Security != nil && len(*op.Security) == 1 && (*op.Security)[0]["sessionCookie"] != nil

			if public == signedIn {
				t.Errorf("%s %s must be either a public sign-in step or require sessionCookie, not both or neither", method, path)
			}
			if !has(op, "400") && op.RequestBody != nil {
				t.Errorf("%s %s takes a body and must document 400", method, path)
			}
			switch {
			case providerList:
				if method != http.MethodGet {
					t.Errorf("%s %s must be a GET", method, path)
				}
			case providerStep:
				if method != http.MethodGet || !has(op, "302") || !has(op, "404") || !has(op, "429") {
					t.Errorf("%s %s must be a GET that documents 302, 404 and 429", method, path)
				}
				if has(op, "302") && (op.Responses.Status(302).Value.Headers["Location"] == nil || op.Responses.Status(302).Value.Headers["Set-Cookie"] == nil) {
					t.Errorf("%s %s: 302 must document Location and Set-Cookie", method, path)
				}
				if op.Responses.Status(429).Value.Headers["Retry-After"] == nil {
					t.Errorf("%s %s: 429 must carry Retry-After", method, path)
				}
			case emailStep:
				for _, code := range []string{"403", "429"} {
					if !has(op, code) {
						t.Errorf("%s %s must document %s", method, path, code)
					}
				}
				if op.Responses.Status(429).Value.Headers["Retry-After"] == nil {
					t.Errorf("%s %s: 429 must carry Retry-After", method, path)
				}
			case signedIn:
				if !has(op, "401") {
					t.Errorf("%s %s requires a session and must document 401", method, path)
				}
				if mutating(method) && (!hasParam(op, "X-CSRF-Token") || !has(op, "403")) {
					t.Errorf("%s %s changes state and must require X-CSRF-Token and document 403", method, path)
				}
			}
			// Nothing in the account API answers with a token or a hash.
			if ok := op.Responses.Status(200); ok == nil || ok.Value == nil {
				continue
			} else if body := ok.Value.Content["application/json"]; body != nil {
				for _, bad := range []string{"token_hash", "code_hash", "session_token", "password"} {
					if strings.Contains(strings.ToLower(mustJSON(t, body.Schema)), bad) {
						t.Errorf("%s %s: response schema mentions %q", method, path, bad)
					}
				}
			}
		}
	}
	sort.Strings(seen)
	want := []string{
		"DELETE /v1/me/identities/{provider}", "DELETE /v1/me/sessions", "DELETE /v1/me/sessions/{id}", "GET /v1/auth/providers",
		"GET /v1/auth/{provider}/callback", "GET /v1/auth/{provider}/start", "GET /v1/me",
		"GET /v1/me/identities", "GET /v1/me/sessions", "PATCH /v1/me",
		"POST /v1/auth/email/start", "POST /v1/auth/email/verify", "POST /v1/auth/logout",
		"POST /v1/me/deletion/confirm", "POST /v1/me/deletion/start", "POST /v1/me/identities/{provider}/start",
	}
	if strings.Join(seen, ",") != strings.Join(want, ",") {
		t.Errorf("operations under /v1 = %v, want %v", seen, want)
	}
}
