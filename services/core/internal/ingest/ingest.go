// Package ingest is the repository analysis worker. Phase 3 only provides the shell: it claims
// analysis jobs, reports progress and marks the project ready. Phase 4 replaces the placeholder
// steps below with the real work (clone, filter, secret scan, parse, graph).
package ingest

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/config"
	"github.com/Gitax18/readmely/services/core/internal/db/store"
	"github.com/Gitax18/readmely/services/core/internal/logging"
	"github.com/Gitax18/readmely/services/core/internal/projects"
	"github.com/Gitax18/readmely/services/core/internal/worker"
)

// placeholderSteps stands in for the real analysis until Phase 4.
var placeholderSteps = []struct {
	name     string
	progress int
}{
	{"fetching", 15}, {"scanning", 40}, {"parsing", 65}, {"summarising", 90},
}

func placeholder(ctx context.Context, _ store.Job, report worker.Reporter) error {
	report("queued", 5)
	for _, s := range placeholderSteps {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
		report(s.name, s.progress)
	}
	return nil
}

// Main runs the ingest worker until ctx ends and returns the process exit code.
func Main(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.LoadWorker(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error:\n%v\n", err)
		return 2
	}
	logger := logging.New(stdout, cfg.LogLevel, "ingest")
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot open the database", "error", err)
		return 1
	}
	defer pool.Close()

	logger.Info("worker started", "env", cfg.Env)
	(&worker.Runner{
		Pool:     pool,
		Handlers: map[string]worker.Handler{projects.IngestKind: placeholder},
		Logger:   logger,
	}).Run(ctx)
	logger.Info("worker stopped")
	return 0
}
