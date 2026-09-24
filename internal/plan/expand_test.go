package plan

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

var update = flag.Bool("update", false, "rewrite golden files")

func exampleDoc(t *testing.T) Doc {
	t.Helper()
	doc, ps := Validate(readFixture(t, "upper_lower.json"), testCatalog)
	if ps.HasErrors() {
		t.Fatal(ps)
	}
	return doc
}

func TestDaysForWeek(t *testing.T) {
	doc := exampleDoc(t)
	if got := DaysForWeek(doc, 1); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("week 1 days = %v", got)
	}
	if got := DaysForWeek(doc, 4); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("week 4 days = %v", got)
	}
}

func TestExpandPicksPerWeekValues(t *testing.T) {
	doc := exampleDoc(t)
	w3, ok := Expand(doc, 3, 0, calc.UnitKg)
	if !ok {
		t.Fatal("week 3 day 0 missing")
	}
	bench := w3.Groups[0].Exercises[0]
	// 2 warm-ups + 4 working sets at RPE 9 + 1 drop set.
	if len(bench.Sets) != 7 || bench.Sets[2].TargetRPE != 9 || bench.Sets[6].Kind != "drop" {
		t.Fatalf("week 3 bench = %+v", bench.Sets)
	}
	if bench.Name != "barbell-bench-press" || bench.Notes != "pause first rep" {
		t.Fatalf("slot fields: %+v", bench)
	}

	w4, _ := Expand(doc, 4, 0, calc.UnitKg)
	bench4 := w4.Groups[0].Exercises[0]
	if len(bench4.Sets) != 4 || bench4.Sets[3].Kind != "working" { // null count drops the drop set
		t.Fatalf("week 4 bench = %+v", bench4.Sets)
	}
	if w4.Groups[1].RestS != 60 || w3.Groups[0].RestS != 180 {
		t.Fatalf("rest = %d / %d", w4.Groups[1].RestS, w3.Groups[0].RestS)
	}

	lower, _ := Expand(doc, 2, 1, calc.UnitKg)
	if lower.Groups[0].RestS != DefaultRestS || lower.Groups[1].Exercises[0].Sets[0].DurationS != 50 {
		t.Fatalf("lower day week 2 = %+v", lower)
	}
	if _, ok := Expand(doc, 1, 2, calc.UnitKg); ok {
		t.Fatal("the deload day must not exist in week 1")
	}
	if _, ok := Expand(doc, 5, 0, calc.UnitKg); ok {
		t.Fatal("week 5 must not exist")
	}
}

func TestExpandConvertsPlanUnit(t *testing.T) {
	raw := []byte(`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 1, "reps": 5, "load": {"weight": 45}}]}]}]}]}`)
	doc, ps := Validate(raw, testCatalog)
	if ps.HasErrors() {
		t.Fatal(ps)
	}
	day, _ := Expand(doc, 1, 0, calc.UnitLb) // the user's unit applies when the plan has none
	if got := day.Groups[0].Exercises[0].Sets[0].LoadValue; got != 45*calc.KgPerLb {
		t.Fatalf("45 lb = %v kg", got)
	}
}

func ptr(v float64) *float64 { return &v }

func TestResolve(t *testing.T) {
	doc := exampleDoc(t)
	bar := &calc.Equipment{Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{25, 20, 15, 10, 5, 2.5, 1.25}}}
	ctx := map[string]calc.LoadContext{
		"barbell-bench-press": {TMKg: ptr(100), E1RMKg: ptr(110), Equipment: bar, Unit: calc.UnitKg},
		"barbell-back-squat":  {TMKg: ptr(140), Equipment: bar, Unit: calc.UnitKg},
	}
	lookup := func(slug string) calc.LoadContext {
		if c, ok := ctx[slug]; ok {
			return c
		}
		return calc.LoadContext{Unit: calc.UnitKg}
	}

	day, _ := Expand(doc, 2, 0, calc.UnitKg)
	Resolve(&day, lookup)
	bench := day.Groups[0].Exercises[0].Sets
	kgs := func(sets []PrescribedSet) []float64 {
		var out []float64
		for _, s := range sets {
			if s.Kg == nil {
				out = append(out, -1)
			} else {
				out = append(out, *s.Kg)
			}
		}
		return out
	}
	// warm-up 50% of 100 = 50; 5 @ RPE 8 = 0.811 * 110 = 89.2 -> 87.5; drop 20% of 87.5 = 70.
	if got := kgs(bench); !slices.Equal(got, []float64{50, 50, 87.5, 87.5, 87.5, 70}) {
		t.Fatalf("bench kg = %v", got)
	}
	if !slices.Equal(bench[2].PerSide, []float64{25, 5, 2.5, 1.25}) {
		t.Fatalf("plates = %v", bench[2].PerSide)
	}
	// pull-ups have no load; lateral raises are an absolute 7.5 kg.
	if got := kgs(day.Groups[1].Exercises[0].Sets); !slices.Equal(got, []float64{-1, -1, -1}) {
		t.Fatalf("pull-up kg = %v", got)
	}
	if got := kgs(day.Groups[1].Exercises[1].Sets); !slices.Equal(got, []float64{7.5, 7.5, 7.5}) {
		t.Fatalf("raise kg = %v", got)
	}

	// Without an e1RM the RPE sets, and the drop set after them, are left open.
	delete(ctx, "barbell-bench-press")
	Resolve(&day, lookup)
	if got := kgs(day.Groups[0].Exercises[0].Sets); !slices.Equal(got, []float64{-1, -1, -1, -1, -1, -1}) {
		t.Fatalf("unresolved bench kg = %v", got)
	}

	// AMRAP sets with an absolute weight still resolve.
	deload, _ := Expand(doc, 4, 2, calc.UnitKg)
	Resolve(&deload, lookup)
	if got := kgs(deload.Groups[0].Exercises[0].Sets); !slices.Equal(got, []float64{10, 10}) {
		t.Fatalf("deload curls = %v", got)
	}
}

// The golden file pins the full expansion of the example plan.
func TestExpandGolden(t *testing.T) {
	doc := exampleDoc(t)
	var weeks [][]ExpandedDay
	for w := 1; w <= doc.Weeks; w++ {
		var days []ExpandedDay
		for d := range DaysForWeek(doc, w) {
			day, _ := Expand(doc, w, d, calc.UnitKg)
			days = append(days, day)
		}
		weeks = append(weeks, days)
	}
	got, err := json.MarshalIndent(weeks, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/upper_lower.golden.json"
	if *update {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), got) {
		t.Fatalf("expansion differs from %s; run go test ./internal/plan/ -update and review the diff", path)
	}
}
