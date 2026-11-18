package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io"
)

func run(ctx context.Context, cmd string, db *sql.DB, roles []RolePassword, stdout io.Writer) error {
	switch cmd {
	case "up":
		n, err := Up(ctx, db)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "ok   %d migration(s) applied\n", n)
		}
		return err
	case "down":
		if err := Down(ctx, db); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, "ok   reverted the latest migration")
		return err
	case "reset":
		n, err := DownAll(ctx, db)
		if err == nil {
			_, err = fmt.Fprintf(stdout, "ok   %d migration(s) reverted\n", n)
		}
		return err
	case "status":
		return Status(ctx, db, stdout)
	case "seed":
		if err := Seed(ctx, db); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, "ok   development seed data loaded")
		return err
	case "roles":
		if err := SetPasswords(ctx, db, roles); err != nil {
			return err
		}
		_, err := fmt.Fprintln(stdout, "ok   service roles can log in")
		return err
	}
	return nil
}
