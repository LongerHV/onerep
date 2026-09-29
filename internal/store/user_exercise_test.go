package store

import (
	"context"
	"errors"
	"testing"
)

func kg(v float64) *float64 { return &v }

func TestTrainingMaxHistory(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	if ue, err := db.UserExercise(ctx, u.ID, "squat"); err != nil || ue.TrainingMaxKg != nil || ue.EquipmentID != "" {
		t.Fatalf("empty settings = %+v, %v", ue, err)
	}
	for _, step := range []struct {
		kg     *float64
		source string
	}{{kg(140), "web"}, {kg(140), "web"}, {kg(145), "mcp"}, {nil, "web"}} {
		if err := db.SetTrainingMax(ctx, u.ID, "squat", step.kg, step.source, "note"); err != nil {
			t.Fatal(err)
		}
	}
	if ue, _ := db.UserExercise(ctx, u.ID, "squat"); ue.TrainingMaxKg != nil {
		t.Fatalf("cleared TM still set: %v", *ue.TrainingMaxKg)
	}

	hist, err := db.TrainingMaxHistory(ctx, u.ID, "squat")
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; setting 140 twice records one change.
	if len(hist) != 3 {
		t.Fatalf("history has %d entries, want 3: %+v", len(hist), hist)
	}
	if h := hist[0]; h.NewKg != nil || *h.OldKg != 145 || h.Source != "web" {
		t.Fatalf("clear entry: %+v", h)
	}
	if h := hist[1]; *h.OldKg != 140 || *h.NewKg != 145 || h.Source != "mcp" {
		t.Fatalf("mcp entry: %+v", h)
	}
	if h := hist[2]; h.OldKg != nil || *h.NewKg != 140 {
		t.Fatalf("first entry: %+v", h)
	}
}

func TestSetExerciseEquipment(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	bar, err := db.SaveEquipment(ctx, barbell(alice.ID, "bar", false))
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetTrainingMax(ctx, alice.ID, "squat", kg(100), "web", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetExerciseEquipment(ctx, alice.ID, "squat", bar.ID); err != nil {
		t.Fatal(err)
	}
	ue, _ := db.UserExercise(ctx, alice.ID, "squat")
	if ue.EquipmentID != bar.ID || *ue.TrainingMaxKg != 100 {
		t.Fatalf("settings = %+v (linking equipment must keep the TM)", ue)
	}

	if err := db.SetExerciseEquipment(ctx, bob.ID, "squat", bar.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("linking another user's equipment must be ErrNotFound, got %v", err)
	}

	// Deleting the profile unlinks it.
	if err := db.DeleteEquipment(ctx, alice.ID, bar.ID); err != nil {
		t.Fatal(err)
	}
	if ue, _ := db.UserExercise(ctx, alice.ID, "squat"); ue.EquipmentID != "" {
		t.Fatalf("deleted equipment still linked: %+v", ue)
	}
}

func TestSaveExerciseSettings(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	bar, err := db.SaveEquipment(ctx, barbell(alice.ID, "bar", false))
	if err != nil {
		t.Fatal(err)
	}

	err = db.SaveExerciseSettings(ctx, alice.ID, "squat", ExerciseSettings{
		EquipmentID: bar.ID, SetTrainingMax: true, TrainingMaxKg: kg(100), Source: "web"})
	if err != nil {
		t.Fatal(err)
	}
	ue, _ := db.UserExercise(ctx, alice.ID, "squat")
	if ue.EquipmentID != bar.ID || ue.TrainingMaxKg == nil || *ue.TrainingMaxKg != 100 {
		t.Fatalf("settings = %+v", ue)
	}
	if hist, _ := db.TrainingMaxHistory(ctx, alice.ID, "squat"); len(hist) != 1 {
		t.Fatalf("history = %+v", hist)
	}

	// Without SetTrainingMax the TM is left alone.
	if err := db.SaveExerciseSettings(ctx, alice.ID, "squat", ExerciseSettings{}); err != nil {
		t.Fatal(err)
	}
	if ue, _ := db.UserExercise(ctx, alice.ID, "squat"); ue.EquipmentID != "" || *ue.TrainingMaxKg != 100 {
		t.Fatalf("unlink only = %+v", ue)
	}

	// Another user's profile fails the whole save.
	err = db.SaveExerciseSettings(ctx, bob.ID, "squat", ExerciseSettings{
		EquipmentID: bar.ID, SetTrainingMax: true, TrainingMaxKg: kg(50), Source: "web"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's equipment must be ErrNotFound, got %v", err)
	}
	if ue, _ := db.UserExercise(ctx, bob.ID, "squat"); ue.TrainingMaxKg != nil {
		t.Fatalf("TM saved although the save failed: %+v", ue)
	}
}
