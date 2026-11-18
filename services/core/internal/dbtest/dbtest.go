// Package dbtest gives each test its own throwaway database on the real local Postgres, so
// tests can run in parallel, never touch development data, and always start from the
// migrations. Tests skip when DATABASE_URL (an owner connection that may create databases)
// is not set; run them through `make test-integration`.
package dbtest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gitax18/readmely/services/core/internal/migrate"
)

// DB is one throwaway database.
type DB struct {
	Name string
	URL  string // owner connection to this database
	Pool *pgxpool.Pool
}

func adminURL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		t.Skip("DATABASE_URL is not set; run through `make test-integration`")
	}
	return u
}

func withDatabase(t testing.TB, raw, name string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal("DATABASE_URL is not a valid URL")
	}
	u.Path = "/" + name
	return u.String()
}

func randomName(t testing.TB) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "rm_test_" + hex.EncodeToString(b)
}

// NewEmpty creates a database with nothing in it.
func NewEmpty(t testing.TB) *DB {
	t.Helper()
	ctx := context.Background()
	admin := adminURL(t)
	name := randomName(t)

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("cannot connect to the admin database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("cannot create a test database: %v", err)
	}

	d := &DB{Name: name, URL: withDatabase(t, admin, name)}
	pool, err := pgxpool.New(ctx, d.URL)
	if err != nil {
		t.Fatal("cannot open a pool on the test database")
	}
	d.Pool = pool

	t.Cleanup(func() {
		d.Pool.Close()
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			t.Logf("cleanup: cannot connect to drop %s", name)
			return
		}
		defer func() { _ = c.Close(context.Background()) }()
		if _, err := c.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("cleanup: cannot drop %s: %v", name, err)
		}
	})
	return d
}

// New creates a database with every migration applied.
func New(t testing.TB) *DB {
	t.Helper()
	d := NewEmpty(t)
	db, err := migrate.Open(d.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := migrate.Up(ctx, db); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
	return d
}

// SQL opens a database/sql handle (what the migrate package uses) on the test database.
func (d *DB) SQL(t testing.TB) *sql.DB {
	t.Helper()
	db, err := migrate.Open(d.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// AsRole returns a connection that has the privileges of role (and only those): the owner
// connects, then SET ROLE drops to the role for the whole session. This exercises exactly
// what the role may do without needing its password.
func (d *DB) AsRole(t testing.TB, role string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, d.URL)
	if err != nil {
		t.Fatalf("cannot connect: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	if _, err := conn.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatalf("cannot switch to role %s: %v", role, err)
	}
	return conn
}

// URLFor returns a connection URL for a login role on this database.
func (d *DB) URLFor(t testing.TB, user, password string) string {
	t.Helper()
	u, err := url.Parse(d.URL)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(user, password)
	return u.String()
}
