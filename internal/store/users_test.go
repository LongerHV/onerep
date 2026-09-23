package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestUpsertOIDCUser(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	first, err := db.UpsertOIDCUser(ctx, "https://id", "sub-1", "a@example.com", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.Unit != "kg" || first.E1RMWindowDays != 30 {
		t.Fatalf("unexpected new user: %+v", first)
	}

	again, err := db.UpsertOIDCUser(ctx, "https://id", "sub-1", "alice@example.com", "Alice A.")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != first.ID {
		t.Fatalf("same (issuer, sub) must keep the user id: %s != %s", again.ID, first.ID)
	}
	if again.Email != "alice@example.com" || again.Name != "Alice A." {
		t.Fatalf("profile not refreshed: %+v", again)
	}

	other, err := db.UpsertOIDCUser(ctx, "https://other-id", "sub-1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == first.ID {
		t.Fatal("same subject from a different issuer must be a different user")
	}

	got, err := db.UserByID(ctx, first.ID)
	if err != nil || got.Email != "alice@example.com" {
		t.Fatalf("UserByID = %+v, %v", got, err)
	}
	if _, err := db.UserByID(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestAuthSessions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u, err := db.UpsertOIDCUser(ctx, "iss", "sub", "", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	s := AuthSession{IDHash: "h1", UserID: u.ID, CSRFToken: "csrf", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	if err := db.CreateAuthSession(ctx, s); err != nil {
		t.Fatal(err)
	}

	gotS, gotU, err := db.AuthSessionByHash(ctx, "h1", now)
	if err != nil {
		t.Fatal(err)
	}
	if gotS.CSRFToken != "csrf" || !gotS.ExpiresAt.Equal(s.ExpiresAt) || gotU.ID != u.ID {
		t.Fatalf("got %+v / %+v", gotS, gotU)
	}

	if _, _, err := db.AuthSessionByHash(ctx, "h1", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session must be ErrNotFound, got %v", err)
	}

	if err := db.ExtendAuthSession(ctx, "h1", now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AuthSessionByHash(ctx, "h1", now.Add(2*time.Hour)); err != nil {
		t.Fatalf("extended session must be valid: %v", err)
	}

	n, err := db.DeleteExpiredAuthSessions(ctx, now.Add(4*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpiredAuthSessions = %d, %v", n, err)
	}

	if err := db.CreateAuthSession(ctx, AuthSession{IDHash: "h2", UserID: u.ID, CSRFToken: "c", ExpiresAt: now.Add(time.Hour), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAuthSession(ctx, "h2"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.AuthSessionByHash(ctx, "h2", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session must be ErrNotFound, got %v", err)
	}
}

func TestAuthSessionRequiresExistingUser(t *testing.T) {
	db := newTestDB(t)
	now := time.Now()
	err := db.CreateAuthSession(context.Background(), AuthSession{IDHash: "x", UserID: "nope", CSRFToken: "c", ExpiresAt: now, CreatedAt: now})
	if err == nil {
		t.Fatal("foreign key must reject a session for an unknown user")
	}
}

func TestConcurrentWritesDoNotFail(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	errs := make(chan error, 50)
	for i := range 50 {
		go func() {
			_, err := db.UpsertOIDCUser(ctx, "iss", fmt.Sprintf("sub-%d", i%5), "", "")
			errs <- err
		}()
	}
	for range 50 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent upsert: %v", err)
		}
	}
}
