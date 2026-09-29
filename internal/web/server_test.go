package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
	"github.com/LongerHV/onerep/internal/training"
)

// newApp serves the full router with the given dev user ("" disables the bypass).
func newApp(t *testing.T, devUser string) (*httptest.Server, *http.Client) {
	t.Helper()
	srv, c, _ := newAppDB(t, devUser)
	return srv, c
}

// newAppDB is newApp that also returns the database, with the catalog seeded.
func newAppDB(t *testing.T, devUser string) (*httptest.Server, *http.Client, *store.DB) {
	t.Helper()
	db := storetest.New(t)
	if err := exercise.Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := &Server{DB: db, Sessions: &auth.Sessions{Store: db}, DevUser: devUser,
		BaseURL: "http://example.test", Tokens: &auth.Tokens{Store: db},
		Exercises: &exercise.Service{Store: db}, Account: &account.Service{Store: db}}
	s.Plans = &plan.Service{Store: db, Exercises: s.Exercises, History: db}
	s.Training = &training.Service{Store: db, Plans: s.Plans, Exercises: s.Exercises}
	s.Stats = &stats.Service{Store: db, Exercises: s.Exercises}
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return srv, client, db
}

func read(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustGet(t *testing.T, c *http.Client, u string) *http.Response {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

var csrfInput = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func TestHomeWithDevUser(t *testing.T) {
	srv, c := newApp(t, "alice")
	resp := mustGet(t, c, srv.URL+"/")
	html := read(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(html, "Hi, alice") {
		t.Fatalf("home: %d\n%s", resp.StatusCode, html)
	}
	if !strings.Contains(html, `hx-headers="{&#34;X-CSRF-Token&#34;:`) {
		t.Fatalf("htmx CSRF header not configured:\n%s", html)
	}
	if !strings.Contains(html, `href="https://github.com/LongerHV/onerep"`) {
		t.Fatalf("source code link (AGPL section 13) missing:\n%s", html)
	}
}

func TestLogoutRequiresCSRF(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/"))
	m := csrfInput.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no csrf input in page:\n%s", html)
	}

	resp, err := c.PostForm(srv.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without token: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = c.PostForm(srv.URL+"/auth/logout", url.Values{auth.CSRFField: {m[1]}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// The browser drops its offline copy of this user's data (the IndexedDB
	// outbox, cached workout pages), so the next user never sees or syncs it.
	if got := resp.Header.Get("Clear-Site-Data"); got != `"cache", "storage"` {
		t.Fatalf("Clear-Site-Data = %q", got)
	}
}

// Signed-in pages and /api/csrf name their user, so companion.js discards
// offline data another user left and never syncs it under the wrong session.
// The signed-out page asks it to clear offline data, for browsers that
// ignore Clear-Site-Data.
func TestOfflineDataOwnerMarkers(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)
	m := regexp.MustCompile(`data-user="([0-9a-f-]{36})"`).FindStringSubmatch(read(t, mustGet(t, c, srv.URL+"/")))
	if m == nil {
		t.Fatal("signed-in page does not name its user")
	}
	if body := read(t, mustGet(t, c, srv.URL+"/api/csrf")); !strings.Contains(body, `"user_id":"`+m[1]+`"`) {
		t.Fatalf("/api/csrf does not name user %s: %s", m[1], body)
	}
	html := read(t, mustGet(t, c, srv.URL+"/auth/signed-out"))
	if !strings.Contains(html, "data-signed-out") {
		t.Fatalf("signed-out page has no data-signed-out marker:\n%s", html)
	}
}

func TestAnonymousIsRedirectedToLogin(t *testing.T) {
	srv, c := newApp(t, "")
	resp := mustGet(t, c, srv.URL+"/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login") {
		t.Fatalf("got %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestHealthzAndStatic(t *testing.T) {
	srv, c := newApp(t, "")
	resp := mustGet(t, c, srv.URL+"/healthz")
	if body := read(t, resp); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("healthz: %d %q", resp.StatusCode, body)
	}
	for _, path := range []string{"/static/app.css", "/static/vendor/htmx.min.js"} {
		resp := mustGet(t, c, srv.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
}

func TestNotFoundPageShowsRequestID(t *testing.T) {
	srv, c := newApp(t, "alice")
	resp := mustGet(t, c, srv.URL+"/nope")
	html := read(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(html, "Request ID") {
		t.Fatalf("got %d\n%s", resp.StatusCode, html)
	}
}

func TestDevLoginOnlyOnAppRoutes(t *testing.T) {
	srv, c := newApp(t, "alice")
	for _, path := range []string{"/healthz", "/static/app.css"} {
		resp := mustGet(t, c, srv.URL+path)
		resp.Body.Close()
		for _, ck := range resp.Cookies() {
			if ck.Name == auth.CookieName {
				t.Fatalf("%s created a dev session", path)
			}
		}
	}
}

func TestUserNameIsHTMLEscaped(t *testing.T) {
	srv, c := newApp(t, `<img src=x onerror=alert(1)>`)
	html := read(t, mustGet(t, c, srv.URL+"/"))
	if strings.Contains(html, "<img src=x") {
		t.Fatalf("user name rendered unescaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("escaped name missing:\n%s", html)
	}
}
func TestMCPIsMountedWithoutCookiesOrCSRF(t *testing.T) {
	db := storetest.New(t)
	called := false
	sessions := &auth.Sessions{Store: db}
	s := &Server{DB: db, Sessions: sessions, MCP: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})}
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	// A browser's session cookie rides along, but without a CSRF token.
	u, err := db.UpsertOIDCUser(context.Background(), "iss", "alice", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if _, err := sessions.Start(context.Background(), rec, u); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/mcp", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range rec.Result().Cookies() {
		req.AddCookie(ck)
	}
	if len(req.Cookies()) == 0 {
		t.Fatal("no session cookie")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !called || resp.StatusCode != http.StatusTeapot {
		t.Fatalf("/mcp = %d (handler called %v), want the MCP handler without CSRF", resp.StatusCode, called)
	}
}
