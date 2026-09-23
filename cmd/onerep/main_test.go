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
