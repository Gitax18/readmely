// Package oauth implements the provider side of Google and GitHub sign-in
// (plans/phase-2b-oauth-login.md): PKCE, the single-use attempt store, and the two provider
// clients. It never stores a provider token: the access token lives for one profile request.
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// NewVerifier returns a PKCE code verifier: 256 random bits, base64url (43 characters, which is
// inside RFC 7636's 43 to 128).
func NewVerifier() (string, error) { return randomToken(32) }

// Challenge is the S256 transform of a verifier, sent to the provider at the start.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
