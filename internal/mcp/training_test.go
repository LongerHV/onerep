package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/store"
)

// logSession stores a finished session with squat sets for user, done at when.
func (e env) logSession(t *testing.T, u store.User, when time.Time, kgs ...float64) store.Session {
	t.Helper()
	ctx := context.Background()
	s, err := e.db.CreateSession(ctx, store.Session{UserID: u.ID, Name: "Squat day", Snapshot: []byte(`{"name":"Squat day","groups":[]}`),
		StartedAt: when})
	if err != nil {
		t.Fatal(err)
	}
	for i, kg := range kgs {
		kg, reps, rpe := kg, 5, 8.0
		e1rm := kg / 0.811 // RTS 5 @ 8
		done := when.Add(time.Duration(i) * time.Minute)
		id := s.ID[:24] + strings.Repeat("0", 11) + string(rune('a'+i))
		if _, err := e.db.UpsertSet(ctx, u.ID, store.Set{ID: id, SessionID: s.ID, Slug: "barbell-back-squat", Kind: "working", SetPos: i,
			WeightKg: &kg, Reps: &reps, RPE: &rpe, E1RMKg: &e1rm, DoneAt: &done, UpdatedAt: done}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.db.FinishSession(ctx, u.ID, s.ID, when.Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTrainingTools(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC()
	first := e.logSession(t, e.alice, now.AddDate(0, 0, -8), 100, 100)
	second := e.logSession(t, e.alice, now.AddDate(0, 0, -1), 105, 107.5)
	cs := connect(t, e.url, e.aliceToken)

	var sessions struct {
		Sessions []struct {
			ID        string   `json:"id"`
			Sets      int      `json:"sets"`
			Exercises []string `json:"exercises"`
		} `json:"sessions"`
	}
	call(t, cs, "list_sessions", map[string]any{"exercise": "barbell-back-squat"}, &sessions)
	if len(sessions.Sessions) != 2 || sessions.Sessions[0].ID != second.ID || sessions.Sessions[0].Exercises[0] != "barbell-back-squat" {
		t.Fatalf("sessions = %+v", sessions)
	}
	from := now.AddDate(0, 0, -3).Format("2006-01-02")
	call(t, cs, "list_sessions", map[string]any{"from": from}, &sessions)
	if len(sessions.Sessions) != 1 {
		t.Fatalf("from %s: %d sessions", from, len(sessions.Sessions))
	}
	if msg := call(t, cs, "list_sessions", map[string]any{"from": "last week"}, nil); !strings.Contains(msg, "YYYY-MM-DD") {
		t.Fatalf("bad date: %q", msg)
	}

	var sess struct {
		Unit       string `json:"unit"`
		Prescribed any    `json:"prescribed"`
		Sets       []struct {
			WeightKg *float64 `json:"weight_kg"`
			PR       bool     `json:"pr"`
		} `json:"sets"`
	}
	call(t, cs, "get_session", map[string]any{"id": second.ID}, &sess)
	if sess.Unit != "kg" || sess.Prescribed == nil || len(sess.Sets) != 2 || !sess.Sets[0].PR || !sess.Sets[1].PR {
		t.Fatalf("session = %+v", sess)
	}
	_ = first

	var st struct {
		Unit          string   `json:"unit"`
		TrainingMaxKg *float64 `json:"training_max_kg"`
		E1RMSeries    []any    `json:"e1rm_series"`
		RepMaxes      []struct {
			Reps     int     `json:"reps"`
			WeightKg float64 `json:"weight_kg"`
		} `json:"rep_maxes"`
		RecentBestSets []struct {
			WeightKg float64 `json:"weight_kg"`
		} `json:"recent_best_sets"`
	}
	call(t, cs, "get_exercise_stats", map[string]any{"slug": "barbell-back-squat"}, &st)
	if len(st.E1RMSeries) != 2 || len(st.RepMaxes) != 1 || st.RepMaxes[0].WeightKg != 107.5 ||
		len(st.RecentBestSets) != 2 || st.RecentBestSets[0].WeightKg != 107.5 {
		t.Fatalf("stats = %+v", st)
	}

	var vol struct {
		Weeks   []string `json:"weeks"`
		Muscles []struct {
			Muscle string    `json:"muscle"`
			Sets   []float64 `json:"sets"`
		} `json:"muscles"`
	}
	call(t, cs, "get_weekly_muscle_volume", map[string]any{}, &vol)
	if len(vol.Weeks) != 12 || len(vol.Muscles) == 0 {
		t.Fatalf("volume = %+v", vol)
	}
	if msg := call(t, cs, "get_weekly_muscle_volume", map[string]any{"from": "2020-01-01", "to": "2026-01-01"}, nil); !strings.Contains(msg, "104") {
		t.Fatalf("too long a range: %q", msg)
	}

	prompts, err := cs.ListPrompts(context.Background(), nil)
	if err != nil || len(prompts.Prompts) != 2 {
		t.Fatalf("prompts = %+v, %v", prompts, err)
	}
	for _, name := range []string{"review_block", "build_plan"} {
		got, err := cs.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: name})
		if err != nil || len(got.Messages) == 0 {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestToolsAreIsolatedPerUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	aliceSession := e.logSession(t, e.alice, time.Now().UTC().AddDate(0, 0, -1), 100)
	alice := connect(t, e.url, e.aliceToken)
	var draft struct {
		PlanID    string `json:"plan_id"`
		VersionID string `json:"version_id"`
	}
	call(t, alice, "save_plan_draft", map[string]any{"doc": planDoc}, &draft)
	call(t, alice, "create_exercise", map[string]any{"slug": "zercher-squat", "name": "Zercher Squat", "measurement": "weight_reps",
		"equipment_kind": "barbell", "primary_muscles": []string{"quads"}}, nil)

	bob := connect(t, e.url, e.bobToken)
	for tool, args := range map[string]map[string]any{
		"get_session":        {"id": aliceSession.ID},
		"get_plan":           {"plan_id": draft.PlanID},
		"get_plan#version":   {"version_id": draft.VersionID},
		"save_plan_draft":    {"doc": planDoc, "version_id": draft.VersionID},
		"save_plan_draft#pl": {"doc": planDoc, "plan_id": draft.PlanID},
		"get_exercise_stats": {"slug": "zercher-squat"},
		"set_training_max":   {"slug": "zercher-squat", "training_max_kg": 100},
	} {
		name, _, _ := strings.Cut(tool, "#")
		if msg := call(t, bob, name, args, nil); !strings.Contains(msg, "not found") {
			t.Errorf("bob %s(%v) = %q, want not found", name, args, msg)
		}
	}
	var sessions struct {
		Sessions []any `json:"sessions"`
	}
	call(t, bob, "list_sessions", map[string]any{}, &sessions)
	var plans struct {
		Plans []any `json:"plans"`
	}
	call(t, bob, "list_plans", map[string]any{}, &plans)
	var st struct {
		E1RMSeries []any `json:"e1rm_series"`
	}
	call(t, bob, "get_exercise_stats", map[string]any{"slug": "barbell-back-squat"}, &st)
	if len(sessions.Sessions) != 0 || len(plans.Plans) != 0 || len(st.E1RMSeries) != 0 {
		t.Fatalf("bob sees alice's data: %d sessions, %d plans, %d e1RM points", len(sessions.Sessions), len(plans.Plans), len(st.E1RMSeries))
	}
	if v, _ := e.Plans().Version(ctx, e.alice, draft.VersionID); !strings.Contains(string(v.Doc), "Block") {
		t.Fatal("alice's draft was changed")
	}
}
