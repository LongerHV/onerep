package views

import (
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

// ExerciseDetail is everything the exercise page shows.
type ExerciseDetail struct {
	Exercise     store.Exercise
	Settings     exercise.Settings
	Equipment    []store.Equipment // the user's profiles, for the link select
	Alternatives []exercise.AlternativeView
	Candidates   []store.Exercise // exercises that can be added as alternatives
	History      []store.TrainingMaxChange
	TMInput      string // training max form value, in the user's unit
	Errors       map[string]string
}

// ExerciseForm is the create/edit exercise form.
type ExerciseForm struct {
	New         bool
	Input       exercise.Input
	AliasesText string
	Errors      map[string]string
}

// EquipmentForm is the create/edit equipment form. List fields hold the text
// the user typed so it can be shown again next to errors.
type EquipmentForm struct {
	ID         string // "" for a new profile
	Name       string
	Kind       string
	Unit       string
	IsDefault  bool
	Bar        string
	Plates     string
	PlatePairs string
	Weights    string
	Stack      string
	Errors     map[string]string
}

// CalcResult is the percent-of-TM calculator output.
type CalcResult struct {
	PctText   string
	TMKg      float64
	Equipment store.Equipment // profile used for rounding; zero if none
	Kg        float64
	PerSide   []float64
	Error     string
}
