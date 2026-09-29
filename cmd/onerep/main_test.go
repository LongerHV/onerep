package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mistyped ONEREP_DB (or an unmounted volume) must fail the backup instead
// of creating an empty database and "successfully" backing it up.
func TestBackupMissingDatabaseFails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "typo.db")
	dest := filepath.Join(dir, "backup.db")
	t.Setenv("ONEREP_ENV", "dev")
	t.Setenv("ONEREP_DEV_USER", "alice")
	t.Setenv("ONEREP_DB", src)

	err := run(context.Background(), []string{"backup", dest})
	if err == nil || !strings.Contains(err.Error(), src) {
		t.Fatalf("want error naming %s, got %v", src, err)
	}
	for _, p := range []string{src, dest} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s was created", p)
		}
	}
}

// clearServerEnv unsets everything the serve command needs beyond the database.
func clearServerEnv(t *testing.T) {
	for _, k := range []string{"ONEREP_ENV", "ONEREP_DEV_USER", "ONEREP_BASE_URL",
		"ONEREP_OIDC_ISSUER", "ONEREP_OIDC_CLIENT_ID", "ONEREP_OIDC_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
}

// migrate runs with nothing but the database path: no base URL, no OIDC.
func TestMigrateNeedsOnlyTheDatabase(t *testing.T) {
	clearServerEnv(t)
	db := filepath.Join(t.TempDir(), "onerep.db")
	t.Setenv("ONEREP_DB", db)

	if err := run(context.Background(), []string{"migrate"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db); err != nil {
		t.Fatalf("database not created: %v", err)
	}
}

func TestServeRequiresServerSettings(t *testing.T) {
	clearServerEnv(t)
	t.Setenv("ONEREP_DB", filepath.Join(t.TempDir(), "onerep.db"))

	err := run(context.Background(), []string{"serve"})
	if err == nil || !strings.Contains(err.Error(), "ONEREP_BASE_URL") {
		t.Fatalf("want ONEREP_BASE_URL error, got %v", err)
	}
}
