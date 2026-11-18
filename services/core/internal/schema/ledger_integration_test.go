//go:build integration

package schema_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Gitax18/readmely/services/core/internal/dbtest"
	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

const (
	insufficientCredits = "CR001"
	keyReused           = "CR002"
	restrictViolation   = "23001"
)

func mustGrant(t *testing.T, db *dbtest.DB, userID string, amount int64, key string) {
	t.Helper()
	if _, err := db.Pool.Exec(ctxT(t),
		`INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key) VALUES ($1, $2, 'grant', $3)`, userID, amount, key); err != nil {
		t.Fatalf("grant: %v", err)
	}
}

func reserve(ctx context.Context, db *dbtest.DB, userID string, amount int64, jobID *string, key string) (string, error) {
	var id string
	err := db.Pool.QueryRow(ctx, `SELECT (credit_reserve($1, $2, $3, $4)).id::text`, userID, amount, jobID, key).Scan(&id)
	return id, err
}

func balance(t *testing.T, db *dbtest.DB, userID string) int64 {
	t.Helper()
	var b int64
	if err := db.Pool.QueryRow(ctxT(t), `SELECT COALESCE((SELECT balance FROM credit_balances WHERE user_id = $1), 0)`, userID).Scan(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLedgerIsAppendOnly(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	user := mustUser(t, db, "ada@example.invalid")
	mustGrant(t, db, user, 100, "grant-00000001")

	t.Run("even the owner cannot change an entry", func(t *testing.T) {
		for _, q := range []string{
			`UPDATE credit_ledger SET delta = 1000000`,
			`UPDATE credit_ledger SET kind = 'adjustment'`,
			`UPDATE credit_ledger SET note = 'edited'`,
			`UPDATE credit_ledger SET created_at = now() - interval '1 year'`,
			`UPDATE credit_ledger SET idempotency_key = 'something-else'`,
		} {
			_, err := db.Pool.Exec(ctx, q)
			wantRejected(t, err, restrictViolation, "")
		}
	})
	t.Run("even the owner cannot delete or truncate", func(t *testing.T) {
		_, err := db.Pool.Exec(ctx, `DELETE FROM credit_ledger`)
		wantRejected(t, err, restrictViolation, "")
		_, err = db.Pool.Exec(ctx, `TRUNCATE credit_ledger`)
		wantRejected(t, err, restrictViolation, "")
		// Truncating the user table must not reach the ledger through CASCADE either.
		_, err = db.Pool.Exec(ctx, `TRUNCATE users CASCADE`)
		wantRejected(t, err, restrictViolation, "")
	})
	t.Run("an entry cannot be moved to another user", func(t *testing.T) {
		other := mustUser(t, db, "bob@example.invalid")
		_, err := db.Pool.Exec(ctx, `UPDATE credit_ledger SET user_id = $1`, other)
		wantRejected(t, err, restrictViolation, "")
	})
	t.Run("the application role is refused before the trigger is reached", func(t *testing.T) {
		app := db.AsRole(t, migrate.RoleApp)
		for _, q := range []string{`UPDATE credit_ledger SET note = 'x'`, `DELETE FROM credit_ledger`, `TRUNCATE credit_ledger`} {
			if _, err := app.Exec(ctx, q); pgErr(err) == nil || pgErr(err).Code != "42501" {
				t.Errorf("%q: want permission denied, got %v", q, err)
			}
		}
		if _, err := app.Exec(ctx, `INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key) VALUES ($1, 5, 'grant', 'grant-by-app-1')`, user); err != nil {
			t.Errorf("the application role must be able to append: %v", err)
		}
	})
	t.Run("nothing was lost", func(t *testing.T) {
		if got := balance(t, db, user); got != 105 {
			t.Fatalf("balance %d, want 105", got)
		}
	})
}

func TestLedgerEntryRules(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	bob := mustUser(t, db, "bob@example.invalid")
	mustGrant(t, db, ada, 1000, "grant-ada-0001")
	mustGrant(t, db, bob, 1000, "grant-bob-0001")
	held, err := reserve(ctx, db, ada, 100, nil, "reserve-ada-01")
	if err != nil {
		t.Fatal(err)
	}
	bobHeld, err := reserve(ctx, db, bob, 100, nil, "reserve-bob-01")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `INSERT INTO credit_ledger (user_id, delta, kind, idempotency_key) VALUES ($1, 5, 'grant', 'grant-ada-0002')`, ada); err != nil {
		t.Fatal(err)
	}
	var grantID string
	if err := db.Pool.QueryRow(ctx, `SELECT id::text FROM credit_ledger WHERE idempotency_key = 'grant-ada-0002'`).Scan(&grantID); err != nil {
		t.Fatal(err)
	}

	const ins = `INSERT INTO credit_ledger (user_id, delta, kind, reservation_id, idempotency_key) VALUES ($1, $2, $3, $4, $5)`
	rejected := []struct {
		name         string
		args         []any
		code, constr string
	}{
		{"a negative grant", []any{ada, -5, "grant", nil, "key-neg-grant"}, "23514", "credit_ledger_delta_check"},
		{"a zero grant", []any{ada, 0, "grant", nil, "key-zero-grant"}, "23514", "credit_ledger_delta_check"},
		{"a positive reserve", []any{ada, 5, "reserve", nil, "key-pos-reserve"}, "23514", "credit_ledger_delta_check"},
		{"a negative settle", []any{ada, -5, "settle", held, "key-neg-settle"}, "23514", "credit_ledger_delta_check"},
		// Postgres runs the trigger before the CHECKs, so a refund of the wrong size is refused by
		// the trigger's "exactly the reserve" rule (a zero refund is one such size).
		{"a zero refund", []any{ada, 0, "refund", held, "key-zero-refund"}, "23514", "credit_ledger_resolution_check"},
		{"a zero adjustment", []any{ada, 0, "adjustment", nil, "key-zero-adjust"}, "23514", "credit_ledger_delta_check"},
		{"an unknown kind", []any{ada, 5, "bonus", nil, "key-unknown-kind"}, "23514", "credit_ledger_kind_check"},
		{"a settle without a reserve", []any{ada, 5, "settle", nil, "key-settle-none"}, "23514", "credit_ledger_reservation_id_check"},
		{"a grant that points at a reserve", []any{ada, 5, "grant", held, "key-grant-resv"}, "23514", "credit_ledger_reservation_id_check"},
		{"a short key", []any{ada, 5, "grant", nil, "short"}, "23514", "credit_ledger_idempotency_key_check"},
		{"a repeated key", []any{ada, 5, "grant", nil, "grant-ada-0001"}, "23505", "credit_ledger_user_id_idempotency_key_key"},
		{"a settle that answers a grant", []any{ada, 5, "settle", grantID, "key-settle-grant"}, "23514", "credit_ledger_resolution_check"},
		{"a settle for someone else's reserve", []any{ada, 5, "settle", bobHeld, "key-settle-other"}, "23514", "credit_ledger_resolution_check"},
		{"a settle that returns more than was held", []any{ada, 101, "settle", held, "key-settle-more"}, "23514", "credit_ledger_resolution_check"},
		{"a partial refund", []any{ada, 60, "refund", held, "key-refund-part"}, "23514", "credit_ledger_resolution_check"},
		{"an entry with no user", []any{nil, 5, "grant", nil, "key-no-user-01"}, "23514", "credit_ledger_user_id_check"},
	}
	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.Pool.Exec(ctx, ins, tt.args...)
			wantRejected(t, err, tt.code, tt.constr)
		})
	}

	t.Run("a reserve is settled at most once", func(t *testing.T) {
		if _, err := db.Pool.Exec(ctx, ins, ada, 30, "settle", held, "key-settle-ok-1"); err != nil {
			t.Fatalf("a valid settle was rejected: %v", err)
		}
		_, err := db.Pool.Exec(ctx, ins, ada, 100, "refund", held, "key-refund-late")
		wantRejected(t, err, "23505", "credit_ledger_reservation_id_key")
	})
	t.Run("a refund returns exactly the reserve", func(t *testing.T) {
		if _, err := db.Pool.Exec(ctx, ins, bob, 100, "refund", bobHeld, "key-refund-ok-1"); err != nil {
			t.Fatalf("a valid refund was rejected: %v", err)
		}
		if got := balance(t, db, bob); got != 1000 {
			t.Fatalf("balance after refund %d, want 1000", got)
		}
	})
}

func TestLedgerSurvivesDeletionWithoutThePerson(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	bob := mustUser(t, db, "bob@example.invalid")
	project := mustProject(t, db, ada, "Readmely")
	job := mustJob(t, db, ada, &project, "job-ada-0001")
	mustGrant(t, db, ada, 100, "grant-ada-0001")
	mustGrant(t, db, bob, 50, "grant-bob-0001")
	if _, err := reserve(ctx, db, ada, 10, &job, "reserve-ada-01"); err != nil {
		t.Fatal(err)
	}
	before := count(t, db, `SELECT count(*) FROM credit_ledger`)

	// Deleting the project removes the job; the entry that mentioned it keeps its amount.
	if _, err := db.Pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, project); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT count(*) FROM credit_ledger WHERE kind = 'reserve' AND job_id IS NULL AND delta = -10`); n != 1 {
		t.Fatalf("the reserve should survive its job with job_id cleared, found %d", n)
	}

	// Deleting the account clears user_id and keeps every other fact.
	if _, err := db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ada); err != nil {
		t.Fatal(err)
	}
	if after := count(t, db, `SELECT count(*) FROM credit_ledger`); after != before {
		t.Fatalf("ledger rows %d -> %d: none may be lost", before, after)
	}
	if n := count(t, db, `SELECT count(*) FROM credit_ledger WHERE user_id IS NULL AND delta IN (100, -10)`); n != 2 {
		t.Fatalf("the deleted user's entries should be anonymised, found %d", n)
	}
	if got := balance(t, db, bob); got != 50 {
		t.Fatalf("another user's balance changed: %d", got)
	}
	if n := count(t, db, `SELECT count(*) FROM credit_balances WHERE user_id IS NULL`); n != 0 {
		t.Fatal("anonymised entries must not show up as a balance")
	}
}

func TestReserve(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	mustGrant(t, db, ada, 100, "grant-ada-0001")

	t.Run("holds credits and lowers the balance", func(t *testing.T) {
		if _, err := reserve(ctx, db, ada, 30, nil, "reserve-key-001"); err != nil {
			t.Fatal(err)
		}
		if got := balance(t, db, ada); got != 70 {
			t.Fatalf("balance %d, want 70", got)
		}
	})
	t.Run("replaying the same request charges nothing", func(t *testing.T) {
		first, err := reserve(ctx, db, ada, 30, nil, "reserve-key-001")
		if err != nil {
			t.Fatal(err)
		}
		again, err := reserve(ctx, db, ada, 30, nil, "reserve-key-001")
		if err != nil || again != first {
			t.Fatalf("replay returned %q, %v; want the first entry %q", again, err, first)
		}
		if got := balance(t, db, ada); got != 70 {
			t.Fatalf("balance %d after replays, want 70", got)
		}
	})
	t.Run("reusing a key for a different amount is refused", func(t *testing.T) {
		_, err := reserve(ctx, db, ada, 31, nil, "reserve-key-001")
		wantRejected(t, err, keyReused, "")
	})
	t.Run("more than the balance is refused and changes nothing", func(t *testing.T) {
		_, err := reserve(ctx, db, ada, 71, nil, "reserve-key-002")
		wantRejected(t, err, insufficientCredits, "")
		if got := balance(t, db, ada); got != 70 {
			t.Fatalf("balance %d, want 70", got)
		}
	})
	t.Run("exactly the balance is allowed", func(t *testing.T) {
		if _, err := reserve(ctx, db, ada, 70, nil, "reserve-key-003"); err != nil {
			t.Fatal(err)
		}
		if got := balance(t, db, ada); got != 0 {
			t.Fatalf("balance %d, want 0", got)
		}
	})
	t.Run("zero and negative amounts are refused", func(t *testing.T) {
		for _, amount := range []int64{0, -5} {
			_, err := reserve(ctx, db, ada, amount, nil, fmt.Sprintf("reserve-bad-%d", amount))
			wantRejected(t, err, "22023", "")
		}
	})
	t.Run("a user with no entries has no credits", func(t *testing.T) {
		bob := mustUser(t, db, "bob@example.invalid")
		_, err := reserve(ctx, db, bob, 1, nil, "reserve-bob-001")
		wantRejected(t, err, insufficientCredits, "")
	})
	t.Run("only the application role may call it", func(t *testing.T) {
		ai := db.AsRole(t, migrate.RoleAI)
		_, err := ai.Exec(ctx, `SELECT credit_reserve($1, 1, NULL, 'reserve-by-ai-1')`, ada)
		if pgErr(err) == nil || pgErr(err).Code != "42501" {
			t.Fatalf("the AI role must be denied, got %v", err)
		}
	})
}

// The test that matters most: many requests for the same user at the same instant must never
// spend more than the balance.
func TestConcurrentReservesNeverOverspend(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	mustGrant(t, db, ada, 100, "grant-ada-0001")

	const workers = 50
	var ok, refused, other atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := reserve(ctx, db, ada, 10, nil, fmt.Sprintf("race-key-%04d", i))
			switch {
			case err == nil:
				ok.Add(1)
			case pgErr(err) != nil && pgErr(err).Code == insufficientCredits:
				refused.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if ok.Load() != 10 || refused.Load() != workers-10 || other.Load() != 0 {
		t.Fatalf("succeeded %d, refused %d, other %d; want 10 / 40 / 0", ok.Load(), refused.Load(), other.Load())
	}
	if got := balance(t, db, ada); got != 0 {
		t.Fatalf("final balance %d, want 0 (never negative)", got)
	}
	if n := count(t, db, `SELECT count(*) FROM credit_ledger WHERE kind = 'reserve'`); n != 10 {
		t.Fatalf("%d reserve rows, want 10", n)
	}
}

func TestConcurrentReplaysChargeOnce(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	ctx := ctxT(t)
	ada := mustUser(t, db, "ada@example.invalid")
	mustGrant(t, db, ada, 100, "grant-ada-0001")

	const workers = 30
	ids := make(chan string, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := reserve(ctx, db, ada, 40, nil, "same-request-key")
			if err != nil {
				t.Errorf("replay failed: %v", err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	distinct := map[string]bool{}
	for id := range ids {
		distinct[id] = true
	}
	if len(distinct) != 1 {
		t.Fatalf("%d distinct entries for one request, want 1", len(distinct))
	}
	if got := balance(t, db, ada); got != 60 {
		t.Fatalf("charged %d, want 40 once", 100-got)
	}
}
