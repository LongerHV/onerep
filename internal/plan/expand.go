package plan

import (
	"slices"

	"github.com/LongerHV/onerep/internal/calc"
)

// DefaultRestS is the rest after a group's round when rest_s is not given.
const DefaultRestS = 120

// ExpandedDay is one concrete training day: per-week values picked, set
// counts expanded, weights in kg. It is what a session snapshots (spec §6).
type ExpandedDay struct {
	Week   int             `json:"week"`
	Day    int             `json:"day"` // index among the days of this week
	Name   string          `json:"name"`
	Groups []ExpandedGroup `json:"groups"`
}

type ExpandedGroup struct {
	RestS     int            `json:"rest_s"`
	Exercises []ExpandedSlot `json:"exercises"`
}

type ExpandedSlot struct {
	Slug         string          `json:"slug"`
	Name         string          `json:"name"` // filled in by the caller when known
	Alternatives []string        `json:"alternatives,omitempty"`
	Notes        string          `json:"notes,omitempty"`
	Sets         []PrescribedSet `json:"sets"`
}

// Load kinds of a prescribed set.
const (
	LoadNone    = ""
	LoadWeight  = "weight"
	LoadPctTM   = "pct_tm"
	LoadRPE     = "rpe"
	LoadDropPct = "drop_pct"
)

// PrescribedSet is one set as planned, plus its resolved weight.
type PrescribedSet struct {
	Kind      string    `json:"kind"`
	Reps      Reps      `json:"reps,omitzero"` // zero for timed sets
	DurationS int       `json:"duration_s,omitempty"`
	TargetRPE float64   `json:"target_rpe,omitempty"` // 0 = none
	LoadKind  string    `json:"load_kind,omitempty"`
	LoadValue float64   `json:"load_value,omitempty"` // kg for weight, fraction for pct_tm/drop_pct, RPE for rpe
	Kg        *float64  `json:"kg,omitempty"`         // resolved weight; nil = the lifter picks
	PerSide   []float64 `json:"per_side,omitempty"`   // barbell plates per side, in the equipment's unit
}

// DaysForWeek returns the indexes into doc.Days that apply to week.
func DaysForWeek(doc Doc, week int) []int {
	var out []int
	for i, d := range doc.Days {
		if len(d.OnlyWeeks) == 0 || slices.Contains(d.OnlyWeeks, week) {
			out = append(out, i)
		}
	}
	return out
}

// Expand returns day index day (among the days of week) of week. ok is false
// when that day does not exist. Absolute weights are converted from the
// plan's unit (defaultUnit when the plan has none) to kg.
func Expand(doc Doc, week, day int, defaultUnit string) (ExpandedDay, bool) {
	days := DaysForWeek(doc, week)
	if week < 1 || week > doc.Weeks || day < 0 || day >= len(days) {
		return ExpandedDay{}, false
	}
	unit := doc.Unit
	if unit == "" {
		unit = defaultUnit
	}
	src := doc.Days[days[day]]
	out := ExpandedDay{Week: week, Day: day, Name: src.Name}
	for _, g := range src.Groups {
		rest := DefaultRestS
		if g.RestS.Set() {
			rest = g.RestS.At(week)
		}
		eg := ExpandedGroup{RestS: rest}
		for _, slot := range g.Exercises {
			es := ExpandedSlot{Slug: slot.Slug, Name: slot.Slug, Alternatives: slot.Alternatives, Notes: slot.Notes}
			for _, line := range slot.Sets {
				count := line.Count.At(week)
				if count == nil {
					continue
				}
				ps := PrescribedSet{Kind: line.Kind, Reps: line.Reps.At(week), DurationS: line.DurationS.At(week), TargetRPE: line.RPE.At(week)}
				if ps.Kind == "" {
					ps.Kind = "working"
				}
				if l := line.Load; l != nil {
					switch {
					case l.Weight.Set():
						ps.LoadKind, ps.LoadValue = LoadWeight, calc.ToKg(l.Weight.At(week), unit)
					case l.PctTM.Set():
						ps.LoadKind, ps.LoadValue = LoadPctTM, l.PctTM.At(week)
					case l.RPE.Set():
						ps.LoadKind, ps.LoadValue = LoadRPE, l.RPE.At(week)
						ps.TargetRPE = ps.LoadValue
					case l.DropPct.Set():
						ps.LoadKind, ps.LoadValue = LoadDropPct, l.DropPct.At(week)
					}
				}
				for range *count {
					es.Sets = append(es.Sets, ps)
				}
			}
			if len(es.Sets) > 0 {
				eg.Exercises = append(eg.Exercises, es)
			}
		}
		if len(eg.Exercises) > 0 {
			out.Groups = append(out.Groups, eg)
		}
	}
	return out, true
}

// Resolve fills in Kg and PerSide for every set it can (spec §6):
// weight as given, pct_tm from the training max, rpe from the e1RM (lower
// bound of a rep range), drop_pct from the previous set's resolved weight.
// ctxFor supplies what the lifter knows about each exercise.
func Resolve(day *ExpandedDay, ctxFor func(slug string) calc.LoadContext) {
	for gi := range day.Groups {
		for si := range day.Groups[gi].Exercises {
			slot := &day.Groups[gi].Exercises[si]
			ctx := ctxFor(slot.Slug)
			var prev *float64
			for i := range slot.Sets {
				s := &slot.Sets[i]
				s.Kg, s.PerSide = nil, nil
				var r calc.Rounded
				ok := false
				switch s.LoadKind {
				case LoadWeight:
					r, ok = calc.ResolveLoad(calc.Load{Weight: &s.LoadValue}, s.Reps.Min, ctx)
				case LoadPctTM:
					r, ok = calc.ResolveLoad(calc.Load{PctTM: &s.LoadValue}, s.Reps.Min, ctx)
				case LoadRPE:
					if !s.Reps.AMRAP {
						r, ok = calc.ResolveLoad(calc.Load{RPE: &s.LoadValue}, s.Reps.Min, ctx)
					}
				case LoadDropPct:
					if prev != nil {
						r, ok = calc.DropLoad(*prev, s.LoadValue, ctx), true
					}
				}
				if ok {
					kg := r.Kg
					s.Kg, s.PerSide = &kg, r.PerSide
				}
				prev = s.Kg
			}
		}
	}
}
