package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

func TestTokens(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	u, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	tokens := &Tokens{Store: db}

	if _, _, err := tokens.Create(ctx, u.ID, "   "); !errors.Is(err, ErrTokenName) {
		t.Fatalf("blank name: %v", err)
	}
	if _, _, err := tokens.Create(ctx, u.ID, strings.Repeat("x", 101)); !errors.Is(err, ErrTokenName) {
		t.Fatalf("long name: %v", err)
	}
	secret, tok, err := tokens.Create(ctx, u.ID, "  Claude  ")
	if err != nil || tok.Name != "Claude" || !strings.HasPrefix(secret, TokenPrefix) || len(secret) != len(TokenPrefix)+43 {
		t.Fatalf("created %q %+v %v", secret, tok, err)
	}
	got, err := tokens.Verify(ctx, secret)
	if err != nil || got.ID != u.ID {
		t.Fatalf("verify = %+v, %v", got, err)
	}
	for _, bad := range []string{"", "onerep_", "not-a-token", secret + "x"} {
		if _, err := tokens.Verify(ctx, bad); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("verify(%q) = %v, want not found", bad, err)
		}
	}
	if err := tokens.Revoke(ctx, u.ID, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Verify(ctx, secret); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("revoked token verifies: %v", err)
	}
}
