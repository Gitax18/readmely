// Command migrate applies database migrations and sets the service roles' passwords.
//
//	migrate up        apply pending migrations
//	migrate roles     set the passwords of the service roles from the environment
//	migrate status    list migrations
//	migrate down      revert the latest migration   (only with APP_ENV=development)
//	migrate reset     revert everything             (only with APP_ENV=development)
//	migrate seed      load fake development data    (only with APP_ENV=development)
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
