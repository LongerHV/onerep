package calc

// Load is how a set's weight is prescribed. Exactly one field is set.
type Load struct {
	Weight *float64 `json:"weight,omitempty"` // absolute, kg
	PctTM  *float64 `json:"pct_tm,omitempty"` // fraction of training max
	RPE    *float64 `json:"rpe,omitempty"`    // target RPE, weight from e1RM
}

// LoadContext is what a user knows about an exercise.
type LoadContext struct {
	TMKg      *float64   `json:"tm_kg,omitempty"`
	E1RMKg    *float64   `json:"e1rm_kg,omitempty"`
	Equipment *Equipment `json:"equipment,omitempty"`
	Unit      string     `json:"unit"` // the user's unit, for rounding without equipment
}

// ResolveLoad turns a prescription into an achievable weight. It reports false
// when the needed training max or e1RM is unknown (the user picks the weight).
// Absolute weights are returned as given.
func ResolveLoad(l Load, reps int, ctx LoadContext) (Rounded, bool) {
	switch {
	case l.Weight != nil:
		return Rounded{Kg: *l.Weight}, true
	case l.PctTM != nil:
		if ctx.TMKg == nil {
			return Rounded{}, false
		}
		return Round(*l.PctTM**ctx.TMKg, ctx.Equipment, ctx.Unit), true
	case l.RPE != nil:
		pct, ok := RTSPercent(reps, *l.RPE)
		if !ok || ctx.E1RMKg == nil {
			return Rounded{}, false
		}
		return Round(pct**ctx.E1RMKg, ctx.Equipment, ctx.Unit), true
	}
	return Rounded{}, false
}

// DropLoad is the weight for a drop set pct below the previous set's weight.
func DropLoad(prevKg, pct float64, ctx LoadContext) Rounded {
	return Round(prevKg*(1-pct), ctx.Equipment, ctx.Unit)
}
