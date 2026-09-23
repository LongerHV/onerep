// Package auth handles login (OIDC and the dev bypass), cookie sessions, and CSRF.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"time"

	"github.com/LongerHV/onerep/internal/store"
)

const (
	// CookieName is the session cookie.
	CookieName = "onerep_session"
	// SessionTTL is the sliding lifetime of a session.
	SessionTTL = 30 * 24 * time.Hour
	// extendAfter limits session expiry writes to at most one per hour per session.
	extendAfter = time.Hour
)

// Store is the persistence the auth package needs. *store.DB implements it.
type Store interface {
	UpsertOIDCUser(ctx context.Context, issuer, subject, email, name string) (store.User, error)
	CreateAuthSession(ctx context.Context, s store.AuthSession) error
	AuthSessionByHash(ctx context.Context, idHash string, now time.Time) (store.AuthSession, store.User, error)
	ExtendAuthSession(ctx context.Context, idHash string, expiresAt time.Time) error
	DeleteAuthSession(ctx context.Context, idHash string) error
}

// Identity is the authenticated user of a request.
type Identity struct {
	User      store.User
	CSRFToken string
}

type identityKey struct{}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// FromContext returns the request's identity, if the request is authenticated.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(identityKey{}).(Identity)
	return id, ok
}

// Sessions issues and validates cookie sessions.
type Sessions struct {
	Store  Store
	Secure bool             // set the Secure cookie attribute
	Now    func() time.Time // defaults to time.Now
}

func (s *Sessions) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the value stored in the database for a session or API token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Start creates a session for user and sets the session cookie.
func (s *Sessions) Start(ctx context.Context, w http.ResponseWriter, user store.User) (Identity, error) {
	token, err := randomToken()
	if err != nil {
		return Identity{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return Identity{}, err
	}
	now := s.now()
	err = s.Store.CreateAuthSession(ctx, store.AuthSession{
		IDHash:    HashToken(token),
		UserID:    user.ID,
		CSRFToken: csrf,
		ExpiresAt: now.Add(SessionTTL),
		CreatedAt: now,
	})
	if err != nil {
		return Identity{}, err
	}
	s.setCookie(w, token, now.Add(SessionTTL))
	return Identity{User: user, CSRFToken: csrf}, nil
}

func (s *Sessions) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// End deletes the current session (if any) and clears the cookie.
func (s *Sessions) End(w http.ResponseWriter, r *http.Request) error {
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.Secure, SameSite: http.SameSiteLaxMode,
	})
	c, err := r.Cookie(CookieName)
	if err != nil {
		return nil
	}
	return s.Store.DeleteAuthSession(r.Context(), HashToken(c.Value))
}

// Middleware attaches the Identity of a valid session cookie to the request
// context and slides the session expiry. Requests without a valid session pass
// through unauthenticated.
func (s *Sessions) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		now := s.now()
		hash := HashToken(c.Value)
		sess, user, err := s.Store.AuthSessionByHash(r.Context(), hash, now)
		if errors.Is(err, store.ErrNotFound) {
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			http.Error(w, "session lookup failed", http.StatusInternalServerError)
			return
		}
		if newExp := now.Add(SessionTTL); newExp.Sub(sess.ExpiresAt) > extendAfter {
			if err := s.Store.ExtendAuthSession(r.Context(), hash, newExp); err == nil {
				s.setCookie(w, c.Value, newExp)
			}
		}
		ctx := WithIdentity(r.Context(), Identity{User: user, CSRFToken: sess.CSRFToken})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
