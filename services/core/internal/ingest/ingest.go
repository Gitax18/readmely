// Package ingest is the repository analysis worker. It claims analysis jobs, downloads the
// repository as an archive at one commit, reads it in memory (nothing from it is ever written to
// disk or executed), removes secrets, reads the source files, builds the import graph and the facts
// about the project, and stores the result.
package ingest

import (
	"context"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/config"
	"github.com/Gitax18/readmely/services/core/internal/github"
	"github.com/Gitax18/readmely/services/core/internal/logging"
	"github.com/Gitax18/readmely/services/core/internal/projects"
	"github.com/Gitax18/readmely/services/core/internal/worker"
)

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
		Handlers: map[string]worker.Handler{projects.IngestKind: (&Pipeline{Pool: pool, GitHub: github.NewClient(cfg.GitHubAPIToken)}).Handle},
		Logger:   logger,
	}).Run(ctx)
	logger.Info("worker stopped")
	return 0
}
