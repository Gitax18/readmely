package migrate

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const usage = "usage: migrate up | roles | status | down | reset | seed"

// seedEnvironments are the only places fake data may be loaded. An unset APP_ENV is a developer's
// machine. Anything else, production and staging included, is refused.
var seedEnvironments = map[string]bool{"": true, "development": true, "test": true}

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
	if (cmd == "down" || cmd == "reset") && strings.EqualFold(strings.TrimSpace(getenv("APP_ENV")), "production") {
		_, _ = fmt.Fprintf(stderr, "refusing to run %q when APP_ENV=production: migrations only go forward there\n", cmd)
		return 2
	}
	if env := strings.ToLower(strings.TrimSpace(getenv("APP_ENV"))); cmd == "seed" && !seedEnvironments[env] {
		_, _ = fmt.Fprintf(stderr, "refusing to load seed data when APP_ENV=%s: it is for development only\n", env)
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
		for _, r := range roles {
			if err := ValidatePassword(r.Role, r.Password); err != nil {
				_, _ = fmt.Fprintf(stderr, "configuration error: %v (set APP_DB_PASSWORD and AI_DB_PASSWORD)\n", err)
				return 2
			}
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
