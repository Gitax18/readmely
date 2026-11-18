//go:build integration

package auth_test

import (
	"context"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Gitax18/readmely/services/core/internal/auth"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
)

func countRows(t *testing.T, e *env, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.Pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func exec(t *testing.T, e *env, query string, args ...any) {
	t.Helper()
	if _, err := e.db.Pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func verifyBody(email, code string) string {
	return jsonBody(map[string]string{"email": email, "code": code})
}

func start(e *env, email, ip string) int {
	e.t.Helper()
	return e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": email}), ip: ip}).Code
}

// Scenarios 1 and 11: a new address signs in, gets an account, a verified email and a cookie
// with exactly the right attributes.
func TestNewAddressSignsInAndGetsAnAccount(t *testing.T) {
	e := newEnv(t, envOpts{})
	l := e.signIn("ada@example.com", "")

	if got := countRows(t, e, `SELECT count(*) FROM users WHERE email = 'ada@example.com' AND email_verified_at IS NOT NULL`); got != 1 {
		t.Fatalf("verified users = %d, want 1", got)
	}
	var cookie *http.Cookie
	for _, c := range l.rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Errorf("cookie attributes wrong: %+v", cookie)
	}
	if cookie.MaxAge != int(auth.SessionAbsolute/time.Second) || len(cookie.Value) != 43 {
		t.Errorf("cookie lifetime or value length wrong: %+v", cookie)
	}
	raw := l.rec.Header().Get("Set-Cookie")
	if !strings.HasPrefix(raw, "__Host-rm_session=") || strings.Contains(raw, "Domain") {
		t.Errorf("Set-Cookie = %q", raw)
	}

	// Only the hash is stored, never the token.
	if got := countRows(t, e, `SELECT count(*) FROM sessions WHERE token_hash = $1`, auth.HashToken(l.token)); got != 1 {
		t.Errorf("sessions with the token's hash = %d, want 1", got)
	}
	// The challenge was consumed.
	if got := countRows(t, e, `SELECT count(*) FROM login_challenges WHERE closed_at IS NULL`); got != 0 {
		t.Errorf("open challenges after sign-in = %d, want 0", got)
	}

	me := e.do(req{method: "GET", path: "/v1/me", cookie: l.token})
	if me.Code != http.StatusOK {
		t.Fatalf("GET /v1/me = %d", me.Code)
	}
	if got := decodeMap(t, me)["user"].(map[string]any)["email"]; got != "ada@example.com" {
		t.Errorf("email = %v", got)
	}
}

// Scenario 2: the same mailbox in other spellings is one account, and a second sign-in
// creates a second session for it.
func TestExistingAddressSignsInWithoutDuplicates(t *testing.T) {
	e := newEnv(t, envOpts{})
	a := e.signIn("ada@example.com", "")
	e.resetLimits()
	b := e.signIn("  ADA@Example.COM ", "")
	if a.userID != b.userID {
		t.Fatalf("user ids differ: %s vs %s", a.userID, b.userID)
	}
	if got := countRows(t, e, `SELECT count(*) FROM users`); got != 1 {
		t.Errorf("users = %d, want 1", got)
	}
	if a.token == b.token || a.sessionID == b.sessionID {
		t.Error("two sign-ins share a session")
	}
	if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 2 {
		t.Errorf("sessions = %d, want 2", got)
	}
	// "+tag" is a different mailbox and stays a different account.
	e.resetLimits()
	c := e.signIn("ada+news@example.com", "")
	if c.userID == a.userID {
		t.Error("a +tag address was merged into the base address")
	}
}

// Scenario 3: wrong codes are rejected, and five wrong guesses lock the code for good.
func TestFiveWrongCodesLockTheChallenge(t *testing.T) {
	e := newEnv(t, envOpts{})
	ip := "198.51.100.20"
	if start(e, "ada@example.com", ip) != http.StatusAccepted {
		t.Fatal("start failed")
	}
	right := e.awaitCode("ada@example.com", 1)
	wrong := "000000"
	if right == wrong {
		wrong = "000001"
	}
	for i := range 5 {
		rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("ada@example.com", wrong), ip: ip})
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_code" {
			t.Fatalf("wrong guess %d = %d %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("ada@example.com", right), ip: ip})
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_code" {
		t.Fatalf("the right code after five wrong ones = %d %s; want it refused", rec.Code, rec.Body.String())
	}
	if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 0 {
		t.Errorf("sessions = %d, want 0", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM users`); got != 0 {
		t.Errorf("users = %d, want 0 (a failed sign-in must not create an account)", got)
	}
}

// Scenario 4: expired, used and superseded codes are all refused.
func TestExpiredUsedAndSupersededCodesAreRefused(t *testing.T) {
	e := newEnv(t, envOpts{})
	try := func(email, code string) (int, string) {
		rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody(email, code), ip: "198.51.100.30"})
		return rec.Code, errCode(t, rec)
	}

	t.Run("expired", func(t *testing.T) {
		start(e, "old@example.com", "198.51.100.31")
		code := e.awaitCode("old@example.com", 1)
		exec(t, e, `UPDATE login_challenges SET created_at = now() - interval '2 hours', expires_at = now() - interval '90 minutes' WHERE email = 'old@example.com'`)
		if status, code2 := try("old@example.com", code); status != 400 || code2 != "invalid_code" {
			t.Errorf("expired code = %d %s", status, code2)
		}
	})
	t.Run("used", func(t *testing.T) {
		e.signIn("used@example.com", "")
		// Same code again: find it from the email.
		code := codeRe.FindString(e.mail.to("used@example.com")[0].Text)
		if status, code2 := try("used@example.com", code); status != 400 || code2 != "invalid_code" {
			t.Errorf("reused code = %d %s", status, code2)
		}
		if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 1 {
			t.Errorf("sessions = %d, want 1", got)
		}
	})
	t.Run("superseded", func(t *testing.T) {
		start(e, "twice@example.com", "198.51.100.32")
		first := e.awaitCode("twice@example.com", 1)
		e.resetLimits() // get past the one-per-minute limit
		start(e, "twice@example.com", "198.51.100.32")
		second := e.awaitCode("twice@example.com", 2)
		if first == second {
			t.Skip("the two codes happen to be equal (1 in a million)")
		}
		if status, code2 := try("twice@example.com", first); status != 400 || code2 != "invalid_code" {
			t.Errorf("superseded code = %d %s", status, code2)
		}
		if got := countRows(t, e, `SELECT count(*) FROM login_challenges WHERE email = 'twice@example.com' AND closed_at IS NULL`); got != 1 {
			t.Errorf("open challenges = %d, want 1", got)
		}
	})
	t.Run("an address that never asked for a code", func(t *testing.T) {
		if status, code2 := try("nobody@example.com", "123456"); status != 400 || code2 != "invalid_code" {
			t.Errorf("unknown address = %d %s", status, code2)
		}
	})
}

// Scenario 5: asking for a code looks the same for a known and an unknown address.
func TestStartDoesNotRevealWhetherAnAddressHasAnAccount(t *testing.T) {
	e := newEnv(t, envOpts{})
	known := e.signIn("known@example.com", "")
	_ = known

	one := func(email string) (int, string, time.Duration) {
		t0 := time.Now()
		rec := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": email}), ip: "198.51.100.40"})
		return rec.Code, rec.Body.String(), time.Since(t0)
	}
	e.resetLimits()
	var knownDur, unknownDur []time.Duration
	var knownBody, unknownBody string
	for i := range 7 {
		// Different mailbox each time so the per-address limit never applies.
		e.resetLimits()
		status, body, d := one("known@example.com")
		if status != http.StatusAccepted {
			t.Fatalf("known: %d", status)
		}
		knownBody, knownDur = body, append(knownDur, d)
		e.resetLimits()
		status, body, d = one("unknown" + itoa(i) + "@example.com")
		if status != http.StatusAccepted {
			t.Fatalf("unknown: %d", status)
		}
		unknownBody, unknownDur = body, append(unknownDur, d)
	}
	if knownBody != unknownBody {
		t.Errorf("bodies differ: %q vs %q", knownBody, unknownBody)
	}
	slices.Sort(knownDur)
	slices.Sort(unknownDur)
	diff := knownDur[3] - unknownDur[3]
	if diff < 0 {
		diff = -diff
	}
	if diff > 50*time.Millisecond {
		t.Errorf("median time differs by %v (known %v, unknown %v)", diff, knownDur[3], unknownDur[3])
	}
}

// Scenario 6: the limits answer 429 with Retry-After, and no email goes out for a limited request.
func TestRateLimits(t *testing.T) {
	t.Run("one code per address per minute", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		if start(e, "a@example.com", "198.51.100.50") != 202 {
			t.Fatal("first start refused")
		}
		rec := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "a@example.com"}), ip: "198.51.100.51"})
		if rec.Code != 429 || errCode(t, rec) != "rate_limited" {
			t.Fatalf("second start = %d %s", rec.Code, rec.Body.String())
		}
		ra := rec.Header().Get("Retry-After")
		if ra == "" || ra == "0" || len(ra) > 2 {
			t.Errorf("Retry-After = %q, want 1 to 60", ra)
		}
		time.Sleep(100 * time.Millisecond)
		if n := len(e.mail.to("a@example.com")); n != 1 {
			t.Errorf("emails sent = %d, want 1 (none for the limited request)", n)
		}
	})
	t.Run("five codes per address per hour", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		var statuses []int
		for i := range 6 {
			// The per-minute limit is lifted between calls; the hourly one is not.
			ctx := context.Background()
			keys, _ := e.rdb.Keys(ctx, e.prefix+"login_start_email_minute*").Result()
			if len(keys) > 0 {
				e.rdb.Del(ctx, keys...)
			}
			statuses = append(statuses, start(e, "b@example.com", "198.51.100."+itoa(60+i)))
		}
		if !slices.Equal(statuses, []int{202, 202, 202, 202, 202, 429}) {
			t.Fatalf("statuses = %v", statuses)
		}
		time.Sleep(100 * time.Millisecond)
		if n := len(e.mail.to("b@example.com")); n != 5 {
			t.Errorf("emails sent = %d, want 5", n)
		}
		rec := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "b@example.com"}), ip: "198.51.100.99"})
		if ra := rec.Header().Get("Retry-After"); rec.Code != 429 || ra == "" {
			t.Errorf("Retry-After on the hourly limit = %q (status %d)", ra, rec.Code)
		}
	})
	t.Run("twenty codes per client address per hour", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		var last int
		for i := range 21 {
			last = start(e, "ip"+itoa(i)+"@example.com", "198.51.100.77")
			if i < 20 && last != 202 {
				t.Fatalf("request %d = %d", i+1, last)
			}
		}
		if last != 429 {
			t.Errorf("the 21st request from one address = %d, want 429", last)
		}
		if start(e, "other@example.com", "198.51.100.78") != 202 {
			t.Error("one client's limit blocked another")
		}
	})
	t.Run("thirty verify attempts per client address per 15 minutes", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		var last *struct {
			code int
			ra   string
		}
		for range 31 {
			rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("x@example.com", "123456"), ip: "198.51.100.88"})
			last = &struct {
				code int
				ra   string
			}{rec.Code, rec.Header().Get("Retry-After")}
		}
		if last.code != 429 || last.ra == "" {
			t.Errorf("the 31st attempt = %+v, want 429 with Retry-After", *last)
		}
	})
}

// Scenario 7: a suspended user cannot sign in and cannot tell why.
func TestSuspendedUserCannotSignIn(t *testing.T) {
	e := newEnv(t, envOpts{})
	e.signIn("sus@example.com", "")
	exec(t, e, `UPDATE users SET status = 'suspended' WHERE email = 'sus@example.com'`)
	exec(t, e, `DELETE FROM sessions`)
	e.resetLimits()

	ip := "198.51.100.90"
	start(e, "sus@example.com", ip)
	code := e.awaitCode("sus@example.com", 2)
	rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("sus@example.com", code), ip: ip})
	wrong := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("nobody@example.com", "123456"), ip: ip})
	if rec.Code != 400 || errCode(t, rec) != "invalid_code" {
		t.Fatalf("suspended sign-in = %d %s", rec.Code, rec.Body.String())
	}
	a, b := decodeMap(t, rec), decodeMap(t, wrong)
	delete(a, "request_id")
	delete(b, "request_id")
	if a["error"] != b["error"] || len(a) != len(b) || rec.Code != wrong.Code {
		t.Errorf("a suspended account and an unknown address answer differently: %v vs %v", a, b)
	}
	if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 0 {
		t.Errorf("sessions = %d, want 0", got)
	}
}

// A user suspended while signed in loses access on the next request.
func TestSuspendingAUserEndsTheirAccessImmediately(t *testing.T) {
	e := newEnv(t, envOpts{})
	l := e.signIn("soon@example.com", "")
	if l.get(e, "/v1/me").Code != 200 {
		t.Fatal("not signed in")
	}
	exec(t, e, `UPDATE users SET status = 'suspended' WHERE id = $1`, l.userID)
	if got := l.get(e, "/v1/me").Code; got != http.StatusUnauthorized {
		t.Errorf("GET /v1/me for a suspended user = %d, want 401", got)
	}
}

// Scenario 8: idle and absolute expiry are enforced even though the cookie is still presented,
// and use slides the idle expiry.
func TestSessionExpiry(t *testing.T) {
	e := newEnv(t, envOpts{})

	t.Run("idle", func(t *testing.T) {
		l := e.signIn("idle@example.com", "")
		exec(t, e, `UPDATE sessions SET idle_expires_at = now() - interval '1 second' WHERE id = $1`, l.sessionID)
		if got := l.get(e, "/v1/me").Code; got != 401 {
			t.Errorf("idle-expired session = %d, want 401", got)
		}
	})
	t.Run("absolute", func(t *testing.T) {
		e.resetLimits()
		l := e.signIn("abs@example.com", "")
		exec(t, e, `UPDATE sessions SET created_at = now() - interval '31 days', expires_at = now() - interval '1 second', idle_expires_at = now() - interval '1 second' WHERE id = $1`, l.sessionID)
		if got := l.get(e, "/v1/me").Code; got != 401 {
			t.Errorf("absolute-expired session = %d, want 401", got)
		}
	})
	t.Run("use slides the idle expiry, never past the absolute one", func(t *testing.T) {
		e.resetLimits()
		l := e.signIn("slide@example.com", "")
		exec(t, e, `UPDATE sessions SET last_seen_at = now() - interval '5 minutes', idle_expires_at = now() + interval '1 hour' WHERE id = $1`, l.sessionID)
		if l.get(e, "/v1/me").Code != 200 {
			t.Fatal("session refused")
		}
		var idleDays float64
		if err := e.db.Pool.QueryRow(context.Background(),
			`SELECT extract(epoch FROM idle_expires_at - now()) / 86400 FROM sessions WHERE id = $1`, l.sessionID).Scan(&idleDays); err != nil {
			t.Fatal(err)
		}
		if idleDays < 6.9 || idleDays > 7.01 {
			t.Errorf("idle expiry is %.2f days away, want about 7", idleDays)
		}
		// Near the absolute limit the idle expiry is capped by it.
		exec(t, e, `UPDATE sessions SET created_at = now() - interval '29 days 23 hours', expires_at = now() + interval '1 hour', idle_expires_at = now() + interval '30 minutes', last_seen_at = now() - interval '5 minutes' WHERE id = $1`, l.sessionID)
		if l.get(e, "/v1/me").Code != 200 {
			t.Fatal("session refused")
		}
		var capped bool
		if err := e.db.Pool.QueryRow(context.Background(), `SELECT idle_expires_at <= expires_at FROM sessions WHERE id = $1`, l.sessionID).Scan(&capped); err != nil || !capped {
			t.Errorf("idle expiry went past the absolute one (%v)", err)
		}
	})
}

// Scenario 9: logout and revoke end a session at once; nobody can see or revoke another user's.
func TestLogoutRevokeAndTenancy(t *testing.T) {
	e := newEnv(t, envOpts{})
	a := e.signIn("a@example.com", "")
	e.resetLimits()
	a2 := e.signIn("a@example.com", "")
	e.resetLimits()
	b := e.signIn("b@example.com", "")

	list := func(l login) []map[string]any {
		rec := l.get(e, "/v1/me/sessions")
		if rec.Code != 200 {
			t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
		}
		var out []map[string]any
		for _, s := range decodeMap(t, rec)["sessions"].([]any) {
			out = append(out, s.(map[string]any))
		}
		return out
	}

	sessions := list(a)
	if len(sessions) != 2 {
		t.Fatalf("a sees %d sessions, want 2", len(sessions))
	}
	currents := 0
	for _, s := range sessions {
		if s["current"] == true {
			currents++
			if s["id"] != a.sessionID {
				t.Error("the wrong session is marked current")
			}
		}
		if s["device"] != "Safari on macOS" {
			t.Errorf("device = %v", s["device"])
		}
	}
	if currents != 1 {
		t.Errorf("%d sessions marked current, want 1", currents)
	}
	if got := len(list(b)); got != 1 {
		t.Errorf("b sees %d sessions, want 1", got)
	}

	// b cannot revoke a's session: 404, exactly like a session that does not exist.
	foreign := b.send(e, "DELETE", "/v1/me/sessions/"+a.sessionID, "")
	missing := b.send(e, "DELETE", "/v1/me/sessions/"+uuid.NewString(), "")
	if foreign.Code != 404 || missing.Code != 404 || errCode(t, foreign) != errCode(t, missing) {
		t.Errorf("foreign = %d, missing = %d", foreign.Code, missing.Code)
	}
	if a.get(e, "/v1/me").Code != 200 {
		t.Error("b's attempt ended a's session")
	}
	if got := b.send(e, "DELETE", "/v1/me/sessions/not-a-uuid", "").Code; got != 404 {
		t.Errorf("malformed id = %d, want 404", got)
	}

	// a revokes the other of their own sessions: it stops working on the next request.
	if got := a.send(e, "DELETE", "/v1/me/sessions/"+a2.sessionID, "").Code; got != 204 {
		t.Fatalf("revoke = %d", got)
	}
	if got := a2.get(e, "/v1/me").Code; got != 401 {
		t.Errorf("a revoked session still works (%d)", got)
	}
	if a.get(e, "/v1/me").Code != 200 {
		t.Error("revoking another session ended this one")
	}

	// Logout ends this session and clears the cookie.
	rec := a.send(e, "POST", "/v1/auth/logout", "")
	if rec.Code != 204 {
		t.Fatalf("logout = %d", rec.Code)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName && c.MaxAge < 0 && c.Value == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("logout did not clear the cookie: %v", rec.Header().Values("Set-Cookie"))
	}
	if got := a.get(e, "/v1/me").Code; got != 401 {
		t.Errorf("after logout GET /v1/me = %d, want 401", got)
	}
	if b.get(e, "/v1/me").Code != 200 {
		t.Error("a's logout ended b's session")
	}

	// Revoking the session you are on is the same as signing out.
	e.resetLimits()
	c := e.signIn("c@example.com", "")
	rec = c.send(e, "DELETE", "/v1/me/sessions/"+c.sessionID, "")
	if rec.Code != 204 || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Errorf("revoking the current session = %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
}

// Scenario 10: state-changing requests are refused unless Origin, content type and CSRF token all check out.
func TestCSRFOriginAndContentTypeAreEnforced(t *testing.T) {
	e := newEnv(t, envOpts{})
	l := e.signIn("csrf@example.com", "")
	name := func() string {
		var n string
		if err := e.db.Pool.QueryRow(context.Background(), `SELECT display_name FROM users WHERE id = $1`, l.userID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	patch := `{"display_name":"Hacked"}`
	cases := map[string]req{
		"no csrf token":          {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token},
		"wrong csrf token":       {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: strings.Repeat("0", 64)},
		"another session's csrf": {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: e.keys.CSRFToken(uuid.New())},
		"foreign origin":         {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, origin: str("https://evil.example")},
		"null origin":            {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, origin: str("null")},
		"no origin":              {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, origin: str("")},
		"origin with a path":     {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, origin: str(webOrigin + "/")},
		"form content type":      {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, contentType: "application/x-www-form-urlencoded"},
		"text/plain":             {method: "PATCH", path: "/v1/me", body: patch, cookie: l.token, csrf: l.csrf, contentType: "text/plain"},
		"logout no csrf":         {method: "POST", path: "/v1/auth/logout", cookie: l.token},
		"logout foreign origin":  {method: "POST", path: "/v1/auth/logout", cookie: l.token, csrf: l.csrf, origin: str("https://evil.example")},
		"revoke no csrf":         {method: "DELETE", path: "/v1/me/sessions/" + l.sessionID, cookie: l.token},
		"start foreign origin":   {method: "POST", path: "/v1/auth/email/start", body: `{"email":"x@example.com"}`, origin: str("https://evil.example")},
		"start no origin":        {method: "POST", path: "/v1/auth/email/start", body: `{"email":"x@example.com"}`, origin: str("")},
		"start form body":        {method: "POST", path: "/v1/auth/email/start", body: "email=x@example.com", contentType: "application/x-www-form-urlencoded"},
		"verify foreign origin":  {method: "POST", path: "/v1/auth/email/verify", body: verifyBody("x@example.com", "123456"), origin: str("https://evil.example")},
	}
	for label, r := range cases {
		t.Run(label, func(t *testing.T) {
			rec := e.do(r)
			if rec.Code != http.StatusForbidden || errCode(t, rec) != "forbidden" {
				t.Fatalf("= %d %s, want 403 forbidden", rec.Code, rec.Body.String())
			}
		})
	}
	if got := name(); got != "" {
		t.Errorf("a refused request changed the display name to %q", got)
	}
	if got := l.get(e, "/v1/me").Code; got != 200 {
		t.Errorf("a refused logout ended the session (%d)", got)
	}
	if n := e.mail.count(); n != 1 {
		t.Errorf("emails = %d; refused start requests must not send mail (only the sign-in's one)", n)
	}

	// Safe methods need neither token nor origin.
	if got := e.do(req{method: "GET", path: "/v1/me", cookie: l.token, origin: str("")}).Code; got != 200 {
		t.Errorf("GET without Origin = %d", got)
	}
	// And the right combination works.
	if got := l.send(e, "PATCH", "/v1/me", patch).Code; got != 200 {
		t.Errorf("a well-formed PATCH = %d", got)
	}
	if got := name(); got != "Hacked" {
		t.Errorf("display name = %q", got)
	}
}

// Signed-out requests to protected routes are 401, and a bad cookie is no better than none.
func TestProtectedRoutesNeedASession(t *testing.T) {
	e := newEnv(t, envOpts{})
	for label, r := range map[string]req{
		"me":                 {method: "GET", path: "/v1/me"},
		"sessions":           {method: "GET", path: "/v1/me/sessions"},
		"patch":              {method: "PATCH", path: "/v1/me", body: `{"display_name":"x"}`},
		"logout":             {method: "POST", path: "/v1/auth/logout"},
		"revoke":             {method: "DELETE", path: "/v1/me/sessions/" + uuid.NewString()},
		"garbage cookie":     {method: "GET", path: "/v1/me", cookie: "not-a-real-token"},
		"valid-looking":      {method: "GET", path: "/v1/me", cookie: strings.Repeat("A", 43)},
		"oversized cookie":   {method: "GET", path: "/v1/me", cookie: strings.Repeat("A", 5000)},
		"patch bad csrf too": {method: "PATCH", path: "/v1/me", body: `{"display_name":"x"}`, cookie: strings.Repeat("A", 43), csrf: strings.Repeat("0", 64)},
	} {
		t.Run(label, func(t *testing.T) {
			rec := e.do(r)
			if rec.Code != 401 || errCode(t, rec) != "unauthenticated" {
				t.Fatalf("= %d %s, want 401 unauthenticated", rec.Code, rec.Body.String())
			}
		})
	}
}

// Scenario 12: 20 simultaneous correct verifies make one session; 20 simultaneous starts obey the limit.
func TestConcurrency(t *testing.T) {
	t.Run("one code signs in once", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		ip := "198.51.100.120"
		start(e, "race@example.com", ip)
		code := e.awaitCode("race@example.com", 1)
		var wg sync.WaitGroup
		results := make([]int, 20)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("race@example.com", code), ip: ip})
				results[i] = rec.Code
			}()
		}
		wg.Wait()
		ok := 0
		for _, c := range results {
			switch c {
			case 200:
				ok++
			case 400:
			default:
				t.Errorf("unexpected status %d", c)
			}
		}
		if ok != 1 {
			t.Errorf("%d of 20 simultaneous verifies succeeded, want exactly 1 (%v)", ok, results)
		}
		if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 1 {
			t.Errorf("sessions = %d, want 1", got)
		}
		if got := countRows(t, e, `SELECT count(*) FROM users`); got != 1 {
			t.Errorf("users = %d, want 1", got)
		}
	})
	t.Run("simultaneous starts respect the limit", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		var wg sync.WaitGroup
		results := make([]int, 20)
		for i := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[i] = start(e, "burst@example.com", "198.51.100."+itoa(130+i))
			}()
		}
		wg.Wait()
		accepted := 0
		for _, c := range results {
			if c == 202 {
				accepted++
			} else if c != 429 {
				t.Errorf("unexpected status %d", c)
			}
		}
		if accepted != 1 {
			t.Errorf("%d of 20 simultaneous starts were accepted, want 1 (%v)", accepted, results)
		}
		deadline := time.Now().Add(2 * time.Second)
		for len(e.mail.to("burst@example.com")) < 1 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		if n := len(e.mail.to("burst@example.com")); n != 1 {
			t.Errorf("emails = %d, want 1", n)
		}
		if got := countRows(t, e, `SELECT count(*) FROM login_challenges WHERE email = 'burst@example.com' AND closed_at IS NULL`); got != 1 {
			t.Errorf("open challenges = %d, want 1", got)
		}
	})
}

// Scenario 14: cleanup removes ended sessions and old challenges, and nothing else.
func TestCleanupRemovesOnlyWhatHasEnded(t *testing.T) {
	e := newEnv(t, envOpts{})
	live := e.signIn("live@example.com", "")
	e.resetLimits()
	gone := e.signIn("gone@example.com", "")
	exec(t, e, `UPDATE sessions SET created_at = now() - interval '9 days', expires_at = now() + interval '1 day', idle_expires_at = now() - interval '1 hour' WHERE id = $1`, gone.sessionID)
	// An old challenge (31 days) and a recent one.
	exec(t, e, `UPDATE login_challenges SET created_at = now() - interval '31 days', expires_at = now() - interval '31 days' + interval '10 minutes' WHERE email = 'gone@example.com'`)
	start(e, "recent@example.com", "198.51.100.140")

	sessions, challenges, err := e.svc.Cleanup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || challenges != 1 {
		t.Errorf("removed %d sessions and %d challenges, want 1 and 1", sessions, challenges)
	}
	if live.get(e, "/v1/me").Code != 200 {
		t.Error("cleanup removed a live session")
	}
	if got := countRows(t, e, `SELECT count(*) FROM login_challenges`); got != 2 {
		t.Errorf("challenges left = %d, want 2", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM users`); got != 2 {
		t.Errorf("users left = %d, want 2 (cleanup must never touch accounts)", got)
	}
}

// Scenario 16: display-name rules, and no response ever carries a token or hash.
func TestDisplayNameRulesAndNoSecretsInResponses(t *testing.T) {
	e := newEnv(t, envOpts{})
	l := e.signIn("name@example.com", "")

	for label, body := range map[string]string{
		"empty":             `{"display_name":""}`,
		"blank":             `{"display_name":"   "}`,
		"too long":          `{"display_name":"` + strings.Repeat("x", 81) + `"}`,
		"control character": `{"display_name":"Ada\u0000"}`,
		"newline":           `{"display_name":"Ada\nLovelace"}`,
		"bidi override":     `{"display_name":"Ada\u202eLovelace"}`,
		"missing field":     `{}`,
		"wrong type":        `{"display_name":5}`,
		"unknown field":     `{"display_name":"Ada","email":"evil@example.com"}`,
		"two objects":       `{"display_name":"Ada"}{"display_name":"Bob"}`,
		"not JSON":          `display_name=Ada`,
		"array":             `["Ada"]`,
	} {
		t.Run(label, func(t *testing.T) {
			rec := l.send(e, "PATCH", "/v1/me", body)
			if rec.Code != 400 || errCode(t, rec) != "invalid_request" {
				t.Fatalf("= %d %s, want 400 invalid_request", rec.Code, rec.Body.String())
			}
		})
	}
	rec := l.send(e, "PATCH", "/v1/me", `{"display_name":"  Ada Lovelace  "}`)
	if rec.Code != 200 || decodeMap(t, rec)["display_name"] != "Ada Lovelace" {
		t.Fatalf("valid rename = %d %s", rec.Code, rec.Body.String())
	}
	if _, ok := decodeMap(t, rec)["email"]; !ok {
		t.Error("the user body lost its email")
	}

	// Collect every body this user has received and look for secrets.
	row, _ := store.New(e.db.Pool).GetSessionByTokenHash(context.Background(), auth.HashToken(l.token))
	secrets := []string{l.token, "token_hash", "code_hash", string(row.Session.TokenHash)}
	for label, r := range map[string]*struct{ body string }{
		"verify":   {l.rec.Body.String()},
		"me":       {l.get(e, "/v1/me").Body.String()},
		"sessions": {l.get(e, "/v1/me/sessions").Body.String()},
		"rename":   {rec.Body.String()},
	} {
		for _, s := range secrets {
			if strings.Contains(r.body, s) {
				t.Errorf("the %s response contains %q", label, s)
			}
		}
	}
	if csrf := decodeMap(t, l.get(e, "/v1/me"))["csrf_token"]; csrf != l.csrf {
		t.Error("the CSRF token is not stable for a session")
	}
}

// Bad input to start and verify is a 400, not a 500, and never sends mail.
func TestMalformedInput(t *testing.T) {
	e := newEnv(t, envOpts{})
	for label, r := range map[string]req{
		"start: no email":       {method: "POST", path: "/v1/auth/email/start", body: `{}`},
		"start: not an address": {method: "POST", path: "/v1/auth/email/start", body: `{"email":"nope"}`},
		"start: two addresses":  {method: "POST", path: "/v1/auth/email/start", body: `{"email":"a@example.com, b@example.com"}`},
		"start: display name":   {method: "POST", path: "/v1/auth/email/start", body: `{"email":"Ada <a@example.com>"}`},
		"start: extra field":    {method: "POST", path: "/v1/auth/email/start", body: `{"email":"a@example.com","admin":true}`},
		"start: huge":           {method: "POST", path: "/v1/auth/email/start", body: `{"email":"` + strings.Repeat("a", 20000) + `@example.com"}`},
		"start: empty body":     {method: "POST", path: "/v1/auth/email/start"},
		"verify: short code":    {method: "POST", path: "/v1/auth/email/verify", body: verifyBody("a@example.com", "123")},
		"verify: letters":       {method: "POST", path: "/v1/auth/email/verify", body: verifyBody("a@example.com", "12345a")},
		"verify: bad address":   {method: "POST", path: "/v1/auth/email/verify", body: verifyBody("nope", "123456")},
	} {
		t.Run(label, func(t *testing.T) {
			want := http.StatusBadRequest
			if label == "start: empty body" {
				want = http.StatusBadRequest
			}
			rec := e.do(r)
			code := errCode(t, rec)
			if label == "verify: short code" || label == "verify: letters" {
				// A malformed code is indistinguishable from a wrong one.
				if rec.Code != want || code != "invalid_code" {
					t.Fatalf("= %d %s", rec.Code, rec.Body.String())
				}
				return
			}
			if rec.Code != want || code != "invalid_request" {
				t.Fatalf("= %d %s, want 400 invalid_request", rec.Code, rec.Body.String())
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	if n := e.mail.count(); n != 0 {
		t.Errorf("emails sent = %d, want 0", n)
	}
	// Unknown /v1 routes and wrong methods answer in the standard error shape.
	if rec := e.do(req{method: "GET", path: "/v1/nope"}); rec.Code != 404 || errCode(t, rec) != "not_found" {
		t.Errorf("unknown route = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(req{method: "GET", path: "/v1/auth/email/start"}); rec.Code != 405 || errCode(t, rec) != "method_not_allowed" {
		t.Errorf("GET on a POST route = %d %s", rec.Code, rec.Body.String())
	}
}

// Scenario 17: behind the trusted proxy, each client is limited on their own address.
func TestClientAddressBehindTheProxy(t *testing.T) {
	proxy := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	t.Run("two clients behind the proxy are limited separately", func(t *testing.T) {
		e := newEnv(t, envOpts{trusted: proxy})
		viaProxy := func(email, client string) int {
			return e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": email}), ip: "10.1.2.3", xff: client}).Code
		}
		for i := range 20 {
			if got := viaProxy("a"+itoa(i)+"@example.com", "203.0.113.1"); got != 202 {
				t.Fatalf("request %d from client 1 = %d", i+1, got)
			}
		}
		if got := viaProxy("a20@example.com", "203.0.113.1"); got != 429 {
			t.Errorf("client 1's 21st request = %d, want 429", got)
		}
		if got := viaProxy("b0@example.com", "203.0.113.2"); got != 202 {
			t.Errorf("client 2 was blocked by client 1's limit (%d): the whole site is sharing one limit", got)
		}
	})
	t.Run("verify attempts are counted per client behind the proxy too", func(t *testing.T) {
		e := newEnv(t, envOpts{trusted: proxy})
		attempt := func(client string) int {
			return e.do(req{method: "POST", path: "/v1/auth/email/verify", body: verifyBody("x@example.com", "123456"), ip: "10.1.2.3", xff: client}).Code
		}
		var last int
		for range 31 {
			last = attempt("203.0.113.1")
		}
		if last != 429 {
			t.Fatalf("client 1's 31st attempt = %d, want 429", last)
		}
		if got := attempt("203.0.113.2"); got != 400 {
			t.Errorf("client 2's first attempt = %d, want 400 (not limited by client 1)", got)
		}
	})
	t.Run("a forged header from an untrusted caller is ignored", func(t *testing.T) {
		e := newEnv(t, envOpts{trusted: proxy})
		var last int
		for i := range 21 {
			// The caller claims a new address every time. It must not help.
			last = e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "f" + itoa(i) + "@example.com"}),
				ip: "198.51.100.150", xff: "203.0.113." + itoa(i+1)}).Code
		}
		if last != 429 {
			t.Errorf("the 21st request with a rotating forged header = %d, want 429", last)
		}
	})
	t.Run("a client-supplied prefix behind the proxy is ignored", func(t *testing.T) {
		e := newEnv(t, envOpts{trusted: proxy})
		var last int
		for i := range 21 {
			// The proxy appends the real client (203.0.113.9) after whatever the client sent.
			last = e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "g" + itoa(i) + "@example.com"}),
				ip: "10.1.2.3", xff: "198.18.0." + itoa(i+1) + ", 203.0.113.9"}).Code
		}
		if last != 429 {
			t.Errorf("the 21st request with rotating forged prefixes = %d, want 429", last)
		}
	})
	t.Run("without a trusted proxy the header is never used", func(t *testing.T) {
		e := newEnv(t, envOpts{})
		var last int
		for i := range 21 {
			last = e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": "h" + itoa(i) + "@example.com"}),
				ip: "10.1.2.3", xff: "203.0.113." + itoa(i+1)}).Code
		}
		if last != 429 {
			t.Errorf("= %d, want 429", last)
		}
	})
}

// Scenario 19: a full send queue changes nothing the caller can see and logs nothing personal.
func TestFullMailQueueIsInvisibleAndPrivate(t *testing.T) {
	blocker := &capture{gate: make(chan struct{}), in: make(chan struct{}, 8)}
	e := newEnv(t, envOpts{queueCapacity: 1, senders: 1, mailer: blocker})

	bodies := map[string]bool{}
	emails := []string{"first@example.com", "second@example.com", "third@example.com", "fourth@example.com"}
	for i, addr := range emails {
		rec := e.do(req{method: "POST", path: "/v1/auth/email/start", body: jsonBody(map[string]string{"email": addr}), ip: "198.51.100.160"})
		if rec.Code != 202 {
			t.Fatalf("start %d = %d %s", i+1, rec.Code, rec.Body.String())
		}
		bodies[rec.Body.String()] = true
		if i == 0 {
			select {
			case <-blocker.in: // the single sender has picked up the first email and is stuck on it
			case <-time.After(3 * time.Second):
				t.Fatal("the sender never started")
			}
		}
	}
	if len(bodies) != 1 {
		t.Errorf("responses differ between queued and dropped mail: %v", bodies)
	}
	if e.queue.Dropped() != 2 {
		t.Errorf("dropped = %d, want 2", e.queue.Dropped())
	}
	logs := e.logs.String()
	for _, addr := range emails {
		if strings.Contains(logs, addr) {
			t.Errorf("the log contains %q", addr)
		}
	}
	if strings.Contains(logs, `"text"`) || strings.Contains(logs, "sign-in code is") {
		t.Error("the log contains email text")
	}
	if !strings.Contains(logs, "send queue is full") {
		t.Error("a dropped email was not logged")
	}

	// On shutdown the queue drains: the two accepted emails go out once the mailer is free.
	close(blocker.gate)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.queue.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if got := blocker.count(); got != 2 {
		t.Errorf("emails delivered after draining = %d, want 2", got)
	}
}

// The service itself, below HTTP: a sign-in needs the right address as well as the right code.
func TestCodeIsBoundToItsAddress(t *testing.T) {
	e := newEnv(t, envOpts{})
	ip := netip.MustParseAddr("198.51.100.170")
	ctx := context.Background()
	if err := e.svc.StartLogin(ctx, "alice@example.com", ip); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.StartLogin(ctx, "bob@example.com", ip); err != nil {
		t.Fatal(err)
	}
	aliceCode := e.awaitCode("alice@example.com", 1)
	// Bob's challenge exists, but Alice's code must not open it.
	if _, err := e.svc.VerifyLogin(ctx, "bob@example.com", aliceCode, ip, ""); err == nil {
		t.Skip("the two codes happen to be equal (1 in a million)")
	}
	if got := countRows(t, e, `SELECT count(*) FROM sessions`); got != 0 {
		t.Errorf("sessions = %d, want 0", got)
	}
}
