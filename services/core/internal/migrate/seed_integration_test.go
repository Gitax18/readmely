//go:build integration

package migrate_test

import (
	"strings"
	"testing"

	"github.com/Gitax18/readmely/services/core/internal/dbtest"
	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

func countRows(t *testing.T, d *dbtest.DB, query string) int {
	t.Helper()
	var n int
	if err := d.Pool.QueryRow(ctxT(t), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedLoadsEveryStateAndIsIdempotent(t *testing.T) {
	t.Parallel()
	d := dbtest.New(t)
	db := d.SQL(t)

	tables := map[string]int{"users": 3, "projects": 3, "jobs": 6, "credit_ledger": 7}
	for run := 1; run <= 3; run++ {
		if err := migrate.Seed(ctxT(t), db); err != nil {
			t.Fatalf("seed run %d: %v", run, err)
		}
		for table, want := range tables {
			if got := countRows(t, d, `SELECT count(*) FROM `+table); got != want {
				t.Fatalf("run %d: %s has %d rows, want %d (a re-run must not add or lose rows)", run, table, got, want)
			}
		}
	}

	t.Run("every job state and ledger kind is present", func(t *testing.T) {
		if got := countRows(t, d, `SELECT count(DISTINCT status) FROM jobs`); got != 5 {
			t.Errorf("%d distinct job states, want all 5", got)
		}
		if got := countRows(t, d, `SELECT count(DISTINCT kind) FROM credit_ledger`); got != 4 {
			t.Errorf("%d distinct ledger kinds, want grant, reserve, settle, refund", got)
		}
	})
	t.Run("balances are what the history says", func(t *testing.T) {
		var ada, bob int64
		if err := d.Pool.QueryRow(ctxT(t), `
			SELECT (SELECT balance FROM credit_balances b JOIN users u ON u.id = b.user_id WHERE u.email = 'ada@example.invalid'),
			       (SELECT balance FROM credit_balances b JOIN users u ON u.id = b.user_id WHERE u.email = 'bob@example.invalid')`).Scan(&ada, &bob); err != nil {
			t.Fatal(err)
		}
		if ada != 470 || bob != 100 {
			t.Fatalf("balances ada=%d bob=%d, want 470 and 100", ada, bob)
		}
	})
	t.Run("only fictional addresses", func(t *testing.T) {
		rows, err := d.Pool.Query(ctxT(t), `SELECT email FROM users`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var email string
			if err := rows.Scan(&email); err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(email, "@example.invalid") {
				t.Errorf("%s is not a reserved example address", email)
			}
		}
	})
	t.Run("the queue can work on the seeded jobs", func(t *testing.T) {
		if got := countRows(t, d, `SELECT count(*) FROM jobs WHERE status = 'queued' AND run_after <= now()`); got != 2 {
			t.Fatalf("%d runnable jobs, want 2", got)
		}
	})
}

func TestSeedExplainsAnUnmigratedDatabase(t *testing.T) {
	t.Parallel()
	d := dbtest.NewEmpty(t)
	err := migrate.Seed(ctxT(t), d.SQL(t))
	if err == nil || !strings.Contains(err.Error(), "migrations") {
		t.Fatalf("expected an error that mentions the migrations, got %v", err)
	}
}
