package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/auth"
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
