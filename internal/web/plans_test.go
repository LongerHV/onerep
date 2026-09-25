package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
)

var starter = string(plan.StarterTemplate())

func planID(t *testing.T, resp *http.Response) string {
	t.Helper()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/plans/") {
		t.Fatalf("expected redirect to a plan, got %d %q", resp.StatusCode, loc)
	}
	return strings.TrimSuffix(strings.TrimPrefix(loc, "/plans/"), "?reset")
}

func TestPlanEditorPage(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)
	html := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	if !strings.Contains(html, "Upper/Lower starter") || !strings.Contains(html, `hx-post="/plans/preview"`) {
		t.Fatal("editor misses the template or the preview hook")
	}
	m := regexp.MustCompile(`(?s)<script id="plan-schema" type="application/json">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil || !json.Valid([]byte(m[1])) {
		t.Fatalf("schema is not embedded as parseable JSON:\n%s", html)
	}
}

func TestPlanPreview(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	_, frag := post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {starter}})
	if strings.Contains(frag, "<html") || !strings.Contains(frag, "Week 4") || !strings.Contains(frag, "Barbell Bench Press") ||
		!strings.Contains(frag, "3 × 5 @ 75% TM") {
		t.Fatalf("preview:\n%s", frag)
	}
	_, frag = post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "nope", "sets": [{"count": 1, "reps": 1}]}]}]}]}`}})
	if !strings.Contains(frag, "/days/0/groups/0/exercises/0/slug") || !strings.Contains(frag, "unknown exercise") || strings.Contains(frag, "Week 1") {
		t.Fatalf("error preview:\n%s", frag)
	}
}

func TestFollowPlanAndMoveThroughDays(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	id := planID(t, resp)

	resp, _ = post(t, c, srv.URL+"/plans/"+id+"/follow", csrf, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("follow: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/exercises/barbell-bench-press/settings", csrf, url.Values{"training_max": {"120"}})
	home := read(t, mustGet(t, c, srv.URL+"/"))
	for _, want := range []string{"Next: Upper", "week 1 of 4", "2 × 5 warmup @ 50% TM", "60 kg", "3 × 5 @ 75% TM", "90 kg"} {
		if !strings.Contains(home, want) {
			t.Fatalf("home misses %q:\n%s", want, home)
		}
	}
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "Next: Lower") {
		t.Fatal("skip did not move to Lower")
	}
	post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"4:0"}})
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 4 of 4") || !strings.Contains(home, "Next: Upper") {
		t.Fatal("choose did not move to week 4")
	}
	resp, _ = post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"9:0"}})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("choosing a missing day: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "You finished all 4 weeks") {
		t.Fatal("plan should be complete")
	}
	post(t, c, srv.URL+"/plan/restart", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 1 of 4") {
		t.Fatal("restart did not go back to week 1")
	}
}

func TestReviewDraftThatResetsCursor(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	id := planID(t, resp)
	post(t, c, srv.URL+"/plans/"+id+"/follow", csrf, nil)
	post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"4:1"}})

	shorter := strings.Replace(starter, `"weeks": 4`, `"weeks": 1`, 1)
	shorter = regexp.MustCompile(`\[(\d+(?:\.\d+)?), [^\]]*\]`).ReplaceAllString(shorter, "$1") // per-week arrays -> week 1 value
	resp, _ = post(t, c, srv.URL+"/plans/"+id, csrf, url.Values{"doc": {shorter}, "action": {"draft"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft: %d", resp.StatusCode)
	}
	detail := read(t, mustGet(t, c, srv.URL+"/plans/"+id))
	m := regexp.MustCompile(`/plans/` + id + `/versions/([0-9a-f-]+)/compare`).FindStringSubmatch(detail)
	if m == nil || !strings.Contains(detail, "Review") {
		t.Fatalf("draft not offered for review:\n%s", detail)
	}
	cmp := read(t, mustGet(t, c, srv.URL+"/plans/"+id+"/versions/"+m[1]+"/compare"))
	if !strings.Contains(cmp, "starts the plan over at week 1") || !strings.Contains(cmp, "Week 2 · Upper") {
		t.Fatalf("comparison:\n%s", cmp)
	}
	resp, _ = post(t, c, srv.URL+"/plans/"+id+"/versions/"+m[1]+"/activate", csrf, nil)
	if resp.Header.Get("Location") != "/plans/"+id+"?reset" {
		t.Fatalf("activate redirect: %s", resp.Header.Get("Location"))
	}
	if page := read(t, mustGet(t, c, srv.URL+"/plans/"+id+"?reset")); !strings.Contains(page, "starts over at week 1") {
		t.Fatal("reset notice missing")
	}
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 1 of 1") {
		t.Fatal("cursor not reset")
	}
}

func TestInvalidPlanKeepsTheText(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	bad := `{"name": "Mine", "weeks": 0, "days": []}`
	resp, body := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {bad}, "action": {"activate"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "/weeks") ||
		!strings.Contains(body, `{&#34;name&#34;: &#34;Mine&#34;`) {
		t.Fatalf("got %d:\n%s", resp.StatusCode, body)
	}
}

func TestPlansAreIsolatedAndSchemaIsPublic(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	session(t, srv, c)
	ctx := context.Background()
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	svc := &plan.Service{Store: db, Exercises: &exercise.Service{Store: db}}
	p, v, err := svc.Create(ctx, bob, plan.StarterTemplate(), plan.SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/plans/" + p.ID, "/plans/" + p.ID + "/edit", "/plans/" + p.ID + "/versions/" + v.ID} {
		if resp := mustGet(t, c, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}

	anon, ac := newApp(t, "")
	resp := mustGet(t, ac, anon.URL+"/schema/plan.json")
	if body := read(t, resp); resp.StatusCode != http.StatusOK || !json.Valid([]byte(body)) || resp.Header.Get("Content-Type") != "application/schema+json" {
		t.Fatalf("schema: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestChooseRejectsNegativeDay(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	post(t, c, srv.URL+"/plans/"+planID(t, resp)+"/follow", csrf, nil)
	for _, pos := range []string{"1:-1", "0:0", "x", ""} {
		if resp, _ := post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {pos}}); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("position %q: %d", pos, resp.StatusCode)
		}
	}
}

func TestOversizedPlanDocumentIsRefused(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	big := `{"name": "` + strings.Repeat("x", 1<<20) + `"}`
	resp, body := post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {big}})
	if !strings.Contains(body, "too large") {
		t.Fatalf("preview of a 1 MB document: %d\n%.300s", resp.StatusCode, body)
	}
	resp, body = post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {big}, "action": {"draft"}})
	if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(body, "too large") {
		t.Fatalf("saving a 1 MB document: %d", resp.StatusCode)
	}
}
func TestPlanEditorUsesTheEditorSchema(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	for _, want := range []string{`"format":"per-week"`, `"definitions"`, `data-view-switch`, `id="doc-form"`, `hx-sync="this:replace"`,
		`Barbell Back Squat`} {
		if !strings.Contains(html, want) {
			t.Errorf("editor page lacks %q", want)
		}
	}
	if strings.Contains(html, "jse-theme-dark") || strings.Contains(html, "plan-editor.js") {
		t.Error("the old JSON editor is still loaded")
	}
}

func TestPlanPreviewCarriesProblemsJSON(t *testing.T) {
	srv, c := newApp(t, "alice")
	page := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	csrf := csrfInput.FindStringSubmatch(page)[1]
	resp, err := c.PostForm(srv.URL+"/plans/preview", url.Values{"doc": {`{"name":"x","weeks":0,"days":[]}`}, "csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	html := read(t, resp)
	if !strings.Contains(html, `id="plan-problems"`) || !strings.Contains(html, `data-pointer="/weeks"`) {
		t.Fatalf("preview = %s", html)
	}
}
