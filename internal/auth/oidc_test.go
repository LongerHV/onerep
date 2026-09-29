package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oauth2-proxy/mockoidc"

	"github.com/LongerHV/onerep/internal/store/storetest"
)

// oidcEnv runs a mock identity provider and an onerep-like app using OIDC.
func oidcEnv(t *testing.T) (*mockoidc.MockOIDC, *httptest.Server, *http.Client) {
	t.Helper()
	return oidcEnvWith(t, nil)
}

// oidcEnvWith is oidcEnv with fail rendering the callback's error pages.
func oidcEnvWith(t *testing.T, fail LoginFailure) (*mockoidc.MockOIDC, *httptest.Server, *http.Client) {
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
	mux.HandleFunc("GET /auth/callback", o.Callback(fail))
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
	if got := body(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(got, "sign-in has expired") {
		t.Fatalf("got %d %q", resp.StatusCode, got)
	}
}

func TestOIDCCallbackWithoutFlowCookie(t *testing.T) {
	_, app, client := oidcEnv(t)
	resp, err := client.Get(app.URL + "/auth/callback?code=x&state=y")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); resp.StatusCode != http.StatusBadRequest || !strings.Contains(got, "sign-in has expired") {
		t.Fatalf("got %d %q", resp.StatusCode, got)
	}
}

// startLogin begins a login at the app and returns the identity provider URL
// it redirects to, without following it.
func startLogin(t *testing.T, app *httptest.Server, client *http.Client, next string) string {
	t.Helper()
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := c.Get(app.URL + "/auth/login?next=" + url.QueryEscape(next))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

func flowCookies(t *testing.T, app *httptest.Server, client *http.Client) int {
	t.Helper()
	u, _ := url.Parse(app.URL + "/auth/callback")
	n := 0
	for _, c := range client.Jar.Cookies(u) {
		if strings.HasPrefix(c.Name, flowCookie) {
			n++
		}
	}
	return n
}

// Two tabs signing in at once each keep their own flow state.
func TestOIDCTwoLoginsAtOnce(t *testing.T) {
	m, app, client := oidcEnv(t)
	idpA := startLogin(t, app, client, "/a")
	idpB := startLogin(t, app, client, "/b")
	m.QueueUser(&mockoidc.MockUser{Subject: "u-1", PreferredUsername: "jane"})
	m.QueueUser(&mockoidc.MockUser{Subject: "u-1", PreferredUsername: "jane"})
	for _, tc := range []struct{ idp, next string }{{idpA, "/a"}, {idpB, "/b"}} {
		resp, err := client.Get(tc.idp)
		if err != nil {
			t.Fatal(err)
		}
		if got := body(t, resp); resp.StatusCode != http.StatusOK || got != "jane" || resp.Request.URL.Path != tc.next {
			t.Fatalf("login to %s: %d %q at %s", tc.next, resp.StatusCode, got, resp.Request.URL)
		}
	}
	if n := flowCookies(t, app, client); n != 0 {
		t.Errorf("%d flow cookies left behind after their logins finished", n)
	}
}

// Abandoned logins don't pile up cookies: only the newest few flows are kept.
func TestOIDCFlowCookiesAreBounded(t *testing.T) {
	m, app, client := oidcEnv(t)
	var last string
	for i := range 3 * maxLoginFlows {
		last = startLogin(t, app, client, "/n"+strconv.Itoa(i))
	}
	if n := flowCookies(t, app, client); n == 0 || n > maxLoginFlows {
		t.Fatalf("%d flow cookies, want 1..%d", n, maxLoginFlows)
	}
	m.QueueUser(&mockoidc.MockUser{Subject: "u-1", PreferredUsername: "jane"})
	resp, err := client.Get(last)
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "jane" {
		t.Fatalf("newest login: %d %q", resp.StatusCode, got)
	}
}

// Errors go to the error page renderer with a user-facing message and the
// page to return to after signing in again.
func TestOIDCCallbackErrorPage(t *testing.T) {
	var got LoginError
	fail := func(w http.ResponseWriter, _ *http.Request, e LoginError) {
		got = e
		w.WriteHeader(e.Status)
	}
	_, app, client := oidcEnvWith(t, fail)
	idp := startLogin(t, app, client, "/plans")
	state := mustQuery(t, idp).Get("state")

	resp, err := client.Get(app.URL + "/auth/callback?error=access_denied&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || got.Status != http.StatusForbidden ||
		!strings.Contains(got.Message, "cancelled") || got.Next != "/plans" {
		t.Fatalf("access_denied: %d %+v", resp.StatusCode, got)
	}
	if n := flowCookies(t, app, client); n != 0 {
		t.Errorf("%d flow cookies left behind after a refused login", n)
	}

	got = LoginError{}
	resp, err = client.Get(app.URL + "/auth/callback?code=x&state=unknown")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got.Status != http.StatusBadRequest || !strings.Contains(got.Message, "sign-in has expired") || got.Next != "/" {
		t.Fatalf("unknown state: %+v", got)
	}
}

func mustQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
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

// A public client (no secret) authenticates at the token endpoint with its
// client_id and the PKCE verifier only. mockoidc always wants a secret, so
// a proxy records what onerep sends, then adds the secret for mockoidc.
func TestOIDCPublicClientLoginFlow(t *testing.T) {
	m, err := mockoidc.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Shutdown() })
	var sent url.Values
	var authHeader string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sent, authHeader = r.PostForm, r.Header.Get("Authorization")
		form := url.Values{}
		for k, v := range r.PostForm {
			form[k] = v
		}
		form.Set("client_secret", m.ClientSecret)
		resp, err := http.PostForm(m.TokenEndpoint(), form)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	t.Cleanup(proxy.Close)

	sessions := &Sessions{Store: storetest.New(t)}
	mux := http.NewServeMux()
	app := httptest.NewServer(sessions.Middleware(mux))
	t.Cleanup(app.Close)
	o, err := NewOIDC(context.Background(), m.Issuer(), m.ClientID, "", app.URL, sessions)
	if err != nil {
		t.Fatal(err)
	}
	o.oauth.Endpoint.TokenURL = proxy.URL
	mux.HandleFunc("GET /auth/login", o.Login)
	mux.HandleFunc("GET /auth/callback", o.Callback(nil))
	mux.Handle("GET /", RequireUser(whoami))
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	m.QueueUser(&mockoidc.MockUser{Subject: "u-pub", PreferredUsername: "pat"})
	resp, err := client.Get(app.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "pat" {
		t.Fatalf("after login: %d %q", resp.StatusCode, got)
	}
	if authHeader != "" || sent.Has("client_secret") {
		t.Errorf("a public client must not send a secret: Authorization %q, client_secret %q", authHeader, sent.Get("client_secret"))
	}
	if sent.Get("client_id") != m.ClientID || sent.Get("code_verifier") == "" {
		t.Errorf("token request = %v, want client_id and the PKCE verifier", sent)
	}
}

// claimlessUser is a provider user like Authelia's (4.39+) default: the ID
// token carries only the standard claims, and the profile (name, email) is
// served by the UserInfo endpoint, for subject userinfoSub.
type claimlessUser struct {
	*mockoidc.MockUser
	userinfoSub string
}

func (u claimlessUser) Claims(_ []string, claims *mockoidc.IDTokenClaims) (jwt.Claims, error) {
	return claims, nil
}

func (u claimlessUser) Userinfo([]string) ([]byte, error) {
	return json.Marshal(map[string]string{"sub": u.userinfoSub, "preferred_username": u.PreferredUsername, "email": u.Email})
}

func TestOIDCNameFromUserinfo(t *testing.T) {
	m, app, client := oidcEnv(t)
	m.QueueUser(claimlessUser{&mockoidc.MockUser{Subject: "opaque-uuid", PreferredUsername: "pat", Email: "pat@example.com"}, "opaque-uuid"})
	resp, err := client.Get(app.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "pat" {
		t.Fatalf("name = %q, want the preferred_username from UserInfo", got)
	}
}

// UserInfo about another subject is not trusted (OIDC Core 5.3.2).
func TestOIDCUserinfoForAnotherSubjectIsIgnored(t *testing.T) {
	m, app, client := oidcEnv(t)
	m.QueueUser(claimlessUser{&mockoidc.MockUser{Subject: "opaque-uuid", PreferredUsername: "mallory"}, "someone-else"})
	resp, err := client.Get(app.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if got := body(t, resp); got != "opaque-uuid" {
		t.Fatalf("name = %q, want the subject (UserInfo for another subject ignored)", got)
	}
}
