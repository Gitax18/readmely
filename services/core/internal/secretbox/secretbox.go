// Package secretbox seals small secrets (a provider token) for storage. AES-256-GCM, a fresh
// random nonce per seal, and the owner's id bound in as additional data: a sealed value copied to
// another owner's row does not open.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
)

// KeyBytes is the key length: AES-256.
const KeyBytes = 32

// ErrOpen is returned for every way opening can fail (wrong key, wrong owner, altered or truncated
// data). One error, so nothing is learned from which.
var ErrOpen = errors.New("secretbox: cannot open")

// Box seals and opens values with one key.
type Box struct{ aead cipher.AEAD }

// New builds a Box from a 32-byte key.
func New(key []byte) (*Box, error) {
	if len(key) != KeyBytes {
		return nil, fmt.Errorf("secretbox: key must be %d bytes", KeyBytes)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// NewFromHex builds a Box from a 64-character hex key (the form kept in the environment).
func NewFromHex(key string) (*Box, error) {
	b, err := hex.DecodeString(key)
	if err != nil {
		return nil, errors.New("secretbox: key must be hex")
	}
	return New(b)
}

// Seal returns nonce || ciphertext, bound to owner.
func (b *Box) Seal(plaintext, owner []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plaintext, owner), nil
}

// Open reverses Seal for the same owner.
func (b *Box) Open(sealed, owner []byte) ([]byte, error) {
	n := b.aead.NonceSize()
	if len(sealed) < n+b.aead.Overhead() {
		return nil, ErrOpen
	}
	out, err := b.aead.Open(nil, sealed[:n], sealed[n:], owner)
	if err != nil {
		return nil, ErrOpen
	}
	return out, nil
}
