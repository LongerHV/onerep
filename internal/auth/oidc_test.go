package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oauth2-proxy/mockoidc"

	"github.com/LongerHV/onerep/internal/store/storetest"
)

// oidcEnv runs a mock identity provider and an onerep-like app using OIDC.
func oidcEnv(t *testing.T) (*mockoidc.MockOIDC, *httptest.Server, *http.Client) {
	t.Helper()
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })

	sessions := &Sessions{Store: storetest.New(t)}
	mux := http.NewServeMux()
	app := httptest.NewServer(sessions.Middleware(mux))
	t.Cleanup(app.Close)

	cfg := m.Config()
	o, err := NewOIDC(context.Background(), m.Issuer(), cfg.ClientID, cfg.ClientSecret, app.URL, sessions)
	if err != nil {
		t.Fatal(err)
	}
	mux.HandleFunc("GET /auth/login", o.Login)
	mux.HandleFunc("GET /auth/callback", o.Callback)
	mux.Handle("GET /", RequireUser(whoami))

	jar, _ := cookiejar.New(nil)
	return m, app, &http.Client{Jar: jar}
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOIDCLoginFlow(t *testing.T) {
	m, app, client := oidcEnv(t)
	m.QueueUser(&mockoidc.MockUser{Subject: "u-1", Email: "jane@example.com", PreferredUsername: "jane"})

	// Unauthenticated page -> login -> IdP -> callback -> original page.
	resp, err := client.Get(app.URL + "/history?page=2")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "jane" {
		t.Fatalf("after login: %d %q", resp.StatusCode, got)
	}
	if resp.Request.URL.RequestURI() != "/history?page=2" {
		t.Fatalf("landed on %s, want original page", resp.Request.URL.RequestURI())
	}

	// Session persists without another IdP round trip.
	resp, err = client.Get(app.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "jane" {
		t.Fatalf("second request: %q", got)
	}
}

func TestOIDCCallbackRejectsStateMismatch(t *testing.T) {
	_, app, client := oidcEnv(t)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	resp, err := client.Get(app.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	resp, err = client.Get(app.URL + "/auth/callback?code=x&state=forged")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(got, "state mismatch") {
		t.Fatalf("got %d %q", resp.StatusCode, got)
	}
}

func TestOIDCCallbackWithoutFlowCookie(t *testing.T) {
	_, app, client := oidcEnv(t)
	resp, err := client.Get(app.URL + "/auth/callback?code=x&state=y")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(got, "invalid login state") {
		t.Fatalf("got %d %q", resp.StatusCode, got)
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "/",
		"/plans":               "/plans",
		"/plans?x=1":           "/plans?x=1",
		"//evil.example.com":   "/",
		"/\\evil.example.com":  "/",
		"https://evil.example": "/",
		"/\t/evil.example.com": "/",
		"/\n/evil.example.com": "/",
		"/\x7f/evil.example":   "/",
		"/plans\\x":            "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOIDCLoginWithOnlySubject(t *testing.T) {
	m, app, client := oidcEnv(t)
	m.QueueUser(&mockoidc.MockUser{Subject: "only-sub"})
	resp, err := client.Get(app.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); resp.StatusCode != http.StatusOK || got != "only-sub" {
		t.Fatalf("got %d %q", resp.StatusCode, got)
	}
}
