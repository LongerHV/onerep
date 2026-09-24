package plan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
)

// NextPosition is the cursor position after finishing or skipping (week, day):
// the next day of the week, else the first day of the next week. Past the last
// week the plan is complete (week = doc.Weeks+1, day 0).
func NextPosition(doc Doc, week, day int) (int, int) {
	if day+1 < len(DaysForWeek(doc, week)) {
		return week, day + 1
	}
	return week + 1, 0
}

// ValidPosition reports whether (week, day) is a training day of doc.
func ValidPosition(doc Doc, week, day int) bool {
	return week >= 1 && week <= doc.Weeks && day >= 0 && day < len(DaysForWeek(doc, week))
}

// SetGroup is a run of identical consecutive sets.
type SetGroup struct {
	Count int
	Set   PrescribedSet
}

// GroupSets collapses identical consecutive sets (resolved weight included).
func GroupSets(sets []PrescribedSet) []SetGroup {
	var out []SetGroup
	for _, s := range sets {
		if n := len(out); n > 0 && sameSet(out[n-1].Set, s) {
			out[n-1].Count++
			continue
		}
		out = append(out, SetGroup{Count: 1, Set: s})
	}
	return out
}

func sameSet(a, b PrescribedSet) bool {
	return a.Kind == b.Kind && a.Reps == b.Reps && a.DurationS == b.DurationS && a.TargetRPE == b.TargetRPE &&
		a.LoadKind == b.LoadKind && a.LoadValue == b.LoadValue && ((a.Kg == nil) == (b.Kg == nil)) &&
		(a.Kg == nil || *a.Kg == *b.Kg)
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Prescription describes a set group without its resolved weight, like
// "3 × 5 @ RPE 8" or "2 × 5 warmup @ 50% TM". Weights are shown in unit.
func Prescription(g SetGroup, unit string) string {
	s := g.Set
	var b strings.Builder
	switch {
	case s.DurationS > 0:
		fmt.Fprintf(&b, "%d × %ds", g.Count, s.DurationS)
	default:
		fmt.Fprintf(&b, "%d × %s", g.Count, s.Reps)
	}
	if s.Kind != "working" {
		b.WriteString(" " + s.Kind)
	}
	switch s.LoadKind {
	case LoadWeight:
		fmt.Fprintf(&b, " @ %s %s", num(round2(calc.FromKg(s.LoadValue, unit))), unit)
	case LoadPctTM:
		fmt.Fprintf(&b, " @ %s%% TM", num(round2(s.LoadValue*100)))
	case LoadRPE:
		fmt.Fprintf(&b, " @ RPE %s", num(s.LoadValue))
	case LoadDropPct:
		fmt.Fprintf(&b, " -%s%%", num(round2(s.LoadValue*100)))
	}
	if s.TargetRPE > 0 && s.LoadKind != LoadRPE {
		fmt.Fprintf(&b, " (RPE %s)", num(s.TargetRPE))
	}
	return b.String()
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// DayLines describes a day's prescription as text lines, one per exercise
// and set group, for comparing versions.
func DayLines(day ExpandedDay, unit string) []string {
	var out []string
	for gi, g := range day.Groups {
		out = append(out, fmt.Sprintf("Group %d, rest %ds", gi+1, g.RestS))
		for _, slot := range g.Exercises {
			out = append(out, "  "+slot.Name)
			for _, sg := range GroupSets(stripKg(slot.Sets)) {
				out = append(out, "    "+Prescription(sg, unit))
			}
		}
	}
	return out
}

func stripKg(sets []PrescribedSet) []PrescribedSet {
	out := make([]PrescribedSet, len(sets))
	for i, s := range sets {
		s.Kg, s.PerSide = nil, nil
		out[i] = s
	}
	return out
}
