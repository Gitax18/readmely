//go:build integration

// Package schema tests the database schema itself: its constraints, cascades, privileges and
// the conventions every table must follow. They run against real Postgres.
package schema_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Gitax18/readmely/services/core/internal/dbtest"
	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

func ctxT(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func pgErr(err error) *pgconn.PgError {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe
	}
	return nil
}

// wantRejected fails unless err is a database error with the given SQLSTATE and, when
// constraint is not empty, that exact constraint name.
func wantRejected(t *testing.T, err error, code, constraint string) {
	t.Helper()
	pe := pgErr(err)
	if pe == nil {
		t.Fatalf("expected a database error %s %s, got %v", code, constraint, err)
	}
	if pe.Code != code || (constraint != "" && pe.ConstraintName != constraint) {
		t.Fatalf("expected %s %q, got %s %q (%s)", code, constraint, pe.Code, pe.ConstraintName, pe.Message)
	}
}

func mustUser(t *testing.T, db *dbtest.DB, email string) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(ctxT(t), `INSERT INTO users (email) VALUES ($1) RETURNING id::text`, email).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

func mustProject(t *testing.T, db *dbtest.DB, userID, name string) string {
	t.Helper()
	var id string
	if err := db.Pool.QueryRow(ctxT(t), `INSERT INTO projects (user_id, name) VALUES ($1, $2) RETURNING id::text`, userID, name).Scan(&id); err != nil {
		t.Fatalf("insert project %s: %v", name, err)
	}
	return id
}

func mustJob(t *testing.T, db *dbtest.DB, userID string, projectID *string, key string) string {
	t.Helper()
	var id string
	err := db.Pool.QueryRow(ctxT(t),
		`INSERT INTO jobs (user_id, project_id, kind, idempotency_key) VALUES ($1, $2, 'ingest', $3) RETURNING id::text`,
		userID, projectID, key).Scan(&id)
	if err != nil {
		t.Fatalf("insert job %s: %v", key, err)
	}
	return id
}

func count(t *testing.T, db *dbtest.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.Pool.QueryRow(ctxT(t), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUsers(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	mustUser(t, db, "Ada@Example.invalid")

	rejected := []struct {
		name, sql, code, constraint string
		args                        []any
	}{
		{"same address in other case", `INSERT INTO users (email) VALUES ($1)`, "23505", "users_email_lower_key", []any{"ada@example.INVALID"}},
		{"no at sign", `INSERT INTO users (email) VALUES ($1)`, "23514", "users_email_check", []any{"nobody"}},
		{"leading at sign", `INSERT INTO users (email) VALUES ($1)`, "23514", "users_email_check", []any{"@example.invalid"}},
		{"padded address", `INSERT INTO users (email) VALUES ($1)`, "23514", "users_email_check", []any{" a@example.invalid"}},
		{"address too long", `INSERT INTO users (email) VALUES ($1)`, "23514", "users_email_check", []any{strings.Repeat("a", 251) + "@b.c"}},
		{"unknown status", `INSERT INTO users (email, status) VALUES ($1, 'banned')`, "23514", "users_status_check", []any{"b@example.invalid"}},
		{"display name too long", `INSERT INTO users (email, display_name) VALUES ($1, $2)`, "23514", "users_display_name_check", []any{"c@example.invalid", strings.Repeat("x", 101)}},
		{"null email", `INSERT INTO users (email) VALUES (NULL)`, "23502", "", nil},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, tt.sql, tt.args...)
			wantRejected(t, err, tt.code, tt.constraint)
		})
	}

	t.Run("a valid user gets a v7 id and timestamps", func(t *testing.T) {
		id := mustUser(t, db, "valid@example.invalid")
		if len(id) != 36 || id[14] != '7' {
			t.Fatalf("id %s is not a UUIDv7", id)
		}
	})
}

func TestUpdatedAtMovesAndCreatedAtDoesNot(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	id := mustUser(t, db, "clock@example.invalid")

	var created0, updated0 time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT created_at, updated_at FROM users WHERE id = $1`, id).Scan(&created0, &updated0); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	// A caller that tries to set updated_at itself is overridden by the trigger.
	if _, err := db.Pool.Exec(ctx, `UPDATE users SET display_name = 'Ada', updated_at = '2000-01-01' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var created1, updated1 time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT created_at, updated_at FROM users WHERE id = $1`, id).Scan(&created1, &updated1); err != nil {
		t.Fatal(err)
	}
	if !created1.Equal(created0) {
		t.Fatal("created_at changed")
	}
	if !updated1.After(updated0) {
		t.Fatalf("updated_at did not move: %v then %v", updated0, updated1)
	}

	// An update that changes nothing is not a modification (an idempotent replay relies on this).
	time.Sleep(20 * time.Millisecond)
	if _, err := db.Pool.Exec(ctx, `UPDATE users SET display_name = 'Ada' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	var updated2 time.Time
	if err := db.Pool.QueryRow(ctx, `SELECT updated_at FROM users WHERE id = $1`, id).Scan(&updated2); err != nil {
		t.Fatal(err)
	}
	if !updated2.Equal(updated1) {
		t.Fatalf("a no-op update moved updated_at: %v then %v", updated1, updated2)
	}
}

func TestProjects(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	bob := mustUser(t, db, "bob@example.invalid")
	mustProject(t, db, ada, "Readmely")

	rejected := []struct {
		name, sql, code, constraint string
		args                        []any
	}{
		{"same name, other case, same user", `INSERT INTO projects (user_id, name) VALUES ($1, $2)`, "23505", "projects_user_id_name_lower_key", []any{ada, "READMELY"}},
		{"empty name", `INSERT INTO projects (user_id, name) VALUES ($1, $2)`, "23514", "projects_name_check", []any{ada, ""}},
		{"padded name", `INSERT INTO projects (user_id, name) VALUES ($1, $2)`, "23514", "projects_name_check", []any{ada, " x"}},
		{"name too long", `INSERT INTO projects (user_id, name) VALUES ($1, $2)`, "23514", "projects_name_check", []any{ada, strings.Repeat("x", 101)}},
		{"unknown status", `INSERT INTO projects (user_id, name, status) VALUES ($1, $2, 'done')`, "23514", "projects_status_check", []any{ada, "p1"}},
		{"owner does not exist", `INSERT INTO projects (user_id, name) VALUES (uuid_generate_v7(), $1)`, "23503", "projects_user_id_fkey", []any{"p2"}},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, tt.sql, tt.args...)
			wantRejected(t, err, tt.code, tt.constraint)
		})
	}
	t.Run("another user may reuse the name", func(t *testing.T) { mustProject(t, db, bob, "Readmely") })
}

func TestJobsConstraints(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	bob := mustUser(t, db, "bob@example.invalid")
	adaProject := mustProject(t, db, ada, "Readmely")
	mustJob(t, db, ada, &adaProject, "key-00000001")

	// A job insert with $1 user, $2 project, $3 key, plus any extra columns and values.
	insert := func(extraCols, extraVals string) string {
		return `INSERT INTO jobs (user_id, project_id, kind, idempotency_key` + extraCols + `) VALUES ($1, $2, 'ingest', $3` + extraVals + `)`
	}
	big := `{"x":"` + strings.Repeat("a", 70000) + `"}`

	rejected := []struct {
		name, sql, code, constraint string
		args                        []any
	}{
		{"unknown kind", `INSERT INTO jobs (user_id, kind, idempotency_key) VALUES ($1, 'mine_bitcoin', $2)`, "23514", "jobs_kind_check", []any{ada, "key-00000002"}},
		{"unknown status", insert(", status", ", 'paused'"), "23514", "jobs_status_check", []any{ada, nil, "key-00000003"}},
		{"payload is an array", insert(", payload", ", '[]'::jsonb"), "23514", "jobs_payload_check", []any{ada, nil, "key-00000004"}},
		{"payload over 64 KiB", insert(", payload", ", $4::jsonb"), "23514", "jobs_payload_check", []any{ada, nil, "key-00000005", big}},
		{"zero max attempts", insert(", max_attempts", ", 0"), "23514", "jobs_max_attempts_check", []any{ada, nil, "key-00000006"}},
		{"too many max attempts", insert(", max_attempts", ", 21"), "23514", "jobs_max_attempts_check", []any{ada, nil, "key-00000007"}},
		{"attempts beyond the maximum", insert(", attempts, max_attempts", ", 4, 3"), "23514", "jobs_attempts_check", []any{ada, nil, "key-00000008"}},
		{"short idempotency key", insert("", ""), "23514", "jobs_idempotency_key_check", []any{ada, nil, "short"}},
		{"same key, same user", insert("", ""), "23505", "jobs_user_id_idempotency_key_key", []any{ada, &adaProject, "key-00000001"}},
		{"bad error code", insert(", error_code", ", 'Stack trace: boom'"), "23514", "jobs_error_code_check", []any{ada, nil, "key-00000009"}},
		{"running without a worker", insert(", status", ", 'running'"), "23514", "jobs_lease_check", []any{ada, nil, "key-0000000a"}},
		{"a worker on a queued job", insert(", locked_by, lease_expires_at", ", 'w1', now()"), "23514", "jobs_lease_check", []any{ada, nil, "key-0000000b"}},
		{"a worker without a lease", insert(", status, locked_by", ", 'running', 'w1'"), "23514", "jobs_lease_check", []any{ada, nil, "key-0000000c"}},
		{"finished without a finish time", insert(", status", ", 'succeeded'"), "23514", "jobs_finished_at_check", []any{ada, nil, "key-0000000d"}},
		{"a finish time on a queued job", insert(", finished_at", ", now()"), "23514", "jobs_finished_at_check", []any{ada, nil, "key-0000000e"}},
		{"a project that belongs to someone else", insert("", ""), "23503", "jobs_project_id_user_id_fkey", []any{bob, &adaProject, "key-0000000f"}},
		{"a project that does not exist", insert("", ""), "23503", "jobs_project_id_user_id_fkey", []any{ada, "00000000-0000-7000-8000-000000000000", "key-00000010"}},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, tt.sql, tt.args...)
			wantRejected(t, err, tt.code, tt.constraint)
		})
	}

	t.Run("valid states are accepted", func(t *testing.T) {
		valid := []string{
			`INSERT INTO jobs (user_id, kind, idempotency_key, status, locked_by, lease_expires_at) VALUES ($1, 'ingest', 'key-v0000001', 'running', 'worker-1', now() + interval '1 minute')`,
			`INSERT INTO jobs (user_id, kind, idempotency_key, status, finished_at, error_code) VALUES ($1, 'ingest', 'key-v0000002', 'failed', now(), 'provider_timeout')`,
			`INSERT INTO jobs (user_id, kind, idempotency_key, status, finished_at) VALUES ($1, 'generate_readme', 'key-v0000003', 'succeeded', now())`,
		}
		for _, q := range valid {
			if _, err := db.Pool.Exec(ctx, q, ada); err != nil {
				t.Errorf("%s: %v", q, err)
			}
		}
	})
	t.Run("another user may reuse a key", func(t *testing.T) { mustJob(t, db, bob, nil, "key-00000001") })
}

func TestCascades(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ada := mustUser(t, db, "ada@example.invalid")
	bob := mustUser(t, db, "bob@example.invalid")
	p1, p2 := mustProject(t, db, ada, "one"), mustProject(t, db, ada, "two")
	bp := mustProject(t, db, bob, "bobs")
	mustJob(t, db, ada, &p1, "key-ada-p1-1")
	mustJob(t, db, ada, &p1, "key-ada-p1-2")
	mustJob(t, db, ada, &p2, "key-ada-p2-1")
	mustJob(t, db, ada, nil, "key-ada-none")
	mustJob(t, db, bob, &bp, "key-bob-bp-1")

	if _, err := db.Pool.Exec(ctxT(t), `DELETE FROM projects WHERE id = $1`, p1); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT count(*) FROM jobs WHERE project_id = $1`, p1); n != 0 {
		t.Fatalf("%d jobs survived their project", n)
	}
	if n := count(t, db, `SELECT count(*) FROM jobs WHERE user_id = $1`, ada); n != 2 {
		t.Fatalf("deleting one project must keep the user's other jobs, found %d", n)
	}

	if _, err := db.Pool.Exec(ctxT(t), `DELETE FROM users WHERE id = $1`, ada); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"projects", "jobs"} {
		if n := count(t, db, `SELECT count(*) FROM `+table+` WHERE user_id = $1`, ada); n != 0 {
			t.Fatalf("%d rows left in %s after the user was deleted", n, table)
		}
	}
	if count(t, db, `SELECT count(*) FROM projects WHERE user_id = $1`, bob) != 1 || count(t, db, `SELECT count(*) FROM jobs WHERE user_id = $1`, bob) != 1 {
		t.Fatal("another user's data was removed")
	}
}

func TestServiceRolePrivileges(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)

	app := db.AsRole(t, migrate.RoleApp)
	var id string
	if err := app.QueryRow(ctx, `INSERT INTO users (email) VALUES ('role@example.invalid') RETURNING id::text`).Scan(&id); err != nil {
		t.Fatalf("app role cannot insert: %v", err)
	}
	if _, err := app.Exec(ctx, `UPDATE users SET display_name = 'x' WHERE id = $1`, id); err != nil {
		t.Fatalf("app role cannot update: %v", err)
	}
	if _, err := app.Exec(ctx, `TRUNCATE users CASCADE`); pgErr(err) == nil || pgErr(err).Code != "42501" {
		t.Fatalf("app role must not truncate: %v", err)
	}
	if _, err := app.Exec(ctx, `ALTER TABLE users DISABLE TRIGGER users_set_updated_at`); pgErr(err) == nil || pgErr(err).Code != "42501" {
		t.Fatalf("app role must not alter tables: %v", err)
	}
	if _, err := app.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		t.Fatalf("app role cannot delete: %v", err)
	}

	ai := db.AsRole(t, migrate.RoleAI)
	for _, table := range []string{"users", "projects", "jobs"} {
		if _, err := ai.Exec(ctx, `SELECT 1 FROM `+table+` LIMIT 1`); pgErr(err) == nil || pgErr(err).Code != "42501" {
			t.Errorf("the AI role must not read %s: %v", table, err)
		}
	}
}

// TestConventions holds every table to the rules in plans/phase-1-database.md, so a table added
// later by someone in a hurry cannot quietly break them.
// Audit F4: the service roles are cut off when a statement, a lock wait or an open transaction
// runs too long. Settings are per database and apply at login.
func TestServiceRolesAreTimeBounded(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	want := map[string]string{"statement_timeout": "30s", "lock_timeout": "10s", "idle_in_transaction_session_timeout": "60s"}
	for _, role := range []string{"readmely_app", "readmely_ai"} {
		rows, err := db.Pool.Query(ctxT(t), `
			SELECT split_part(c, '=', 1), split_part(c, '=', 2)
			FROM pg_db_role_setting s
			JOIN pg_roles r ON r.oid = s.setrole
			JOIN pg_database d ON d.oid = s.setdatabase
			CROSS JOIN unnest(s.setconfig) AS c
			WHERE r.rolname = $1 AND d.datname = current_database()`, role)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			got[k] = v
		}
		rows.Close()
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: %s = %q, want %q", role, k, got[k], v)
			}
		}
	}
}

// Audit F12: a function added by a later migration is private until granted.
func TestNewFunctionsArePrivateByDefault(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	if _, err := db.Pool.Exec(ctx, `CREATE FUNCTION audit_probe() RETURNS int LANGUAGE sql AS 'SELECT 1'`); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"readmely_app", "readmely_ai"} {
		var can bool
		if err := db.Pool.QueryRow(ctx, `SELECT has_function_privilege($1, 'audit_probe()', 'EXECUTE')`, role).Scan(&can); err != nil {
			t.Fatal(err)
		}
		if can {
			t.Errorf("%s can execute a function nobody granted it", role)
		}
	}
	// The one function that is granted on purpose still is.
	var can bool
	if err := db.Pool.QueryRow(ctx, `SELECT has_function_privilege('readmely_app', 'credit_reserve(uuid,bigint,uuid,text)', 'EXECUTE')`).Scan(&can); err != nil || !can {
		t.Fatalf("readmely_app lost EXECUTE on credit_reserve (err %v)", err)
	}
}

func TestConventions(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)

	query := func(q string) []string {
		rows, err := db.Pool.Query(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	report := func(rule string, offenders []string) {
		t.Helper()
		if len(offenders) > 0 {
			t.Errorf("%s: %s", rule, strings.Join(offenders, ", "))
		}
	}

	const userTables = `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'goose_db_version'`

	report("tables without a uuid primary key named id that defaults to uuid_generate_v7()", query(`
		SELECT t.relname FROM (`+userTables+`) AS t(relname)
		WHERE NOT EXISTS (
			SELECT 1 FROM pg_attribute a
			JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
			JOIN pg_index i ON i.indrelid = a.attrelid AND i.indisprimary AND i.indnatts = 1 AND i.indkey[0] = a.attnum
			WHERE a.attrelid = t.relname::regclass AND a.attname = 'id' AND a.atttypid = 'uuid'::regtype
			  AND pg_get_expr(d.adbin, d.adrelid) = 'uuid_generate_v7()'
		)`))

	report("timestamp columns that are not timestamptz", query(`
		SELECT table_name || '.' || column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND data_type = 'timestamp without time zone' AND table_name <> 'goose_db_version'`))

	report("tables with updated_at but no set_updated_at trigger", query(`
		SELECT t.relname FROM (`+userTables+`) AS t(relname)
		WHERE EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = t.relname::regclass AND a.attname = 'updated_at')
		  AND NOT EXISTS (SELECT 1 FROM pg_trigger g WHERE g.tgrelid = t.relname::regclass AND NOT g.tgisinternal AND g.tgfoid = 'set_updated_at'::regproc)`))

	report("foreign keys left on the default NO ACTION delete rule (say CASCADE, SET NULL or RESTRICT)", query(`
		SELECT conrelid::regclass || '.' || conname FROM pg_constraint WHERE contype = 'f' AND confdeltype = 'a' AND connamespace = 'public'::regnamespace`))

	report("foreign keys without an index that starts with their columns", query(`
		SELECT c.conrelid::regclass || '.' || c.conname FROM pg_constraint c
		WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace
		  AND NOT EXISTS (
			SELECT 1 FROM pg_index i
			WHERE i.indrelid = c.conrelid AND i.indpred IS NULL
			  AND (i.indkey::int2[])[0:cardinality(c.conkey)-1] = c.conkey
		  )`))

	report("constraints not named <table>_...", query(`
		SELECT conrelid::regclass || '.' || conname FROM pg_constraint
		WHERE connamespace = 'public'::regnamespace AND conrelid <> 0 AND conname NOT LIKE conrelid::regclass::text || '\_%'`))
}
