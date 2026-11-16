// Command render is the diagram and document rendering worker.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gitax18/readmely/services/core/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := worker.Main(ctx, "render", os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
