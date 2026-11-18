package auth

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()
	good := map[string]string{
		"ada@example.com":        "ada@example.com",
		"  ADA@Example.COM \t":   "ada@example.com",
		"ada+news@example.com":   "ada+news@example.com", // sub-addresses are kept, never merged
		"a.b-c_d@sub.example.io": "a.b-c_d@sub.example.io",
	}
	for in, want := range good {
		got, err := NormalizeEmail(in)
		if err != nil || got != want {
			t.Errorf("NormalizeEmail(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "   ", "ada", "ada@", "@example.com", "ada@@example.com", "a@b@example.com",
		"ada@example", "ada@.example.com", "ada@example.com.", "ada@exa..mple.com",
		"ada @example.com", "ada@exa mple.com", "ada@exämple.com", "ada\x00@example.com",
		"Ada <ada@example.com>", "<ada@example.com>", `"ada"@example.com`, "ada@example.com, bob@example.com",
		"ada@example.com;bob@example.com", strings.Repeat("a", 65) + "@example.com",
		strings.Repeat("a", 250) + "@example.com", "ada@example.com\xff",
	}
	for _, in := range bad {
		if got, err := NormalizeEmail(in); err == nil {
			t.Errorf("NormalizeEmail(%q) = %q; want an error", in, got)
		}
	}
}

func TestNewCodeIsSixDigitsAndKeepsLeadingZeros(t *testing.T) {
	t.Parallel()
	sawLeadingZero := false
	for range 3000 {
		c, err := NewCode()
		if err != nil || !ValidCodeFormat(c) {
			t.Fatalf("NewCode() = %q, %v", c, err)
		}
		if c[0] == '0' {
			sawLeadingZero = true
		}
	}
	if !sawLeadingZero {
		t.Error("3000 codes and none started with 0: leading zeros are being dropped or the range is wrong")
	}
}

func TestValidCodeFormat(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"000000", "013842", "999999"} {
		if !ValidCodeFormat(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "12345a", " 12345", "12 456", "１２３４５６", "-12345", "1e5000"} {
		if ValidCodeFormat(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestCodeHashIsKeyedAndBoundToTheAddress(t *testing.T) {
	t.Parallel()
	a, b := NewKeys("key-one-key-one-key-one-key-one-0123"), NewKeys("key-two-key-two-key-two-key-two-0123")
	h := a.CodeHash("ada@example.com", "013842")
	if len(h) != 32 {
		t.Fatalf("hash is %d bytes, want 32", len(h))
	}
	if !bytes.Equal(h, a.CodeHash("ada@example.com", "013842")) {
		t.Error("the hash is not deterministic")
	}
	for name, other := range map[string][]byte{
		"another key":     b.CodeHash("ada@example.com", "013842"),
		"another address": a.CodeHash("bob@example.com", "013842"),
		"another code":    a.CodeHash("ada@example.com", "013843"),
	} {
		if bytes.Equal(h, other) {
			t.Errorf("%s gives the same hash", name)
		}
	}
	// "ab"+"c" and "a"+"bc" must not collide: parts are separated.
	if bytes.Equal(a.CodeHash("ab", "c"), a.CodeHash("a", "bc")) {
		t.Error("address and code run together")
	}
}

func TestCSRFTokenIsPerSessionAndPerKey(t *testing.T) {
	t.Parallel()
	a, b := NewKeys("key-one-key-one-key-one-key-one-0123"), NewKeys("key-two-key-two-key-two-key-two-0123")
	s1, s2 := uuid.New(), uuid.New()
	tok := a.CSRFToken(s1)
	if len(tok) != 64 {
		t.Fatalf("token is %d chars, want 64", len(tok))
	}
	if !a.CSRFMatches(s1, tok) {
		t.Error("the token does not match its own session")
	}
	for name, ok := range map[string]bool{
		"another session": a.CSRFMatches(s2, tok),
		"another key":     b.CSRFMatches(s1, tok),
		"empty":           a.CSRFMatches(s1, ""),
		"truncated":       a.CSRFMatches(s1, tok[:63]),
		"upper case":      a.CSRFMatches(s1, strings.ToUpper(tok)),
		"padded":          a.CSRFMatches(s1, tok+"0"),
	} {
		if ok {
			t.Errorf("%s was accepted", name)
		}
	}
	// A code hash must never be usable as a CSRF token, or the other way round.
	if a.CSRFToken(s1) == strings.ToLower(string(a.CodeHash(s1.String(), ""))) {
		t.Error("labels do not separate the uses of the key")
	}
}

func TestSessionTokens(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 200 {
		tok, hash, err := NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 43 { // 32 bytes, unpadded base64url
			t.Fatalf("token is %d chars, want 43", len(tok))
		}
		if seen[tok] {
			t.Fatal("a token repeated")
		}
		seen[tok] = true
		if !bytes.Equal(hash, HashToken(tok)) || len(hash) != 32 {
			t.Fatal("the stored hash is not the hash of the token")
		}
		if bytes.Contains(hash, []byte(tok)) {
			t.Fatal("the hash contains the token")
		}
	}
}

func TestCleanDisplayName(t *testing.T) {
	t.Parallel()
	good := map[string]string{
		"Ada":                       "Ada",
		"  Ada Lovelace  ":          "Ada Lovelace",
		"Ünï Çödé 日本語":              "Ünï Çödé 日本語",
		strings.Repeat("é", 80):     strings.Repeat("é", 80),
		"O'Brien <not html-ish>&co": "O'Brien <not html-ish>&co", // stored as text; the UI escapes on output
	}
	for in, want := range good {
		if got, err := CleanDisplayName(in); err != nil || got != want {
			t.Errorf("CleanDisplayName(%q) = %q, %v", in, got, err)
		}
	}
	bad := []string{
		"", "   ", "\t\n", strings.Repeat("a", 81), strings.Repeat("é", 81),
		"Ada\x00", "Ada\nLovelace", "Ada\u202eLovelace", "Ada\u200bLovelace", "Ada\u2028Lovelace", "Ada\u2029Lovelace", "Ada\x7f", "bad\xffutf8",
	}
	for _, in := range bad {
		if got, err := CleanDisplayName(in); err == nil {
			t.Errorf("CleanDisplayName(%q) = %q; want an error", in, got)
		}
	}
}

func TestDeviceLabel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"": "",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15": "Safari on macOS",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36":           "Chrome on Windows",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 Edg/120.0":                     "Edge on Windows",
		"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0":                                                "Firefox on Linux",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1":   "Safari on iOS",
		"curl/8.4.0":        "curl",
		"something-unknown": "Browser",
	}
	for ua, want := range cases {
		if got := DeviceLabel(ua); got != want {
			t.Errorf("DeviceLabel(%q) = %q, want %q", ua, got, want)
		}
	}
	if got := DeviceLabel(strings.Repeat("x", 10_000) + " Chrome/1 Windows"); len(got) > 100 {
		t.Errorf("label is %d bytes, the contract allows 100", len(got))
	}
}

func TestTruncateUserAgent(t *testing.T) {
	t.Parallel()
	if got := truncateUserAgent(strings.Repeat("é", 500)); len([]rune(got)) != 200 {
		t.Errorf("kept %d characters, want 200", len([]rune(got)))
	}
	if got := truncateUserAgent("a\x00b\r\nc\xffd"); got != "abcd" {
		t.Errorf("got %q, want control characters and bad bytes removed", got)
	}
}
