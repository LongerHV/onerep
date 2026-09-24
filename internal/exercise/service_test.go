package exercise

import (
	"context"
	"errors"
	"slices"
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
