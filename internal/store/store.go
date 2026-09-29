// Package store is the SQLite persistence layer. All SQL lives here.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNotFound is returned when a requested row does not exist (or belongs to another user).
var ErrNotFound = errors.New("not found")

// timeFormat is how timestamps are stored: UTC RFC 3339 with milliseconds.
const timeFormat = "2006-01-02T15:04:05.000Z07:00"

func formatTime(t time.Time) string { return t.UTC().Format(timeFormat) }

func parseTime(s string) (time.Time, error) { return time.Parse(timeFormat, s) }

// DB holds a single-connection write handle and a pooled read handle to the same file.
type DB struct {
	path  string
	write *sql.DB
	read  *sql.DB
}

// dsn returns the connection string for application connections (foreign keys enforced).
func dsn(path string, extra ...string) string {
	return dsnWithForeignKeys(path, true, extra...)
}

func dsnWithForeignKeys(path string, foreignKeys bool, extra ...string) string {
	fk := "foreign_keys(0)"
	if foreignKeys {
		fk = "foreign_keys(1)"
	}
	q := url.Values{}
	for _, p := range []string{fk, "journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)"} {
		q.Add("_pragma", p)
	}
	for i := 0; i < len(extra); i += 2 {
		q.Add(extra[i], extra[i+1])
	}
	return "file:" + path + "?" + q.Encode()
}

// Open opens (creating if needed) the database at path. It does not run migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	write, err := sql.Open("sqlite", dsn(path, "_txlock", "immediate"))
	if err != nil {
		return nil, err
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		write.Close()
		return nil, err
	}
	read.SetMaxOpenConns(4)

	db := &DB{path: path, write: write, read: read}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// Ping checks that the database is reachable.
func (db *DB) Ping(ctx context.Context) error {
	if err := db.write.PingContext(ctx); err != nil {
		return err
	}
	return db.read.PingContext(ctx)
}

func (db *DB) Close() error {
	return errors.Join(db.write.Close(), db.read.Close())
}

// Migrate applies all pending embedded migrations. It uses its own connection
// because golang-migrate closes the handle it is given.
//
// Foreign keys are off on that connection: golang-migrate wraps each migration
// in a transaction, where the PRAGMA foreign_keys=off that Atlas emits around
// table rebuilds is a no-op, so DROP TABLE would cascade-delete child rows.
// Integrity is checked with foreign_key_check once all migrations are applied.
func Migrate(path string) error {
	if err := migrateFS(path, migrationsFS, "migrations"); err != nil {
		return fmt.Errorf("migrate %s: %w", path, err)
	}
	return nil
}

func migrateFS(path string, fsys fs.FS, dir string) error {
	src, err := iofs.New(fsys, dir)
	if err != nil {
		return err
	}
	conn, err := sql.Open("sqlite", dsnWithForeignKeys(path, false))
	if err != nil {
		return err
	}
	driver, err := sqlitemigrate.WithInstance(conn, &sqlitemigrate.Config{})
	if err != nil {
		conn.Close()
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		conn.Close()
		return err
	}
	defer m.Close()
	err = m.Up()
	var dirty migrate.ErrDirty
	if errors.As(err, &dirty) {
		err = recoverDirty(m, src, dirty.Version)
	}
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return checkForeignKeys(path)
}

// recoverDirty resets a "dirty" version left by a migration that failed or was
// interrupted, then retries. golang-migrate's sqlite driver runs each migration
// in one transaction (NoTxWrap is off, and TestMigrationsHaveNoTransactionControl
// keeps COMMIT out of migration files), so a failed migration left the schema at
// the previous version. Only the version row is stale.
func recoverDirty(m *migrate.Migrate, src source.Driver, version int) error {
	prev := database.NilVersion
	if p, err := src.Prev(uint(version)); err == nil {
		prev = int(p)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("dirty version %d: find previous migration: %w", version, err)
	}
	slog.Warn("migration was left dirty by a failed run; its transaction was rolled back, retrying",
		"dirty_version", version, "reset_to", prev)
	if err := m.Force(prev); err != nil {
		return fmt.Errorf("dirty version %d: force version %d: %w", version, prev, err)
	}
	return m.Up()
}

// checkForeignKeys fails if any row references a missing parent.
func checkForeignKeys(path string) error {
	conn, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return err
	}
	defer conn.Close()
	rows, err := conn.QueryContext(context.Background(), "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	var violations []string
	for rows.Next() {
		var table, parent string
		var rowid sql.NullInt64
		var fkid int
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return err
		}
		violations = append(violations, fmt.Sprintf("%s(rowid %d) -> %s", table, rowid.Int64, parent))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(violations) > 0 {
		return fmt.Errorf("foreign key violations after migration: %v", violations)
	}
	return nil
}

// Backup writes a consistent copy of the database to dest, which must not exist.
// VACUUM INTO would silently overwrite an empty file and fails on any other
// with a misleading "file is not a database", so existence is checked first.
func (db *DB) Backup(ctx context.Context, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("backup to %s: file already exists", dest)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("backup to %s: %w", dest, err)
	}
	if _, err := db.write.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("backup to %s: %w", dest, err)
	}
	return nil
}
