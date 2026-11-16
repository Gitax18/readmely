// Package worker is the shared skeleton of the background binaries (ingest, render).
// For now a worker only starts, logs, and exits cleanly on shutdown;
// job handling is added in later phases.
package worker

import (
	"context"
	"fmt"
	"io"

	"github.com/Gitax18/readmely/services/core/internal/config"
	"github.com/Gitax18/readmely/services/core/internal/logging"
)

// Main runs worker name until ctx is cancelled and returns the process exit code.
func Main(ctx context.Context, name string, getenv func(string) string, stdout, stderr io.Writer) int {
	cfg, err := config.LoadBase(getenv)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error:\n%v\n", err)
		return 2
	}
	logger := logging.New(stdout, cfg.LogLevel, name)
	logger.Info("worker started", "env", cfg.Env)
	<-ctx.Done()
	logger.Info("worker stopped")
	return 0
}
