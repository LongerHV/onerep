package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func seedExercise(slug, name string) Exercise {
	return Exercise{Slug: slug, Name: name, Measurement: "weight_reps", EquipmentKind: "barbell",
		PrimaryMuscles: []string{"quads"}, SecondaryMuscles: []string{"glutes"}, Aliases: []string{"alias"}}
}

func slugs(es []Exercise) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Slug)
	}
	return out
}

func TestSyncSeedUpsertsAndHides(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Squat"), seedExercise("bench", "Bench")},
		map[string][]string{"bench": {"squat"}})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := db.CatalogExercises(ctx, u.ID)
	if err != nil || !slices.Equal(slugs(cat), []string{"bench", "squat"}) {
		t.Fatalf("catalog = %v, %v", slugs(cat), err)
	}
	if got := cat[1]; got.Custom() || got.Overrides || !slices.Equal(got.SecondaryMuscles, []string{"glutes"}) {
		t.Fatalf("global exercise fields: %+v", got)
	}

	// Second sync: squat renamed, bench dropped from the seed.
	if err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Back Squat")}, nil); err != nil {
		t.Fatal(err)
	}
	cat, _ = db.CatalogExercises(ctx, u.ID)
	if !slices.Equal(slugs(cat), []string{"squat"}) || cat[0].Name != "Back Squat" {
		t.Fatalf("after resync: %+v", cat)
	}
	bench, err := db.ExerciseBySlug(ctx, u.ID, "bench")
	if err != nil || !bench.Hidden {
		t.Fatalf("dropped seed exercise must stay reachable but hidden: %+v, %v", bench, err)
	}
	if alts, _ := db.Alternatives(ctx, u.ID, "bench"); len(alts) != 0 {
		t.Fatalf("global alternatives not replaced: %+v", alts)
	}
}

func TestUserExerciseShadowsGlobal(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	if err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Squat")}, nil); err != nil {
		t.Fatal(err)
	}

	custom := seedExercise("squat", "My Squat")
	custom.EquipmentKind = "machine"
	if _, err := db.SaveUserExercise(ctx, alice.ID, custom); err != nil {
		t.Fatal(err)
	}
	own := seedExercise("zercher", "Zercher Squat")
	if _, err := db.SaveUserExercise(ctx, alice.ID, own); err != nil {
		t.Fatal(err)
	}

	got, err := db.ExerciseBySlug(ctx, alice.ID, "squat")
	if err != nil || got.Name != "My Squat" || !got.Custom() || !got.Overrides {
		t.Fatalf("alice squat = %+v, %v", got, err)
	}
	cat, _ := db.CatalogExercises(ctx, alice.ID)
	if !slices.Equal(slugs(cat), []string{"squat", "zercher"}) || cat[0].Name != "My Squat" {
		t.Fatalf("alice catalog = %+v", cat)
	}
	if z := cat[1]; !z.Custom() || z.Overrides {
		t.Fatalf("own exercise flags: %+v", z)
	}

	if got, _ := db.ExerciseBySlug(ctx, bob.ID, "squat"); got.Name != "Squat" {
		t.Fatalf("bob must see the global squat, got %q", got.Name)
	}
	if _, err := db.ExerciseBySlug(ctx, bob.ID, "zercher"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob must not see alice's exercise, got %v", err)
	}

	// Saving again updates in place.
	custom.Name = "My Squat v2"
	if _, err := db.SaveUserExercise(ctx, alice.ID, custom); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ExerciseBySlug(ctx, alice.ID, "squat"); got.Name != "My Squat v2" {
		t.Fatalf("update: %q", got.Name)
	}

	// Deleting the override restores the global exercise.
	if err := db.DeleteUserExercise(ctx, alice.ID, "squat"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ExerciseBySlug(ctx, alice.ID, "squat"); got.Name != "Squat" || got.Custom() {
		t.Fatalf("after reset: %+v", got)
	}
	if err := db.DeleteUserExercise(ctx, alice.ID, "squat"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a global exercise must be ErrNotFound, got %v", err)
	}
}

func TestAlternatives(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	if err := db.SyncSeed(ctx, nil, map[string][]string{"bench": {"db-bench"}}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // adding twice is harmless
		if err := db.AddAlternative(ctx, alice.ID, "bench", "pushup"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddAlternative(ctx, alice.ID, "bench", "db-bench"); err != nil {
		t.Fatal(err)
	}

	alts, err := db.Alternatives(ctx, alice.ID, "bench")
	want := []Alternative{{"db-bench", true}, {"pushup", true}}
	if err != nil || !slices.Equal(alts, want) {
		t.Fatalf("alice alternatives = %+v, %v; want %+v", alts, err, want)
	}
	if alts, _ := db.Alternatives(ctx, bob.ID, "bench"); !slices.Equal(alts, []Alternative{{"db-bench", false}}) {
		t.Fatalf("bob alternatives = %+v", alts)
	}

	if err := db.RemoveAlternative(ctx, alice.ID, "bench", "db-bench"); err != nil {
		t.Fatal(err)
	}
	if alts, _ := db.Alternatives(ctx, alice.ID, "bench"); !slices.Equal(alts, []Alternative{{"db-bench", false}, {"pushup", true}}) {
		t.Fatalf("seeded alternative must survive removing the user's copy: %+v", alts)
	}
	if err := db.RemoveAlternative(ctx, alice.ID, "bench", "db-bench"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing a seeded alternative must be ErrNotFound, got %v", err)
	}
}
