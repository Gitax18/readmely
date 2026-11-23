// Package diagram serves a project's architecture diagram and queues the AI worker job that draws it.
package diagram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/ratelimit"
)

const (
	// Cost is held when a diagram is requested; the worker settles it to what the model calls cost.
	Cost int64 = 15
	// Kind is the job kind the AI worker runs.
	Kind = "generate_diagram"

	maxFocus    = 200
	maxAttempts = 2
)

var ruleGenerate = ratelimit.Rule{Name: "diagram_generate_user", Max: 10, Window: time.Hour}

// Errors the handlers turn into the documented answers.
var (
	ErrNotFound            = errors.New("project not found")
	ErrNotAnalysed         = errors.New("project has not been analysed")
	ErrRunning             = errors.New("a diagram job is already running")
	ErrInsufficientCredits = errors.New("not enough credits")
	ErrInvalid             = errors.New("invalid request")
)

// RateLimitedError carries how long the caller should wait.
type RateLimitedError struct{ RetryAfter time.Duration }

func (e *RateLimitedError) Error() string { return "rate limited" }

// Service is the diagram logic. It has no HTTP in it.
type Service struct {
	pool    *pgxpool.Pool
	q       *store.Queries
	limiter *ratelimit.Limiter
}

// NewService wires the service.
func NewService(pool *pgxpool.Pool, limiter *ratelimit.Limiter) *Service {
	return &Service{pool: pool, q: store.New(pool), limiter: limiter}
}

// State is what the diagram card shows.
type State struct {
	Diagram *store.ProjectDiagram
	Job     *store.Job
}

func (s *Service) project(ctx context.Context, q *store.Queries, userID, id uuid.UUID) (store.Project, error) {
	p, err := q.GetProject(ctx, store.GetProjectParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Project{}, ErrNotFound
	}
	return p, err
}

// Get returns the current diagram and the newest diagram job.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (State, error) {
	if _, err := s.project(ctx, s.q, userID, id); err != nil {
		return State{}, err
	}
	var out State
	switch d, err := s.q.GetProjectDiagram(ctx, id); {
	case err == nil:
		out.Diagram = &d
	case !errors.Is(err, pgx.ErrNoRows):
		return State{}, fmt.Errorf("get diagram: %w", err)
	}
	switch j, err := s.q.GetLatestProjectJob(ctx, store.GetLatestProjectJobParams{ProjectID: &id, UserID: userID, Kind: Kind}); {
	case err == nil:
		out.Job = &j
	case !errors.Is(err, pgx.ErrNoRows):
		return State{}, fmt.Errorf("get diagram: %w", err)
	}
	return out, nil
}

// Generate holds credits and queues a job that draws the diagram from the analysis.
func (s *Service) Generate(ctx context.Context, userID, id uuid.UUID, focus string) (store.Job, error) {
	focus = strings.TrimSpace(focus)
	if utf8.RuneCountInString(focus) > maxFocus || strings.ContainsRune(focus, 0) {
		return store.Job{}, ErrInvalid
	}
	d, err := s.limiter.Allow(ctx, ratelimit.Check{Rule: ruleGenerate, Subject: userID.String()})
	if err != nil {
		return store.Job{}, fmt.Errorf("diagram: %w", err)
	}
	if !d.Allowed {
		return store.Job{}, &RateLimitedError{RetryAfter: d.RetryAfter}
	}
	body, err := json.Marshal(map[string]string{"focus": focus})
	if err != nil {
		return store.Job{}, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return store.Job{}, fmt.Errorf("diagram: %w", err)
	}
	key := Kind + "-" + hex.EncodeToString(b)

	var job store.Job
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		p, err := s.project(ctx, q, userID, id)
		if err != nil {
			return err
		}
		if _, err := q.GetProjectFacts(ctx, store.GetProjectFactsParams{ProjectID: id, UserID: userID}); errors.Is(err, pgx.ErrNoRows) {
			return ErrNotAnalysed
		} else if err != nil {
			return err
		}
		active, err := q.CountActiveProjectJobs(ctx, store.CountActiveProjectJobsParams{ProjectID: &p.ID, UserID: userID, Kind: Kind})
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrRunning
		}
		job, err = q.EnqueueJob(ctx, store.EnqueueJobParams{
			UserID: userID, ProjectID: &p.ID, Kind: Kind, Payload: body, IdempotencyKey: key, MaxAttempts: maxAttempts,
		})
		if err != nil {
			return err
		}
		_, err = q.ReserveCredits(ctx, store.ReserveCreditsParams{UserID: userID, Amount: Cost, JobID: &job.ID, IdempotencyKey: "hold:" + job.ID.String()})
		return err
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "CR001":
		return store.Job{}, ErrInsufficientCredits
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrNotAnalysed), errors.Is(err, ErrRunning):
		return store.Job{}, err
	case err != nil:
		return store.Job{}, fmt.Errorf("queue diagram job: %w", err)
	}
	return job, nil
}
