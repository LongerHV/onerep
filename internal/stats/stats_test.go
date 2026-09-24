package stats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc  *Service
	db   *store.DB
	user store.User
	sess store.Session
}

func newEnv(t *testing.T, now time.Time) env {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := exercise.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	u, err := db.UpsertOIDCUser(ctx, "iss", "u", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := db.CreateSession(ctx, store.Session{UserID: u.ID, Name: "s", Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: db, Exercises: &exercise.Service{Store: db}, Now: func() time.Time { return now }}
	return env{svc: svc, db: db, user: u, sess: sess}
}

var n int

// log stores a set of slug for the env's session.
func (e env) log(t *testing.T, slug, kind string, kg float64, reps int, rpe *float64, done time.Time, e1rm *float64) {
	t.Helper()
	n++
	id := "00000000-0000-7000-8000-" + time.Unix(int64(n), 0).UTC().Format("150405") + "000000"
	s := store.Set{ID: id, SessionID: e.sess.ID, Slug: slug, Kind: kind, WeightKg: &kg, Reps: &reps, RPE: rpe,
		E1RMKg: e1rm, DoneAt: &done, UpdatedAt: done}
	if _, err := e.db.UpsertSet(context.Background(), e.user.ID, s, ""); err != nil {
		t.Fatal(err)
	}
}

func f(v float64) *float64 { return &v }

var monday = time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC) // 2026-W39

func TestExerciseStats(t *testing.T) {
	e := newEnv(t, monday)
	e.log(t, "barbell-back-squat", "working", 100, 5, f(8), monday, f(123.5)) // RTS: RPE-based
	e.log(t, "barbell-back-squat", "working", 100, 13, nil, monday, nil)      // 13 reps: not in the table
	e.log(t, "barbell-back-squat", "working", 90, 1, f(5), monday, f(90))     // RPE 5 is off the RTS table
	st, err := e.svc.ExerciseStats(context.Background(), e.user, "barbell-back-squat")
	if err != nil {
		t.Fatal(err)
	}
	if st.Exercise.Slug != "barbell-back-squat" || len(st.Series) != 1 || !st.Series[0].RPEBased || st.Series[0].E1RMKg != 123.5 {
		t.Fatalf("stats = %+v", st)
	}
	if len(st.RepMaxes) != 2 || st.RepMaxes[0].Reps != 1 || st.RepMaxes[1].Reps != 5 {
		t.Fatalf("rep maxes = %+v, want reps 1 and 5 only", st.RepMaxes)
	}
	if _, err := e.svc.ExerciseStats(context.Background(), e.user, "no-such-exercise"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown slug: %v", err)
	}
}

func TestRPEBasedFollowsCalc(t *testing.T) {
	// Only the method calc.E1RM used decides the style; an RPE off the table is rep-based.
	if isRPEBased(store.E1RMPoint{WeightKg: 90, Reps: 1, RPE: f(5)}) {
		t.Error("RPE 5 single counted as RPE-based")
	}
	if !isRPEBased(store.E1RMPoint{WeightKg: 100, Reps: 5, RPE: f(8)}) || isRPEBased(store.E1RMPoint{WeightKg: 100, Reps: 5}) {
		t.Error("RPE 8 must be RPE-based and no RPE rep-based")
	}
}

func TestWeeklyMuscleSets(t *testing.T) {
	e := newEnv(t, monday)
	// Bench: chest and triceps primary, front delts secondary.
	e.log(t, "barbell-bench-press", "working", 80, 5, nil, monday.Add(time.Hour), nil)
	e.log(t, "barbell-bench-press", "working", 80, 5, f(8), monday.Add(-time.Minute), nil) // Sunday: week 38
	e.log(t, "barbell-bench-press", "working", 60, 5, f(6), monday.Add(2*time.Hour), nil)  // too easy
	e.log(t, "no-longer-exists", "working", 60, 5, nil, monday.Add(3*time.Hour), nil)      // unknown: no muscles
	got, err := e.svc.WeeklyMuscleSets(context.Background(), e.user, monday.AddDate(0, 0, -7), monday.Add(6*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Weeks) != 2 || got.Weeks[0].Label != "2026-W38" || got.Weeks[1].Label != "2026-W39" || !got.Weeks[1].Start.Equal(monday) {
		t.Fatalf("weeks = %+v", got.Weeks)
	}
	want := map[string][]float64{"chest": {1, 1}, "triceps": {1, 1}, "front-delts": {0.5, 0.5}}
	if len(got.Muscles) != len(want) {
		t.Fatalf("muscles = %+v", got.Muscles)
	}
	for _, m := range got.Muscles {
		w := want[m.Muscle]
		if len(m.Sets) != 2 || m.Sets[0] != w[0] || m.Sets[1] != w[1] || m.Total != w[0]+w[1] {
			t.Errorf("%s = %+v, want %v", m.Muscle, m, w)
		}
	}
	// Most volume first; ties in vocabulary order (chest before triceps).
	if got.Muscles[0].Muscle != "chest" || got.Muscles[1].Muscle != "triceps" || got.Muscles[2].Muscle != "front-delts" {
		t.Errorf("order = %s, %s, %s", got.Muscles[0].Muscle, got.Muscles[1].Muscle, got.Muscles[2].Muscle)
	}
}

func TestWeeklyMuscleSetsUseTheUsersMuscles(t *testing.T) {
	e := newEnv(t, monday)
	ctx := context.Background()
	ex := &exercise.Service{Store: e.db}
	if _, err := ex.Update(ctx, e.user.ID, exercise.Input{Slug: "dumbbell-curl", Name: "Dumbbell Curl", Measurement: "weight_reps",
		EquipmentKind: "dumbbell", PrimaryMuscles: []string{"forearms"}}); err != nil {
		t.Fatal(err)
	}
	e.log(t, "dumbbell-curl", "working", 12, 10, nil, monday, nil)
	got, err := e.svc.WeeklyMuscleSets(ctx, e.user, monday, monday)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Muscles) != 1 || got.Muscles[0].Muscle != "forearms" || got.Muscles[0].Sets[0] != 1 {
		t.Fatalf("muscles = %+v, want the customised forearms only", got.Muscles)
	}
}

func TestWeeklyMuscleSetsISOWeeks(t *testing.T) {
	sat := time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC) // ISO 2026-W53
	e := newEnv(t, sat)
	e.log(t, "barbell-back-squat", "working", 100, 5, nil, sat, nil)
	got, err := e.svc.RecentMuscleSets(context.Background(), e.user, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Weeks) != 12 || got.Weeks[11].Label != "2026-W53" || got.Weeks[0].Label != "2026-W42" {
		t.Fatalf("weeks = %v … %v (%d)", got.Weeks[0], got.Weeks[len(got.Weeks)-1], len(got.Weeks))
	}
	for i := 1; i < len(got.Weeks); i++ {
		if !got.Weeks[i].Start.Equal(got.Weeks[i-1].Start.AddDate(0, 0, 7)) {
			t.Fatalf("week %d starts %v after %v", i, got.Weeks[i].Start, got.Weeks[i-1].Start)
		}
	}
	// Glutes and quads tie at 1 set; vocabulary order puts glutes first.
	if len(got.Muscles) != 4 || got.Muscles[1].Muscle != "quads" || got.Muscles[1].Sets[11] != 1 || got.Muscles[1].Sets[0] != 0 {
		t.Fatalf("muscles = %+v", got.Muscles)
	}
	// Next Monday is 2027-W01.
	if w := weekOf(time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)); w.Label != "2027-W01" {
		t.Fatalf("2027-01-04 is %s", w.Label)
	}
}
