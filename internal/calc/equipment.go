package calc

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
)

// Equipment kinds.
const (
	KindBarbell    = "barbell"
	KindDumbbell   = "dumbbell"
	KindMachine    = "machine"
	KindCable      = "cable"
	KindBodyweight = "bodyweight"
)

// Kinds lists every equipment kind.
var Kinds = []string{KindBarbell, KindDumbbell, KindMachine, KindCable, KindBodyweight}

// Equipment describes what loads are achievable. Values in Config are in Unit.
type Equipment struct {
	Kind   string          `json:"kind"`
	Unit   string          `json:"unit"`
	Config EquipmentConfig `json:"config"`
}

// EquipmentConfig holds the kind-specific settings:
//   - barbell: Bar and Plates (sizes available), optional PlatePairs limiting
//     how many pairs of a size exist ("1.25": 2); sizes not listed are unlimited.
//   - dumbbell: Weights.
//   - machine, cable: Stack, or Min/Step/Max.
//   - bodyweight: nothing (loads round like unlinked exercises).
type EquipmentConfig struct {
	Bar        float64        `json:"bar,omitempty"`
	Plates     []float64      `json:"plates,omitempty"`
	PlatePairs map[string]int `json:"plate_pairs,omitempty"`
	Weights    []float64      `json:"weights,omitempty"`
	Stack      []float64      `json:"stack,omitempty"`
	Min        float64        `json:"min,omitempty"`
	Step       float64        `json:"step,omitempty"`
	Max        float64        `json:"max,omitempty"`
}

// Validate reports the first problem that would make rounding meaningless.
func (e Equipment) Validate() error {
	if e.Unit != UnitKg && e.Unit != UnitLb {
		return fmt.Errorf("unit must be %q or %q", UnitKg, UnitLb)
	}
	c := e.Config
	positive := func(field string, vs []float64) error {
		if len(vs) == 0 {
			return fmt.Errorf("%s: at least one value is required", field)
		}
		for _, v := range vs {
			if v <= 0 {
				return fmt.Errorf("%s: values must be positive", field)
			}
		}
		return nil
	}
	switch e.Kind {
	case KindBarbell:
		if c.Bar < 0 {
			return errors.New("bar: must not be negative")
		}
		if err := positive("plates", c.Plates); err != nil {
			return err
		}
		for size, n := range c.PlatePairs {
			v, err := strconv.ParseFloat(size, 64)
			if err != nil || !containsWeight(c.Plates, v) {
				return fmt.Errorf("plate_pairs: %q is not one of the plates", size)
			}
			if n < 0 {
				return fmt.Errorf("plate_pairs: count for %s must not be negative", size)
			}
		}
	case KindDumbbell:
		return positive("weights", c.Weights)
	case KindMachine, KindCable:
		if len(c.Stack) > 0 {
			return positive("stack", c.Stack)
		}
		if c.Step <= 0 || c.Min < 0 || c.Max < c.Min {
			return errors.New("stack: give a list of weights, or min, step > 0 and max >= min")
		}
	case KindBodyweight:
	default:
		return fmt.Errorf("unknown equipment kind %q", e.Kind)
	}
	return nil
}

func containsWeight(vs []float64, v float64) bool {
	for _, x := range vs {
		if cents(x) == cents(v) {
			return true
		}
	}
	return false
}

// Rounded is an achievable load. PerSide lists the plates for one side of a
// barbell, heaviest first, in the equipment's unit.
type Rounded struct {
	Kg      float64   `json:"kg"`
	PerSide []float64 `json:"per_side,omitempty"`
}

// Round returns the heaviest achievable load not above targetKg, or the
// lightest achievable load when nothing is that light. Without equipment (or
// for bodyweight) loads round down to 0.5 kg or 1 lb steps in fallbackUnit.
func Round(targetKg float64, eq *Equipment, fallbackUnit string) Rounded {
	if math.IsNaN(targetKg) || targetKg < 0 {
		targetKg = 0
	}
	if eq == nil || eq.Kind == KindBodyweight {
		unit := fallbackUnit
		if eq != nil {
			unit = eq.Unit
		}
		step := int64(50) // 0.5 kg in cents
		if unit == UnitLb {
			step = 100
		}
		t := cents(FromKg(targetKg, unit))
		if t < 0 {
			t = 0
		}
		return Rounded{Kg: ToKg(float64(t/step*step)/100, unit)}
	}
	t := cents(FromKg(targetKg, eq.Unit))
	c := eq.Config
	switch eq.Kind {
	case KindBarbell:
		return roundBarbell(t, eq.Unit, c)
	case KindDumbbell:
		return Rounded{Kg: ToKg(float64(pickFromList(t, c.Weights))/100, eq.Unit)}
	default: // machine, cable
		if len(c.Stack) > 0 {
			return Rounded{Kg: ToKg(float64(pickFromList(t, c.Stack))/100, eq.Unit)}
		}
		lo, step, hi := cents(c.Min), cents(c.Step), cents(c.Max)
		v := lo
		if t > lo {
			v = lo + (t-lo)/step*step
		}
		if v > hi {
			v = hi
		}
		return Rounded{Kg: ToKg(float64(v)/100, eq.Unit)}
	}
}

// pickFromList returns the largest value <= t, else the smallest value.
func pickFromList(t int64, values []float64) int64 {
	best, smallest := int64(-1), int64(math.MaxInt64)
	for _, v := range values {
		c := cents(v)
		if c <= t && c > best {
			best = c
		}
		if c < smallest {
			smallest = c
		}
	}
	if best >= 0 {
		return best
	}
	return smallest
}

// maxSideCents caps the plate search at 1000 units per side.
const maxSideCents = 100000

func roundBarbell(t int64, unit string, c EquipmentConfig) Rounded {
	bar := cents(c.Bar)
	if t <= bar {
		return Rounded{Kg: ToKg(float64(bar)/100, unit)}
	}
	side := (t - bar) / 2
	if side > maxSideCents {
		side = maxSideCents
	}

	type plate struct {
		size int64
		max  int64 // -1 = unlimited
	}
	var plates []plate
	for _, p := range c.Plates {
		pl := plate{size: cents(p), max: -1}
		for k, n := range c.PlatePairs {
			if v, err := strconv.ParseFloat(k, 64); err == nil && cents(v) == pl.size {
				pl.max = int64(n)
			}
		}
		if pl.size > 0 {
			plates = append(plates, pl)
		}
	}
	sort.Slice(plates, func(i, j int) bool { return plates[i].size > plates[j].size })

	// reach[i][s]: a per-side sum of s is achievable with plates[i:].
	n := len(plates)
	reach := make([][]bool, n+1)
	for i := range reach {
		reach[i] = make([]bool, side+1)
	}
	reach[n][0] = true
	for i := n - 1; i >= 0; i-- {
		p := plates[i]
		for s := int64(0); s <= side; s++ {
			if reach[i+1][s] {
				reach[i][s] = true
				continue
			}
			if p.max < 0 {
				reach[i][s] = s >= p.size && reach[i][s-p.size]
				continue
			}
			for k := int64(1); k <= p.max && k*p.size <= s; k++ {
				if reach[i+1][s-k*p.size] {
					reach[i][s] = true
					break
				}
			}
		}
	}

	best := side
	for !reach[0][best] {
		best--
	}
	// Heaviest plates first: take as many of each size as still leaves a
	// reachable remainder.
	var perSide []float64
	rest := best
	for i, p := range plates {
		k := rest / p.size
		if p.max >= 0 && k > p.max {
			k = p.max
		}
		for ; k > 0 && !reach[i+1][rest-k*p.size]; k-- {
		}
		for j := int64(0); j < k; j++ {
			perSide = append(perSide, float64(p.size)/100)
		}
		rest -= k * p.size
	}
	return Rounded{Kg: ToKg(float64(bar+2*best)/100, unit), PerSide: perSide}
}
