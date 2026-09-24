package store

import (
	"context"
	"testing"
	"time"
)

// logged is set() with explicit kind, reps and rpe.
func logged(sessionID, id, kind string, kg float64, reps int, rpe *float64, done time.Time) Set {
	s := set(sessionID, id, kg, done)
	s.Kind, s.Reps, s.RPE = kind, &reps, rpe
	return s
}

func rpe(v float64) *float64 { return &v }

func mustUpsert(t *testing.T, db *DB, userID string, s Set) {
	t.Helper()
	if _, err := db.UpsertSet(context.Background(), userID, s, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRepMaxesAndPRs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	first := newSession(t, db, u.ID, "first")
	mustUpsert(t, db, u.ID, logged(first.ID, "a", "working", 100, 5, nil, t0))
	mustUpsert(t, db, u.ID, logged(first.ID, "b", "amrap", 90, 8, nil, t0.Add(time.Minute)))
	second := newSession(t, db, u.ID, "second")
	mustUpsert(t, db, u.ID, logged(second.ID, "c", "working", 100, 5, nil, t0.Add(24*time.Hour)))               // tie: not a PR
	mustUpsert(t, db, u.ID, logged(second.ID, "d", "working", 105, 5, nil, t0.Add(24*time.Hour+time.Minute)))   // PR
	mustUpsert(t, db, u.ID, logged(second.ID, "e", "working", 110, 5, nil, t0.Add(24*time.Hour+2*time.Minute))) // PR over d
	mustUpsert(t, db, u.ID, logged(second.ID, "f", "working", 60, 3, nil, t0.Add(24*time.Hour+3*time.Minute)))  // first 3-rep set: not a PR

	maxes, err := db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []RepMax{{3, 60, t0.Add(24*time.Hour + 3*time.Minute), second.ID}, {5, 110, t0.Add(24*time.Hour + 2*time.Minute), second.ID},
		{8, 90, t0.Add(time.Minute), first.ID}}
	if len(maxes) != len(want) {
		t.Fatalf("rep maxes = %+v", maxes)
	}
	for i := range want {
		if maxes[i].Reps != want[i].Reps || maxes[i].WeightKg != want[i].WeightKg || !maxes[i].DoneAt.Equal(want[i].DoneAt) || maxes[i].SessionID != want[i].SessionID {
			t.Errorf("rep max %d = %+v, want %+v", i, maxes[i], want[i])
		}
	}
	// Excluding a session leaves only the other sessions' bests.
	maxes, _ = db.RepMaxes(ctx, u.ID, "barbell-back-squat", second.ID)
	if len(maxes) != 2 || maxes[0].Reps != 5 || maxes[0].WeightKg != 100 {
		t.Fatalf("rep maxes excluding the second session = %+v", maxes)
	}
	// The first date a weight was reached wins a tie.
	mustUpsert(t, db, u.ID, logged(second.ID, "g", "working", 90, 8, nil, t0.Add(48*time.Hour)))
	maxes, _ = db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if maxes[2].SessionID != first.ID {
		t.Fatalf("tied 8-rep max should keep the first session: %+v", maxes[2])
	}

	prs, err := db.SessionPRs(ctx, u.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || !prs["d"] || !prs["e"] {
		t.Fatalf("PRs = %v, want d and e", prs)
	}
	if prs, _ := db.SessionPRs(ctx, u.ID, first.ID); len(prs) != 0 {
		t.Fatalf("first session has no earlier bests, PRs = %v", prs)
	}
}

func TestE1RMSeries(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	one := newSession(t, db, u.ID, "one")
	mustUpsert(t, db, u.ID, withE1RM(logged(one.ID, "a", "working", 100, 5, rpe(8), t0), 120))
	mustUpsert(t, db, u.ID, withE1RM(logged(one.ID, "b", "working", 105, 3, nil, t0.Add(time.Minute)), 115.5))
	two := newSession(t, db, u.ID, "two")
	mustUpsert(t, db, u.ID, withE1RM(logged(two.ID, "c", "working", 110, 3, nil, t0.Add(48*time.Hour)), 121))

	series, err := db.E1RMSeries(ctx, u.ID, "barbell-back-squat")
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %+v", series)
	}
	if p := series[0]; p.SessionID != one.ID || p.E1RMKg != 120 || p.WeightKg != 100 || p.Reps != 5 || p.RPE == nil || *p.RPE != 8 || !p.DoneAt.Equal(t0) {
		t.Errorf("first point = %+v", p)
	}
	if p := series[1]; p.SessionID != two.ID || p.E1RMKg != 121 || p.RPE != nil {
		t.Errorf("second point = %+v", p)
	}
}

func TestHardSets(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "s")
	mustUpsert(t, db, u.ID, logged(s.ID, "no-rpe", "working", 100, 5, nil, t0))
	mustUpsert(t, db, u.ID, logged(s.ID, "rpe7", "drop", 80, 8, rpe(7), t0.Add(time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "amrap", "amrap", 90, 9, rpe(9.5), t0.Add(2*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "easy", "working", 60, 5, rpe(6.5), t0.Add(3*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "warm", "warmup", 60, 5, nil, t0.Add(4*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "late", "working", 100, 5, nil, t0.Add(time.Hour))) // == to: excluded

	hard, err := db.HardSets(ctx, u.ID, t0, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(hard) != 3 || hard[0].Slug != "barbell-back-squat" || !hard[0].DoneAt.Equal(t0) || !hard[2].DoneAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("hard sets = %+v", hard)
	}
}

func TestStatsIgnoreDeletedWarmupAndOtherUsers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	other := newUser(t, db, "other")
	mine := newSession(t, db, u.ID, "mine")
	mustUpsert(t, db, u.ID, withE1RM(logged(mine.ID, "base", "working", 100, 5, nil, t0), 116))
	theirs := newSession(t, db, other.ID, "theirs")
	mustUpsert(t, db, other.ID, withE1RM(logged(theirs.ID, "x", "working", 200, 5, nil, t0), 233))
	later := newSession(t, db, u.ID, "later")
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "warm", "warmup", 150, 5, nil, t0.Add(time.Hour)), 175))
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "gone", "working", 150, 5, nil, t0.Add(2*time.Hour)), 175))
	if _, err := db.DeleteSet(ctx, u.ID, later.ID, "gone", t0.Add(3*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "real", "working", 102.5, 5, nil, t0.Add(4*time.Hour)), 119))

	maxes, _ := db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if len(maxes) != 1 || maxes[0].WeightKg != 102.5 {
		t.Errorf("rep maxes = %+v, want only 102.5", maxes)
	}
	prs, _ := db.SessionPRs(ctx, u.ID, later.ID)
	if len(prs) != 1 || !prs["real"] {
		t.Errorf("PRs = %v, want only real (the warmup, deleted and other user's sets don't count)", prs)
	}
	series, _ := db.E1RMSeries(ctx, u.ID, "barbell-back-squat")
	if len(series) != 2 || series[1].E1RMKg != 119 {
		t.Errorf("series = %+v", series)
	}
	hard, _ := db.HardSets(ctx, u.ID, t0.Add(-time.Hour), t0.Add(24*time.Hour))
	if len(hard) != 2 {
		t.Errorf("hard sets = %+v, want base and real", hard)
	}
	if prs, _ := db.SessionPRs(ctx, other.ID, later.ID); len(prs) != 0 {
		t.Errorf("another user's session leaked PRs: %v", prs)
	}
}
