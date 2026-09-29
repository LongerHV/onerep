package store

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	err := db.Backup(context.Background(), dest)
	if err == nil || !strings.Contains(err.Error(), "backup to "+dest) || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("want an error saying %s exists, got %v", dest, err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "keep me" {
		t.Fatal("existing file was modified")
	}
}

// VACUUM INTO silently overwrites an empty file.
func TestBackupRefusesToOverwriteEmptyFile(t *testing.T) {
	db := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "empty.db")
	if err := os.WriteFile(dest, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	err := db.Backup(context.Background(), dest)
	if err == nil || !strings.Contains(err.Error(), "backup to "+dest) {
		t.Fatalf("want an error naming %s, got %v", dest, err)
	}
	if fi, _ := os.Stat(dest); fi == nil || fi.Size() != 0 {
		t.Fatal("empty file was overwritten")
	}
}

func TestBackupErrorNamesDestination(t *testing.T) {
	db := newTestDB(t)
	dest := filepath.Join(t.TempDir(), "no", "such", "dir", "backup.db")
	err := db.Backup(context.Background(), dest)
	if err == nil || !strings.Contains(err.Error(), "backup to "+dest) {
		t.Fatalf("want an error naming %s, got %v", dest, err)
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

func migrationState(t *testing.T, path string) (version int, dirty bool) {
	t.Helper()
	conn, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.QueryRow("SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	return version, dirty
}

func tableExists(t *testing.T, path, name string) bool {
	t.Helper()
	conn, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var n int
	if err := conn.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// A failed migration leaves golang-migrate's "dirty" flag set, which used to
// fail every later start. Since each migration runs in a transaction, the
// schema is still at the previous version, so the next run recovers.
func TestMigrateRecoversFromDirtyState(t *testing.T) {
	ok := &fstest.MapFile{Data: []byte(`CREATE TABLE a (id TEXT NOT NULL PRIMARY KEY);`)}
	bad := fstest.MapFS{
		"m/1_a.up.sql": ok,
		"m/2_b.up.sql": {Data: []byte(`
			CREATE TABLE b (id TEXT NOT NULL PRIMARY KEY);
			INSERT INTO no_such_table VALUES (1);`)},
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := migrateFS(path, bad, "m"); err == nil {
		t.Fatal("broken migration succeeded")
	}
	if v, dirty := migrationState(t, path); v != 2 || !dirty {
		t.Fatalf("state = %d dirty=%v, want 2 dirty", v, dirty)
	}
	if tableExists(t, path, "b") {
		t.Fatal("failed migration was not rolled back")
	}

	// Still broken: fails again with the migration's own error, not "dirty".
	err := migrateFS(path, bad, "m")
	if err == nil || !strings.Contains(err.Error(), "no_such_table") {
		t.Fatalf("want the migration's error on retry, got %v", err)
	}

	fixed := fstest.MapFS{
		"m/1_a.up.sql": ok,
		"m/2_b.up.sql": {Data: []byte(`CREATE TABLE b (id TEXT NOT NULL PRIMARY KEY);`)},
	}
	if err := migrateFS(path, fixed, "m"); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if v, dirty := migrationState(t, path); v != 2 || dirty {
		t.Fatalf("state = %d dirty=%v, want 2 clean", v, dirty)
	}
	if !tableExists(t, path, "b") {
		t.Fatal("migration 2 was not applied")
	}
}

func TestMigrateRecoversFromDirtyFirstMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	bad := fstest.MapFS{"m/1_a.up.sql": {Data: []byte(`CREATE TABLE a (id TEXT); SELECT * FROM nope;`)}}
	if err := migrateFS(path, bad, "m"); err == nil {
		t.Fatal("broken migration succeeded")
	}
	if tableExists(t, path, "a") {
		t.Fatal("failed migration was not rolled back")
	}
	fixed := fstest.MapFS{"m/1_a.up.sql": {Data: []byte(`CREATE TABLE a (id TEXT);`)}}
	if err := migrateFS(path, fixed, "m"); err != nil {
		t.Fatalf("recovery failed: %v", err)
	}
	if v, dirty := migrationState(t, path); v != 1 || dirty {
		t.Fatalf("state = %d dirty=%v, want 1 clean", v, dirty)
	}
}

// Recovering from a dirty state relies on each migration being a single
// transaction. A COMMIT (or VACUUM) inside a migration file would break that.
func TestMigrationsHaveNoTransactionControl(t *testing.T) {
	re := regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT|END|ROLLBACK|SAVEPOINT|RELEASE|VACUUM)\b`)
	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations: %v", err)
	}
	for _, f := range files {
		b, err := migrationsFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if m := re.Find(b); m != nil {
			t.Errorf("%s contains %q; migrations must not control transactions", f, strings.TrimSpace(string(m)))
		}
	}
}
