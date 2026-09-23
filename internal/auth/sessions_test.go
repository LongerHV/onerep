package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

func newSessions(t *testing.T) (*Sessions, *clock, store.User) {
	t.Helper()
	db := storetest.New(t)
	user, err := db.UpsertOIDCUser(context.Background(), "iss", "sub", "a@example.com", "Alice")
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	return &Sessions{Store: db, Now: c.Now}, c, user
}

// whoami responds with the authenticated user's name, or "anonymous".
var whoami = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if id, ok := FromContext(r.Context()); ok {
		_, _ = w.Write([]byte(id.User.Name))
		return
	}
	_, _ = w.Write([]byte("anonymous"))
})

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatal("no session cookie set")
	return nil
}

func get(h http.Handler, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSessionLifecycle(t *testing.T) {
	s, clk, user := newSessions(t)
	h := s.Middleware(whoami)

	rec := httptest.NewRecorder()
	id, err := s.Start(context.Background(), rec, user)
	if err != nil {
		t.Fatal(err)
	}
	if id.CSRFToken == "" {
		t.Fatal("empty CSRF token")
	}
	c := sessionCookie(t, rec)
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie attributes: %+v", c)
	}

	if body := get(h, c).Body.String(); body != "Alice" {
		t.Fatalf("with cookie: %q", body)
	}
	if body := get(h, &http.Cookie{Name: CookieName, Value: "forged"}).Body.String(); body != "anonymous" {
		t.Fatalf("forged cookie: %q", body)
	}

	// 29 days later the session is still valid and gets extended.
	clk.t = clk.t.Add(29 * 24 * time.Hour)
	rec = get(h, c)
	if rec.Body.String() != "Alice" {
		t.Fatal("session should still be valid after 29 days")
	}
	sessionCookie(t, rec) // refreshed cookie

	// 29 more days: valid only because it was extended.
	clk.t = clk.t.Add(29 * 24 * time.Hour)
	if get(h, c).Body.String() != "Alice" {
		t.Fatal("sliding expiry did not extend the session")
	}

	// 31 idle days: expired.
	clk.t = clk.t.Add(31 * 24 * time.Hour)
	if get(h, c).Body.String() != "anonymous" {
		t.Fatal("session should have expired")
	}
}

func TestSessionEnd(t *testing.T) {
	s, _, user := newSessions(t)
	rec := httptest.NewRecorder()
	if _, err := s.Start(context.Background(), rec, user); err != nil {
		t.Fatal(err)
	}
	c := sessionCookie(t, rec)

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(c)
	out := httptest.NewRecorder()
	if err := s.End(out, req); err != nil {
		t.Fatal(err)
	}
	if cleared := sessionCookie(t, out); cleared.MaxAge >= 0 {
		t.Fatalf("cookie not cleared: %+v", cleared)
	}
	if get(s.Middleware(whoami), c).Body.String() != "anonymous" {
		t.Fatal("session still valid after End")
	}
}
