// Package store is the SQLite persistence layer. All SQL lives here.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/golang-migrate/migrate/v4"
	sqlitemigrate "github.com/golang-migrate/migrate/v4/database/sqlite"
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

func dsn(path string, extra ...string) string {
	q := url.Values{}
	for _, p := range []string{"foreign_keys(1)", "journal_mode(WAL)", "busy_timeout(5000)", "synchronous(NORMAL)"} {
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
func Migrate(path string) error {
	if err := migrateUp(path); err != nil {
		return fmt.Errorf("migrate %s: %w", path, err)
	}
	return nil
}

func migrateUp(path string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	conn, err := sql.Open("sqlite", dsn(path))
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
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Backup writes a consistent copy of the database to dest, which must not exist.
func (db *DB) Backup(ctx context.Context, dest string) error {
	_, err := db.write.ExecContext(ctx, "VACUUM INTO ?", dest)
	return err
}
