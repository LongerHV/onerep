package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

func newUser(t *testing.T, db *DB, sub string) User {
	t.Helper()
	u, err := db.UpsertOIDCUser(context.Background(), "iss", sub, "", sub)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func barbell(userID, name string, isDefault bool) Equipment {
	return Equipment{UserID: userID, Name: name, IsDefault: isDefault, Spec: calc.Equipment{
		Kind: calc.KindBarbell, Unit: calc.UnitKg,
		Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{25, 1.25}, PlatePairs: map[string]int{"1.25": 2}},
	}}
}

func TestEquipmentCRUD(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	saved, err := db.SaveEquipment(ctx, barbell(alice.ID, "Home bar", true))
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.EquipmentByID(ctx, alice.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Home bar" || !got.IsDefault || got.Spec.Config.Bar != 20 || got.Spec.Config.PlatePairs["1.25"] != 2 {
		t.Fatalf("round trip: %+v", got)
	}

	if _, err := db.EquipmentByID(ctx, bob.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's equipment must be ErrNotFound, got %v", err)
	}
	other := barbell(bob.ID, "stolen", false)
	other.ID = saved.ID
	if _, err := db.SaveEquipment(ctx, other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("updating other user's equipment must be ErrNotFound, got %v", err)
	}
	if err := db.DeleteEquipment(ctx, bob.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting other user's equipment must be ErrNotFound, got %v", err)
	}

	got.Name = "Garage bar"
	if _, err := db.SaveEquipment(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListEquipment(ctx, alice.ID)
	if err != nil || len(list) != 1 || list[0].Name != "Garage bar" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteEquipment(ctx, alice.ID, saved.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.ListEquipment(ctx, alice.ID); len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}

func TestOneDefaultPerKind(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	first, err := db.SaveEquipment(ctx, barbell(u.ID, "first", true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.SaveEquipment(ctx, barbell(u.ID, "second", true))
	if err != nil {
		t.Fatal(err)
	}
	def, err := db.DefaultEquipment(ctx, u.ID, calc.KindBarbell)
	if err != nil || def.ID != second.ID {
		t.Fatalf("default = %+v, %v; want second", def, err)
	}
	if f, _ := db.EquipmentByID(ctx, u.ID, first.ID); f.IsDefault {
		t.Fatal("first profile is still marked default")
	}
	if _, err := db.DefaultEquipment(ctx, u.ID, calc.KindDumbbell); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no dumbbell default: want ErrNotFound, got %v", err)
	}
}

func TestInitStarterEquipmentOnlyOnce(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	items := []Equipment{barbell("", "Barbell", true)}

	var wg sync.WaitGroup
	results := make(chan bool, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := db.InitStarterEquipment(ctx, u.ID, items)
			if err != nil {
				t.Error(err)
			}
			results <- created
		}()
	}
	wg.Wait()
	close(results)
	n := 0
	for created := range results {
		if created {
			n++
		}
	}
	list, _ := db.ListEquipment(ctx, u.ID)
	if n != 1 || len(list) != 1 {
		t.Fatalf("created %d times, %d profiles; want exactly once", n, len(list))
	}
	if got, _ := db.UserByID(ctx, u.ID); !got.EquipmentInitialized {
		t.Fatal("user not marked initialized")
	}
}
