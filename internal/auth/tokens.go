package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LongerHV/onerep/internal/store"
)

// TokenPrefix starts every API token, so leaked tokens are recognisable.
const TokenPrefix = "onerep_"

// ErrTokenName rejects a blank or overlong token name.
var ErrTokenName = errors.New("give the token a name of at most 100 characters")

// TokenStore is the persistence API tokens need. *store.DB implements it.
type TokenStore interface {
	CreateAPIToken(ctx context.Context, userID, name, hash string, at time.Time) (store.APIToken, error)
	APITokens(ctx context.Context, userID string) ([]store.APIToken, error)
	RevokeAPIToken(ctx context.Context, userID, id string, at time.Time) error
	UserByAPIToken(ctx context.Context, hash string, now time.Time) (store.User, error)
}

// Tokens manages personal access tokens for the MCP server (spec §11).
type Tokens struct {
	Store TokenStore
	Now   func() time.Time // defaults to time.Now
}

func (t *Tokens) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Create makes a token and returns its secret, which is never stored and
// can't be shown again.
func (t *Tokens) Create(ctx context.Context, userID, name string) (string, store.APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", store.APIToken{}, ErrTokenName
	}
	random, err := randomToken()
	if err != nil {
		return "", store.APIToken{}, err
	}
	secret := TokenPrefix + random
	tok, err := t.Store.CreateAPIToken(ctx, userID, name, HashToken(secret), t.now())
	return secret, tok, err
}

func (t *Tokens) List(ctx context.Context, userID string) ([]store.APIToken, error) {
	return t.Store.APITokens(ctx, userID)
}

func (t *Tokens) Revoke(ctx context.Context, userID, id string) error {
	return t.Store.RevokeAPIToken(ctx, userID, id, t.now())
}

// Verify returns the user of an unrevoked token, or store.ErrNotFound.
func (t *Tokens) Verify(ctx context.Context, secret string) (store.User, error) {
	if !strings.HasPrefix(secret, TokenPrefix) || len(secret) <= len(TokenPrefix) {
		return store.User{}, store.ErrNotFound
	}
	return t.Store.UserByAPIToken(ctx, HashToken(secret), t.now())
}
