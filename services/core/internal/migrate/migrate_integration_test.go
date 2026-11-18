//go:build integration

package migrate_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gitax18/readmely/services/core/internal/dbtest"
	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func functionExists(t *testing.T, d *dbtest.DB, name string) bool {
	t.Helper()
	var n int
	if err := d.Pool.QueryRow(ctxT(t), `SELECT count(*) FROM pg_proc WHERE proname = $1`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

func TestUpDownUp(t *testing.T) {
	t.Parallel()
	d := dbtest.NewEmpty(t)
	db := d.SQL(t)
	ctx := ctxT(t)

	n, err := migrate.Up(ctx, db)
	if err != nil || n == 0 {
		t.Fatalf("first up: applied %d, err %v", n, err)
	}
	if again, err := migrate.Up(ctx, db); err != nil || again != 0 {
		t.Fatalf("second up must change nothing: applied %d, err %v", again, err)
	}
	if !functionExists(t, d, "uuid_generate_v7") {
		t.Fatal("uuid_generate_v7 missing after up")
	}

	var status bytes.Buffer
	if err := migrate.Status(ctx, db, &status); err != nil || !strings.Contains(status.String(), "applied") || strings.Contains(status.String(), "pending") {
		t.Fatalf("status after up: %q, err %v", status.String(), err)
	}

	if reverted, err := migrate.DownAll(ctx, db); err != nil || reverted != n {
		t.Fatalf("down all: reverted %d (want %d), err %v", reverted, n, err)
	}
	if functionExists(t, d, "uuid_generate_v7") || functionExists(t, d, "set_updated_at") {
		t.Fatal("functions still present after down")
	}

	if n2, err := migrate.Up(ctx, db); err != nil || n2 != n {
		t.Fatalf("up after down: applied %d (want %d), err %v", n2, n, err)
	}
}

func TestUUIDv7(t *testing.T) {
	t.Parallel()
	d := dbtest.New(t)
	ctx := ctxT(t)

	rows, err := d.Pool.Query(ctx, `SELECT uuid_generate_v7()::text, (extract(epoch FROM clock_timestamp()) * 1000)::bigint FROM generate_series(1, 10000)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := make(map[string]struct{}, 10000)
	prev := ""
	var nowMs int64
	for rows.Next() {
		var id string
		if err := rows.Scan(&id, &nowMs); err != nil {
			t.Fatal(err)
		}
		if len(id) != 36 || id[14] != '7' || !strings.ContainsRune("89ab", rune(id[19])) {
			t.Fatalf("%s is not a version 7, variant 1 UUID", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = struct{}{}
		ts := id[:8] + id[9:13] // 48-bit millisecond timestamp, hex
		if prev != "" && ts < prev {
			t.Fatalf("timestamp went backwards: %s after %s", ts, prev)
		}
		prev = ts
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 10000 {
		t.Fatalf("got %d ids", len(seen))
	}
	var ms int64
	if err := d.Pool.QueryRow(ctx, `SELECT ('x' || substr(replace($1::text, '-', ''), 1, 12))::bit(48)::bigint`, prevID(t, d)).Scan(&ms); err != nil {
		t.Fatal(err)
	}
	if diff := nowMs - ms; diff < -2000 || diff > 5000 {
		t.Fatalf("embedded time is %d ms away from the clock", diff)
	}
}

func prevID(t *testing.T, d *dbtest.DB) string {
	t.Helper()
	var id string
	if err := d.Pool.QueryRow(ctxT(t), `SELECT uuid_generate_v7()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func TestServiceRolesCannotChangeTheSchema(t *testing.T) {
	t.Parallel()
	d := dbtest.New(t)
	ctx := ctxT(t)

	for _, role := range []string{migrate.RoleApp, migrate.RoleAI} {
		conn := d.AsRole(t, role)
		for _, stmt := range []string{
			`CREATE TABLE sneaky (id int)`,
			`DROP FUNCTION uuid_generate_v7()`,
			`ALTER FUNCTION set_updated_at() RENAME TO other`,
			`CREATE ROLE intruder`,
		} {
			if _, err := conn.Exec(ctx, stmt); sqlState(err) != "42501" {
				t.Errorf("%s: %q should be denied (42501), got %v", role, stmt, err)
			}
		}
	}
	// Using what the schema offers still works.
	if err := d.AsRole(t, migrate.RoleApp).QueryRow(ctx, `SELECT uuid_generate_v7()`).Scan(new(string)); err != nil {
		t.Errorf("app role cannot generate ids: %v", err)
	}
}

func TestRolesCannotLogInUntilPasswordsAreSet(t *testing.T) {
	d := dbtest.New(t)
	ctx := ctxT(t)

	// Throwaway roles: the real ones are cluster-wide and belong to the development stack.
	app := "rm_t_" + strings.TrimPrefix(d.Name, "rm_test_")
	if _, err := d.Pool.Exec(ctx, `CREATE ROLE `+pgx.Identifier{app}.Sanitize()+` NOLOGIN`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Roles are cluster-wide, so a test that leaves one behind pollutes the dev database.
		// The role may hold privileges in this database (dropped with it); the role itself
		// must be dropped from a connection to another database.
		id := pgx.Identifier{app}.Sanitize()
		_, _ = d.Pool.Exec(context.Background(), `DROP OWNED BY `+id)
		admin, err := pgx.Connect(context.Background(), os.Getenv("DATABASE_URL"))
		if err != nil {
			t.Errorf("cleanup: cannot connect to drop role %s", app)
			return
		}
		defer func() { _ = admin.Close(context.Background()) }()
		if _, err := admin.Exec(context.Background(), `DROP ROLE IF EXISTS `+id); err != nil {
			t.Errorf("cleanup: cannot drop role %s: %v", app, err)
		}
	})

	if _, err := pgx.Connect(ctx, d.URLFor(t, app, "anything-at-all-123")); err == nil {
		t.Fatal("a NOLOGIN role could log in")
	}

	// A password full of quoting hazards must be stored literally, not interpreted.
	hostile := `p'a"s;s--w\ord-0123456789`
	if err := migrate.SetPasswords(ctx, d.SQL(t), []migrate.RolePassword{{Role: app, Password: hostile}}); err != nil {
		t.Fatalf("SetPasswords: %v", err)
	}
	conn, err := pgx.Connect(ctx, d.URLFor(t, app, hostile))
	if err != nil {
		t.Fatalf("role cannot log in with its password: %v", err)
	}
	_ = conn.Close(ctx)
	if _, err := pgx.Connect(ctx, d.URLFor(t, app, "wrong-password-0123456789")); err == nil {
		t.Fatal("a wrong password was accepted")
	}

	if err := migrate.SetPasswords(ctx, d.SQL(t), []migrate.RolePassword{{Role: "no_such_role_" + app, Password: hostile}}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing role: %v", err)
	}
	if err := migrate.SetPasswords(ctx, d.SQL(t), []migrate.RolePassword{{Role: app, Password: "short"}}); err == nil {
		t.Fatal("short password accepted")
	}
}
