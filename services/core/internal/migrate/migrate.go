// Package migrate applies the embedded SQL migrations and sets the database roles' passwords.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/pressly/goose/v3"

	// Registers the "pgx" database/sql driver.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Gitax18/readmely/services/core/migrations"
)

// Roles the services connect as. Created by the first migration without a password.
const (
	RoleApp = "readmely_app"
	RoleAI  = "readmely_ai"
)

// MinPasswordLength keeps a short or placeholder password from becoming a login.
const MinPasswordLength = 16

func newProvider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
}

// Up applies every pending migration and returns how many ran.
func Up(ctx context.Context, db *sql.DB) (int, error) {
	p, err := newProvider(db)
	if err != nil {
		return 0, err
	}
	results, err := p.Up(ctx)
	return len(results), err
}

// DownAll reverts every migration. Development only; the caller guards production.
func DownAll(ctx context.Context, db *sql.DB) (int, error) {
	p, err := newProvider(db)
	if err != nil {
		return 0, err
	}
	results, err := p.DownTo(ctx, 0)
	return len(results), err
}

// Down reverts the most recent migration. Development only.
func Down(ctx context.Context, db *sql.DB) error {
	p, err := newProvider(db)
	if err != nil {
		return err
	}
	_, err = p.Down(ctx)
	return err
}

// Status writes one line per migration: applied or pending.
func Status(ctx context.Context, db *sql.DB, w io.Writer) error {
	p, err := newProvider(db)
	if err != nil {
		return err
	}
	statuses, err := p.Status(ctx)
	if err != nil {
		return err
	}
	for _, s := range statuses {
		state := "pending"
		if s.State == goose.StateApplied {
			state = "applied"
		}
		if _, err := fmt.Fprintf(w, "%-8s %d %s\n", state, s.Source.Version, baseName(s.Source.Path)); err != nil {
			return err
		}
	}
	return nil
}

func baseName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// RolePassword is one role and the password it should log in with.
type RolePassword struct {
	Role     string
	Password string
}

// ValidatePassword rejects passwords that are too short to be a credential.
func ValidatePassword(role, password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("the password for %s must be at least %d characters", role, MinPasswordLength)
	}
	return nil
}

// SetPasswords lets each role log in with its password. The statement is built by the
// server with format(%I, %L), so neither the role name nor the password can alter it.
// Passwords are never logged or returned in errors.
func SetPasswords(ctx context.Context, db *sql.DB, roles []RolePassword) error {
	for _, r := range roles {
		if err := ValidatePassword(r.Role, r.Password); err != nil {
			return err
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, r := range roles {
		var stmt string
		err := tx.QueryRowContext(ctx,
			`SELECT format('ALTER ROLE %I LOGIN PASSWORD %L', rolname, $2::text) FROM pg_roles WHERE rolname = $1::text`,
			r.Role, r.Password).Scan(&stmt)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("role %s does not exist: run the migrations first", r.Role)
		}
		if err != nil {
			return fmt.Errorf("preparing role %s failed", r.Role)
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("setting the password of %s failed", r.Role)
		}
	}
	return tx.Commit()
}

// Open connects with the owner URL. It never echoes the URL in errors.
func Open(url string) (*sql.DB, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, errors.New("MIGRATE_DATABASE_URL could not be used")
	}
	return db, nil
}
