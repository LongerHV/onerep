package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/oauth2-proxy/mockoidc"

	"github.com/LongerHV/onerep/internal/auth"
)

// newOIDCApp serves the app with a mock identity provider and no dev bypass.
func newOIDCApp(t *testing.T) (*httptest.Server, *http.Client) {
	t.Helper()
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	s := newServer(t, "")
	s.OIDC, err = auth.NewOIDC(context.Background(), m.Issuer(), m.ClientID, m.ClientSecret, "http://example.test", s.Sessions)
	if err != nil {
		t.Fatal(err)
	}
	return serve(t, s)
}

func TestLoginErrorIsAPage(t *testing.T) {
	srv, c := newOIDCApp(t)

	resp, html := getWith(t, c, srv.URL+"/auth/callback?error=access_denied&state=x")
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(html, "<html") ||
		!strings.Contains(html, "<title>Sign-in failed · onerep</title>") || !strings.Contains(html, "cancelled") {
		t.Fatalf("access_denied: %d\n%s", resp.StatusCode, html)
	}
	if !strings.Contains(html, `<a href="/auth/login" class="underline" hx-boost="false">Sign in again</a>`) || !strings.Contains(html, "Sign in again") {
		t.Fatalf("no way to sign in again:\n%s", html)
	}

	resp, html = getWith(t, c, srv.URL+"/auth/callback?code=x&state=unknown")
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(html, "<html") || !strings.Contains(html, "Sign in again") {
		t.Fatalf("unknown state: %d\n%s", resp.StatusCode, html)
	}
}

// Signing out with an expired session (or none) just shows the signed-out page.
func TestLogoutWithoutSession(t *testing.T) {
	srv, c := newOIDCApp(t)
	resp, err := c.PostForm(srv.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout without a session: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	u, _ := url.Parse(srv.URL)
	c.Jar.SetCookies(u, []*http.Cookie{{Name: auth.CookieName, Value: "expired"}})
	resp, err = c.PostForm(srv.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout with an expired session: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}
