package web

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

// newApp serves the full router with the given dev user ("" disables the bypass).
func newApp(t *testing.T, devUser string) (*httptest.Server, *http.Client) {
	t.Helper()
	db := storetest.New(t)
	s := &Server{DB: db, Sessions: &auth.Sessions{Store: db}, DevUser: devUser}
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return srv, client
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
