// Package plan handles training plan documents: validation, per-week
// expansion, load resolution, versions, and the active plan cursor.
package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Doc is a plan as authored (spec §6). Fields marked per week may hold a
// single value or one value per week.
type Doc struct {
	Name  string `json:"name"`
	Unit  string `json:"unit,omitempty"`
	Weeks int    `json:"weeks"`
	Days  []Day  `json:"days"`
}

type Day struct {
	Name      string  `json:"name"`
	OnlyWeeks []int   `json:"only_weeks,omitempty"`
	Groups    []Group `json:"groups"`
}

type Group struct {
	RestS     PerWeek[int] `json:"rest_s"`
	Exercises []Slot       `json:"exercises"`
}

type Slot struct {
	Slug         string    `json:"slug"`
	Alternatives []string  `json:"alternatives,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	Sets         []SetLine `json:"sets"`
}

type SetLine struct {
	Kind      string            `json:"kind,omitempty"`
	Count     PerWeek[*int]     `json:"count"`
	Reps      PerWeek[Reps]     `json:"reps"`
	DurationS PerWeek[int]      `json:"duration_s"`
	RPE       PerWeek[float64]  `json:"rpe"`
	Load      *LoadPrescription `json:"load,omitempty"`
}

// LoadPrescription holds exactly one of its fields.
type LoadPrescription struct {
	Weight  PerWeek[float64] `json:"weight"`
	PctTM   PerWeek[float64] `json:"pct_tm"`
	RPE     PerWeek[float64] `json:"rpe"`
	DropPct PerWeek[float64] `json:"drop_pct"`
}

// PerWeek is a value that is either the same every week or listed per week.
type PerWeek[T any] struct {
	Values []T  // one value, or one per week
	List   bool // true when written as an array
}

// Set reports whether the field was present.
func (p PerWeek[T]) Set() bool { return len(p.Values) > 0 }

// At returns the value for week (1-based); zero if unset or out of range.
func (p PerWeek[T]) At(week int) T {
	var zero T
	switch {
	case !p.Set():
		return zero
	case !p.List:
		return p.Values[0]
	case week >= 1 && week <= len(p.Values):
		return p.Values[week-1]
	}
	return zero
}

func (p *PerWeek[T]) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*p = PerWeek[T]{}
		return nil
	}
	if len(b) > 0 && b[0] == '[' {
		p.List = true
		return json.Unmarshal(b, &p.Values)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	p.Values, p.List = []T{v}, false
	return nil
}

func (p PerWeek[T]) MarshalJSON() ([]byte, error) {
	switch {
	case !p.Set():
		return []byte("null"), nil
	case p.List:
		return json.Marshal(p.Values)
	}
	return json.Marshal(p.Values[0])
}

// Reps is a rep target: a number, a range "lo-hi", or AMRAP.
type Reps struct {
	Min, Max int
	AMRAP    bool
}

func (r *Reps) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*r = Reps{Min: n, Max: n}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("reps: want a number or a string")
	}
	if s == "AMRAP" {
		*r = Reps{AMRAP: true}
		return nil
	}
	lo, hi, ok := strings.Cut(s, "-")
	a, err1 := strconv.Atoi(lo)
	c, err2 := strconv.Atoi(hi)
	if !ok || err1 != nil || err2 != nil {
		return fmt.Errorf("reps: %q is not a number, range or AMRAP", s)
	}
	*r = Reps{Min: a, Max: c}
	return nil
}

func (r Reps) MarshalJSON() ([]byte, error) {
	switch {
	case r.AMRAP:
		return json.Marshal("AMRAP")
	case r.Min == r.Max:
		return json.Marshal(r.Min)
	}
	return json.Marshal(fmt.Sprintf("%d-%d", r.Min, r.Max))
}

// String formats reps for display: "5", "6-10" or "AMRAP".
func (r Reps) String() string {
	switch {
	case r.AMRAP:
		return "AMRAP"
	case r.Min == r.Max:
		return strconv.Itoa(r.Min)
	}
	return fmt.Sprintf("%d-%d", r.Min, r.Max)
}
