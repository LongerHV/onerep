package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestDB returns a migrated database in a temporary directory.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	if err := Migrate(path); err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	for range 2 {
		if err := Migrate(path); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenAppliesPragmas(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	var fk int
	if err := db.read.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, %v", fk, err)
	}
	var mode string
	if err := db.write.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q, %v", mode, err)
	}
}

func TestBackup(t *testing.T) {
	db := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "backup.db")
	if err := db.Backup(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	backup, err := Open(context.Background(), dest)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var n int
	if err := backup.read.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("backup is missing schema: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatal(err)
	}
}

func TestBackupRefusesToOverwrite(t *testing.T) {
	db := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(dest, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Backup(context.Background(), dest); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
	if b, _ := os.ReadFile(dest); string(b) != "keep me" {
		t.Fatal("existing file was modified")
	}
}

func TestOpenAndMigrateMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no", "such", "dir", "onerep.db")
	if err := Migrate(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Migrate: want error naming %s, got %v", path, err)
	}
	if _, err := Open(context.Background(), path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Open: want error naming %s, got %v", path, err)
	}
}
