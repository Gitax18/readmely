package oauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// What the person was doing when they left for the provider.
const (
	IntentLogin = "login"
	IntentLink  = "link"
	// IntentRepos asks the signed-in person's GitHub for permission to read their private repositories.
	IntentRepos = "repos"
)

// AttemptTTL is how long a person has to finish at the provider.
const AttemptTTL = 10 * time.Minute

const maxStateLen = 256

// ErrInvalidState covers every way a callback can fail the state check: unknown, used, expired,
// the wrong browser, the wrong provider. One error, so nothing is learned from which.
var ErrInvalidState = errors.New("invalid oauth state")

// Attempt is one trip to a provider, remembered on the server.
type Attempt struct {
	Provider  string `json:"provider"`
	Intent    string `json:"intent"`
	Verifier  string `json:"verifier"`
	UserID    string `json:"user_id,omitempty"`    // link and repos: who is connecting
	SessionID string `json:"session_id,omitempty"` // link and repos: from which session
}

type record struct {
	Attempt
	BindingHash string `json:"binding_hash"`
}

// Store keeps attempts in Redis. The state is the lookup key (hashed), and a second random value,
// the binding, travels only in the browser's cookie: a callback is accepted only from the browser
// that started the attempt, which is what stops an attacker feeding a victim a code of their own.
type Store struct {
	rdb    redis.Cmdable
	prefix string
}

// NewStore returns a store whose keys start with prefix.
func NewStore(rdb redis.Cmdable, prefix string) *Store { return &Store{rdb: rdb, prefix: prefix} }

func (s *Store) key(state string) string {
	sum := sha256.Sum256([]byte(state))
	return s.prefix + "oauth:" + hex.EncodeToString(sum[:])
}

func bindingHash(binding string) string {
	sum := sha256.Sum256([]byte(binding))
	return hex.EncodeToString(sum[:])
}

// Begin stores the attempt and returns the state (sent to the provider) and the binding (sent to
// the browser as a cookie). Only hashes of both are ever kept on the server.
func (s *Store) Begin(ctx context.Context, a Attempt) (state, binding string, err error) {
	if state, err = randomToken(32); err != nil {
		return "", "", err
	}
	if binding, err = randomToken(32); err != nil {
		return "", "", err
	}
	body, err := json.Marshal(record{Attempt: a, BindingHash: bindingHash(binding)})
	if err != nil {
		return "", "", fmt.Errorf("oauth attempt: %w", err)
	}
	if err := s.rdb.Set(ctx, s.key(state), body, AttemptTTL).Err(); err != nil {
		return "", "", fmt.Errorf("oauth attempt: %w", err)
	}
	return state, binding, nil
}

// Take consumes the attempt for a state, exactly once, even when two callbacks race: the read and
// the delete are one Redis command. The attempt is spent even if the binding then turns out to be
// wrong. Redis failures are returned as they are so callers fail closed; every other failure is
// ErrInvalidState.
func (s *Store) Take(ctx context.Context, state, binding string) (*Attempt, error) {
	if state == "" || len(state) > maxStateLen || binding == "" || len(binding) > maxStateLen {
		return nil, ErrInvalidState
	}
	raw, err := s.rdb.GetDel(ctx, s.key(state)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrInvalidState
	}
	if err != nil {
		return nil, fmt.Errorf("oauth attempt: %w", err)
	}
	var r record
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, ErrInvalidState
	}
	if subtle.ConstantTimeCompare([]byte(r.BindingHash), []byte(bindingHash(binding))) != 1 {
		return nil, ErrInvalidState
	}
	return &r.Attempt, nil
}
