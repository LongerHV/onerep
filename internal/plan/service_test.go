package plan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc   *Service
	ex    *exercise.Service
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
	alice, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	if err := ex.EnsureStarterEquipment(ctx, alice); err != nil {
		t.Fatal(err)
	}
	return env{svc: &Service{Store: db, Exercises: ex, History: db}, ex: ex, alice: alice, bob: bob}
}

func TestStarterTemplateIsValid(t *testing.T) {
	e := newEnv(t)
	_, ps := e.svc.Validate(context.Background(), e.alice, StarterTemplate())
	if ps.HasErrors() {
		t.Fatalf("starter template: %v", ps)
	}
}

// weeksDoc is a small plan with two days a week for n weeks.
func weeksDoc(n int) []byte {
	return []byte(strings.NewReplacer("N", string(rune('0'+n))).Replace(`{"name": "Test", "weeks": N, "days": [
		{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5, "load": {"pct_tm": 0.75}}]}]}]},
		{"name": "B", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10"}]}]}]}]}`))
}

func TestFollowAndMoveThroughPlan(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	p, draft, err := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Follow(ctx, e.alice, p.ID); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("following a draft-only plan: %v", err)
	}
	if _, err := e.svc.Activate(ctx, e.alice, draft.ID, DocHash(draft.Doc)); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Follow(ctx, e.bob, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob follows alice's plan: %v", err)
	}
	if err := e.svc.Follow(ctx, e.alice, p.ID); err != nil {
		t.Fatal(err)
	}

	tm := 140.0
	if err := e.ex.SetTrainingMax(ctx, e.alice.ID, "barbell-back-squat", &tm, "web"); err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Next(ctx, e.alice)
	if err != nil || n == nil || n.Week != 1 || n.Day != 0 || n.Today.Name != "A" {
		t.Fatalf("next = %+v, %v", n, err)
	}
	squat := n.Today.Groups[0].Exercises[0]
	if squat.Name != "Barbell Back Squat" || squat.Sets[0].Kg == nil || *squat.Sets[0].Kg != 105 {
		t.Fatalf("resolved squat = %+v", squat)
	}

	for _, want := range [][2]int{{1, 1}, {2, 0}, {2, 1}} {
		if err := e.svc.Skip(ctx, e.alice); err != nil {
			t.Fatal(err)
		}
		n, _ = e.svc.Next(ctx, e.alice)
		if [2]int{n.Week, n.Day} != want {
			t.Fatalf("after skip: (%d, %d), want %v", n.Week, n.Day, want)
		}
	}
	_ = e.svc.Skip(ctx, e.alice)
	if n, _ = e.svc.Next(ctx, e.alice); !n.Complete {
		t.Fatalf("plan should be complete: %+v", n)
	}
	if err := e.svc.Skip(ctx, e.alice); err != nil {
		t.Fatal("skipping a complete plan must be harmless")
	}
	if err := e.svc.Restart(ctx, e.alice); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Choose(ctx, e.alice, 3, 0); !errors.Is(err, ErrNoSuchDay) {
		t.Fatalf("choosing week 3 of 2: %v", err)
	}
	if err := e.svc.Choose(ctx, e.alice, 2, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n.Week != 2 || n.Day != 1 {
		t.Fatalf("chosen day: (%d, %d)", n.Week, n.Day)
	}

	// A new version with the same days keeps the cursor...
	if _, m, err := e.svc.Save(ctx, e.alice, p.ID, weeksDoc(3), SaveActivate, "web", ""); err != nil || m.Moved() {
		t.Fatalf("save 3 weeks: move=%+v err=%v", m, err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n.Week != 2 || n.Day != 1 {
		t.Fatalf("cursor moved: (%d, %d)", n.Week, n.Day)
	}
	// ...one without the current week completes the plan, and the comparison says so first.
	draft1, _, err := e.svc.Save(ctx, e.alice, p.ID, weeksDoc(1), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := e.svc.Compare(ctx, e.alice, draft1.ID)
	if err != nil || cmp.Cursor == nil || cmp.Cursor.Kind != CursorPastEnd || cmp.Base == nil || !Changed(cmp.JSON) || len(cmp.Days) != 4 {
		t.Fatalf("comparison = %+v, %v", cmp, err)
	}
	m, err := e.svc.Activate(ctx, e.alice, draft1.ID, cmp.DocHash)
	if err != nil || m.Kind != CursorPastEnd || !m.Complete {
		t.Fatalf("activate 1-week version: move=%+v err=%v", m, err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); !n.Complete || n.Week != 2 {
		t.Fatalf("plan should be complete: %+v", n)
	}

	// Archiving the followed plan stops following it, and it can't be followed until restored.
	if err := e.svc.Archive(ctx, e.alice, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n != nil {
		t.Fatal("archived plan still followed")
	}
	if err := e.svc.Follow(ctx, e.alice, p.ID); !errors.Is(err, ErrArchived) {
		t.Fatalf("following an archived plan: %v", err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n != nil {
		t.Fatal("archived plan followed")
	}
}

func TestInvalidDocumentIsNotSaved(t *testing.T) {
	e := newEnv(t)
	_, _, err := e.svc.Create(context.Background(), e.alice, []byte(`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "nope", "sets": [{"count": 1, "reps": 1}]}]}]}]}`), SaveActivate, "web", "")
	var ps Problems
	if !errors.As(err, &ps) || ps[0].Pointer != "/days/0/groups/0/exercises/0/slug" {
		t.Fatalf("got %v", err)
	}
	plans, _ := e.svc.Plans(context.Background(), e.alice)
	if len(plans) != 0 {
		t.Fatalf("invalid plan was stored: %+v", plans)
	}
}

func TestPlansSummary(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, _, _ := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveActivate, "web", "")
	_, _, _ = e.svc.Save(ctx, e.alice, p.ID, weeksDoc(2), SaveDraft, "mcp", "")
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	list, err := e.svc.Plans(ctx, e.alice)
	if err != nil || len(list) != 1 || list[0].ActiveVersion != 1 || list[0].Drafts != 1 || !list[0].Following || list[0].Name != "Test" {
		t.Fatalf("plans = %+v, %v", list, err)
	}
	if other, _ := e.svc.Plans(ctx, e.bob); len(other) != 0 {
		t.Fatal("bob sees alice's plans")
	}
}

// Absolute weights mean what the lifter meant when saving: a plan without
// "unit" is pinned to the user's unit at save time.
func TestSavedPlanKeepsItsUnit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	raw := []byte(`{"name": "Fixed", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "dumbbell-curl", "sets": [{"count": 1, "reps": 10, "load": {"weight": 20}}]}]}]}]}`)
	p, v, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if doc, _ := Decode(v.Doc); doc.Unit != "kg" {
		t.Fatalf("stored unit = %q", doc.Unit)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	lbUser := e.alice
	lbUser.Unit = "lb"
	n, err := e.svc.Next(ctx, lbUser)
	if err != nil {
		t.Fatal(err)
	}
	// 20 kg dumbbells (the kg starter set has 20), not 20 lb.
	if kg := n.Today.Groups[0].Exercises[0].Sets[0].Kg; kg == nil || *kg != 20 {
		t.Fatalf("resolved = %v", kg)
	}
}

func TestPlanSurvivesDeletedCustomExercise(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := exercise.Input{Slug: "zercher-squat", Name: "Zercher Squat", Measurement: "weight_reps", EquipmentKind: "barbell", PrimaryMuscles: []string{"quads"}}
	if _, err := e.ex.Create(ctx, e.alice.ID, in); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"name": "Z", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "zercher-squat", "sets": [{"count": 3, "reps": 5}]}]}]}]}`)
	p, _, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	if err := e.ex.Delete(ctx, e.alice.ID, "zercher-squat"); err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Next(ctx, e.alice)
	if err != nil || n.Today.Groups[0].Exercises[0].Name != "zercher-squat" {
		t.Fatalf("next = %+v, %v", n, err)
	}
}

// Weeks often repeat a day name (A/B/A); each occurrence is compared on its own.
func TestCompareRepeatedDayNames(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	doc := func(thirdSets int) []byte {
		return []byte(`{"name": "ABA", "weeks": 1, "days": [
			{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5}]}]}]},
			{"name": "B", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": 5}]}]}]},
			{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": ` + string(rune('0'+thirdSets)) + `, "reps": 5}]}]}]}]}`)
	}
	p, _, err := e.svc.Create(ctx, e.alice, doc(3), SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, _, err := e.svc.Save(ctx, e.alice, p.ID, doc(5), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := e.svc.Compare(ctx, e.alice, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.Days) != 1 || cmp.Days[0].Name != "A (2nd)" || cmp.Days[0].Week != 1 {
		t.Fatalf("changed days = %+v", cmp.Days)
	}
}

// The plan's name is the active version's name: drafts (from the editor or,
// later, the AI) must not rename a plan the user is training.
func TestPlanNameFollowsTheActiveVersion(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	named := func(name string) []byte {
		return []byte(strings.Replace(string(weeksDoc(2)), `"name": "Test"`, `"name": "`+name+`"`, 1))
	}
	p, v1, err := e.svc.Create(ctx, e.alice, named("Block A"), SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	name := func() string {
		got, _, err := e.svc.Plan(ctx, e.alice, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Name
	}
	draft, _, err := e.svc.Save(ctx, e.alice, p.ID, named("Block B idea"), SaveDraft, "mcp", "")
	if err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block A" {
		t.Fatalf("a draft renamed the plan to %q", n)
	}
	if _, err := e.svc.Activate(ctx, e.alice, draft.ID, DocHash(draft.Doc)); err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block B idea" {
		t.Fatalf("after activating the draft: %q", n)
	}
	if _, err := e.svc.Activate(ctx, e.alice, v1.ID, DocHash(v1.Doc)); err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block A" {
		t.Fatalf("after rolling back: %q", n)
	}
}

// Logged sets give RPE targets a weight: the best e1RM in the user's window.
func TestRPELoadsUseLoggedE1RM(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	raw := []byte(`{"name": "R", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5, "load": {"rpe": 8}}]}]}]}]}`)
	p, _, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	n, _ := e.svc.Next(ctx, e.alice)
	if n.Today.Groups[0].Exercises[0].Sets[0].Kg != nil {
		t.Fatal("without history the RPE weight must be left open")
	}

	db := e.svc.Store.(*store.DB)
	s, _ := db.CreateSession(ctx, store.Session{UserID: e.alice.ID, Name: "log", Snapshot: []byte(`{}`)})
	kg, reps, e1rm, now := 120.0, 5, 150.0, time.Now()
	if _, err := db.UpsertSet(ctx, e.alice.ID, store.Set{ID: "s1", SessionID: s.ID, Slug: "barbell-back-squat", Kind: "working",
		WeightKg: &kg, Reps: &reps, E1RMKg: &e1rm, DoneAt: &now, UpdatedAt: now}, ""); err != nil {
		t.Fatal(err)
	}
	n, _ = e.svc.Next(ctx, e.alice)
	// 5 @ RPE 8 = 81.1% of 150 = 121.65 -> 120 on the starter barbell.
	if got := n.Today.Groups[0].Exercises[0].Sets[0].Kg; got == nil || *got != 120 {
		t.Fatalf("RPE weight = %v", got)
	}
}

func TestAdvanceFrom(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, _, _ := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveActivate, "web", "")
	_ = e.svc.Follow(ctx, e.alice, p.ID)

	// Finishing a day the cursor is not on (an older session) changes nothing.
	if err := e.svc.AdvanceFrom(ctx, e.alice, p.ID, 2, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Week != 1 || n.Day != 0 {
		t.Fatalf("cursor moved: (%d, %d)", n.Week, n.Day)
	}
	if err := e.svc.AdvanceFrom(ctx, e.alice, p.ID, 1, 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Week != 1 || n.Day != 1 {
		t.Fatalf("cursor after finishing day 1: (%d, %d)", n.Week, n.Day)
	}
	if err := e.svc.AdvanceFrom(ctx, e.alice, "other-plan", 1, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Day != 1 {
		t.Fatal("another plan's session moved the cursor")
	}
}
func TestSaveDraftNeverTouchesActiveVersions(t *testing.T) {
	e := newEnv(t)
	ctx, svc, user := context.Background(), e.svc, e.alice
	doc := func(name string) []byte {
		return []byte(`{"name":"` + name + `","weeks":1,"days":[{"name":"D","groups":[{"exercises":[{"slug":"barbell-back-squat","sets":[{"count":3,"reps":5}]}]}]}]}`)
	}
	d, err := svc.SaveDraft(ctx, user, DraftInput{Doc: doc("New"), Source: "mcp"})
	if err != nil || d.Version.Status != store.PlanDraft || d.Version.Source != "mcp" || d.Plan.Name != "New" {
		t.Fatalf("new plan draft = %+v, %v", d, err)
	}
	d2, err := svc.SaveDraft(ctx, user, DraftInput{PlanID: d.Plan.ID, Doc: doc("New v2"), Source: "mcp", Note: "heavier"})
	if err != nil || d2.Version.Version != 2 || d2.Version.Status != store.PlanDraft {
		t.Fatalf("second draft = %+v, %v", d2, err)
	}
	d3, err := svc.SaveDraft(ctx, user, DraftInput{VersionID: d2.Version.ID, Doc: doc("New v2b"), Source: "mcp"})
	if err != nil || d3.Version.ID != d2.Version.ID || !strings.Contains(string(d3.Version.Doc), "New v2b") {
		t.Fatalf("replaced draft = %+v, %v", d3, err)
	}
	if _, err := svc.Activate(ctx, user, d3.Version.ID, DocHash(d3.Version.Doc)); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveDraft(ctx, user, DraftInput{VersionID: d3.Version.ID, Doc: doc("Sneaky"), Source: "mcp"}); !errors.Is(err, store.ErrNotDraft) {
		t.Fatalf("replacing the active version: %v", err)
	}
	var ps Problems
	if _, err := svc.SaveDraft(ctx, user, DraftInput{Doc: []byte(`{"name":"x"}`), Source: "mcp"}); !errors.As(err, &ps) || !ps.HasErrors() {
		t.Fatalf("invalid doc: %v", err)
	}
}

// An AI may only replace its own drafts: a draft the user wrote in the web
// editor is never overwritten (there is no undo).
func TestSaveDraftKeepsOthersDrafts(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	doc := []byte(`{"name":"Mine","weeks":1,"days":[{"name":"D","groups":[{"exercises":[{"slug":"barbell-back-squat","sets":[{"count":3,"reps":5}]}]}]}]}`)
	_, v, err := e.svc.Create(ctx, e.alice, doc, SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{VersionID: v.ID, Doc: doc, Source: "mcp"}); !errors.Is(err, ErrForeignDraft) {
		t.Fatalf("replacing the user's draft: %v", err)
	}
	if got, _ := e.svc.Version(ctx, e.alice, v.ID); got.Source != "web" {
		t.Fatalf("the user's draft was taken over: %+v", got)
	}
}

// Changing versions never restarts a plan (the lifter decides when to restart).
func TestVersionChangeKeepsProgress(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, v2, err := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	db := e.svc.Store.(*store.DB)
	at := func(week, day int) {
		t.Helper()
		if err := db.SetActivePlan(ctx, e.alice.ID, store.ActivePlan{PlanID: p.ID, Week: week, Day: day}); err != nil {
			t.Fatal(err)
		}
	}
	cursor := func() [2]int {
		t.Helper()
		a, err := db.ActivePlan(ctx, e.alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		return [2]int{a.Week, a.Day}
	}

	// A finished 2-week plan stays complete when a 3-week version is activated...
	at(3, 0)
	v3, _, _ := e.svc.Save(ctx, e.alice, p.ID, weeksDoc(3), SaveDraft, "web", "")
	if cmp, _ := e.svc.Compare(ctx, e.alice, v3.ID); cmp.Cursor == nil || cmp.Cursor.Kind != CursorStaysComplete {
		t.Fatalf("comparison cursor = %+v", cmp.Cursor)
	}
	if m, err := e.svc.Activate(ctx, e.alice, v3.ID, DocHash(v3.Doc)); err != nil || m.Kind != CursorStaysComplete {
		t.Fatalf("activate: %+v %v", m, err)
	}
	if got := cursor(); got != [2]int{4, 0} {
		t.Fatalf("cursor after a longer version = %v", got)
	}
	// ...and when the shorter one comes back.
	if _, err := e.svc.Activate(ctx, e.alice, v2.ID, DocHash(v2.Doc)); err != nil {
		t.Fatal(err)
	}
	if got := cursor(); got != [2]int{3, 0} {
		t.Fatalf("cursor after a shorter version = %v", got)
	}

	// A day that no longer exists moves on to the next day that does.
	at(1, 1)
	oneDay := []byte(`{"name": "Test", "weeks": 2, "days": [
		{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5}]}]}]}]}`)
	if _, m, err := e.svc.Save(ctx, e.alice, p.ID, oneDay, SaveActivate, "web", ""); err != nil || m.Kind != CursorDayMissing || m.Complete {
		t.Fatalf("save one-day version: %+v %v", m, err)
	}
	if got := cursor(); got != [2]int{2, 0} {
		t.Fatalf("cursor after the day disappeared = %v", got)
	}

	// Next never silently restarts on a position the version doesn't have.
	at(1, 3)
	if n, err := e.svc.Next(ctx, e.alice); err != nil || n.Week != 2 || n.Day != 0 || n.Today.Name != "A" {
		t.Fatalf("next from a missing day = %+v, %v", n, err)
	}
	at(2, 3)
	if n, err := e.svc.Next(ctx, e.alice); err != nil || !n.Complete {
		t.Fatalf("next from a missing last day = %+v, %v", n, err)
	}
}

func namedDoc(name string) []byte {
	return []byte(`{"name":"` + name + `","weeks":1,"days":[{"name":"D","groups":[{"exercises":[{"slug":"barbell-back-squat","sets":[{"count":3,"reps":5}]}]}]}]}`)
}

// Activation takes the document the user reviewed: if the AI replaced the
// draft since the compare page rendered it, activation is refused.
func TestActivateRefusesAChangedDraft(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{Doc: namedDoc("First"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := e.svc.Compare(ctx, e.alice, d.Version.ID)
	if err != nil || cmp.DocHash == "" || cmp.DocHash != DocHash(d.Version.Doc) {
		t.Fatalf("compare hash = %q, %v", cmp.DocHash, err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{VersionID: d.Version.ID, Doc: namedDoc("Second"), Source: "mcp"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Activate(ctx, e.alice, d.Version.ID, cmp.DocHash); !errors.Is(err, ErrDocChanged) {
		t.Fatalf("activating a changed draft: %v", err)
	}
	if _, err := e.svc.Activate(ctx, e.alice, d.Version.ID, ""); !errors.Is(err, ErrDocChanged) {
		t.Fatalf("activating without a hash: %v", err)
	}
	if v, _ := e.svc.Version(ctx, e.alice, d.Version.ID); v.Status != store.PlanDraft {
		t.Fatalf("refused activation changed the version: %+v", v)
	}
	cmp, _ = e.svc.Compare(ctx, e.alice, d.Version.ID)
	if _, err := e.svc.Activate(ctx, e.bob, d.Version.ID, cmp.DocHash); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob activates alice's version: %v", err)
	}
	if _, err := e.svc.Activate(ctx, e.alice, d.Version.ID, cmp.DocHash); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := e.svc.Plan(ctx, e.alice, d.Plan.ID); p.Name != "Second" {
		t.Fatalf("plan name = %q", p.Name)
	}
}

func TestSaveDraftRejectsAVersionOfAnotherPlan(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	a, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{Doc: namedDoc("A"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{Doc: namedDoc("B"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{PlanID: b.Plan.ID, VersionID: a.Version.ID, Doc: namedDoc("A2"), Source: "mcp"}); !errors.Is(err, ErrVersionNotInPlan) {
		t.Fatalf("plan_id of another plan: %v", err)
	}
	if v, _ := e.svc.Version(ctx, e.alice, a.Version.ID); !strings.Contains(string(v.Doc), `"A"`) {
		t.Fatalf("the draft changed: %s", v.Doc)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{PlanID: a.Plan.ID, VersionID: a.Version.ID, Doc: namedDoc("A2"), Source: "mcp"}); err != nil {
		t.Fatalf("matching plan_id: %v", err)
	}
}

// An archived plan is read-only until restored, from the web editor and from the AI alike.
func TestArchivedPlansTakeNoNewVersions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{Doc: namedDoc("Old"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Archive(ctx, e.alice, d.Plan.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{PlanID: d.Plan.ID, Doc: namedDoc("New"), Source: "mcp"}); !errors.Is(err, ErrArchived) {
		t.Fatalf("new draft of an archived plan: %v", err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{VersionID: d.Version.ID, Doc: namedDoc("New"), Source: "mcp"}); !errors.Is(err, ErrArchived) {
		t.Fatalf("replacing a draft of an archived plan: %v", err)
	}
	for _, status := range []string{SaveDraft, SaveActivate} {
		if _, _, err := e.svc.Save(ctx, e.alice, d.Plan.ID, namedDoc("New"), status, "web", ""); !errors.Is(err, ErrArchived) {
			t.Fatalf("saving an archived plan from the editor (%s): %v", status, err)
		}
	}
	if _, vs, _ := e.svc.Plan(ctx, e.alice, d.Plan.ID); len(vs) != 1 || !strings.Contains(string(vs[0].Doc), "Old") {
		t.Fatalf("versions = %+v", vs)
	}
	if err := e.svc.Archive(ctx, e.alice, d.Plan.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{PlanID: d.Plan.ID, Doc: namedDoc("New"), Source: "mcp"}); err != nil {
		t.Fatalf("restored plan: %v", err)
	}
}

// A plan is named after its active version; until it has one, after its only draft.
func TestReplacingTheOnlyDraftRenamesThePlan(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	d, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{Doc: namedDoc("Working title"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{VersionID: d.Version.ID, Doc: namedDoc("Final title"), Source: "mcp"})
	if err != nil || r.Plan.Name != "Final title" {
		t.Fatalf("replaced draft plan = %+v, %v", r.Plan, err)
	}
	if p, _, _ := e.svc.Plan(ctx, e.alice, d.Plan.ID); p.Name != "Final title" {
		t.Fatalf("stored plan name = %q", p.Name)
	}

	// With more than one version, drafts leave the name alone.
	d2, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{PlanID: d.Plan.ID, Doc: namedDoc("Other idea"), Source: "mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SaveDraft(ctx, e.alice, DraftInput{VersionID: d2.Version.ID, Doc: namedDoc("Other idea b"), Source: "mcp"}); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := e.svc.Plan(ctx, e.alice, d.Plan.ID); p.Name != "Final title" {
		t.Fatalf("a second draft renamed the plan: %q", p.Name)
	}
}
