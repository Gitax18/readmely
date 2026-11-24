// Package githubaccess keeps a person's GitHub token, so Readmely can list and read their private
// repositories. The token is sealed before it is stored, opened only to make a GitHub request on
// that person's behalf, and never logged or sent anywhere else.
package githubaccess

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/secretbox"
)

// Scope is what is asked for: the whole `repo` scope is the only way an OAuth App can read private
// repositories. Readmely only ever sends GET requests with it.
const Scope = "repo"

// ErrNone means the person has not granted access (or it was taken away).
var ErrNone = errors.New("github access not granted")

// Store reads and writes the sealed tokens.
type Store struct {
	q   *store.Queries
	box *secretbox.Box
}

// New wires the store. A nil box is not allowed: build the store only when the key is set.
func New(db store.DBTX, box *secretbox.Box) *Store {
	return &Store{q: store.New(db), box: box}
}

func owner(userID uuid.UUID) []byte { return []byte(userID.String()) }

// Save seals and stores the token, replacing an earlier one.
func (s *Store) Save(ctx context.Context, userID uuid.UUID, token, scope string) error {
	sealed, err := s.box.Seal([]byte(token), owner(userID))
	if err != nil {
		return err
	}
	if err := s.q.UpsertGitHubAccess(ctx, store.UpsertGitHubAccessParams{UserID: userID, Sealed: sealed, Scope: scope}); err != nil {
		return fmt.Errorf("save github access: %w", err)
	}
	return nil
}

// Token returns the person's token, or ErrNone.
func (s *Store) Token(ctx context.Context, userID uuid.UUID) (string, error) {
	row, err := s.q.GetGitHubAccess(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNone
	}
	if err != nil {
		return "", fmt.Errorf("read github access: %w", err)
	}
	t, err := s.box.Open(row.Sealed, owner(userID))
	if err != nil {
		// A key that changed or a damaged row: the access is as good as gone.
		return "", ErrNone
	}
	return string(t), nil
}

// Has reports whether the person has granted access that still opens.
func (s *Store) Has(ctx context.Context, userID uuid.UUID) (bool, error) {
	_, err := s.Token(ctx, userID)
	if errors.Is(err, ErrNone) {
		return false, nil
	}
	return err == nil, err
}

// Remove deletes the token. Removing what is not there is not an error.
func (s *Store) Remove(ctx context.Context, userID uuid.UUID) error {
	if _, err := s.q.DeleteGitHubAccess(ctx, userID); err != nil {
		return fmt.Errorf("remove github access: %w", err)
	}
	return nil
}
