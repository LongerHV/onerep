package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/training"
)

// startPlanned follows the starter plan and starts its first day; it returns
// the session id.
func startPlanned(t *testing.T, srvURL string, c *http.Client, csrf string) string {
	t.Helper()
	resp, _ := post(t, c, srvURL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	post(t, c, srvURL+"/plans/"+planID(t, resp)+"/follow", csrf, nil)
	post(t, c, srvURL+"/exercises/barbell-bench-press/settings", csrf, url.Values{"training_max": {"100"}})
	resp, _ = post(t, c, srvURL+"/sessions", csrf, url.Values{"kind": {"plan"}})
	m := regexp.MustCompile(`^/sessions/([0-9a-f-]+)/live$`).FindStringSubmatch(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || m == nil {
		t.Fatalf("start: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	return m[1]
}

func bootOf(t *testing.T, html string) training.Bootstrap {
	t.Helper()
	m := regexp.MustCompile(`(?s)<script id="companion-boot" type="application/json">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no bootstrap in page:\n%.500s", html)
	}
	var b training.Bootstrap
	if err := json.Unmarshal([]byte(m[1]), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStartAndContinueWorkout(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)

	b := bootOf(t, read(t, mustGet(t, c, srv.URL+"/sessions/"+id+"/live")))
	bench := b.Exercises["barbell-bench-press"]
	if b.Session.ID != id || b.Snapshot.Name != "Upper" || bench.TMKg == nil || *bench.TMKg != 100 || b.Unit != "kg" {
		t.Fatalf("bootstrap = %+v", b.Session)
	}
	home := read(t, mustGet(t, c, srv.URL+"/"))
	if !strings.Contains(home, "Workout in progress: Upper") || strings.Contains(home, "Start workout") {
		t.Fatal("home does not offer to continue the open workout")
	}
	// Starting again continues the open workout.
	resp, _ := post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"adhoc"}})
	if resp.Header.Get("Location") != "/sessions/"+id+"/live" {
		t.Fatalf("second start: %s", resp.Header.Get("Location"))
	}
}

func TestStartWithoutPlan(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, body := post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"plan"}})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "start an empty workout") {
		t.Fatalf("planned start without a plan: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"adhoc"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("empty workout: %d", resp.StatusCode)
	}
	b := bootOf(t, read(t, mustGet(t, c, srv.URL+resp.Header.Get("Location"))))
	if b.Session.Name != "Workout" || len(b.Snapshot.Groups) != 0 || len(b.Catalog) < 100 {
		t.Fatalf("ad-hoc bootstrap: %+v, %d catalog entries", b.Session, len(b.Catalog))
	}
}

func syncOps(t *testing.T, c *http.Client, srvURL, csrf string, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srvURL+"/api/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, read(t, resp)
}

func setOpJSON(opID, setID, sessionID string, kg float64, reps int) string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	op := map[string]any{"op_id": opID, "op": "upsert_set", "client_ts": now, "payload": map[string]any{
		"id": setID, "session_id": sessionID, "slug": "barbell-bench-press", "group_pos": 0, "exercise_pos": 0,
		"set_pos": 2, "kind": "working", "weight_kg": kg, "reps": reps, "rpe": 8, "done_at": now, "updated_at": now}}
	b, _ := json.Marshal(op)
	return string(b)
}

func TestSyncAPI(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)
	op := setOpJSON("01900000-0000-7000-8000-0000000000a1", "01900000-0000-7000-8000-0000000000b1", id, 80, 5)

	if resp, _ := syncOps(t, c, srv.URL, "", `{"ops": [`+op+`]}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF: %d", resp.StatusCode)
	}
	resp, body := syncOps(t, c, srv.URL, csrf, `{"ops": [`+op+`, `+op+`]}`)
	var out struct {
		Results []training.OpResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("sync: %d %s", resp.StatusCode, body)
	}
	if len(out.Results) != 2 || out.Results[0].Status != "applied" || out.Results[1].Status != "duplicate" {
		t.Fatalf("results = %+v", out.Results)
	}
	if resp, _ := syncOps(t, c, srv.URL, csrf, `{"ops": [`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad JSON: %d", resp.StatusCode)
	}
	many := strings.TrimSuffix(strings.Repeat(`{"op_id": "x", "op": "upsert_set"},`, training.MaxOps+1), ",")
	if resp, body := syncOps(t, c, srv.URL, csrf, `{"ops": [`+many+`]}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "at most") {
		t.Fatalf("too many ops: %d %s", resp.StatusCode, body)
	}

	resp, body = getWith(t, c, srv.URL+"/api/csrf")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, csrf) {
		t.Fatalf("csrf endpoint: %d %s", resp.StatusCode, body)
	}
	// The synced set is in the next bootstrap, so another device picks it up.
	if b := bootOf(t, read(t, mustGet(t, c, srv.URL+"/sessions/"+id+"/live"))); len(b.Sets) != 1 || *b.Sets[0].WeightKg != 80 {
		t.Fatalf("bootstrap sets = %+v", b.Sets)
	}
}

func TestHistoryEditor(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)
	setID := "01900000-0000-7000-8000-0000000000b2"
	syncOps(t, c, srv.URL, csrf, `{"ops": [`+setOpJSON("01900000-0000-7000-8000-0000000000a2", setID, id, 80, 5)+`]}`)

	list := read(t, mustGet(t, c, srv.URL+"/history"))
	if !strings.Contains(list, "/history/"+id) || !strings.Contains(list, "in progress") || !strings.Contains(list, "1 sets") {
		t.Fatalf("history list:\n%.600s", list)
	}
	page := read(t, mustGet(t, c, srv.URL+"/history/"+id))
	if !strings.Contains(page, "Barbell Bench Press") || !strings.Contains(page, `value="80"`) {
		t.Fatalf("history detail:\n%.600s", page)
	}

	// Correct the weight (in the user's unit), add a set, delete it again.
	edit := url.Values{"set_id": {setID}, "slug": {"barbell-bench-press"}, "kind": {"working"}, "group_pos": {"0"},
		"exercise_pos": {"0"}, "set_pos": {"2"}, "weight": {"82,5"}, "reps": {"5"}, "rpe": {"8"}}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, edit); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit: %d", resp.StatusCode)
	}
	add := url.Values{"slug": {"dumbbell-curl"}, "group_pos": {"5"}, "weight": {"12"}, "reps": {"10"}}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, add); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	page = read(t, mustGet(t, c, srv.URL+"/history/"+id))
	if !strings.Contains(page, `value="82.5"`) || !strings.Contains(page, "Dumbbell Curl") {
		t.Fatalf("after edit and add:\n%.800s", page)
	}
	bad := url.Values{"slug": {"dumbbell-curl"}, "weight": {"heavy"}}
	if resp, body := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, bad); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "weight must be a number") {
		t.Fatalf("bad weight: %d", resp.StatusCode)
	}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets/"+setID+"/delete", csrf, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete set: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/history/"+id+"/notes", csrf, url.Values{"notes": {"shoulder ok"}})
	if page := read(t, mustGet(t, c, srv.URL+"/history/"+id)); strings.Contains(page, `value="82.5"`) || !strings.Contains(page, "shoulder ok") {
		t.Fatal("delete or notes did not apply")
	}

	// Finishing from history advances the plan to its next day.
	post(t, c, srv.URL+"/history/"+id+"/finish", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "Next: Lower") || strings.Contains(home, "Workout in progress") {
		t.Fatal("finishing did not advance the plan")
	}

	// Another user's workout is not there.
	ctx := context.Background()
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	ex := &exercise.Service{Store: db}
	ts := &training.Service{Store: db, Plans: &plan.Service{Store: db, Exercises: ex, History: db}, Exercises: ex}
	bobs, err := ts.StartAdHoc(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/history/" + bobs.ID, "/sessions/" + bobs.ID + "/live"} {
		if resp := mustGet(t, c, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
	if resp, _ := post(t, c, srv.URL+"/history/"+bobs.ID+"/delete", csrf, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting bob's workout: %d", resp.StatusCode)
	}

	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/delete", csrf, nil); resp.Header.Get("Location") != "/history" {
		t.Fatalf("delete workout: %s", resp.Header.Get("Location"))
	}
}

func TestServiceWorkerAndOfflinePage(t *testing.T) {
	srv, c := newApp(t, "")
	resp, body := getWith(t, c, srv.URL+"/sw.js")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") ||
		strings.Contains(body, "__VERSION__") || !regexp.MustCompile(`const VERSION = "[0-9a-f]{12}"`).MatchString(body) {
		t.Fatalf("sw.js: %d %s\n%.200s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	resp, body = getWith(t, c, srv.URL+"/offline")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "You are offline") || !strings.Contains(body, "<html") {
		t.Fatalf("offline page: %d", resp.StatusCode)
	}
}

// The companion keeps its queue when the session cookie has expired and asks
// the user to sign in again; that needs a 401, not a redirect to the IdP.
func TestSyncWithoutSessionIs401(t *testing.T) {
	srv, c := newApp(t, "")
	resp, _ := syncOps(t, c, srv.URL, "", `{"ops": []}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d", resp.StatusCode)
	}
}
