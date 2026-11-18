package migrate

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const usage = "usage: migrate up | roles | status | down | reset | seed"

// devOnly are the commands that delete data or load fake data. They run only when APP_ENV says
// development explicitly: an unset or misspelled APP_ENV is refused, because a missing variable is
// the most likely mistake in a production deployment (audit F1).
var devOnly = map[string]bool{"down": true, "reset": true, "seed": true}

// Main runs the migrate command. Exit codes: 0 success, 1 failure, 2 bad usage or configuration.
func Main(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(stderr, usage)
		return 2
	}
	cmd := args[0]
	switch cmd {
	case "up", "roles", "status", "down", "reset", "seed":
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n%s\n", cmd, usage)
		return 2
	}
	if env := strings.ToLower(strings.TrimSpace(getenv("APP_ENV"))); devOnly[cmd] && env != "development" {
		shown := env
		if shown == "" {
			shown = "(unset)"
		}
		_, _ = fmt.Fprintf(stderr, "refusing to run %q when APP_ENV=%s: it deletes or fakes data, so it needs APP_ENV=development\n", cmd, shown)
		return 2
	}
	url := strings.TrimSpace(getenv("MIGRATE_DATABASE_URL"))
	if url == "" {
		_, _ = fmt.Fprintln(stderr, "configuration error: MIGRATE_DATABASE_URL is required (the owner connection, not a service role)")
		return 2
	}
	var roles []RolePassword
	if cmd == "roles" {
		roles = []RolePassword{
			{RoleApp, getenv("APP_DB_PASSWORD")},
			{RoleAI, getenv("AI_DB_PASSWORD")},
		}
		if err := ValidatePasswords(roles); err != nil {
			_, _ = fmt.Fprintf(stderr, "configuration error: %v (set APP_DB_PASSWORD and AI_DB_PASSWORD; openssl rand -hex 24)\n", err)
			return 2
		}
	}

	db, err := Open(url)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "configuration error: %v\n", err)
		return 2
	}
	defer func() { _ = db.Close() }()

	if err := run(ctx, cmd, db, roles, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s failed: %v\n", cmd, err)
		return 1
	}
	return 0
}
