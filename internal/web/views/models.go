package views

import (
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/stats"
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
	Stats        stats.ExerciseStats
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

// PlanEditor is the plan editor page.
type PlanEditor struct {
	PlanID string // "" for a new plan
	Title  string
	Doc    string // the document text shown in the editor
	Schema string // the plan JSON Schema, for the in-browser validator
	Errors plan.Problems
}

// PlanPage is a plan with its versions.
type PlanPage struct {
	Plan      store.Plan
	Versions  []store.PlanVersion
	Following bool
	Notice    string
}

// PlanVersionPage shows one version expanded week by week.
type PlanVersionPage struct {
	Plan    store.Plan
	Version store.PlanVersion
	Weeks   [][]plan.ExpandedDay
	JSON    string
}

// HomePage is the start page.
type HomePage struct {
	Next   *plan.Next
	Open   *store.Session // a workout in progress
	Notice string
}

// HistoryDetail is one session in the history editor.
type HistoryDetail struct {
	Session      store.Session
	Groups       []HistoryGroup // consecutive sets of one exercise
	Exercises    map[string]store.Exercise
	Catalog      []store.Exercise // for adding a set
	NextGroupPos int
	PRs          map[string]bool // ids of PR sets
	Error        string
}

// HistoryGroup is a run of sets of one exercise within a session.
type HistoryGroup struct {
	Slug     string
	GroupPos int
	Sets     []store.Set
}

// TokensData is the API tokens page. Secret is set only right after creation.
type TokensData struct {
	Tokens []store.APIToken
	Secret string
	MCPURL string
	Error  string
}
