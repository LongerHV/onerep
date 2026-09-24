package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAPITokens(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	tok, err := db.CreateAPIToken(ctx, alice.ID, "laptop", "hash-1", t0)
	if err != nil || tok.Name != "laptop" || tok.Scope != "mcp" || tok.LastUsedAt != nil || !tok.CreatedAt.Equal(t0) {
		t.Fatalf("created %+v, %v", tok, err)
	}
	if _, err := db.CreateAPIToken(ctx, alice.ID, "second", "hash-2", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	u, err := db.UserByAPIToken(ctx, "hash-1", t0.Add(time.Minute))
	if err != nil || u.ID != alice.ID {
		t.Fatalf("user by token = %+v, %v", u, err)
	}
	list, _ := db.APITokens(ctx, alice.ID)
	if len(list) != 2 || list[0].Name != "second" || list[1].LastUsedAt == nil || !list[1].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("tokens = %+v", list)
	}
	// Uses within a minute of the last recorded one don't write.
	_, _ = db.UserByAPIToken(ctx, "hash-1", t0.Add(90*time.Second))
	list, _ = db.APITokens(ctx, alice.ID)
	if !list[1].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("last used moved within a minute: %v", list[1].LastUsedAt)
	}
	_, _ = db.UserByAPIToken(ctx, "hash-1", t0.Add(3*time.Minute))
	list, _ = db.APITokens(ctx, alice.ID)
	if !list[1].LastUsedAt.Equal(t0.Add(3 * time.Minute)) {
		t.Fatalf("last used = %v, want +3m", list[1].LastUsedAt)
	}

	if err := db.RevokeAPIToken(ctx, bob.ID, tok.ID, t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob revoking alice's token: %v", err)
	}
	if err := db.RevokeAPIToken(ctx, alice.ID, tok.ID, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeAPIToken(ctx, alice.ID, tok.ID, t0.Add(5*time.Minute)); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	if _, err := db.UserByAPIToken(ctx, "hash-1", t0.Add(6*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked token still works: %v", err)
	}
	if _, err := db.UserByAPIToken(ctx, "nope", t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	list, _ = db.APITokens(ctx, alice.ID)
	if list[1].RevokedAt == nil || !list[1].RevokedAt.Equal(t0.Add(4*time.Minute)) {
		t.Fatalf("revoked_at = %v, want the first revocation", list[1].RevokedAt)
	}
	if other, _ := db.APITokens(ctx, bob.ID); len(other) != 0 {
		t.Fatalf("bob sees %d tokens", len(other))
	}
}
