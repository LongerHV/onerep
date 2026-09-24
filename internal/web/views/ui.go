package views

import (
	"strconv"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

// Shared Tailwind class sets.
const (
	btn          = "inline-block rounded bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
	btnSecondary = "inline-block rounded border border-zinc-300 px-3 py-1.5 text-sm hover:bg-zinc-100 dark:border-zinc-700 dark:hover:bg-zinc-800"
	btnDanger    = "inline-block rounded border border-red-300 px-3 py-1.5 text-sm text-red-700 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
	input        = "mt-1 block w-full rounded border border-zinc-300 bg-white px-2 py-1.5 dark:border-zinc-700 dark:bg-zinc-900"
	label        = "block text-sm font-medium"
	hint         = "mt-1 text-xs text-zinc-500"
	errorText    = "mt-1 text-sm text-red-600 dark:text-red-400"
	badge        = "rounded bg-zinc-200 px-1.5 py-0.5 text-xs dark:bg-zinc-800"
	h1           = "text-2xl font-semibold"
	h2           = "mt-8 text-lg font-semibold"
	card         = "rounded border border-zinc-200 p-4 dark:border-zinc-800"
)

// Weight formats kg in unit, like "102.5 kg" or "225 lb".
func Weight(kg float64, unit string) string {
	return exercise.FormatNumber(calc.FromKg(kg, unit)) + " " + unit
}

// Number formats a value that is already in the right unit.
func Number(v float64) string { return exercise.FormatNumber(v) }

var measurementLabels = map[string]string{
	"weight_reps":   "Weight × reps",
	"bw_reps":       "Bodyweight (+ added weight) × reps",
	"reps":          "Reps only",
	"time":          "Time",
	"distance_time": "Distance and time",
}

// MeasurementLabel is the human name of a measurement type.
func MeasurementLabel(m string) string { return measurementLabels[m] }

// Label turns an identifier like "front-delts" into "front delts".
func Label(s string) string { return strings.NewReplacer("-", " ", "_", " ").Replace(s) }

func joinLabels(vs []string) string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = Label(v)
	}
	return strings.Join(out, ", ")
}

// EquipmentSummary describes a profile's loads in one line.
func EquipmentSummary(e store.Equipment) string {
	c, u := e.Spec.Config, e.Spec.Unit
	switch e.Spec.Kind {
	case calc.KindBarbell:
		s := Number(c.Bar) + " " + u + " bar, plates " + FormatList(c.Plates) + " " + u
		if len(c.PlatePairs) > 0 {
			s += " (limited: " + exercise.FormatPlatePairs(c.PlatePairs) + ")"
		}
		return s
	case calc.KindDumbbell:
		return FormatList(c.Weights) + " " + u
	case calc.KindMachine, calc.KindCable:
		if len(c.Stack) > 0 {
			return FormatList(c.Stack) + " " + u
		}
		return Number(c.Min) + "-" + Number(c.Max) + "/" + Number(c.Step) + " " + u
	}
	return "rounds to " + map[string]string{calc.UnitKg: "0.5 kg", calc.UnitLb: "1 lb"}[u]
}

// FormatList formats weights using ranges (see exercise.FormatWeights).
func FormatList(vs []float64) string { return exercise.FormatWeights(vs) }

func plates(vs []float64) string {
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = Number(v)
	}
	return strings.Join(out, " · ")
}

func has(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

// DayOption is one entry of the "go to day" select.
type DayOption struct {
	Week, Day    int
	Value, Label string
}

// DayOptions lists every training day of a plan.
func DayOptions(doc plan.Doc) []DayOption {
	var out []DayOption
	for w := 1; w <= doc.Weeks; w++ {
		for d, i := range plan.DaysForWeek(doc, w) {
			out = append(out, DayOption{Week: w, Day: d, Value: strconv.Itoa(w) + ":" + strconv.Itoa(d),
				Label: "Week " + strconv.Itoa(w) + " · " + doc.Days[i].Name})
		}
	}
	return out
}

func optWeight(kg *float64, unit string) string {
	if kg == nil {
		return ""
	}
	return Number(calc.FromKg(*kg, unit))
}

func optFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return Number(*v)
}

func optInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
