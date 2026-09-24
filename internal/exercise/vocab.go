// Package exercise manages the exercise catalog, equipment profiles, and each
// user's per-exercise settings (equipment link, training max).
package exercise

import "slices"

// Muscles is the fixed muscle vocabulary (spec §14).
var Muscles = []string{
	"chest", "front-delts", "side-delts", "rear-delts", "lats", "upper-back", "traps",
	"biceps", "triceps", "forearms", "abs", "obliques", "lower-back", "glutes", "quads",
	"hamstrings", "adductors", "abductors", "calves", "neck",
}

// Measurements are the ways a set of an exercise is recorded.
var Measurements = []string{"weight_reps", "bw_reps", "reps", "time", "distance_time"}

func isMuscle(m string) bool      { return slices.Contains(Muscles, m) }
func isMeasurement(m string) bool { return slices.Contains(Measurements, m) }
