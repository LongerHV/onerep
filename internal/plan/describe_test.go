package plan

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

func TestNextPosition(t *testing.T) {
	doc := exampleDoc(t) // weeks 1-3: 2 days, week 4: 3 days
	steps := [][2]int{{1, 0}, {1, 1}, {2, 0}, {2, 1}, {3, 0}, {3, 1}, {4, 0}, {4, 1}, {4, 2}, {5, 0}}
	w, d := 1, 0
	for i, want := range steps[1:] {
		w, d = NextPosition(doc, w, d)
		if [2]int{w, d} != want {
			t.Fatalf("step %d: got (%d, %d), want %v", i+1, w, d, want)
		}
	}
	if ValidPosition(doc, 5, 0) || ValidPosition(doc, 1, 2) || !ValidPosition(doc, 4, 2) {
		t.Fatal("ValidPosition wrong")
	}
}

func TestPrescriptionText(t *testing.T) {
	doc := exampleDoc(t)
	day, _ := Expand(doc, 1, 0, calc.UnitKg)
	day.Groups[0].Exercises[0].Name = "Bench"
	got := DayLines(day, calc.UnitLb)
	want := []string{
		"Group 1, rest 180s",
		"  Bench",
		"    2 × 5 warmup @ 50% TM",
		"    3 × 5 @ RPE 7",
		"    1 × 10 drop -20%",
		"Group 2, rest 90s",
		"  pull-up",
		"    3 × 6-10 (RPE 8)",
		"  cable-lateral-raise",
		"    3 × 15 @ 16.53 lb",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
	plank, _ := Expand(doc, 1, 1, calc.UnitKg)
	if lines := DayLines(plank, calc.UnitKg); lines[len(lines)-1] != "    3 × 45s" {
		t.Fatalf("timed sets: %q", lines[len(lines)-1])
	}
}

func TestGroupSetsKeepsDifferentWeightsApart(t *testing.T) {
	a, b := 100.0, 102.5
	sets := []PrescribedSet{{Kind: "working", Kg: &a}, {Kind: "working", Kg: &a}, {Kind: "working", Kg: &b}, {Kind: "working"}}
	if got := GroupSets(sets); len(got) != 3 || got[0].Count != 2 {
		t.Fatalf("groups = %+v", got)
	}
}

func TestDiffLines(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "d", "e"}
	got := DiffLines(a, b)
	want := []DiffLine{{' ', "a"}, {'-', "b"}, {' ', "c"}, {' ', "d"}, {'+', "e"}}
	if !slices.Equal(got, want) || !Changed(got) || Changed(DiffLines(a, a)) {
		t.Fatalf("diff = %q", got)
	}
	big := make([]string, 3000)
	for i := range big {
		big[i] = strings.Repeat("x", i%7)
	}
	// Differ at both ends, so no common prefix or suffix narrows the table.
	other := append([]string{"start"}, big[1:]...)
	other[len(other)-1] = "end"
	if got := DiffLines(big, other); len(got) != len(big)+len(other) {
		t.Fatalf("oversized inputs should be shown as replaced, got %d lines", len(got))
	}
}

// Long documents with a local edit (a 12-week plan is thousands of lines)
// must still diff to just the edit.
func TestDiffLinesLongDocumentWithOneEdit(t *testing.T) {
	a := make([]string, 3000)
	for i := range a {
		a[i] = strconv.Itoa(i)
	}
	b := slices.Clone(a)
	b[1500] = "changed"
	var changed []DiffLine
	for _, l := range DiffLines(a, b) {
		if l.Op != ' ' {
			changed = append(changed, l)
		}
	}
	if !slices.Equal(changed, []DiffLine{{'-', "1500"}, {'+', "changed"}}) {
		t.Fatalf("changed lines = %d: %.80q", len(changed), changed)
	}
}
