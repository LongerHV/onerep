// Package calc holds onerep's training math: RPE percentages, estimated 1RM,
// load resolution, and rounding to real equipment. It is pure (no I/O) and is
// mirrored by internal/web/static/js/calc.js; testdata/calc_cases.json keeps
// the two implementations in agreement.
package calc

import "math"

// KgPerLb is the exact international pound.
const KgPerLb = 0.45359237

const (
	UnitKg = "kg"
	UnitLb = "lb"
)

// ToKg converts v in unit to kilograms.
func ToKg(v float64, unit string) float64 {
	if unit == UnitLb {
		return v * KgPerLb
	}
	return v
}

// FromKg converts kilograms to unit.
func FromKg(kg float64, unit string) float64 {
	if unit == UnitLb {
		return kg / KgPerLb
	}
	return kg
}

// cents converts a weight to integer hundredths, tolerating float noise
// (99.99999999 counts as 100.00).
func cents(v float64) int64 { return int64(math.Floor(v*100 + 1e-6)) }
