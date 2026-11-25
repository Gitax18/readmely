package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

// Deleting an account is the one action that cannot be undone, so it needs more than a session:
// a fresh code sent to the account's own address. The limits are per user, not per address.
var (
	ruleDeleteStart   = ratelimit.Rule{Name: "delete_start_user", Max: 3, Window: time.Hour}
	ruleDeleteConfirm = ratelimit.Rule{Name: "delete_confirm_user", Max: 10, Window: 15 * time.Minute}
)

// RevokeOtherSessions ends every session of the user except the one making the request.
func (s *Service) RevokeOtherSessions(ctx context.Context, p *Principal) error {
	if _, err := s.q.DeleteOtherUserSessions(ctx, store.DeleteOtherUserSessionsParams{UserID: p.User.ID, KeepID: p.Session.ID}); err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	return nil
}

// StartDeletion emails a code to the account's own address.
func (s *Service) StartDeletion(ctx context.Context, p *Principal) error {
	email, err := NormalizeEmail(p.User.Email)
	if err != nil {
		return fmt.Errorf("start deletion: %w", err)
	}
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: ruleDeleteStart, Subject: p.User.ID.String()})
	if err != nil {
		return fmt.Errorf("start deletion: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	code, err := NewCode()
	if err != nil {
		return err
	}
	if _, err := s.q.CreateLoginChallenge(ctx, store.CreateLoginChallengeParams{
		Email:       email,
		CodeHash:    s.keys.CodeHash(email, code),
		TtlSeconds:  int32(CodeTTL / time.Second),
		MaxAttempts: MaxCodeAttempts,
	}); err != nil {
		return fmt.Errorf("start deletion: %w", err)
	}
	s.mail.Enqueue(s.brand.DeletionCode(email, code, int(CodeTTL/time.Minute)))
	return nil
}

// ConfirmDeletion checks the code and deletes the account. A wrong, expired or used code is
// ErrInvalidCode, the same single answer as at sign-in.
func (s *Service) ConfirmDeletion(ctx context.Context, p *Principal, code string) error {
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: ruleDeleteConfirm, Subject: p.User.ID.String()})
	if err != nil {
		return fmt.Errorf("confirm deletion: %w", err)
	}
	if !d.Allowed {
		return &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	if !ValidCodeFormat(code) {
		return ErrInvalidCode
	}
	email, err := NormalizeEmail(p.User.Email)
	if err != nil {
		return fmt.Errorf("confirm deletion: %w", err)
	}
	attempt, err := s.q.AttemptLoginChallenge(ctx, store.AttemptLoginChallengeParams{Email: email, CodeHash: s.keys.CodeHash(email, code)})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCode
	}
	if err != nil {
		return fmt.Errorf("confirm deletion: %w", err)
	}
	if attempt.ClosedAt == nil {
		return ErrInvalidCode
	}
	n, err := s.q.DeleteUser(ctx, p.User.ID)
	if err != nil {
		return fmt.Errorf("confirm deletion: %w", err)
	}
	if n == 0 {
		return ErrUnauthenticated // already gone
	}
	s.logger.InfoContext(ctx, "account deleted", "user_id", p.User.ID)
	s.mail.Enqueue(s.brand.Notice(email, "Your Readmely account was deleted", "Account deleted", "Your account was deleted",
		fmt.Sprintf("Your Readmely account and everything in it were deleted on %s. Nothing more is needed from you.\n\nIf you did not do this, sign in again to start a new account and contact us.\n", alertTime())))
	return nil
}
