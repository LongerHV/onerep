package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/store"
)

func withIdentity(r *http.Request, csrf string) *http.Request {
	return r.WithContext(WithIdentity(r.Context(), Identity{User: store.User{Name: "Alice"}, CSRFToken: csrf}))
}

func TestRequireUser(t *testing.T) {
	h := RequireUser(whoami)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/plans?x=1", nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/auth/login?next=%2Fplans%3Fx%3D1" {
		t.Fatalf("page load: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	req := httptest.NewRequest(http.MethodGet, "/plans", nil)
	req.Header.Set("HX-Request", "true")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized || !strings.HasPrefix(rec.Header().Get("HX-Redirect"), "/auth/login") {
		t.Fatalf("htmx: %d %q", rec.Code, rec.Header().Get("HX-Redirect"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/sync", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("api: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withIdentity(httptest.NewRequest(http.MethodGet, "/", nil), "t"))
	if rec.Code != http.StatusOK || rec.Body.String() != "Alice" {
		t.Fatalf("authenticated: %d %q", rec.Code, rec.Body.String())
	}
}

func TestCSRF(t *testing.T) {
	h := CSRF(whoami)
	cases := []struct {
		name string
		req  func() *http.Request
		want int
	}{
		{"get needs no token", func() *http.Request {
			return withIdentity(httptest.NewRequest(http.MethodGet, "/", nil), "tok")
		}, http.StatusOK},
		{"post without token", func() *http.Request {
			return withIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "tok")
		}, http.StatusForbidden},
		{"post with wrong header", func() *http.Request {
			r := withIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "tok")
			r.Header.Set(CSRFHeader, "nope")
			return r
		}, http.StatusForbidden},
		{"post with header", func() *http.Request {
			r := withIdentity(httptest.NewRequest(http.MethodPost, "/", nil), "tok")
			r.Header.Set(CSRFHeader, "tok")
			return r
		}, http.StatusOK},
		{"post with form field", func() *http.Request {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{CSRFField: {"tok"}}.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return withIdentity(r, "tok")
		}, http.StatusOK},
		{"delete without token", func() *http.Request {
			return withIdentity(httptest.NewRequest(http.MethodDelete, "/", nil), "tok")
		}, http.StatusForbidden},
		{"anonymous post is left to RequireUser", func() *http.Request {
			return httptest.NewRequest(http.MethodPost, "/", nil)
		}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tc.req())
			if rec.Code != tc.want {
				t.Fatalf("got %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestDevLogin(t *testing.T) {
	s, _, _ := newSessions(t)
	h := s.Middleware(DevLogin(s, "bob")(whoami))

	first := get(h)
	if first.Body.String() != "bob" {
		t.Fatalf("dev login: %q", first.Body.String())
	}
	c := sessionCookie(t, first)

	// The issued cookie is a normal session: the next request reuses it
	// instead of creating another session.
	second := get(h, c)
	if second.Body.String() != "bob" {
		t.Fatalf("reuse: %q", second.Body.String())
	}
	for _, rc := range second.Result().Cookies() {
		if rc.Name == CookieName && rc.Value != c.Value {
			t.Fatal("dev login created a second session despite a valid cookie")
		}
	}
}
