// Command migrate applies database migrations and sets the service roles' passwords.
//
//	migrate up        apply pending migrations
//	migrate roles     set the passwords of the service roles from the environment
//	migrate status    list migrations
//	migrate down      revert the latest migration   (refused in production)
//	migrate reset     revert everything             (refused in production)
//	migrate seed      load fake development data    (development and test only)
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := migrate.Main(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
