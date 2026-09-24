package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
)

// logSet stores a finished working set for the dev user directly.
func logSet(t *testing.T, db *store.DB, userID, sessionID, id, slug string, kg float64, reps int, rpe *float64, e1rm *float64, done time.Time) {
	t.Helper()
	s := store.Set{ID: id, SessionID: sessionID, Slug: slug, Kind: "working", WeightKg: &kg, Reps: &reps, RPE: rpe,
		E1RMKg: e1rm, DoneAt: &done, UpdatedAt: done}
	if _, err := db.UpsertSet(context.Background(), userID, s, ""); err != nil {
		t.Fatal(err)
	}
}

// devUser signs the dev user in (by visiting /) and returns them.
func devUser(t *testing.T, srvURL string, c *http.Client, db *store.DB) store.User {
	t.Helper()
	read(t, mustGet(t, c, srvURL+"/"))
	u, err := db.UpsertOIDCUser(context.Background(), auth.DevIssuer, "alice", "alice@localhost", "alice") // returns the existing user
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func newSessionFor(t *testing.T, db *store.DB, userID string) store.Session {
	t.Helper()
	s, err := db.CreateSession(context.Background(), store.Session{UserID: userID, Name: "Workout", Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ptr(v float64) *float64 { return &v }

func TestStatsAPI(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	now := time.Now().UTC()
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000001", "barbell-bench-press", 100, 5, ptr(8), ptr(123.46), now)

	var e1rm struct {
		Unit   string `json:"unit"`
		Points []struct {
			T        int64   `json:"t"`
			E1RM     float64 `json:"e1rm"`
			RPEBased bool    `json:"rpe_based"`
		} `json:"points"`
	}
	resp := mustGet(t, c, srv.URL+"/api/stats/exercises/barbell-bench-press/e1rm")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("e1rm API = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if err := json.Unmarshal([]byte(read(t, resp)), &e1rm); err != nil {
		t.Fatal(err)
	}
	if e1rm.Unit != "kg" || len(e1rm.Points) != 1 || e1rm.Points[0].E1RM != 123.5 || !e1rm.Points[0].RPEBased || e1rm.Points[0].T != now.Unix() {
		t.Fatalf("e1rm = %+v", e1rm)
	}

	var muscles struct {
		Weeks   []string `json:"weeks"`
		Muscles []struct {
			Muscle string    `json:"muscle"`
			Label  string    `json:"label"`
			Sets   []float64 `json:"sets"`
		} `json:"muscles"`
	}
	if err := json.Unmarshal([]byte(read(t, mustGet(t, c, srv.URL+"/api/stats/muscles"))), &muscles); err != nil {
		t.Fatal(err)
	}
	if len(muscles.Weeks) != 12 || len(muscles.Muscles) != 3 || muscles.Muscles[2].Label != "front delts" || muscles.Muscles[2].Sets[11] != 0.5 {
		t.Fatalf("muscles = %+v", muscles)
	}

	if resp := mustGet(t, c, srv.URL+"/api/stats/exercises/no-such-thing/e1rm"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown exercise = %d, want 404", resp.StatusCode)
	}
}

func TestStatsAPIInPounds(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	if err := db.UpdateUserSettings(context.Background(), u.ID, "lb", u.E1RMWindowDays); err != nil {
		t.Fatal(err)
	}
	sess := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000002", "barbell-bench-press", 100, 5, nil, ptr(116.67), time.Now())
	body := read(t, mustGet(t, c, srv.URL+"/api/stats/exercises/barbell-bench-press/e1rm"))
	var got struct {
		Unit   string `json:"unit"`
		Points []struct {
			E1RM float64 `json:"e1rm"`
		} `json:"points"`
	}
	_ = json.Unmarshal([]byte(body), &got)
	if got.Unit != "lb" || len(got.Points) != 1 || got.Points[0].E1RM != 257.2 {
		t.Fatalf("lb e1rm = %s", body)
	}
}

// A chart fetch after the login expired must see 401, not the login page.
func TestStatsAPIWithoutSessionIs401(t *testing.T) {
	srv, c := newApp(t, "")
	for _, path := range []string{"/api/stats/muscles", "/api/stats/exercises/barbell-bench-press/e1rm"} {
		resp := mustGet(t, c, srv.URL+path)
		read(t, resp)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", path, resp.StatusCode)
		}
	}
}
func TestExercisePageShowsProgress(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	day := time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000011", "barbell-bench-press", 102.5, 3, ptr(9), ptr(112), day)
	html := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-bench-press"))
	for _, want := range []string{
		`data-chart="e1rm"`, `data-src="/api/stats/exercises/barbell-bench-press/e1rm"`,
		"Rep maxes", "102.5 kg", "2026-09-21", `href="/history/` + sess.ID + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("exercise page lacks %q", want)
		}
	}
	if strings.Count(html, `data-rep-max`) != 12 {
		t.Errorf("rep-max table should have 12 rows, has %d", strings.Count(html, `data-rep-max`))
	}
}

func TestExercisePageWithoutHistory(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-bench-press"))
	if !strings.Contains(html, "No working sets logged yet.") || strings.Contains(html, `data-chart=`) {
		t.Error("an exercise without sets should say so and draw no chart")
	}
	// Timed exercises have neither an e1RM nor weight PRs.
	html = read(t, mustGet(t, c, srv.URL+"/exercises/plank"))
	if strings.Contains(html, "Rep maxes") || strings.Contains(html, `data-chart=`) {
		t.Error("a timed exercise shows no rep maxes or e1RM chart")
	}
}

func TestMusclesPage(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000021", "barbell-back-squat", 100, 5, nil, nil, time.Now())
	html := read(t, mustGet(t, c, srv.URL+"/stats/muscles"))
	for _, want := range []string{`data-chart="muscles"`, "quads", "adductors", "0.5", "Hard sets", `href="/stats/muscles"`} {
		if !strings.Contains(html, want) {
			t.Errorf("muscles page lacks %q", want)
		}
	}
}

func TestHistoryShowsPRBadges(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	old := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, old.ID, "00000000-0000-7000-8000-000000000031", "barbell-bench-press", 100, 5, nil, nil, time.Now().Add(-48*time.Hour))
	cur := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, cur.ID, "00000000-0000-7000-8000-000000000032", "barbell-bench-press", 105, 5, nil, nil, time.Now())
	html := read(t, mustGet(t, c, srv.URL+"/history/"+cur.ID))
	if strings.Count(html, `data-pr-badge`) != 1 {
		t.Errorf("history should badge the one PR set, found %d", strings.Count(html, `data-pr-badge`))
	}
	if html := read(t, mustGet(t, c, srv.URL+"/history/"+old.ID)); strings.Contains(html, `data-pr-badge`) {
		t.Error("the first set at a rep count is not a PR")
	}
}

// brokenStats fails every query, to reach the API's server-error path.
type brokenStats struct{}

var errBroken = errors.New("disk on fire")

func (brokenStats) RepMaxes(context.Context, string, string, string) ([]store.RepMax, error) {
	return nil, errBroken
}
func (brokenStats) E1RMSeries(context.Context, string, string) ([]store.E1RMPoint, error) {
	return nil, errBroken
}
func (brokenStats) SessionPRs(context.Context, string, string) (map[string]bool, error) {
	return nil, errBroken
}
func (brokenStats) HardSets(context.Context, string, time.Time, time.Time) ([]store.HardSet, error) {
	return nil, errBroken
}

// A failed chart request carries the request id (spec §16), so it can be
// matched to the server log.
func TestStatsAPIErrorsCarryRequestID(t *testing.T) {
	s := &Server{Stats: &stats.Service{Store: brokenStats{}}}
	h := middleware.RequestID(http.HandlerFunc(s.apiMuscles))
	req := httptest.NewRequest(http.MethodGet, "/api/stats/muscles", nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{User: store.User{ID: "u"}}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusInternalServerError || body["request_id"] == "" {
		t.Fatalf("server error = %d %s, want 500 with a request_id", rec.Code, rec.Body)
	}
}
