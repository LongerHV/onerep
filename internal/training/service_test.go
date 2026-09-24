package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc   *Service
	plans *plan.Service
	ex    *exercise.Service
	db    *store.DB
	alice store.User
	bob   store.User
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := exercise.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	ex := &exercise.Service{Store: db}
	plans := &plan.Service{Store: db, Exercises: ex, History: db}
	alice, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	for _, u := range []store.User{alice, bob} {
		if err := ex.EnsureStarterEquipment(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	return env{svc: &Service{Store: db, Plans: plans, Exercises: ex}, plans: plans, ex: ex, db: db, alice: alice, bob: bob}
}

const twoDays = `{"name": "Test", "weeks": 2, "days": [
	{"name": "Squat day", "groups": [{"rest_s": 180, "exercises": [{"slug": "barbell-back-squat", "sets": [
		{"count": 3, "reps": 5, "load": {"pct_tm": 0.75}}]}]}]},
	{"name": "Pull day", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10"}]}]}]}]}`

// follow creates, activates and follows the two-day plan with a 140 kg squat TM.
func (e env) follow(t *testing.T) store.Plan {
	t.Helper()
	ctx := context.Background()
	p, _, err := e.plans.Create(ctx, e.alice, []byte(twoDays), plan.SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.plans.Follow(ctx, e.alice, p.ID); err != nil {
		t.Fatal(err)
	}
	tm := 140.0
	if err := e.ex.SetTrainingMax(ctx, e.alice.ID, "barbell-back-squat", &tm, "web"); err != nil {
		t.Fatal(err)
	}
	return p
}

var opSeq int

// opID returns a fresh UUID-shaped operation id.
func opID() string {
	opSeq++
	return fmt.Sprintf("00000000-0000-7000-8000-%012d", opSeq)
}

func setOp(id, sessionID, slug string, kg float64, reps int, rpe float64, updated time.Time) Op {
	in := SetInput{ID: id, SessionID: sessionID, Slug: slug, Kind: "working", WeightKg: &kg, Reps: &reps, DoneAt: &updated, UpdatedAt: updated}
	if rpe > 0 {
		in.RPE = &rpe
	}
	payload, _ := json.Marshal(in)
	return Op{OpID: opID(), Op: OpUpsertSet, Payload: payload}
}

func payload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestStartSessions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.StartPlanned(ctx, e.alice); !errors.Is(err, ErrNothingToStart) {
		t.Fatalf("no plan: %v", err)
	}
	e.follow(t)
	sess, err := e.svc.StartPlanned(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}
	var day plan.ExpandedDay
	if err := json.Unmarshal(sess.Snapshot, &day); err != nil {
		t.Fatal(err)
	}
	if sess.Name != "Squat day" || sess.Week != 1 || sess.Day != 0 || day.Groups[0].Exercises[0].Sets[0].Kg == nil ||
		*day.Groups[0].Exercises[0].Sets[0].Kg != 105 {
		t.Fatalf("session = %+v, snapshot = %+v", sess, day)
	}
	var open OpenSessionError
	if _, err := e.svc.StartPlanned(ctx, e.alice); !errors.As(err, &open) || open.ID != sess.ID {
		t.Fatalf("second start: %v", err)
	}
	if _, err := e.svc.StartAdHoc(ctx, e.alice); !errors.As(err, &open) {
		t.Fatalf("ad-hoc while open: %v", err)
	}
	adhoc, err := e.svc.StartAdHoc(ctx, e.bob)
	if err != nil || adhoc.PlanID != "" || adhoc.Name != "Workout" {
		t.Fatalf("bob's ad-hoc session = %+v, %v", adhoc, err)
	}
}

func TestApplyOps(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	sess, _ := e.svc.StartPlanned(ctx, e.alice)
	bobs, _ := e.svc.StartAdHoc(ctx, e.bob)
	now := time.Now().UTC()
	setID := "01900000-0000-7000-8000-000000000001"

	first := setOp(setID, sess.ID, "barbell-back-squat", 100, 5, 8, now)
	ops := []Op{
		first,
		{OpID: first.OpID, Op: first.Op, Payload: first.Payload}, // replayed
		setOp("not-a-uuid", sess.ID, "barbell-back-squat", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000002", sess.ID, "no-such-exercise", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000003", sess.ID, "barbell-back-squat", 5000, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000004", bobs.ID, "barbell-back-squat", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000005", sess.ID, "pull-up", 10, 8, 8, now),
		{OpID: opID(), Op: OpEditNotes, Payload: payload(map[string]any{"session_id": sess.ID, "notes": "good day", "updated_at": now.Add(time.Second)})},
		{OpID: opID(), Op: "explode", Payload: payload(map[string]any{})},
	}
	results, err := e.svc.ApplyOps(ctx, e.alice, ops)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	for _, r := range results {
		statuses = append(statuses, r.Status)
	}
	want := []string{"applied", "duplicate", "rejected", "rejected", "rejected", "rejected", "applied", "applied", "rejected"}
	if !slices.Equal(statuses, want) {
		t.Fatalf("statuses = %v\nresults = %+v", statuses, results)
	}

	sets, _ := e.db.SessionSets(ctx, e.alice.ID, sess.ID)
	if len(sets) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	// 100 x 5 @ RPE 8 = 100 / 0.811; pull-ups are bodyweight, so no e1RM.
	for _, s := range sets {
		switch s.Slug {
		case "barbell-back-squat":
			if s.E1RMKg == nil || *s.E1RMKg < 123.3 || *s.E1RMKg > 123.31 {
				t.Fatalf("squat e1RM = %v", s.E1RMKg)
			}
		case "pull-up":
			if s.E1RMKg != nil {
				t.Fatalf("pull-up e1RM = %v", *s.E1RMKg)
			}
		}
	}
	if got, _ := e.db.SessionByID(ctx, e.alice.ID, sess.ID); got.Notes != "good day" {
		t.Fatalf("notes = %q", got.Notes)
	}

	tooMany := make([]Op, MaxOps+1)
	var invalid InvalidError
	if _, err := e.svc.ApplyOps(ctx, e.alice, tooMany); !errors.As(err, &invalid) {
		t.Fatalf("oversized batch: %v", err)
	}
}

func TestFinishAdvancesThePlanOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	sess, _ := e.svc.StartPlanned(ctx, e.alice)
	finish := func() {
		res, err := e.svc.ApplyOps(ctx, e.alice, []Op{{OpID: opID(), Op: OpFinishSession,
			Payload: payload(map[string]any{"session_id": sess.ID, "finished_at": time.Now().UTC()})}})
		if err != nil || res[0].Status != "applied" {
			t.Fatalf("finish: %+v %v", res, err)
		}
	}
	finish()
	finish() // a second device, or a retry with a new op id
	n, _ := e.plans.Next(ctx, e.alice)
	if n.Week != 1 || n.Day != 1 {
		t.Fatalf("cursor = (%d, %d), want (1, 1)", n.Week, n.Day)
	}
	if _, err := e.svc.Open(ctx, e.alice); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("finished session still open")
	}
}

func TestFutureTimestampsAreClamped(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	future := time.Now().Add(1000 * time.Hour)
	if _, err := e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-000000000009", sess.ID, "barbell-back-squat", 100, 5, 0, future)}); err != nil {
		t.Fatal(err)
	}
	sets, _ := e.db.SessionSets(ctx, e.alice.ID, sess.ID)
	if len(sets) != 1 || sets[0].UpdatedAt.After(time.Now().Add(6*time.Minute)) {
		t.Fatalf("updated_at not clamped: %+v", sets)
	}
	// done_at too: a set from 2036 would count as "recent" for e1RM for a decade.
	if sets[0].DoneAt == nil || sets[0].DoneAt.After(time.Now().Add(6*time.Minute)) {
		t.Fatalf("done_at not clamped: %v", sets[0].DoneAt)
	}
}

// failingStore fails the upsert of one set id with a server-side error.
type failingStore struct {
	Store
	failID string
}

func (f failingStore) UpsertSet(ctx context.Context, userID string, s store.Set, opID string) (store.Outcome, error) {
	if s.ID == f.failID {
		return "", errors.New("disk on fire")
	}
	return f.Store.UpsertSet(ctx, userID, s, opID)
}

// One operation hitting a server error must not block the ones after it:
// otherwise the client retries the same batch forever.
func TestServerErrorRejectsOnlyThatOp(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	bad := "01900000-0000-7000-8000-0000000000e1"
	svc := *e.svc
	svc.Store = failingStore{Store: e.db, failID: bad}
	now := time.Now().UTC()
	results, err := svc.ApplyOps(ctx, e.alice, []Op{
		setOp(bad, sess.ID, "barbell-back-squat", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-0000000000e2", sess.ID, "barbell-back-squat", 100, 5, 0, now),
	})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "rejected" || results[0].Reason != "server error" || results[1].Status != "applied" {
		t.Fatalf("results = %+v", results)
	}
}

// A history edit wins even against a set whose (clamped) client timestamp is
// a few minutes ahead of the server: the user saw "saved", so it must be.
func TestHistoryEditBeatsClockAhead(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	id := "01900000-0000-7000-8000-0000000000e3"
	if _, err := e.svc.ApplyOps(ctx, e.alice, []Op{setOp(id, sess.ID, "barbell-back-squat", 100, 5, 0, time.Now().Add(4*time.Minute))}); err != nil {
		t.Fatal(err)
	}
	kg, reps := 105.0, 5
	if err := e.svc.SaveSet(ctx, e.alice, SetInput{ID: id, SessionID: sess.ID, Slug: "barbell-back-squat", Kind: "working", WeightKg: &kg, Reps: &reps}); err != nil {
		t.Fatal(err)
	}
	if _, sets, _ := e.svc.Session(ctx, e.alice, sess.ID); *sets[0].WeightKg != 105 {
		t.Fatalf("history edit lost: %v", *sets[0].WeightKg)
	}
}

func TestBootstrap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	prev, _ := e.svc.StartPlanned(ctx, e.alice)
	then := time.Now().UTC().Add(-time.Hour)
	_, _ = e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-00000000000a", prev.ID, "barbell-back-squat", 110, 5, 9, then)})
	_ = e.svc.Finish(ctx, e.alice, prev.ID)
	_ = e.plans.Choose(ctx, e.alice, 1, 0)
	sess, err := e.svc.StartPlanned(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}

	b, err := e.svc.Bootstrap(ctx, e.alice, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	squat, ok := b.Exercises["barbell-back-squat"]
	if !ok || squat.Name != "Barbell Back Squat" || squat.TMKg == nil || *squat.TMKg != 140 || squat.E1RMKg == nil ||
		squat.Equipment == nil || len(squat.Last) != 1 || *squat.Last[0].WeightKg != 110 {
		t.Fatalf("squat = %+v", squat)
	}
	if !slices.Contains(squat.Alternatives, "hack-squat") {
		t.Fatalf("catalog alternatives missing: %v", squat.Alternatives)
	}
	if _, ok := b.Exercises["hack-squat"]; !ok {
		t.Fatal("an alternative's context is needed to swap offline")
	}
	if len(b.Catalog) < 100 || b.Defaults["barbell"] == nil || b.Defaults["dumbbell"] == nil || b.Unit != "kg" {
		t.Fatalf("catalog %d, defaults %v, unit %q", len(b.Catalog), b.Defaults, b.Unit)
	}
	if b.Session.ID != sess.ID || b.Snapshot.Name != "Squat day" || len(b.Sets) != 0 {
		t.Fatalf("session part = %+v", b.Session)
	}
	if _, err := e.svc.Bootstrap(ctx, e.bob, sess.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob bootstraps alice's session: %v", err)
	}
}

func TestHistoryEditing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	id := "01900000-0000-7000-8000-00000000000b"
	kg, reps := 60.0, 8
	in := SetInput{ID: id, SessionID: sess.ID, Slug: "barbell-bench-press", Kind: "working", WeightKg: &kg, Reps: &reps}
	if err := e.svc.SaveSet(ctx, e.alice, in); err != nil {
		t.Fatal(err)
	}
	kg = 62.5
	if err := e.svc.SaveSet(ctx, e.alice, in); err != nil {
		t.Fatal(err)
	}
	_, sets, _ := e.svc.Session(ctx, e.alice, sess.ID)
	if len(sets) != 1 || *sets[0].WeightKg != 62.5 {
		t.Fatalf("sets = %+v", sets)
	}
	var invalid InvalidError
	reps = -1
	if err := e.svc.SaveSet(ctx, e.alice, in); !errors.As(err, &invalid) {
		t.Fatalf("negative reps: %v", err)
	}
	if err := e.svc.SetNotes(ctx, e.alice, sess.ID, "note"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteSet(ctx, e.alice, sess.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, sets, _ := e.svc.Session(ctx, e.alice, sess.ID); len(sets) != 0 {
		t.Fatal("set not deleted")
	}
	if err := e.svc.Finish(ctx, e.alice, sess.ID); err != nil {
		t.Fatal(err)
	}
	list, more, err := e.svc.History(ctx, e.alice, 0)
	if err != nil || more || len(list) != 1 || list[0].FinishedAt == nil {
		t.Fatalf("history = %+v %v %v", list, more, err)
	}
	if err := e.svc.Delete(ctx, e.bob, sess.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob deletes alice's session: %v", err)
	}
	if err := e.svc.Delete(ctx, e.alice, sess.ID); err != nil {
		t.Fatal(err)
	}
}

// The bootstrap's PR table holds other sessions' bests (warmups excluded);
// the session's own sets are judged by the companion.
func TestBootstrapPRs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	prev, _ := e.svc.StartPlanned(ctx, e.alice)
	then := time.Now().UTC().Add(-time.Hour)
	warm := setOp("01900000-0000-7000-8000-00000000001b", prev.ID, "barbell-back-squat", 130, 5, 0, then)
	var in SetInput
	_ = json.Unmarshal(warm.Payload, &in)
	in.Kind = "warmup"
	warm.Payload = payload(in)
	if _, err := e.svc.ApplyOps(ctx, e.alice, []Op{
		setOp("01900000-0000-7000-8000-00000000001a", prev.ID, "barbell-back-squat", 110, 5, 9, then), warm}); err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Finish(ctx, e.alice, prev.ID)
	_ = e.plans.Choose(ctx, e.alice, 1, 0)
	sess, err := e.svc.StartPlanned(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.Bootstrap(ctx, e.alice, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Exercises["barbell-back-squat"].PRs; got[5] != 110 || len(got) != 1 {
		t.Fatalf("bootstrap PRs = %v, want {5: 110} (warmups don't count)", got)
	}
	if _, err := e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-00000000001c", sess.ID, "barbell-back-squat", 120, 5, 9, time.Now().UTC())}); err != nil {
		t.Fatal(err)
	}
	b, _ = e.svc.Bootstrap(ctx, e.alice, sess.ID)
	if got := b.Exercises["barbell-back-squat"].PRs; got[5] != 110 {
		t.Fatalf("the session's own sets leaked into its PR table: %v", got)
	}
}
