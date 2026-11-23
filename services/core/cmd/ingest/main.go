// Command ingest is the repository analysis worker.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gitax18/readmely/services/core/internal/ingest"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := ingest.Main(ctx, os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
