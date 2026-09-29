package exercise

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

func newService(t *testing.T) (*Service, store.User) {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	u, err := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: db}, u
}

func slugsOf(es []store.Exercise) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Slug)
	}
	return out
}

func validInput(slug string) Input {
	return Input{Slug: slug, Name: "Zercher Squat", Measurement: "weight_reps", EquipmentKind: "barbell",
		PrimaryMuscles: []string{"quads"}, SecondaryMuscles: []string{"upper-back"}, Aliases: []string{" zercher ", "", "zercher"}}
}

func TestCatalogSearch(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	got, err := s.Catalog(ctx, u.ID, "Bench DUMBBELL")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(slugsOf(got), []string{"dumbbell-bench-press", "dumbbell-incline-bench-press"}) {
		t.Fatalf("search = %v", slugsOf(got))
	}
	if got, _ := s.Catalog(ctx, u.ID, "rdl"); !slices.Contains(slugsOf(got), "barbell-romanian-deadlift") {
		t.Fatalf("alias search = %v", slugsOf(got))
	}
	if all, _ := s.Catalog(ctx, u.ID, "  "); len(all) < 100 {
		t.Fatalf("empty query returned %d exercises", len(all))
	}
}

func TestCreateAndCustomize(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()

	created, err := s.Create(ctx, u.ID, validInput("zercher-squat"))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Custom() || !slices.Equal(created.Aliases, []string{"zercher"}) {
		t.Fatalf("created = %+v", created)
	}

	var fe FieldErrors
	_, err = s.Create(ctx, u.ID, validInput("barbell-back-squat"))
	if !errors.As(err, &fe) || fe["slug"] == "" {
		t.Fatalf("slug taken by the seed: want slug error, got %v", err)
	}
	bad := validInput("Bad Slug")
	bad.PrimaryMuscles = nil
	bad.SecondaryMuscles = []string{"wings"}
	_, err = s.Create(ctx, u.ID, bad)
	if !errors.As(err, &fe) || fe["slug"] == "" || fe["primary_muscles"] == "" {
		t.Fatalf("invalid input: got %v", err)
	}
	overlap := validInput("overlap")
	overlap.SecondaryMuscles = []string{"quads"}
	if _, err := s.Create(ctx, u.ID, overlap); !errors.As(err, &fe) || fe["secondary_muscles"] == "" {
		t.Fatalf("overlapping muscles: got %v", err)
	}

	unknown := validInput("unknown-secondary")
	unknown.SecondaryMuscles = []string{"wings"}
	if _, err := s.Create(ctx, u.ID, unknown); !errors.As(err, &fe) || fe["secondary_muscles"] == "" || fe["primary_muscles"] != "" {
		t.Fatalf("unknown secondary muscle should be reported under secondary_muscles: got %v", err)
	}
	dup := validInput("dup-muscles")
	dup.PrimaryMuscles = []string{"quads", "glutes", "quads"}
	dup.SecondaryMuscles = []string{"upper-back", "upper-back"}
	got, err := s.Create(ctx, u.ID, dup)
	if err != nil || !slices.Equal(got.PrimaryMuscles, []string{"quads", "glutes"}) || !slices.Equal(got.SecondaryMuscles, []string{"upper-back"}) {
		t.Fatalf("duplicate muscles: %+v, %v", got, err)
	}

	// Updating a seeded exercise creates the user's customized copy.
	in := validInput("barbell-back-squat")
	in.Name = "Low Bar Squat"
	updated, err := s.Update(ctx, u.ID, in)
	if err != nil || !updated.Overrides || updated.Name != "Low Bar Squat" {
		t.Fatalf("customize = %+v, %v", updated, err)
	}
	if err := s.Delete(ctx, u.ID, "barbell-back-squat"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, u.ID, "barbell-back-squat"); got.Name != "Barbell Back Squat" {
		t.Fatalf("reset: %q", got.Name)
	}
	if _, err := s.Update(ctx, u.ID, validInput("does-not-exist")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update unknown: %v", err)
	}
}

func TestAlternatives(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	var fe FieldErrors
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "pull-up"); !errors.As(err, &fe) {
		t.Fatalf("self alternative: %v", err)
	}
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown alternative: %v", err)
	}
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "inverted-row"); err != nil {
		t.Fatal(err)
	}
	alts, err := s.Alternatives(ctx, u.ID, "pull-up")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range alts {
		names = append(names, a.Exercise.Name)
	}
	if !slices.Contains(names, "Inverted Row") || !slices.Contains(names, "Chin-Up") {
		t.Fatalf("alternatives = %v", names)
	}
}

func TestEquipmentLookupOrder(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	squat, _ := s.Get(ctx, u.ID, "barbell-back-squat")

	st, err := s.Settings(ctx, u.ID, squat)
	if err != nil || st.Equipment != nil {
		t.Fatalf("no profiles yet: %+v, %v", st, err)
	}

	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Settings(ctx, u.ID, squat)
	if st.Equipment == nil || st.Equipment.Name != "Barbell" || st.Linked {
		t.Fatalf("default for kind: %+v", st)
	}

	home, err := s.SaveEquipment(ctx, store.Equipment{UserID: u.ID, Name: "Home bar", Spec: calc.Equipment{
		Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 15, Plates: []float64{10}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LinkEquipment(ctx, u.ID, squat.Slug, home.ID); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Settings(ctx, u.ID, squat)
	if st.Equipment == nil || st.Equipment.ID != home.ID || !st.Linked {
		t.Fatalf("explicit link: %+v", st)
	}
	if err := s.LinkEquipment(ctx, u.ID, squat.Slug, ""); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.Settings(ctx, u.ID, squat); st.Linked {
		t.Fatalf("unlink: %+v", st)
	}
}

func TestStarterEquipmentOnceAndInUnit(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListEquipment(ctx, u.ID)
	if len(list) != 2 {
		t.Fatalf("starter profiles: %+v", list)
	}
	for _, e := range list {
		if err := s.DeleteEquipment(ctx, u.ID, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	// u is stale (EquipmentInitialized false) like an old session; the store
	// flag still prevents re-creating what the user deleted.
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListEquipment(ctx, u.ID); len(list) != 0 {
		t.Fatalf("starter profiles came back after deletion: %+v", list)
	}

	lb := StarterEquipment(calc.UnitLb)
	if lb[0].Spec.Unit != calc.UnitLb || lb[0].Spec.Config.Bar != 45 || lb[1].Spec.Config.Weights[0] != 5 {
		t.Fatalf("lb starters: %+v", lb)
	}
	for _, e := range append(StarterEquipment(calc.UnitKg), lb...) {
		if err := e.Spec.Validate(); err != nil {
			t.Fatalf("starter %s invalid: %v", e.Name, err)
		}
	}
}

func TestSaveEquipmentValidates(t *testing.T) {
	s, u := newService(t)
	var fe FieldErrors
	_, err := s.SaveEquipment(context.Background(), store.Equipment{UserID: u.ID, Name: " ",
		Spec: calc.Equipment{Kind: calc.KindDumbbell, Unit: calc.UnitKg}})
	if !errors.As(err, &fe) || fe["name"] == "" || fe["config"] == "" {
		t.Fatalf("got %v", err)
	}
}

func TestPercentOfTM(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	squat, _ := s.Get(ctx, u.ID, "barbell-back-squat")

	if _, err := s.PercentOfTM(ctx, u, squat, 0.75); !errors.Is(err, ErrNoTrainingMax) {
		t.Fatalf("without TM: %v", err)
	}
	var fe FieldErrors
	if err := s.SetTrainingMax(ctx, u.ID, squat.Slug, ptr(-1), "web"); !errors.As(err, &fe) {
		t.Fatalf("negative TM: %v", err)
	}
	for _, v := range []float64{math.NaN(), math.Inf(1)} {
		if err := s.SetTrainingMax(ctx, u.ID, squat.Slug, ptr(v), "web"); !errors.As(err, &fe) {
			t.Fatalf("TM %v: %v", v, err)
		}
	}
	if err := s.SetTrainingMax(ctx, u.ID, squat.Slug, ptr(140), "web"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err := s.PercentOfTM(ctx, u, squat, 0.75)
	if err != nil || got.Kg != 105 || !slices.Equal(got.PerSide, []float64{25, 15, 2.5}) {
		t.Fatalf("75%% of 140 = %+v, %v", got, err)
	}
	if hist, _ := s.TrainingMaxHistory(ctx, u.ID, squat.Slug); len(hist) != 1 {
		t.Fatalf("history: %+v", hist)
	}
}

func ptr(v float64) *float64 { return &v }

// resync syncs the embedded seed plus extra entries, or without the slugs in
// drop, as a later release of the seed file would.
func resync(t *testing.T, s *Service, extra []store.Exercise, extraAlts map[string][]string, drop ...string) {
	t.Helper()
	exercises, alts, err := loadSeed()
	if err != nil {
		t.Fatal(err)
	}
	exercises = slices.DeleteFunc(append(exercises, extra...), func(e store.Exercise) bool { return slices.Contains(drop, e.Slug) })
	for k, v := range extraAlts {
		alts[k] = append(alts[k], v...)
	}
	if err := s.Store.(*store.DB).SyncSeed(context.Background(), exercises, alts); err != nil {
		t.Fatal(err)
	}
}

func TestSaveSettingsValidatesEverythingFirst(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	home, err := s.SaveEquipment(ctx, store.Equipment{UserID: u.ID, Name: "Home bar", Spec: calc.Equipment{
		Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 15, Plates: []float64{10}}}})
	if err != nil {
		t.Fatal(err)
	}
	bells, _ := s.Store.DefaultEquipment(ctx, u.ID, calc.KindDumbbell)
	const squat = "barbell-back-squat"

	var fe FieldErrors
	err = s.SaveSettings(ctx, u.ID, squat, SettingsInput{EquipmentID: home.ID, SetTrainingMax: true, TrainingMaxKg: ptr(2000)}, "web")
	if !errors.As(err, &fe) || !strings.Contains(fe["training_max"], "1,500 kg") {
		t.Fatalf("TM over the limit: %v", err)
	}
	if ue, _ := s.Store.UserExercise(ctx, u.ID, squat); ue.EquipmentID != "" || ue.TrainingMaxKg != nil {
		t.Fatalf("a rejected save changed the settings: %+v", ue)
	}

	err = s.SaveSettings(ctx, u.ID, squat, SettingsInput{EquipmentID: bells.ID, SetTrainingMax: true, TrainingMaxKg: ptr(140)}, "web")
	if !errors.As(err, &fe) || fe["equipment_id"] == "" {
		t.Fatalf("dumbbells for a barbell exercise: %v", err)
	}
	if ue, _ := s.Store.UserExercise(ctx, u.ID, squat); ue.EquipmentID != "" || ue.TrainingMaxKg != nil {
		t.Fatalf("a rejected save changed the settings: %+v", ue)
	}
	if err := s.LinkEquipment(ctx, u.ID, squat, bells.ID); !errors.As(err, &fe) || fe["equipment_id"] == "" {
		t.Fatalf("LinkEquipment with the wrong kind: %v", err)
	}

	other, _ := s.Store.(*store.DB).UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	if err := s.SaveSettings(ctx, other.ID, squat, SettingsInput{EquipmentID: home.ID}, "web"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("another user's profile: %v", err)
	}
	if err := s.SaveSettings(ctx, u.ID, "nope", SettingsInput{}, "web"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown exercise: %v", err)
	}

	err = s.SaveSettings(ctx, u.ID, squat, SettingsInput{EquipmentID: home.ID, SetTrainingMax: true, TrainingMaxKg: ptr(140)}, "web")
	if err != nil {
		t.Fatal(err)
	}
	if ue, _ := s.Store.UserExercise(ctx, u.ID, squat); ue.EquipmentID != home.ID || *ue.TrainingMaxKg != 140 {
		t.Fatalf("saved: %+v", ue)
	}
	// Without SetTrainingMax only the link changes.
	if err := s.SaveSettings(ctx, u.ID, squat, SettingsInput{}, "web"); err != nil {
		t.Fatal(err)
	}
	if ue, _ := s.Store.UserExercise(ctx, u.ID, squat); ue.EquipmentID != "" || *ue.TrainingMaxKg != 140 {
		t.Fatalf("link only: %+v", ue)
	}
}

// A link made before the exercise's kind changed is ignored: the exercise
// rounds with the default profile for its new kind.
func TestSettingsIgnoresLinkOfAnotherKind(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	bar, _ := s.Store.DefaultEquipment(ctx, u.ID, calc.KindBarbell)
	if _, err := s.Create(ctx, u.ID, validInput("zercher-squat")); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkEquipment(ctx, u.ID, "zercher-squat", bar.ID); err != nil {
		t.Fatal(err)
	}
	in := validInput("zercher-squat")
	in.EquipmentKind = calc.KindDumbbell
	ex, err := s.Update(ctx, u.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Settings(ctx, u.ID, ex)
	if err != nil || st.Linked || st.Equipment == nil || st.Equipment.Spec.Kind != calc.KindDumbbell {
		t.Fatalf("settings after the kind changed: %+v, %v", st, err)
	}
}

// An exercise the user created stays theirs when a later seed adds its slug:
// it is not "customized", and the seed's alternatives don't attach to it.
func TestCreatedExerciseSurvivesSeedCollision(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, u.ID, validInput("zercher-squat")); err != nil {
		t.Fatal(err)
	}
	seeded := store.Exercise{Slug: "zercher-squat", Name: "Seeded Zercher", Measurement: "weight_reps",
		EquipmentKind: "barbell", PrimaryMuscles: []string{"quads"}}
	resync(t, s, []store.Exercise{seeded}, map[string][]string{
		"zercher-squat": {"barbell-front-squat"}, "barbell-back-squat": {"zercher-squat"}})

	got, err := s.Get(ctx, u.ID, "zercher-squat")
	if err != nil || got.Name != "Zercher Squat" || !got.Custom() || got.Overrides {
		t.Fatalf("created exercise after the collision: %+v, %v", got, err)
	}
	if alts, _ := s.Alternatives(ctx, u.ID, "zercher-squat"); len(alts) != 0 {
		t.Fatalf("seeded alternatives attached to the user's exercise: %+v", alts)
	}
	alts, _ := s.Alternatives(ctx, u.ID, "barbell-back-squat")
	for _, a := range alts {
		if a.Exercise.Slug == "zercher-squat" {
			t.Fatalf("the user's exercise became a seeded alternative: %+v", alts)
		}
	}
	// Alternatives the user adds still count.
	if err := s.AddAlternative(ctx, u.ID, "zercher-squat", "barbell-front-squat"); err != nil {
		t.Fatal(err)
	}
	if alts, _ := s.Alternatives(ctx, u.ID, "zercher-squat"); len(alts) != 1 || !alts[0].UserAdded {
		t.Fatalf("user-added alternative: %+v", alts)
	}
}

// A seeded exercise dropped from the seed is hidden, not gone: its slug can be
// created again as the user's own exercise, which then owns that slug's history
// and settings.
func TestCreateOverHiddenSeed(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	resync(t, s, nil, nil, "barbell-back-squat")
	if hidden, _ := s.Get(ctx, u.ID, "barbell-back-squat"); !hidden.Hidden {
		t.Fatalf("not hidden: %+v", hidden)
	}

	in := validInput("barbell-back-squat")
	in.Name = "My Back Squat"
	got, err := s.Create(ctx, u.ID, in)
	if err != nil || !got.Custom() || got.Overrides || got.Name != "My Back Squat" {
		t.Fatalf("create over a hidden seed: %+v, %v", got, err)
	}
	if cat, _ := s.Catalog(ctx, u.ID, "My Back Squat"); len(cat) != 1 {
		t.Fatalf("not in the catalog: %+v", cat)
	}
	var fe FieldErrors
	if _, err := s.Create(ctx, u.ID, in); !errors.As(err, &fe) || fe["slug"] == "" {
		t.Fatalf("creating it twice: %v", err)
	}

	// Even if the seed brings the slug back, it stays the user's exercise.
	resync(t, s, nil, nil)
	if got, _ := s.Get(ctx, u.ID, "barbell-back-squat"); got.Name != "My Back Squat" || got.Overrides {
		t.Fatalf("after the seed brought it back: %+v", got)
	}
}

// Deleting a custom exercise keeps its settings: history is keyed by slug, so
// the training max, its log, the equipment link and added alternatives stay
// with the slug and come back if it is created again.
func TestDeleteCustomExerciseKeepsSettings(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	bar, _ := s.Store.DefaultEquipment(ctx, u.ID, calc.KindBarbell)
	const slug = "zercher-squat"
	if _, err := s.Create(ctx, u.ID, validInput(slug)); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSettings(ctx, u.ID, slug, SettingsInput{EquipmentID: bar.ID, SetTrainingMax: true, TrainingMaxKg: ptr(120)}, "web"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAlternative(ctx, u.ID, slug, "barbell-front-squat"); err != nil {
		t.Fatal(err)
	}

	if err := s.Delete(ctx, u.ID, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, u.ID, slug); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted exercise still found: %v", err)
	}

	ex, err := s.Create(ctx, u.ID, validInput(slug))
	if err != nil {
		t.Fatal(err)
	}
	st, err := s.Settings(ctx, u.ID, ex)
	if err != nil || !st.Linked || st.Equipment.ID != bar.ID || st.TrainingMaxKg == nil || *st.TrainingMaxKg != 120 {
		t.Fatalf("settings after re-creating: %+v, %v", st, err)
	}
	if hist, _ := s.TrainingMaxHistory(ctx, u.ID, slug); len(hist) != 1 {
		t.Fatalf("TM history after re-creating: %+v", hist)
	}
	if alts, _ := s.Alternatives(ctx, u.ID, slug); len(alts) != 1 {
		t.Fatalf("alternatives after re-creating: %+v", alts)
	}
}
