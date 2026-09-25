package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// get sends a GET with extra headers.
func getWith(t *testing.T, c *http.Client, u string, headers ...string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	for i := 0; i < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, read(t, resp)
}

const exercisesTitle = "<title>Exercises · onerep</title>"

func TestPagesAreFullOrPartial(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)

	resp, full := getWith(t, c, srv.URL+"/exercises")
	if !strings.Contains(full, "<html") || !strings.Contains(full, "<nav") || !strings.Contains(full, exercisesTitle) {
		t.Fatalf("direct visit is not a full page:\n%.400s", full)
	}
	if !strings.Contains(resp.Header.Get("Vary"), "HX-Request") {
		t.Fatalf("Vary = %q", resp.Header.Get("Vary"))
	}

	_, partial := getWith(t, c, srv.URL+"/exercises", "HX-Request", "true", "HX-Boosted", "true")
	if strings.Contains(partial, "<html") || strings.Contains(partial, "<nav") ||
		!strings.Contains(partial, exercisesTitle) || !strings.Contains(partial, "Barbell Back Squat") {
		t.Fatalf("htmx navigation is not a partial with a title:\n%.400s", partial)
	}

	// A history-cache miss must rebuild the whole page.
	_, restored := getWith(t, c, srv.URL+"/exercises", "HX-Request", "true", "HX-History-Restore-Request", "true")
	if !strings.Contains(restored, "<html") || !strings.Contains(restored, "<nav") {
		t.Fatal("history restore got a partial")
	}
}

func TestLayoutKeepsStatusCodes(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)

	resp, full := getWith(t, c, srv.URL+"/exercises/nope")
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(full, "<html") {
		t.Fatalf("full 404: %d", resp.StatusCode)
	}
	resp, partial := getWith(t, c, srv.URL+"/nope", "HX-Request", "true")
	if resp.StatusCode != http.StatusNotFound || strings.Contains(partial, "<html") || !strings.Contains(partial, "Page not found") {
		t.Fatalf("partial 404: %d\n%.300s", resp.StatusCode, partial)
	}

	// A boosted form with errors comes back as a 422 partial.
	form := url.Values{"csrf_token": {csrf}, "name": {""}, "slug": {"x"}, "measurement": {"weight_reps"}, "equipment_kind": {"barbell"}}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL+"/exercises", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Boosted", "true")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body := read(t, resp)
	if resp.StatusCode != http.StatusUnprocessableEntity || strings.Contains(body, "<html") || !strings.Contains(body, "name is required") {
		t.Fatalf("partial 422: %d\n%.300s", resp.StatusCode, body)
	}
	// An error must not replace the URL of the page the user was on.
	if resp.Header.Get("HX-Push-Url") != "false" {
		t.Fatalf("HX-Push-Url = %q", resp.Header.Get("HX-Push-Url"))
	}
}

func TestFragmentsAndRedirectsPassThrough(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	frag := htmx(t, c, srv.URL+"/exercises?q=squat", "exercise-results")
	if strings.Contains(frag, "<title>") || strings.Contains(frag, "<html") {
		t.Fatalf("fragment was wrapped:\n%.300s", frag)
	}
	resp, _ := post(t, c, srv.URL+"/settings", csrf, url.Values{"unit": {"kg"}, "e1rm_window_days": {"30"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/settings?saved" {
		t.Fatalf("redirect: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := mustGet(t, c, srv.URL+"/healthz"); resp.Header.Get("Vary") != "" {
		t.Fatal("non-page responses must not vary on HX-Request")
	}
}

func TestLayoutBoostsNavigation(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)
	full := read(t, mustGet(t, c, srv.URL+"/"))
	for _, want := range []string{
		`hx-boost="true"`, `hx-target="#main"`, `id="main"`, `name="htmx-config"`,
		`action="/auth/logout" hx-boost="false"`, `src="/static/js/plan-form.js"`,
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("layout misses %s", want)
		}
	}
}

func TestPanicPageHasTheLayout(t *testing.T) {
	s := &Server{}
	h := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "<html") ||
		!strings.Contains(rec.Body.String(), "Something went wrong") {
		t.Fatalf("got %d:\n%.300s", rec.Code, rec.Body.String())
	}
}
