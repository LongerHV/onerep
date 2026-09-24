package calc

import "math"

// rtsSequence is the Tuchscherer RPE chart flattened: the percentage of 1RM
// for (reps, rpe) is rtsSequence[2*(reps-1) + 2*(10-rpe)]. One extra rep costs
// the same as one full RPE point; half an RPE point is one step.
var rtsSequence = [...]float64{
	1.000, 0.978, 0.955, 0.939, 0.922, 0.907, 0.892, 0.878, 0.863, 0.850,
	0.837, 0.824, 0.811, 0.799, 0.786, 0.774, 0.762, 0.751, 0.739, 0.723,
	0.707, 0.694, 0.680, 0.667, 0.653, 0.640, 0.626, 0.613, 0.599, 0.586,
	0.572,
}

// RTSPercent returns the fraction of 1RM that reps at rpe corresponds to.
// Valid for reps 1–12 and RPE 6–10 in 0.5 steps.
func RTSPercent(reps int, rpe float64) (float64, bool) {
	if reps < 1 || reps > 12 || rpe < 6 || rpe > 10 {
		return 0, false
	}
	halfSteps := (10 - rpe) * 2
	if halfSteps != math.Trunc(halfSteps) {
		return 0, false
	}
	return rtsSequence[2*(reps-1)+int(halfSteps)], true
}
