package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
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

// A migration that rebuilds a parent table (Atlas does this for most column
// changes) must not cascade-delete child rows.
func TestTableRebuildMigrationKeepsChildRows(t *testing.T) {
	fsys := fstest.MapFS{
		"m/1_init.up.sql": {Data: []byte(`
			CREATE TABLE parents (id TEXT NOT NULL PRIMARY KEY, v TEXT NOT NULL);
			CREATE TABLE children (id TEXT NOT NULL PRIMARY KEY,
				parent_id TEXT NOT NULL REFERENCES parents (id) ON DELETE CASCADE);`)},
		"m/2_seed.up.sql": {Data: []byte(`
			INSERT INTO parents VALUES ('p', 'x');
			INSERT INTO children VALUES ('c', 'p');`)},
		"m/3_rebuild.up.sql": {Data: []byte(`
			PRAGMA foreign_keys = off;
			CREATE TABLE new_parents (id TEXT NOT NULL PRIMARY KEY, v TEXT NOT NULL, w TEXT);
			INSERT INTO new_parents (id, v) SELECT id, v FROM parents;
			DROP TABLE parents;
			ALTER TABLE new_parents RENAME TO parents;
			PRAGMA foreign_keys = on;`)},
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := migrateFS(path, fsys, "m"); err != nil {
		t.Fatal(err)
	}
	db, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.read.QueryRow("SELECT count(*) FROM children").Scan(&n); err != nil || n != 1 {
		t.Fatalf("children after rebuild = %d, %v; want 1", n, err)
	}
}

func TestMigrationLeavingDanglingForeignKeysFails(t *testing.T) {
	fsys := fstest.MapFS{
		"m/1_init.up.sql": {Data: []byte(`
			CREATE TABLE parents (id TEXT NOT NULL PRIMARY KEY);
			CREATE TABLE children (id TEXT NOT NULL PRIMARY KEY,
				parent_id TEXT NOT NULL REFERENCES parents (id));
			INSERT INTO children VALUES ('c', 'missing');`)},
	}
	err := migrateFS(filepath.Join(t.TempDir(), "test.db"), fsys, "m")
	if err == nil || !strings.Contains(err.Error(), "foreign key") {
		t.Fatalf("want foreign key violation error, got %v", err)
	}
}
