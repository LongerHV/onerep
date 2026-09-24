package exercise

import (
	"slices"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

// The seed is data, so it gets the same validation as user input, plus
// referential checks on alternatives.
func TestSeedIsValid(t *testing.T) {
	exercises, alternatives, err := loadSeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) < 100 {
		t.Fatalf("seed has only %d exercises", len(exercises))
	}
	seen := map[string]bool{}
	for _, e := range exercises {
		if seen[e.Slug] {
			t.Errorf("duplicate slug %s", e.Slug)
		}
		seen[e.Slug] = true
		in := Input{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind,
			PrimaryMuscles: e.PrimaryMuscles, SecondaryMuscles: e.SecondaryMuscles, Aliases: e.Aliases}
		if errs := in.validate(); len(errs) > 0 {
			t.Errorf("%s: %v", e.Slug, errs)
		}
		if !slices.Contains(calc.Kinds, e.EquipmentKind) {
			t.Errorf("%s: equipment %q", e.Slug, e.EquipmentKind)
		}
	}
	for slug, alts := range alternatives {
		for _, alt := range alts {
			if !seen[alt] || alt == slug {
				t.Errorf("%s: alternative %q is not another seeded exercise", slug, alt)
			}
		}
	}
}
