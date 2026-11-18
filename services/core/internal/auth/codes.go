// Package auth implements passwordless email sign-in and sessions
// (plans/phase-2a-email-login.md): one-time codes, session cookies, CSRF tokens.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	codeDigits   = 6
	maxEmailLen  = 254
	maxNameRunes = 80
	tokenBytes   = 32
)

// ErrInvalidEmail is returned for an address that cannot be a mailbox.
var ErrInvalidEmail = errors.New("invalid email address")

// NormalizeEmail trims and lower-cases an address and rejects anything that is not a plain
// single mailbox. "+tag" parts are kept as given: merging them would merge different mailboxes
// on some providers.
//
// Addresses are ASCII only for now. Non-ASCII mailboxes are rare, and accepting them means
// Go's and Postgres's idea of "the same address in lower case" must agree for every script,
// which is exactly the kind of gap account-takeover bugs hide in.
func NormalizeEmail(raw string) (string, error) {
	e := strings.TrimSpace(raw)
	if e == "" || len(e) > maxEmailLen {
		return "", ErrInvalidEmail
	}
	for i := range len(e) {
		c := e[i]
		if c <= ' ' || c >= 0x7f || c == '<' || c == '>' || c == '"' || c == ',' || c == ';' {
			return "", ErrInvalidEmail
		}
	}
	e = strings.ToLower(e)
	at := strings.IndexByte(e, '@')
	if at < 1 || at != strings.LastIndexByte(e, '@') || at > 64 {
		return "", ErrInvalidEmail
	}
	domain := e[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", ErrInvalidEmail
	}
	// The standard library has the final say, and must read it back unchanged (no display name).
	parsed, err := mail.ParseAddress(e)
	if err != nil || parsed.Address != e || parsed.Name != "" {
		return "", ErrInvalidEmail
	}
	return e, nil
}

// NewCode returns a uniformly random six-digit code, leading zeros kept.
func NewCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate code: %w", err)
	}
	return fmt.Sprintf("%0*d", codeDigits, n.Int64()), nil
}

// ValidCodeFormat reports whether s is exactly six ASCII digits.
func ValidCodeFormat(s string) bool {
	if len(s) != codeDigits {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// Keys derives every secret-dependent value from one key, each under its own label so a value
// made for one purpose can never stand in for another.
type Keys struct{ key []byte }

// NewKeys wraps the server secret (AUTH_CODE_KEY).
func NewKeys(secret string) Keys { return Keys{key: []byte(secret)} }

func (k Keys) mac(label string, parts ...string) []byte {
	m := hmac.New(sha256.New, k.key)
	m.Write([]byte(label))
	for _, p := range parts {
		m.Write([]byte{0})
		m.Write([]byte(p))
	}
	return m.Sum(nil)
}

// CodeHash is what is stored for a code. A six-digit code is far too small to protect with a
// bare hash, so it is keyed, and bound to the address it was issued for.
func (k Keys) CodeHash(email, code string) []byte { return k.mac("login-code", email, code) }

// CSRFToken is the per-session token the page sends as X-CSRF-Token.
func (k Keys) CSRFToken(sessionID uuid.UUID) string {
	return hex.EncodeToString(k.mac("csrf", sessionID.String()))
}

// CSRFMatches compares a presented token with the one for the session, in constant time.
func (k Keys) CSRFMatches(sessionID uuid.UUID, presented string) bool {
	want := k.CSRFToken(sessionID)
	return len(presented) == len(want) && hmac.Equal([]byte(presented), []byte(want))
}

// NewSessionToken returns a 256-bit random cookie value and the hash that is stored.
func NewSessionToken() (token string, hash []byte, err error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken is the stored form of a session token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// ErrInvalidName is returned for a display name that breaks the rules.
var ErrInvalidName = errors.New("invalid display name")

// CleanDisplayName trims a display name and enforces 1 to 80 characters with no control,
// format (zero-width, bidirectional override) or line-separator characters.
func CleanDisplayName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxNameRunes {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// DeviceLabel makes a short "Browser on OS" label from a user agent. It is a convenience for
// recognising your own sessions, not an identification: the header is whatever the client sent.
func DeviceLabel(ua string) string {
	if ua == "" {
		return ""
	}
	browser := "Browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"), strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"), strings.Contains(ua, "CriOS/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	case strings.Contains(ua, "curl/"):
		browser = "curl"
	}
	system := ""
	switch {
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		system = "iOS"
	case strings.Contains(ua, "Android"):
		system = "Android"
	case strings.Contains(ua, "Windows"):
		system = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		system = "macOS"
	case strings.Contains(ua, "CrOS"):
		system = "ChromeOS"
	case strings.Contains(ua, "Linux"), strings.Contains(ua, "X11"):
		system = "Linux"
	}
	if system == "" {
		return browser
	}
	return browser + " on " + system
}
