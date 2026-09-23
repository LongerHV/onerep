// Package storetest provides a migrated temporary database for tests.
package storetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/LongerHV/onerep/internal/store"
)

// New returns a migrated database in t.TempDir(), closed when the test ends.
func New(t testing.TB) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	if err := store.Migrate(path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
