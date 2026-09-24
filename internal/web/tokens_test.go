package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var secretRe = regexp.MustCompile(`onerep_[A-Za-z0-9_-]{43}`)

func TestTokensPage(t *testing.T) {
	srv, c := newApp(t, "alice")
	page := read(t, mustGet(t, c, srv.URL+"/settings/tokens"))
	csrf := csrfInput.FindStringSubmatch(page)[1]
	if !strings.Contains(read(t, mustGet(t, c, srv.URL+"/settings")), `href="/settings/tokens"`) {
		t.Error("settings page doesn't link to the tokens page")
	}

	resp, err := c.PostForm(srv.URL+"/settings/tokens", url.Values{"name": {"Claude"}, "csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	created := read(t, resp)
	secret := secretRe.FindString(created)
	if resp.StatusCode != http.StatusOK || secret == "" {
		t.Fatalf("create = %d, secret %q", resp.StatusCode, secret)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || !strings.Contains(created, `hx-history="false"`) {
		t.Error("the page showing a token must not be cached by the browser or htmx")
	}
	if !strings.Contains(created, "http://example.test/mcp") || !strings.Contains(created, "Bearer "+secret) {
		t.Error("the page should show how to connect an assistant")
	}

	list := read(t, mustGet(t, c, srv.URL+"/settings/tokens"))
	if secretRe.MatchString(list) || !strings.Contains(list, "Claude") {
		t.Fatal("the token list must name the token but never show its secret again")
	}
	if resp, _ := c.PostForm(srv.URL+"/settings/tokens", url.Values{"name": {" "}, "csrf_token": {csrf}}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank name = %d", resp.StatusCode)
	}

	id := regexp.MustCompile(`/settings/tokens/([0-9a-f-]{36})/revoke`).FindStringSubmatch(list)[1]
	resp, _ = c.PostForm(srv.URL+"/settings/tokens/"+id+"/revoke", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("revoke = %d", resp.StatusCode)
	}
	if !strings.Contains(read(t, mustGet(t, c, srv.URL+"/settings/tokens")), "revoked") {
		t.Error("a revoked token should say so")
	}
	resp, _ = c.PostForm(srv.URL+"/settings/tokens/00000000-0000-7000-8000-000000000000/revoke", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoking an unknown token = %d", resp.StatusCode)
	}
}
