package calc

// Method says how an e1RM was estimated.
type Method string

const (
	MethodRPE  Method = "rpe"  // weight / RTS percentage
	MethodReps Method = "reps" // Epley, used when no valid RPE was recorded
)

// E1RM estimates the one-rep max from a set. rpe 0 means "not recorded".
// Sets above 12 reps or without weight give no estimate.
func E1RM(weightKg float64, reps int, rpe float64) (float64, Method, bool) {
	if weightKg <= 0 || reps < 1 || reps > 12 {
		return 0, "", false
	}
	if pct, ok := RTSPercent(reps, rpe); ok {
		return weightKg / pct, MethodRPE, true
	}
	if reps == 1 {
		return weightKg, MethodReps, true
	}
	return weightKg * (1 + float64(reps)/30), MethodReps, true
}
