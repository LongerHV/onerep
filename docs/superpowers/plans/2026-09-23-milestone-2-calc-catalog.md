# onerep Milestone 2: Calculations and Exercise Catalog — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The training math (RPE table, e1RM, load resolution, rounding to real equipment) in Go and in browser JS, kept in agreement by shared test vectors. A seeded exercise catalog that users can search, customize and extend. Equipment profiles with a default per kind and starter profiles. Per-exercise training maxes with history, and a settings page for units.

**Architecture:** `internal/calc` is pure math with no I/O. `internal/web/static/js/calc.js` is a line-for-line port of it, and both run `testdata/calc_cases.json`. `internal/store` gets tables for equipment, exercises, alternatives, per-user exercise settings and the training-max log. `internal/exercise` holds the catalog rules, the embedded seed, equipment profiles and training maxes. `internal/account` holds the user's preferences. `internal/web` adds pages for exercises, equipment and settings, plus a middleware that creates starter equipment on a user's first request.

**Tech Stack:** unchanged from milestone 1. Node 24's built-in `node --test` runs the JS tests; it's only a dev/CI dependency.

**Spec:** `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. This plan implements §5 (equipment, exercises, exercise_alternatives, user_exercise, training_max_log), §6 rounding and load resolution, §7 calculations and shared vectors, and §14 seed catalog. It covers the parts of §13 that don't need history: the training-max history, but not e1RM charts or PRs.

## Global Constraints

Everything in milestone 1's Global Constraints still applies: Nix shell, `CGO_ENABLED=0`, Conventional Commits, UUIDv7, UTC timestamps, SQL only in `internal/store`, `NOT NULL` TEXT primary keys, committed generated files, real SQLite in tests, AGPL-compatible dependencies. In addition:

- Commit trailer for agent-authored commits: `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Weights are stored in kg. Equipment profile values are stored in the profile's own unit (`kg` or `lb`). 1 lb = 0.45359237 kg exactly.
- RTS table (spec §7): the percentage for (reps, RPE) is `seq[2*(reps-1) + 2*(10-RPE)]`, valid for reps 1–12 and RPE 6–10 in steps of 0.5, where seq = 1.000, 0.978, 0.955, 0.939, 0.922, 0.907, 0.892, 0.878, 0.863, 0.850, 0.837, 0.824, 0.811, 0.799, 0.786, 0.774, 0.762, 0.751, 0.739, 0.723, 0.707, 0.694, 0.680, 0.667, 0.653, 0.640, 0.626, 0.613, 0.599, 0.586, 0.572.
- e1RM: `weight / pct` when there's a valid RPE and reps ≤ 12, otherwise Epley `weight × (1 + reps/30)` (reps = 1 → weight). Reps > 12 or no weight gives no estimate.
- Rounding: the heaviest achievable load not above the target, or the lightest achievable load if nothing is that light. Equipment lookup order is the explicit link, then the user's default profile for the exercise's `equipment_kind`, then 0.5 kg / 1 lb steps. Absolute `weight` loads pass through unrounded.
- Starter profiles, created once per user on their first request: kg users get a 20 kg bar with plates 25/20/15/10/5/2.5/1.25 and dumbbells 2–50 kg in 2 kg steps; lb users get a 45 lb bar with plates 45/35/25/10/5/2.5 and dumbbells 5–100 lb in 5 lb steps.
- Muscle vocabulary (spec §14): chest, front-delts, side-delts, rear-delts, lats, upper-back, traps, biceps, triceps, forearms, abs, obliques, lower-back, glutes, quads, hamstrings, adductors, abductors, calves, neck.
- `internal/calc` and `calc.js` must stay in agreement: any change to one goes into both, with a new case in `testdata/calc_cases.json`.

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **Absurd or broken targets** (1e9 kg from a typo, NaN, negative). Rounding must stay bounded in time and memory and never return garbage. Tests: `TestRoundAbsurdTargetsStayBounded` (Task 1) and the "absurd targets" JS test (Task 2).
2. **Two tabs making a new user's first request at the same moment.** Starter equipment must be created exactly once. Test: `TestInitStarterEquipmentOnlyOnce` (Task 3).
3. **A user deletes the starter profiles.** They must not come back on the next request, even with a stale session. Test: `TestStarterEquipmentOnceAndInUnit` (Task 7).
4. **Plates stored heaviest-first** (as the starters are). The edit form must show a list that parses back. Test: `TestFormatWeightsSortsInput` (Task 6).
5. **A training max typed with a decimal comma** (`142,5`, common in Polish and other locales). It must save as 142.5. Test: `TestTrainingMaxAndCalculator` (Task 9).

Also covered: another user's equipment can't be viewed, deleted or linked (`TestEquipmentPages`, `TestSetExerciseEquipment`); ranges that expand to huge lists are refused (`TestParseWeights`).

## File Map

```
testdata/calc_cases.json                 shared Go/JS vectors
internal/calc/{units,rts,e1rm,equipment,load}.go, calc_test.go
internal/web/static/js/calc.js           JS port (served for milestone 4's companion mode)
internal/web/jstest/calc.test.mjs        node --test runner for the vectors
internal/store/schema.sql                + equipment, exercises, exercise_alternatives, user_exercise, training_max_log; users.equipment_initialized
internal/store/migrations/*_catalog_equipment.{up,down}.sql, atlas.sum   generated
internal/store/tx.go                     tx(), mustAffect, newID, JSON list helpers
internal/store/users.go, auth_sessions.go   + EquipmentInitialized, UpdateUserSettings
internal/store/equipment.go(+_test)      profiles, default per kind, InitStarterEquipment
internal/store/exercises.go(+_test)      catalog with shadowing, SyncSeed, alternatives
internal/store/user_exercise.go(+_test)  equipment link, training max + log
internal/exercise/vocab.go, errors.go    muscles, measurements, FieldErrors
internal/exercise/weights.go(+_test)     "2-50/2" list parsing/formatting, plate pairs
internal/exercise/seed/exercises.json    116 curated exercises
internal/exercise/seed.go(+_test)        embedded seed, Seed()
internal/exercise/service.go(+_test)     catalog/equipment/TM rules
internal/account/account.go(+_test)      unit + e1RM window
internal/web/views/ui.go, models.go      class constants, formatting, view models
internal/web/views/{layout,pages,exercises,equipment,settings}.templ
internal/web/{exercises,equipment,settings}.go   handlers
internal/web/server.go, server_test.go   new services, routes, helpers
internal/web/catalog_test.go             page tests
cmd/onerep/main.go                       seed at startup, wire services
Taskfile.yml, AGENTS.md
```

---

### Task 1: `calc` package

**Files:**
- Create: `testdata/calc_cases.json`
- Create: `internal/calc/units.go`, `rts.go`, `e1rm.go`, `equipment.go`, `load.go`
- Test: `internal/calc/calc_test.go`

**Interfaces:**
- Consumes: nothing
- Produces:
  - `calc.KgPerLb`, `calc.UnitKg = "kg"`, `calc.UnitLb = "lb"`
  - `calc.ToKg(v float64, unit string) float64`, `calc.FromKg(kg float64, unit string) float64`
  - `calc.RTSPercent(reps int, rpe float64) (float64, bool)`
  - `calc.E1RM(weightKg float64, reps int, rpe float64) (float64, calc.Method, bool)`, with `calc.MethodRPE` and `calc.MethodReps`
  - Kinds `calc.KindBarbell|KindDumbbell|KindMachine|KindCable|KindBodyweight` and `calc.Kinds []string`
  - `calc.Equipment{Kind, Unit string; Config calc.EquipmentConfig}` with `Validate() error`
  - `calc.EquipmentConfig{Bar float64; Plates []float64; PlatePairs map[string]int; Weights, Stack []float64; Min, Step, Max float64}` (JSON: `bar`, `plates`, `plate_pairs`, `weights`, `stack`, `min`, `step`, `max`)
  - `calc.Rounded{Kg float64; PerSide []float64}` and `calc.Round(targetKg float64, eq *calc.Equipment, fallbackUnit string) calc.Rounded`
  - `calc.Load{Weight, PctTM, RPE *float64}` and `calc.LoadContext{TMKg, E1RMKg *float64; Equipment *calc.Equipment; Unit string}`
  - `calc.ResolveLoad(l calc.Load, reps int, ctx calc.LoadContext) (calc.Rounded, bool)` and `calc.DropLoad(prevKg, pct float64, ctx calc.LoadContext) calc.Rounded`

- [ ] **Step 1: Write the shared vectors**

`testdata/calc_cases.json` (expected values were derived by hand from the RTS chart and plate arithmetic, not generated by the code under test):
```json
{
  "_comment": "Shared test vectors for internal/calc (Go) and internal/web/static/js/calc.js. Weights in kg unless the equipment says otherwise. A null expectation means 'no result'.",
  "rts": [
    {"reps": 1, "rpe": 10, "pct": 1.0},
    {"reps": 2, "rpe": 10, "pct": 0.955},
    {"reps": 5, "rpe": 8, "pct": 0.811},
    {"reps": 3, "rpe": 9.5, "pct": 0.907},
    {"reps": 8, "rpe": 7, "pct": 0.707},
    {"reps": 12, "rpe": 6, "pct": 0.572},
    {"reps": 0, "rpe": 8, "pct": null},
    {"reps": 13, "rpe": 8, "pct": null},
    {"reps": 5, "rpe": 5.5, "pct": null},
    {"reps": 5, "rpe": 10.5, "pct": null},
    {"reps": 5, "rpe": 7.25, "pct": null}
  ],
  "e1rm": [
    {"weight_kg": 100, "reps": 5, "rpe": 8, "kg": 123.30456226880394, "method": "rpe"},
    {"weight_kg": 100, "reps": 1, "rpe": 10, "kg": 100, "method": "rpe"},
    {"weight_kg": 80, "reps": 12, "rpe": 6, "kg": 139.86013986013987, "method": "rpe"},
    {"weight_kg": 100, "reps": 5, "rpe": 0, "kg": 116.66666666666667, "method": "reps"},
    {"weight_kg": 100, "reps": 1, "rpe": 0, "kg": 100, "method": "reps"},
    {"weight_kg": 100, "reps": 5, "rpe": 5, "kg": 116.66666666666667, "method": "reps"},
    {"weight_kg": 60, "reps": 15, "rpe": 0, "kg": null},
    {"weight_kg": 0, "reps": 5, "rpe": 8, "kg": null}
  ],
  "round": [
    {"name": "kg barbell rounds down", "target_kg": 101.3,
     "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}},
     "kg": 100, "per_side": [25, 15]},
    {"name": "kg barbell uses change plates", "target_kg": 102.6,
     "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}},
     "kg": 102.5, "per_side": [25, 15, 1.25]},
    {"name": "below the bar gives the bar", "target_kg": 15,
     "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}},
     "kg": 20, "per_side": []},
    {"name": "greedy would miss 10+10", "target_kg": 60,
     "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [15, 10]}},
     "kg": 60, "per_side": [10, 10]},
    {"name": "limited pairs", "target_kg": 120,
     "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20], "plate_pairs": {"25": 1}}},
     "kg": 110, "per_side": [25, 20]},
    {"name": "lb barbell", "target_kg": 100,
     "equipment": {"kind": "barbell", "unit": "lb", "config": {"bar": 45, "plates": [45, 35, 25, 10, 5, 2.5]}},
     "kg": 99.7903214, "per_side": [45, 35, 5, 2.5]},
    {"name": "dumbbell rounds down", "target_kg": 23.9,
     "equipment": {"kind": "dumbbell", "unit": "kg", "config": {"weights": [2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26]}},
     "kg": 22, "per_side": []},
    {"name": "dumbbell below lightest", "target_kg": 1,
     "equipment": {"kind": "dumbbell", "unit": "kg", "config": {"weights": [4, 2, 6]}},
     "kg": 2, "per_side": []},
    {"name": "machine min/step/max", "target_kg": 47,
     "equipment": {"kind": "machine", "unit": "kg", "config": {"min": 5, "step": 5, "max": 100}},
     "kg": 45, "per_side": []},
    {"name": "machine below min", "target_kg": 3,
     "equipment": {"kind": "machine", "unit": "kg", "config": {"min": 5, "step": 5, "max": 100}},
     "kg": 5, "per_side": []},
    {"name": "machine above max", "target_kg": 500,
     "equipment": {"kind": "cable", "unit": "kg", "config": {"min": 5, "step": 5, "max": 100}},
     "kg": 100, "per_side": []},
    {"name": "stack list", "target_kg": 25,
     "equipment": {"kind": "cable", "unit": "kg", "config": {"stack": [5, 10, 15, 22.5, 30]}},
     "kg": 22.5, "per_side": []},
    {"name": "no equipment, kg", "target_kg": 101.3, "fallback_unit": "kg", "equipment": null,
     "kg": 101, "per_side": []},
    {"name": "no equipment, lb", "target_kg": 50, "fallback_unit": "lb", "equipment": null,
     "kg": 49.8951607, "per_side": []},
    {"name": "bodyweight equipment", "target_kg": 10.7,
     "equipment": {"kind": "bodyweight", "unit": "kg", "config": {}},
     "kg": 10.5, "per_side": []},
    {"name": "negative target", "target_kg": -5, "fallback_unit": "kg", "equipment": null,
     "kg": 0, "per_side": []}
  ],
  "resolve": [
    {"name": "absolute weight as given", "load": {"weight": 102.3}, "reps": 5,
     "ctx": {"unit": "kg"}, "kg": 102.3, "per_side": []},
    {"name": "percent of TM", "load": {"pct_tm": 0.75}, "reps": 5,
     "ctx": {"unit": "kg", "tm_kg": 140, "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}}},
     "kg": 105, "per_side": [25, 15, 2.5]},
    {"name": "percent of TM without TM", "load": {"pct_tm": 0.75}, "reps": 5,
     "ctx": {"unit": "kg"}, "kg": null},
    {"name": "RPE target from e1RM", "load": {"rpe": 8}, "reps": 5,
     "ctx": {"unit": "kg", "e1rm_kg": 150, "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}}},
     "kg": 120, "per_side": [25, 25]},
    {"name": "RPE target outside the table", "load": {"rpe": 8}, "reps": 13,
     "ctx": {"unit": "kg", "e1rm_kg": 150}, "kg": null},
    {"name": "RPE target without e1RM", "load": {"rpe": 8}, "reps": 5,
     "ctx": {"unit": "kg"}, "kg": null},
    {"name": "empty load", "load": {}, "reps": 5, "ctx": {"unit": "kg"}, "kg": null}
  ],
  "drop": [
    {"name": "20% drop", "prev_kg": 100, "pct": 0.2,
     "ctx": {"unit": "kg", "equipment": {"kind": "barbell", "unit": "kg", "config": {"bar": 20, "plates": [25, 20, 15, 10, 5, 2.5, 1.25]}}},
     "kg": 80, "per_side": [25, 5]}
  ]
}
```

- [ ] **Step 2: Write the failing tests**

`internal/calc/calc_test.go`:
```go
package calc

import (
	"encoding/json"
	"math"
	"os"
	"slices"
	"strings"
	"testing"
)

type cases struct {
	RTS []struct {
		Reps int      `json:"reps"`
		RPE  float64  `json:"rpe"`
		Pct  *float64 `json:"pct"`
	} `json:"rts"`
	E1RM []struct {
		WeightKg float64  `json:"weight_kg"`
		Reps     int      `json:"reps"`
		RPE      float64  `json:"rpe"`
		Kg       *float64 `json:"kg"`
		Method   Method   `json:"method"`
	} `json:"e1rm"`
	Round []struct {
		Name         string     `json:"name"`
		TargetKg     float64    `json:"target_kg"`
		FallbackUnit string     `json:"fallback_unit"`
		Equipment    *Equipment `json:"equipment"`
		Kg           float64    `json:"kg"`
		PerSide      []float64  `json:"per_side"`
	} `json:"round"`
	Resolve []struct {
		Name    string      `json:"name"`
		Load    Load        `json:"load"`
		Reps    int         `json:"reps"`
		Ctx     LoadContext `json:"ctx"`
		Kg      *float64    `json:"kg"`
		PerSide []float64   `json:"per_side"`
	} `json:"resolve"`
	Drop []struct {
		Name    string      `json:"name"`
		PrevKg  float64     `json:"prev_kg"`
		Pct     float64     `json:"pct"`
		Ctx     LoadContext `json:"ctx"`
		Kg      float64     `json:"kg"`
		PerSide []float64   `json:"per_side"`
	} `json:"drop"`
}

func loadCases(t *testing.T) cases {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/calc_cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c cases
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func samePlates(a, b []float64) bool {
	return slices.EqualFunc(a, b, near) || (len(a) == 0 && len(b) == 0)
}

func TestRTSVectors(t *testing.T) {
	for _, c := range loadCases(t).RTS {
		got, ok := RTSPercent(c.Reps, c.RPE)
		switch {
		case c.Pct == nil && ok:
			t.Errorf("RTSPercent(%d, %v) = %v, want no result", c.Reps, c.RPE, got)
		case c.Pct != nil && (!ok || !near(got, *c.Pct)):
			t.Errorf("RTSPercent(%d, %v) = %v, %v; want %v", c.Reps, c.RPE, got, ok, *c.Pct)
		}
	}
}

func TestE1RMVectors(t *testing.T) {
	for _, c := range loadCases(t).E1RM {
		got, method, ok := E1RM(c.WeightKg, c.Reps, c.RPE)
		switch {
		case c.Kg == nil && ok:
			t.Errorf("E1RM(%v, %d, %v) = %v, want no result", c.WeightKg, c.Reps, c.RPE, got)
		case c.Kg != nil && (!ok || !near(got, *c.Kg) || method != c.Method):
			t.Errorf("E1RM(%v, %d, %v) = %v %s %v; want %v %s", c.WeightKg, c.Reps, c.RPE, got, method, ok, *c.Kg, c.Method)
		}
	}
}

func TestRoundVectors(t *testing.T) {
	for _, c := range loadCases(t).Round {
		got := Round(c.TargetKg, c.Equipment, c.FallbackUnit)
		if !near(got.Kg, c.Kg) || !samePlates(got.PerSide, c.PerSide) {
			t.Errorf("%s: got %v %v, want %v %v", c.Name, got.Kg, got.PerSide, c.Kg, c.PerSide)
		}
	}
}

func TestResolveVectors(t *testing.T) {
	for _, c := range loadCases(t).Resolve {
		got, ok := ResolveLoad(c.Load, c.Reps, c.Ctx)
		switch {
		case c.Kg == nil && ok:
			t.Errorf("%s: got %v, want no result", c.Name, got)
		case c.Kg != nil && (!ok || !near(got.Kg, *c.Kg) || !samePlates(got.PerSide, c.PerSide)):
			t.Errorf("%s: got %v %v %v, want %v %v", c.Name, got.Kg, got.PerSide, ok, *c.Kg, c.PerSide)
		}
	}
}

func TestDropVectors(t *testing.T) {
	for _, c := range loadCases(t).Drop {
		got := DropLoad(c.PrevKg, c.Pct, c.Ctx)
		if !near(got.Kg, c.Kg) || !samePlates(got.PerSide, c.PerSide) {
			t.Errorf("%s: got %v %v, want %v %v", c.Name, got.Kg, got.PerSide, c.Kg, c.PerSide)
		}
	}
}

func TestRoundAbsurdTargetsStayBounded(t *testing.T) {
	bar := &Equipment{Kind: KindBarbell, Unit: UnitKg, Config: EquipmentConfig{Bar: 20, Plates: []float64{25, 1.25}}}
	got := Round(1e9, bar, UnitKg)
	if !near(got.Kg, 20+2*1000) {
		t.Fatalf("huge target: got %v, want capped at 1000 per side", got.Kg)
	}
	if got := Round(math.NaN(), nil, UnitKg); got.Kg != 0 {
		t.Fatalf("NaN target: got %v", got.Kg)
	}
}

func TestEquipmentValidate(t *testing.T) {
	cases := map[string]struct {
		eq   Equipment
		want string
	}{
		"ok barbell":         {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{25}}}, ""},
		"bad unit":           {Equipment{KindBarbell, "st", EquipmentConfig{Bar: 20, Plates: []float64{25}}}, "unit"},
		"no plates":          {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20}}, "plates"},
		"zero plate":         {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{0}}}, "positive"},
		"pairs unknown size": {Equipment{KindBarbell, UnitKg, EquipmentConfig{Bar: 20, Plates: []float64{25}, PlatePairs: map[string]int{"20": 1}}}, "plate_pairs"},
		"no dumbbells":       {Equipment{KindDumbbell, UnitKg, EquipmentConfig{}}, "weights"},
		"machine stack":      {Equipment{KindMachine, UnitKg, EquipmentConfig{Stack: []float64{5, 10}}}, ""},
		"machine range":      {Equipment{KindCable, UnitLb, EquipmentConfig{Min: 5, Step: 5, Max: 100}}, ""},
		"machine no step":    {Equipment{KindCable, UnitLb, EquipmentConfig{Min: 5, Max: 100}}, "stack"},
		"bodyweight":         {Equipment{KindBodyweight, UnitKg, EquipmentConfig{}}, ""},
		"unknown kind":       {Equipment{"kettlebell", UnitKg, EquipmentConfig{}}, "unknown"},
	}
	for name, c := range cases {
		err := c.eq.Validate()
		if c.want == "" && err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: want error containing %q, got %v", name, c.want, err)
		}
	}
}

func TestUnitConversionRoundTrips(t *testing.T) {
	if !near(FromKg(ToKg(225, UnitLb), UnitLb), 225) || ToKg(100, UnitKg) != 100 {
		t.Fatal("conversion does not round-trip")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/calc/`
Expected: FAIL, build errors such as `undefined: Method`, `undefined: Equipment`, `undefined: RTSPercent`.

- [ ] **Step 4: Implement**

`internal/calc/units.go`:
```go
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
```

`internal/calc/rts.go`:
```go
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
```

`internal/calc/e1rm.go`:
```go
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
```

`internal/calc/equipment.go` (barbell rounding is a bounded knapsack over plate sizes in integer hundredths. Greedy would fail on e.g. plates {15, 10} with 20 per side):
```go
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
```

`internal/calc/load.go`:
```go
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/calc/`
Expected: `ok  github.com/LongerHV/onerep/internal/calc`

- [ ] **Step 6: Commit**

```bash
git add testdata internal/calc
git commit -m "feat(calc): add RTS table, e1RM, load resolution and equipment rounding"
```

---

### Task 2: Browser port of `calc` with Node tests

**Files:**
- Create: `internal/web/static/js/calc.js`
- Test: `internal/web/jstest/calc.test.mjs`
- Replace: `Taskfile.yml` (adds `test:js` and runs it in `ci`)

**Interfaces:**
- Consumes: `testdata/calc_cases.json` (Task 1)
- Produces (ES module `/static/js/calc.js`): `KG_PER_LB`, `toKg(v, unit)`, `fromKg(kg, unit)`, `rtsPercent(reps, rpe) → number|null`, `e1rm(weightKg, reps, rpe) → {kg, method}|null`, `round(targetKg, equipment, fallbackUnit) → {kg, per_side}`, `resolveLoad(load, reps, ctx) → {kg, per_side}|null`, `dropLoad(prevKg, pct, ctx) → {kg, per_side}`. Objects use the same JSON field names as the Go types (`tm_kg`, `e1rm_kg`, `pct_tm`, `plate_pairs`, …).

- [ ] **Step 1: Write the failing test**

`internal/web/jstest/calc.test.mjs` (under `internal/web/`, not `static/`, so it isn't embedded or served):
```javascript
// Runs the shared calc vectors against the browser implementation.
// Run with: node --test internal/web/jstest/
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dropLoad, e1rm, resolveLoad, round, rtsPercent } from "../static/js/calc.js";

const cases = JSON.parse(readFileSync(new URL("../../../testdata/calc_cases.json", import.meta.url)));

const near = (a, b) => Math.abs(a - b) < 1e-6;
const samePlates = (a, b) => a.length === b.length && a.every((v, i) => near(v, b[i]));

test("rts", () => {
  for (const c of cases.rts) {
    const got = rtsPercent(c.reps, c.rpe);
    if (c.pct === null) assert.equal(got, null, `rts(${c.reps}, ${c.rpe})`);
    else assert.ok(got !== null && near(got, c.pct), `rts(${c.reps}, ${c.rpe}) = ${got}, want ${c.pct}`);
  }
});

test("e1rm", () => {
  for (const c of cases.e1rm) {
    const got = e1rm(c.weight_kg, c.reps, c.rpe);
    if (c.kg === null) assert.equal(got, null, `e1rm(${c.weight_kg}, ${c.reps}, ${c.rpe})`);
    else assert.ok(got && near(got.kg, c.kg) && got.method === c.method,
      `e1rm(${c.weight_kg}, ${c.reps}, ${c.rpe}) = ${JSON.stringify(got)}, want ${c.kg} ${c.method}`);
  }
});

test("round", () => {
  for (const c of cases.round) {
    const got = round(c.target_kg, c.equipment, c.fallback_unit);
    assert.ok(near(got.kg, c.kg) && samePlates(got.per_side, c.per_side),
      `${c.name}: got ${JSON.stringify(got)}, want ${c.kg} ${JSON.stringify(c.per_side)}`);
  }
});

test("resolve", () => {
  for (const c of cases.resolve) {
    const got = resolveLoad(c.load, c.reps, c.ctx);
    if (c.kg === null) assert.equal(got, null, c.name);
    else assert.ok(got && near(got.kg, c.kg) && samePlates(got.per_side, c.per_side),
      `${c.name}: got ${JSON.stringify(got)}, want ${c.kg} ${JSON.stringify(c.per_side)}`);
  }
});

test("drop", () => {
  for (const c of cases.drop) {
    const got = dropLoad(c.prev_kg, c.pct, c.ctx);
    assert.ok(near(got.kg, c.kg) && samePlates(got.per_side, c.per_side), c.name);
  }
});

test("absurd targets stay bounded", () => {
  const bar = { kind: "barbell", unit: "kg", config: { bar: 20, plates: [25, 1.25] } };
  assert.ok(near(round(1e9, bar, "kg").kg, 2020));
  assert.equal(round(NaN, null, "kg").kg, 0);
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `node --test "internal/web/jstest/*.test.mjs"`
Expected: FAIL with `ERR_MODULE_NOT_FOUND` for `static/js/calc.js`. Note: pass the file glob; `node --test internal/web/jstest/` treats the directory as a single file and fails for an unrelated reason.

- [ ] **Step 3: Implement**

`internal/web/static/js/calc.js`:
```javascript
// Training math shared with the server. This is a line-for-line port of
// internal/calc (Go); testdata/calc_cases.json keeps both in agreement.
// Weights are kg unless an equipment profile says otherwise.

export const KG_PER_LB = 0.45359237;

export function toKg(v, unit) {
  return unit === "lb" ? v * KG_PER_LB : v;
}

export function fromKg(kg, unit) {
  return unit === "lb" ? kg / KG_PER_LB : kg;
}

// Integer hundredths, tolerating float noise (99.99999999 counts as 100.00).
function cents(v) {
  return Math.floor(v * 100 + 1e-6);
}

const RTS_SEQUENCE = [
  1.0, 0.978, 0.955, 0.939, 0.922, 0.907, 0.892, 0.878, 0.863, 0.85,
  0.837, 0.824, 0.811, 0.799, 0.786, 0.774, 0.762, 0.751, 0.739, 0.723,
  0.707, 0.694, 0.68, 0.667, 0.653, 0.64, 0.626, 0.613, 0.599, 0.586,
  0.572,
];

// Fraction of 1RM for reps at rpe (reps 1–12, RPE 6–10 in 0.5 steps), or null.
export function rtsPercent(reps, rpe) {
  if (!Number.isInteger(reps) || reps < 1 || reps > 12 || !(rpe >= 6 && rpe <= 10)) {
    return null;
  }
  const halfSteps = (10 - rpe) * 2;
  if (!Number.isInteger(halfSteps)) return null;
  return RTS_SEQUENCE[2 * (reps - 1) + halfSteps];
}

// Estimated 1RM as {kg, method} ("rpe" or "reps"), or null. rpe 0 = not recorded.
export function e1rm(weightKg, reps, rpe) {
  if (!(weightKg > 0) || reps < 1 || reps > 12) return null;
  const pct = rtsPercent(reps, rpe);
  if (pct !== null) return { kg: weightKg / pct, method: "rpe" };
  if (reps === 1) return { kg: weightKg, method: "reps" };
  return { kg: weightKg * (1 + reps / 30), method: "reps" };
}

const MAX_SIDE_CENTS = 100000;

// Heaviest achievable load not above targetKg (or the lightest achievable one),
// as {kg, per_side}. per_side lists one side's plates in the equipment's unit.
export function round(targetKg, equipment, fallbackUnit) {
  if (Number.isNaN(targetKg) || targetKg < 0) targetKg = 0;
  if (!equipment || equipment.kind === "bodyweight") {
    const unit = equipment ? equipment.unit : fallbackUnit;
    const step = unit === "lb" ? 100 : 50;
    const t = Math.max(0, cents(fromKg(targetKg, unit)));
    return { kg: toKg((Math.floor(t / step) * step) / 100, unit), per_side: [] };
  }
  const unit = equipment.unit;
  const c = equipment.config || {};
  const t = cents(fromKg(targetKg, unit));
  switch (equipment.kind) {
    case "barbell":
      return roundBarbell(t, unit, c);
    case "dumbbell":
      return { kg: toKg(pickFromList(t, c.weights) / 100, unit), per_side: [] };
    default: {
      if (c.stack && c.stack.length > 0) {
        return { kg: toKg(pickFromList(t, c.stack) / 100, unit), per_side: [] };
      }
      const lo = cents(c.min || 0), step = cents(c.step), hi = cents(c.max || 0);
      let v = lo;
      if (t > lo) v = lo + Math.floor((t - lo) / step) * step;
      if (v > hi) v = hi;
      return { kg: toKg(v / 100, unit), per_side: [] };
    }
  }
}

function pickFromList(t, values) {
  let best = -1, smallest = Infinity;
  for (const v of values) {
    const c = cents(v);
    if (c <= t && c > best) best = c;
    if (c < smallest) smallest = c;
  }
  return best >= 0 ? best : smallest;
}

function roundBarbell(t, unit, c) {
  const bar = cents(c.bar || 0);
  if (t <= bar) return { kg: toKg(bar / 100, unit), per_side: [] };
  const side = Math.min(Math.floor((t - bar) / 2), MAX_SIDE_CENTS);

  const pairs = c.plate_pairs || {};
  const plates = (c.plates || [])
    .map((p) => {
      const size = cents(p);
      let max = -1;
      for (const [k, n] of Object.entries(pairs)) {
        if (cents(parseFloat(k)) === size) max = n;
      }
      return { size, max };
    })
    .filter((p) => p.size > 0)
    .sort((a, b) => b.size - a.size);

  // reach[i][s]: a per-side sum of s is achievable with plates[i:].
  const n = plates.length;
  const reach = Array.from({ length: n + 1 }, () => new Uint8Array(side + 1));
  reach[n][0] = 1;
  for (let i = n - 1; i >= 0; i--) {
    const p = plates[i];
    for (let s = 0; s <= side; s++) {
      if (reach[i + 1][s]) {
        reach[i][s] = 1;
        continue;
      }
      if (p.max < 0) {
        reach[i][s] = s >= p.size && reach[i][s - p.size] ? 1 : 0;
        continue;
      }
      for (let k = 1; k <= p.max && k * p.size <= s; k++) {
        if (reach[i + 1][s - k * p.size]) {
          reach[i][s] = 1;
          break;
        }
      }
    }
  }

  let best = side;
  while (!reach[0][best]) best--;
  const perSide = [];
  let rest = best;
  plates.forEach((p, i) => {
    let k = Math.floor(rest / p.size);
    if (p.max >= 0 && k > p.max) k = p.max;
    while (k > 0 && !reach[i + 1][rest - k * p.size]) k--;
    for (let j = 0; j < k; j++) perSide.push(p.size / 100);
    rest -= k * p.size;
  });
  return { kg: toKg((bar + 2 * best) / 100, unit), per_side: perSide };
}

// Achievable weight for a prescription {weight} | {pct_tm} | {rpe}, or null
// when the needed training max or e1RM is unknown. Absolute weights pass through.
export function resolveLoad(load, reps, ctx) {
  if (load.weight != null) return { kg: load.weight, per_side: [] };
  if (load.pct_tm != null) {
    if (ctx.tm_kg == null) return null;
    return round(load.pct_tm * ctx.tm_kg, ctx.equipment, ctx.unit);
  }
  if (load.rpe != null) {
    const pct = rtsPercent(reps, load.rpe);
    if (pct === null || ctx.e1rm_kg == null) return null;
    return round(pct * ctx.e1rm_kg, ctx.equipment, ctx.unit);
  }
  return null;
}

// Weight for a drop set pct below the previous set's weight.
export function dropLoad(prevKg, pct, ctx) {
  return round(prevKg * (1 - pct), ctx.equipment, ctx.unit);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `node --test "internal/web/jstest/*.test.mjs"`
Expected: `ℹ pass 6`, `ℹ fail 0`.

- [ ] **Step 5: Add the task and CI step**

Replace `Taskfile.yml` with:
```yaml
version: "3"

vars:
  MIGRATIONS_DIR: "file://internal/store/migrations?format=golang-migrate"
  DEV_DB_URL: "sqlite://dev?mode=memory"

tasks:
  generate:
    desc: Generate templ components and the Tailwind stylesheet
    cmds:
      - templ generate
      - tailwindcss -i internal/web/styles/input.css -o internal/web/static/app.css --minify

  build:
    desc: Build the onerep binary into ./bin
    deps: [generate]
    cmds:
      - go build -trimpath -o bin/onerep ./cmd/onerep

  test:
    desc: Run all Go tests
    cmds:
      - go test ./...

  test:js:
    desc: Run the browser JS tests (shared calc vectors) with Node
    cmds:
      - node --test "internal/web/jstest/*.test.mjs"

  lint:
    desc: Run golangci-lint
    cmds:
      - golangci-lint run ./...

  dev:
    desc: Run with live reload, signed in as the dev user (no IdP needed)
    env:
      ONEREP_ENV: dev
      ONEREP_DEV_USER: '{{.USER | default "alice"}}'
      ONEREP_DB: ./tmp/onerep.db
    cmds:
      - mkdir -p tmp
      - air

  dev:oidc:
    desc: Run with live reload against the local Dex (start `task dex` first)
    env:
      ONEREP_ENV: dev
      ONEREP_DB: ./tmp/onerep.db
      ONEREP_OIDC_ISSUER: http://127.0.0.1:5556/dex
      ONEREP_OIDC_CLIENT_ID: onerep
      ONEREP_OIDC_CLIENT_SECRET: onerep-dev-secret
    cmds:
      - mkdir -p tmp
      - air

  dex:
    desc: Run the local Dex identity provider (alice@example.com / password)
    cmds:
      - dex serve dev/dex.yaml

  migrate:diff:
    desc: Generate a migration from internal/store/schema.sql (usage task migrate:diff NAME=add_foo)
    requires:
      vars: [NAME]
    cmds:
      - atlas migrate diff {{.NAME}} --dir "{{.MIGRATIONS_DIR}}" --to file://internal/store/schema.sql --dev-url "{{.DEV_DB_URL}}"

  migrate:check:
    desc: Verify migration integrity (atlas.sum) and that migrations match schema.sql
    cmds:
      - atlas migrate validate --dir "{{.MIGRATIONS_DIR}}" --dev-url "{{.DEV_DB_URL}}"
      - |
        out=$(atlas schema diff --from "{{.MIGRATIONS_DIR}}" --to file://internal/store/schema.sql --dev-url "{{.DEV_DB_URL}}" 2>/dev/null)
        if ! echo "$out" | grep -q "Schemas are synced"; then
          echo "schema.sql and migrations differ; run: task migrate:diff NAME=<name>"
          echo "$out"
          exit 1
        fi

  check:generated:
    desc: Fail if generated files (templ, CSS) are out of date
    deps: [generate]
    cmds:
      - |
        if [ -n "$(git status --porcelain -- '*_templ.go' internal/web/static/app.css)" ]; then
          git status --porcelain -- '*_templ.go' internal/web/static/app.css
          echo "generated files are stale; run: task generate"
          exit 1
        fi

  licenses:check:
    desc: Fail if a Go dependency of the binary has a license incompatible with AGPL-3.0
    cmds:
      - go-licenses check ./cmd/onerep --ignore github.com/LongerHV/onerep --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,ISC,0BSD,Apache-2.0

  licenses:
    desc: Collect onerep and dependency license texts into the image data dir (cmd/onerep/kodata)
    cmds:
      - rm -rf cmd/onerep/kodata/third_party
      - go-licenses save ./cmd/onerep --ignore github.com/LongerHV/onerep --save_path cmd/onerep/kodata/third_party
      - cp LICENSE cmd/onerep/kodata/LICENSE

  ci:
    desc: Everything CI checks (except the image build)
    cmds:
      - task: check:generated
      - task: lint
      - task: test
      - task: test:js
      - task: migrate:check
      - task: licenses:check

  image:
    desc: Build the container image locally without pushing
    deps: [licenses]
    cmds:
      - KO_DOCKER_REPO=ko.local ko build --bare --push=false ./cmd/onerep
```

Run: `task test:js`
Expected: `ℹ pass 6`.

- [ ] **Step 6: Commit**

```bash
git add internal/web/static/js internal/web/jstest Taskfile.yml
git commit -m "feat(web): port calc to browser JS, verified by shared vectors"
```

---

### Task 3: Schema, migration, and equipment store

**Files:**
- Replace: `internal/store/schema.sql`
- Create (generated): `internal/store/migrations/<timestamp>_catalog_equipment.{up,down}.sql`, updated `atlas.sum`
- Create: `internal/store/tx.go`, `internal/store/equipment.go`
- Replace: `internal/store/users.go`, `internal/store/auth_sessions.go`
- Test: `internal/store/equipment_test.go`

**Interfaces:**
- Consumes: `calc.Equipment`, `calc.Kind*`, `calc.Unit*` (Task 1); `newTestDB` (milestone 1)
- Produces:
  - `store.User.EquipmentInitialized bool`, loaded everywhere users are loaded, including `AuthSessionByHash`
  - `(*DB).UpdateUserSettings(ctx, userID, unit string, e1rmWindowDays int) error`
  - Unexported helpers `(*DB).tx(ctx, func(*sql.Tx) error) error`, `mustAffect(sql.Result) error`, `newID() (string, error)`, `jsonList([]string) string`, `parseList(string) ([]string, error)`, `nullString(string) sql.NullString`
  - `store.Equipment{ID, UserID, Name string; IsDefault bool; Spec calc.Equipment; CreatedAt time.Time}`
  - `(*DB).ListEquipment(ctx, userID) ([]Equipment, error)`, `EquipmentByID(ctx, userID, id) (Equipment, error)`, `DefaultEquipment(ctx, userID, kind) (Equipment, error)`, `SaveEquipment(ctx, Equipment) (Equipment, error)` (an empty ID inserts), `DeleteEquipment(ctx, userID, id) error`, `InitStarterEquipment(ctx, userID string, items []Equipment) (bool, error)`
  - Test helpers in package `store`: `newUser(t, db, sub) User`, `barbell(userID, name string, isDefault bool) Equipment`

- [ ] **Step 1: Extend the schema**

Replace `internal/store/schema.sql` with:
```sql
-- Desired database schema. Edit this file, then run `task migrate:diff NAME=<name>`
-- to generate a versioned migration in ./migrations.

CREATE TABLE users (
  id               TEXT    NOT NULL PRIMARY KEY,
  oidc_issuer      TEXT    NOT NULL,
  oidc_sub         TEXT    NOT NULL,
  email            TEXT    NOT NULL DEFAULT '',
  name             TEXT    NOT NULL DEFAULT '',
  unit             TEXT    NOT NULL DEFAULT 'kg' CHECK (unit IN ('kg', 'lb')),
  e1rm_window_days INTEGER NOT NULL DEFAULT 30,
  -- set once the starter equipment profiles have been created
  equipment_initialized INTEGER NOT NULL DEFAULT 0,
  created_at       TEXT    NOT NULL,
  UNIQUE (oidc_issuer, oidc_sub)
);

CREATE TABLE auth_sessions (
  id_hash    TEXT NOT NULL PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX auth_sessions_user_id ON auth_sessions (user_id);

CREATE TABLE equipment (
  id         TEXT    NOT NULL PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  kind       TEXT    NOT NULL CHECK (kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')),
  unit       TEXT    NOT NULL CHECK (unit IN ('kg', 'lb')),
  config     TEXT    NOT NULL, -- calc.EquipmentConfig as JSON, values in unit
  is_default INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL
);

CREATE INDEX equipment_user_id ON equipment (user_id);
CREATE UNIQUE INDEX equipment_one_default_per_kind ON equipment (user_id, kind) WHERE is_default = 1;

-- Global rows (user_id NULL) are the seeded catalog. A user row with the same
-- slug shadows the global one for that user.
CREATE TABLE exercises (
  id                TEXT    NOT NULL PRIMARY KEY,
  user_id           TEXT    REFERENCES users (id) ON DELETE CASCADE,
  slug              TEXT    NOT NULL,
  name              TEXT    NOT NULL,
  measurement       TEXT    NOT NULL CHECK (measurement IN ('weight_reps', 'bw_reps', 'reps', 'time', 'distance_time')),
  equipment_kind    TEXT    NOT NULL CHECK (equipment_kind IN ('barbell', 'dumbbell', 'machine', 'cable', 'bodyweight')),
  primary_muscles   TEXT    NOT NULL DEFAULT '[]',
  secondary_muscles TEXT    NOT NULL DEFAULT '[]',
  aliases           TEXT    NOT NULL DEFAULT '[]',
  hidden            INTEGER NOT NULL DEFAULT 0, -- global rows dropped from the seed file
  created_at        TEXT    NOT NULL,
  updated_at        TEXT    NOT NULL
);

CREATE UNIQUE INDEX exercises_global_slug ON exercises (slug) WHERE user_id IS NULL;
CREATE UNIQUE INDEX exercises_user_slug ON exercises (user_id, slug) WHERE user_id IS NOT NULL;

-- Global rows come from the seed file; user rows add to them.
CREATE TABLE exercise_alternatives (
  user_id  TEXT REFERENCES users (id) ON DELETE CASCADE,
  slug     TEXT NOT NULL,
  alt_slug TEXT NOT NULL
);

CREATE UNIQUE INDEX exercise_alternatives_global ON exercise_alternatives (slug, alt_slug) WHERE user_id IS NULL;
CREATE UNIQUE INDEX exercise_alternatives_user ON exercise_alternatives (user_id, slug, alt_slug) WHERE user_id IS NOT NULL;

CREATE TABLE user_exercise (
  user_id         TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  slug            TEXT NOT NULL,
  equipment_id    TEXT REFERENCES equipment (id) ON DELETE SET NULL,
  training_max_kg REAL,
  updated_at      TEXT NOT NULL,
  PRIMARY KEY (user_id, slug)
);

CREATE INDEX user_exercise_equipment_id ON user_exercise (equipment_id);

CREATE TABLE training_max_log (
  id         TEXT NOT NULL PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  slug       TEXT NOT NULL,
  old_kg     REAL,
  new_kg     REAL,
  source     TEXT NOT NULL CHECK (source IN ('web', 'mcp')),
  note       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX training_max_log_user_slug ON training_max_log (user_id, slug, created_at);
```

- [ ] **Step 2: Generate and check the migration**

Run: `task migrate:diff NAME=catalog_equipment && task migrate:check`
Expected: a new `<timestamp>_catalog_equipment.up.sql` that starts with ``ALTER TABLE `users` ADD COLUMN `equipment_initialized` ...`` and creates the five tables, including the partial unique indexes (`... WHERE user_id IS NULL`, `... WHERE is_default = 1`); `migrate:check` exits 0.

- [ ] **Step 3: Write the failing tests**

`internal/store/equipment_test.go`:
```go
package store

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

func newUser(t *testing.T, db *DB, sub string) User {
	t.Helper()
	u, err := db.UpsertOIDCUser(context.Background(), "iss", sub, "", sub)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func barbell(userID, name string, isDefault bool) Equipment {
	return Equipment{UserID: userID, Name: name, IsDefault: isDefault, Spec: calc.Equipment{
		Kind: calc.KindBarbell, Unit: calc.UnitKg,
		Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{25, 1.25}, PlatePairs: map[string]int{"1.25": 2}},
	}}
}

func TestEquipmentCRUD(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	saved, err := db.SaveEquipment(ctx, barbell(alice.ID, "Home bar", true))
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.EquipmentByID(ctx, alice.ID, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Home bar" || !got.IsDefault || got.Spec.Config.Bar != 20 || got.Spec.Config.PlatePairs["1.25"] != 2 {
		t.Fatalf("round trip: %+v", got)
	}

	if _, err := db.EquipmentByID(ctx, bob.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user's equipment must be ErrNotFound, got %v", err)
	}
	other := barbell(bob.ID, "stolen", false)
	other.ID = saved.ID
	if _, err := db.SaveEquipment(ctx, other); !errors.Is(err, ErrNotFound) {
		t.Fatalf("updating other user's equipment must be ErrNotFound, got %v", err)
	}
	if err := db.DeleteEquipment(ctx, bob.ID, saved.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting other user's equipment must be ErrNotFound, got %v", err)
	}

	got.Name = "Garage bar"
	if _, err := db.SaveEquipment(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListEquipment(ctx, alice.ID)
	if err != nil || len(list) != 1 || list[0].Name != "Garage bar" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteEquipment(ctx, alice.ID, saved.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.ListEquipment(ctx, alice.ID); len(list) != 0 {
		t.Fatalf("not deleted: %+v", list)
	}
}

func TestOneDefaultPerKind(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	first, err := db.SaveEquipment(ctx, barbell(u.ID, "first", true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.SaveEquipment(ctx, barbell(u.ID, "second", true))
	if err != nil {
		t.Fatal(err)
	}
	def, err := db.DefaultEquipment(ctx, u.ID, calc.KindBarbell)
	if err != nil || def.ID != second.ID {
		t.Fatalf("default = %+v, %v; want second", def, err)
	}
	if f, _ := db.EquipmentByID(ctx, u.ID, first.ID); f.IsDefault {
		t.Fatal("first profile is still marked default")
	}
	if _, err := db.DefaultEquipment(ctx, u.ID, calc.KindDumbbell); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no dumbbell default: want ErrNotFound, got %v", err)
	}
}

func TestInitStarterEquipmentOnlyOnce(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	items := []Equipment{barbell("", "Barbell", true)}

	var wg sync.WaitGroup
	results := make(chan bool, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			created, err := db.InitStarterEquipment(ctx, u.ID, items)
			if err != nil {
				t.Error(err)
			}
			results <- created
		}()
	}
	wg.Wait()
	close(results)
	n := 0
	for created := range results {
		if created {
			n++
		}
	}
	list, _ := db.ListEquipment(ctx, u.ID)
	if n != 1 || len(list) != 1 {
		t.Fatalf("created %d times, %d profiles; want exactly once", n, len(list))
	}
	if got, _ := db.UserByID(ctx, u.ID); !got.EquipmentInitialized {
		t.Fatal("user not marked initialized")
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL, `undefined: Equipment` and `db.SaveEquipment undefined`.

- [ ] **Step 5: Implement the helpers and user changes**

`internal/store/tx.go`:
```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// tx runs fn in a write transaction, committing if fn returns nil.
func (db *DB) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	t, err := db.write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(t); err != nil {
		return errors.Join(err, t.Rollback())
	}
	return t.Commit()
}

// mustAffect turns "no rows changed" into ErrNotFound.
func mustAffect(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// newID returns a new UUIDv7 string.
func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// jsonList encodes a string list for a JSON TEXT column; nil becomes [].
func jsonList(vs []string) string {
	if vs == nil {
		vs = []string{}
	}
	b, _ := json.Marshal(vs)
	return string(b)
}

func parseList(s string) ([]string, error) {
	var vs []string
	return vs, json.Unmarshal([]byte(s), &vs)
}

// nullString maps "" to NULL.
func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
```

Replace `internal/store/users.go` with:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type User struct {
	ID             string
	OIDCIssuer     string
	OIDCSubject    string
	Email          string
	Name           string
	Unit           string
	E1RMWindowDays int
	// EquipmentInitialized is set once the starter equipment profiles exist.
	EquipmentInitialized bool
	CreatedAt            time.Time
}

const userColumns = `id, oidc_issuer, oidc_sub, email, name, unit, e1rm_window_days, equipment_initialized, created_at`

func scanUser(row interface{ Scan(...any) error }) (User, error) {
	var u User
	var created string
	err := row.Scan(&u.ID, &u.OIDCIssuer, &u.OIDCSubject, &u.Email, &u.Name, &u.Unit, &u.E1RMWindowDays, &u.EquipmentInitialized, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u.CreatedAt, err = parseTime(created)
	return u, err
}

// UpsertOIDCUser returns the user identified by (issuer, subject), creating it on
// first login. Email and name are refreshed from the identity provider every time.
func (db *DB) UpsertOIDCUser(ctx context.Context, issuer, subject, email, name string) (User, error) {
	id, err := newID()
	if err != nil {
		return User{}, err
	}
	row := db.write.QueryRowContext(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_sub, email, name, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (oidc_issuer, oidc_sub) DO UPDATE SET email = excluded.email, name = excluded.name
		RETURNING `+userColumns,
		id, issuer, subject, email, name, formatTime(time.Now()))
	return scanUser(row)
}

func (db *DB) UserByID(ctx context.Context, id string) (User, error) {
	return scanUser(db.read.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// UpdateUserSettings changes the display unit and the e1RM look-back window.
func (db *DB) UpdateUserSettings(ctx context.Context, userID, unit string, e1rmWindowDays int) error {
	res, err := db.write.ExecContext(ctx, `UPDATE users SET unit = ?, e1rm_window_days = ? WHERE id = ?`,
		unit, e1rmWindowDays, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}
```

Replace `internal/store/auth_sessions.go` with:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type AuthSession struct {
	IDHash    string
	UserID    string
	CSRFToken string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (db *DB) CreateAuthSession(ctx context.Context, s AuthSession) error {
	_, err := db.write.ExecContext(ctx, `
		INSERT INTO auth_sessions (id_hash, user_id, csrf_token, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		s.IDHash, s.UserID, s.CSRFToken, formatTime(s.ExpiresAt), formatTime(s.CreatedAt))
	return err
}

// AuthSessionByHash returns the session and its user. Expired sessions are
// reported as ErrNotFound.
func (db *DB) AuthSessionByHash(ctx context.Context, idHash string, now time.Time) (AuthSession, User, error) {
	row := db.read.QueryRowContext(ctx, `
		SELECT s.id_hash, s.user_id, s.csrf_token, s.expires_at, s.created_at,
		       u.id, u.oidc_issuer, u.oidc_sub, u.email, u.name, u.unit, u.e1rm_window_days, u.equipment_initialized, u.created_at
		FROM auth_sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id_hash = ? AND s.expires_at > ?`, idHash, formatTime(now))
	var s AuthSession
	var u User
	var sExp, sCreated, uCreated string
	err := row.Scan(&s.IDHash, &s.UserID, &s.CSRFToken, &sExp, &sCreated,
		&u.ID, &u.OIDCIssuer, &u.OIDCSubject, &u.Email, &u.Name, &u.Unit, &u.E1RMWindowDays, &u.EquipmentInitialized, &uCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, User{}, ErrNotFound
	}
	if err != nil {
		return AuthSession{}, User{}, err
	}
	if s.ExpiresAt, err = parseTime(sExp); err != nil {
		return AuthSession{}, User{}, err
	}
	if s.CreatedAt, err = parseTime(sCreated); err != nil {
		return AuthSession{}, User{}, err
	}
	if u.CreatedAt, err = parseTime(uCreated); err != nil {
		return AuthSession{}, User{}, err
	}
	return s, u, nil
}

func (db *DB) ExtendAuthSession(ctx context.Context, idHash string, expiresAt time.Time) error {
	_, err := db.write.ExecContext(ctx, `UPDATE auth_sessions SET expires_at = ? WHERE id_hash = ?`,
		formatTime(expiresAt), idHash)
	return err
}

func (db *DB) DeleteAuthSession(ctx context.Context, idHash string) error {
	_, err := db.write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteExpiredAuthSessions removes sessions that expired before now.
func (db *DB) DeleteExpiredAuthSessions(ctx context.Context, now time.Time) (int64, error) {
	res, err := db.write.ExecContext(ctx, `DELETE FROM auth_sessions WHERE expires_at <= ?`, formatTime(now))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
```

- [ ] **Step 6: Implement the equipment store**

`internal/store/equipment.go`:
```go
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
)

// Equipment is a user's equipment profile.
type Equipment struct {
	ID        string
	UserID    string
	Name      string
	IsDefault bool // the default profile for Spec.Kind
	Spec      calc.Equipment
	CreatedAt time.Time
}

const equipmentColumns = `id, user_id, name, kind, unit, config, is_default, created_at`

func scanEquipment(row interface{ Scan(...any) error }) (Equipment, error) {
	var e Equipment
	var config, created string
	err := row.Scan(&e.ID, &e.UserID, &e.Name, &e.Spec.Kind, &e.Spec.Unit, &config, &e.IsDefault, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Equipment{}, ErrNotFound
	}
	if err != nil {
		return Equipment{}, err
	}
	if err := json.Unmarshal([]byte(config), &e.Spec.Config); err != nil {
		return Equipment{}, err
	}
	e.CreatedAt, err = parseTime(created)
	return e, err
}

func (db *DB) ListEquipment(ctx context.Context, userID string) ([]Equipment, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? ORDER BY kind, name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Equipment
	for rows.Next() {
		e, err := scanEquipment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (db *DB) EquipmentByID(ctx context.Context, userID, id string) (Equipment, error) {
	return scanEquipment(db.read.QueryRowContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? AND id = ?`, userID, id))
}

// DefaultEquipment returns the user's default profile for kind, or ErrNotFound.
func (db *DB) DefaultEquipment(ctx context.Context, userID, kind string) (Equipment, error) {
	return scanEquipment(db.read.QueryRowContext(ctx, `SELECT `+equipmentColumns+` FROM equipment
		WHERE user_id = ? AND kind = ? AND is_default = 1`, userID, kind))
}

// SaveEquipment inserts e (empty ID) or updates it. Marking a profile as
// default clears the flag on the user's other profiles of the same kind.
func (db *DB) SaveEquipment(ctx context.Context, e Equipment) (Equipment, error) {
	err := db.tx(ctx, func(tx *sql.Tx) error { return saveEquipment(ctx, tx, &e) })
	return e, err
}

func saveEquipment(ctx context.Context, tx *sql.Tx, e *Equipment) error {
	config, err := json.Marshal(e.Spec.Config)
	if err != nil {
		return err
	}
	if e.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE equipment SET is_default = 0
			WHERE user_id = ? AND kind = ? AND id != ?`, e.UserID, e.Spec.Kind, e.ID); err != nil {
			return err
		}
	}
	if e.ID != "" {
		res, err := tx.ExecContext(ctx, `UPDATE equipment SET name = ?, kind = ?, unit = ?, config = ?, is_default = ?
			WHERE id = ? AND user_id = ?`,
			e.Name, e.Spec.Kind, e.Spec.Unit, string(config), e.IsDefault, e.ID, e.UserID)
		if err != nil {
			return err
		}
		return mustAffect(res)
	}
	if e.ID, err = newID(); err != nil {
		return err
	}
	e.CreatedAt = time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO equipment (`+equipmentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.Name, e.Spec.Kind, e.Spec.Unit, string(config), e.IsDefault, formatTime(e.CreatedAt))
	return err
}

func (db *DB) DeleteEquipment(ctx context.Context, userID, id string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM equipment WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// InitStarterEquipment creates the given profiles unless the user's starter
// profiles were already created. It reports whether it created them; exactly
// one of several concurrent callers does.
func (db *DB) InitStarterEquipment(ctx context.Context, userID string, items []Equipment) (bool, error) {
	created := false
	err := db.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET equipment_initialized = 1
			WHERE id = ? AND equipment_initialized = 0`, userID)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n == 0 {
			return err
		}
		for _, e := range items {
			e.ID, e.UserID = "", userID
			if err := saveEquipment(ctx, tx, &e); err != nil {
				return err
			}
		}
		created = true
		return nil
	})
	return created, err
}
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/store/ ./internal/auth/ ./internal/web/`
Expected: all `ok`. The auth and web packages load users through the changed queries, so they must still pass.

- [ ] **Step 8: Commit**

```bash
git add internal/store
git commit -m "feat(store): add equipment profiles with one default per kind"
```

---

### Task 4: Exercise catalog store

**Files:**
- Create: `internal/store/exercises.go`
- Test: `internal/store/exercises_test.go`

**Interfaces:**
- Consumes: `tx`, `newID`, `jsonList`, `parseList`, `mustAffect`, `newUser` (Task 3)
- Produces:
  - `store.Exercise{ID, UserID, Slug, Name, Measurement, EquipmentKind string; PrimaryMuscles, SecondaryMuscles, Aliases []string; Hidden, Overrides bool; CreatedAt, UpdatedAt time.Time}` with `Custom() bool`
  - `(*DB).CatalogExercises(ctx, userID) ([]Exercise, error)`
  - `(*DB).ExerciseBySlug(ctx, userID, slug) (Exercise, error)`
  - `(*DB).SaveUserExercise(ctx, userID string, e Exercise) (Exercise, error)`
  - `(*DB).DeleteUserExercise(ctx, userID, slug) error`
  - `(*DB).SyncSeed(ctx, exercises []Exercise, alternatives map[string][]string) error`
  - `store.Alternative{Slug string; UserAdded bool}`, `(*DB).Alternatives(ctx, userID, slug) ([]Alternative, error)`, `AddAlternative(ctx, userID, slug, altSlug) error`, `RemoveAlternative(ctx, userID, slug, altSlug) error`

- [ ] **Step 1: Write the failing tests**

`internal/store/exercises_test.go`:
```go
package store

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func seedExercise(slug, name string) Exercise {
	return Exercise{Slug: slug, Name: name, Measurement: "weight_reps", EquipmentKind: "barbell",
		PrimaryMuscles: []string{"quads"}, SecondaryMuscles: []string{"glutes"}, Aliases: []string{"alias"}}
}

func slugs(es []Exercise) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Slug)
	}
	return out
}

func TestSyncSeedUpsertsAndHides(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Squat"), seedExercise("bench", "Bench")},
		map[string][]string{"bench": {"squat"}})
	if err != nil {
		t.Fatal(err)
	}
	cat, err := db.CatalogExercises(ctx, u.ID)
	if err != nil || !slices.Equal(slugs(cat), []string{"bench", "squat"}) {
		t.Fatalf("catalog = %v, %v", slugs(cat), err)
	}
	if got := cat[1]; got.Custom() || got.Overrides || !slices.Equal(got.SecondaryMuscles, []string{"glutes"}) {
		t.Fatalf("global exercise fields: %+v", got)
	}

	// Second sync: squat renamed, bench dropped from the seed.
	if err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Back Squat")}, nil); err != nil {
		t.Fatal(err)
	}
	cat, _ = db.CatalogExercises(ctx, u.ID)
	if !slices.Equal(slugs(cat), []string{"squat"}) || cat[0].Name != "Back Squat" {
		t.Fatalf("after resync: %+v", cat)
	}
	bench, err := db.ExerciseBySlug(ctx, u.ID, "bench")
	if err != nil || !bench.Hidden {
		t.Fatalf("dropped seed exercise must stay reachable but hidden: %+v, %v", bench, err)
	}
	if alts, _ := db.Alternatives(ctx, u.ID, "bench"); len(alts) != 0 {
		t.Fatalf("global alternatives not replaced: %+v", alts)
	}
}

func TestUserExerciseShadowsGlobal(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	if err := db.SyncSeed(ctx, []Exercise{seedExercise("squat", "Squat")}, nil); err != nil {
		t.Fatal(err)
	}

	custom := seedExercise("squat", "My Squat")
	custom.EquipmentKind = "machine"
	if _, err := db.SaveUserExercise(ctx, alice.ID, custom); err != nil {
		t.Fatal(err)
	}
	own := seedExercise("zercher", "Zercher Squat")
	if _, err := db.SaveUserExercise(ctx, alice.ID, own); err != nil {
		t.Fatal(err)
	}

	got, err := db.ExerciseBySlug(ctx, alice.ID, "squat")
	if err != nil || got.Name != "My Squat" || !got.Custom() || !got.Overrides {
		t.Fatalf("alice squat = %+v, %v", got, err)
	}
	cat, _ := db.CatalogExercises(ctx, alice.ID)
	if !slices.Equal(slugs(cat), []string{"squat", "zercher"}) || cat[0].Name != "My Squat" {
		t.Fatalf("alice catalog = %+v", cat)
	}
	if z := cat[1]; !z.Custom() || z.Overrides {
		t.Fatalf("own exercise flags: %+v", z)
	}

	if got, _ := db.ExerciseBySlug(ctx, bob.ID, "squat"); got.Name != "Squat" {
		t.Fatalf("bob must see the global squat, got %q", got.Name)
	}
	if _, err := db.ExerciseBySlug(ctx, bob.ID, "zercher"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob must not see alice's exercise, got %v", err)
	}

	// Saving again updates in place.
	custom.Name = "My Squat v2"
	if _, err := db.SaveUserExercise(ctx, alice.ID, custom); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ExerciseBySlug(ctx, alice.ID, "squat"); got.Name != "My Squat v2" {
		t.Fatalf("update: %q", got.Name)
	}

	// Deleting the override restores the global exercise.
	if err := db.DeleteUserExercise(ctx, alice.ID, "squat"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.ExerciseBySlug(ctx, alice.ID, "squat"); got.Name != "Squat" || got.Custom() {
		t.Fatalf("after reset: %+v", got)
	}
	if err := db.DeleteUserExercise(ctx, alice.ID, "squat"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting a global exercise must be ErrNotFound, got %v", err)
	}
}

func TestAlternatives(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	if err := db.SyncSeed(ctx, nil, map[string][]string{"bench": {"db-bench"}}); err != nil {
		t.Fatal(err)
	}
	for range 2 { // adding twice is harmless
		if err := db.AddAlternative(ctx, alice.ID, "bench", "pushup"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddAlternative(ctx, alice.ID, "bench", "db-bench"); err != nil {
		t.Fatal(err)
	}

	alts, err := db.Alternatives(ctx, alice.ID, "bench")
	want := []Alternative{{"db-bench", true}, {"pushup", true}}
	if err != nil || !slices.Equal(alts, want) {
		t.Fatalf("alice alternatives = %+v, %v; want %+v", alts, err, want)
	}
	if alts, _ := db.Alternatives(ctx, bob.ID, "bench"); !slices.Equal(alts, []Alternative{{"db-bench", false}}) {
		t.Fatalf("bob alternatives = %+v", alts)
	}

	if err := db.RemoveAlternative(ctx, alice.ID, "bench", "db-bench"); err != nil {
		t.Fatal(err)
	}
	if alts, _ := db.Alternatives(ctx, alice.ID, "bench"); !slices.Equal(alts, []Alternative{{"db-bench", false}, {"pushup", true}}) {
		t.Fatalf("seeded alternative must survive removing the user's copy: %+v", alts)
	}
	if err := db.RemoveAlternative(ctx, alice.ID, "bench", "db-bench"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing a seeded alternative must be ErrNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL, `undefined: Exercise`.

- [ ] **Step 3: Implement**

`internal/store/exercises.go`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Exercise is a catalog entry. Global (seeded) exercises have an empty UserID.
type Exercise struct {
	ID               string
	UserID           string
	Slug             string
	Name             string
	Measurement      string
	EquipmentKind    string
	PrimaryMuscles   []string
	SecondaryMuscles []string
	Aliases          []string
	Hidden           bool
	// Overrides is true for a user row that shadows a global exercise.
	Overrides bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Custom reports whether the exercise belongs to a user (created or customized).
func (e Exercise) Custom() bool { return e.UserID != "" }

const exerciseColumns = `e.id, coalesce(e.user_id, ''), e.slug, e.name, e.measurement, e.equipment_kind,
	e.primary_muscles, e.secondary_muscles, e.aliases, e.hidden,
	e.user_id IS NOT NULL AND EXISTS (SELECT 1 FROM exercises g WHERE g.user_id IS NULL AND g.slug = e.slug),
	e.created_at, e.updated_at`

func scanExercise(row interface{ Scan(...any) error }) (Exercise, error) {
	var e Exercise
	var primary, secondary, aliases, created, updated string
	err := row.Scan(&e.ID, &e.UserID, &e.Slug, &e.Name, &e.Measurement, &e.EquipmentKind,
		&primary, &secondary, &aliases, &e.Hidden, &e.Overrides, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Exercise{}, ErrNotFound
	}
	if err != nil {
		return Exercise{}, err
	}
	if e.PrimaryMuscles, err = parseList(primary); err != nil {
		return Exercise{}, err
	}
	if e.SecondaryMuscles, err = parseList(secondary); err != nil {
		return Exercise{}, err
	}
	if e.Aliases, err = parseList(aliases); err != nil {
		return Exercise{}, err
	}
	if e.CreatedAt, err = parseTime(created); err != nil {
		return Exercise{}, err
	}
	e.UpdatedAt, err = parseTime(updated)
	return e, err
}

func scanExercises(rows *sql.Rows, err error) ([]Exercise, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Exercise
	for rows.Next() {
		e, err := scanExercise(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CatalogExercises returns the user's effective catalog: their own exercises
// plus visible global ones they have not customized, ordered by name.
func (db *DB) CatalogExercises(ctx context.Context, userID string) ([]Exercise, error) {
	return scanExercises(db.read.QueryContext(ctx, `SELECT `+exerciseColumns+` FROM exercises e
		WHERE e.user_id = ?
		   OR (e.user_id IS NULL AND e.hidden = 0 AND NOT EXISTS (
		       SELECT 1 FROM exercises u WHERE u.user_id = ? AND u.slug = e.slug))
		ORDER BY e.name COLLATE NOCASE`, userID, userID))
}

// ExerciseBySlug returns the user's own exercise with slug, else the global
// one (hidden global exercises included, since history may refer to them).
func (db *DB) ExerciseBySlug(ctx context.Context, userID, slug string) (Exercise, error) {
	return scanExercise(db.read.QueryRowContext(ctx, `SELECT `+exerciseColumns+` FROM exercises e
		WHERE e.slug = ? AND (e.user_id = ? OR e.user_id IS NULL)
		ORDER BY e.user_id IS NULL LIMIT 1`, slug, userID))
}

// SaveUserExercise creates or replaces the user's own exercise with e.Slug.
func (db *DB) SaveUserExercise(ctx context.Context, userID string, e Exercise) (Exercise, error) {
	id, err := newID()
	if err != nil {
		return Exercise{}, err
	}
	now := formatTime(time.Now())
	_, err = db.write.ExecContext(ctx, `
		INSERT INTO exercises (id, user_id, slug, name, measurement, equipment_kind,
			primary_muscles, secondary_muscles, aliases, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id, slug) WHERE user_id IS NOT NULL DO UPDATE SET
			name = excluded.name, measurement = excluded.measurement, equipment_kind = excluded.equipment_kind,
			primary_muscles = excluded.primary_muscles, secondary_muscles = excluded.secondary_muscles,
			aliases = excluded.aliases, updated_at = excluded.updated_at`,
		id, userID, e.Slug, e.Name, e.Measurement, e.EquipmentKind,
		jsonList(e.PrimaryMuscles), jsonList(e.SecondaryMuscles), jsonList(e.Aliases), now, now)
	if err != nil {
		return Exercise{}, err
	}
	return db.ExerciseBySlug(ctx, userID, e.Slug)
}

// DeleteUserExercise deletes the user's own exercise with slug. For a
// customized global exercise this restores the global version.
func (db *DB) DeleteUserExercise(ctx context.Context, userID, slug string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM exercises WHERE user_id = ? AND slug = ?`, userID, slug)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// SyncSeed makes the global catalog match the seed: exercises are upserted by
// slug, global exercises missing from the seed are hidden (never deleted), and
// global alternatives are replaced.
func (db *DB) SyncSeed(ctx context.Context, exercises []Exercise, alternatives map[string][]string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		now := formatTime(time.Now())
		if _, err := tx.ExecContext(ctx, `UPDATE exercises SET hidden = 1 WHERE user_id IS NULL`); err != nil {
			return err
		}
		for _, e := range exercises {
			id, err := newID()
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `
				INSERT INTO exercises (id, user_id, slug, name, measurement, equipment_kind,
					primary_muscles, secondary_muscles, aliases, hidden, created_at, updated_at)
				VALUES (?, NULL, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
				ON CONFLICT (slug) WHERE user_id IS NULL DO UPDATE SET
					name = excluded.name, measurement = excluded.measurement, equipment_kind = excluded.equipment_kind,
					primary_muscles = excluded.primary_muscles, secondary_muscles = excluded.secondary_muscles,
					aliases = excluded.aliases, hidden = 0, updated_at = excluded.updated_at`,
				id, e.Slug, e.Name, e.Measurement, e.EquipmentKind,
				jsonList(e.PrimaryMuscles), jsonList(e.SecondaryMuscles), jsonList(e.Aliases), now, now)
			if err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM exercise_alternatives WHERE user_id IS NULL`); err != nil {
			return err
		}
		for slug, alts := range alternatives {
			for _, alt := range alts {
				if _, err := tx.ExecContext(ctx, `INSERT INTO exercise_alternatives (user_id, slug, alt_slug)
					VALUES (NULL, ?, ?) ON CONFLICT DO NOTHING`, slug, alt); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Alternative is an exercise that can replace another one.
type Alternative struct {
	Slug      string
	UserAdded bool // false for alternatives from the seed
}

// Alternatives lists alternatives for slug: the seed's plus the user's own.
func (db *DB) Alternatives(ctx context.Context, userID, slug string) ([]Alternative, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT alt_slug, max(user_id IS NOT NULL) FROM exercise_alternatives
		WHERE slug = ? AND (user_id IS NULL OR user_id = ?)
		GROUP BY alt_slug ORDER BY alt_slug`, slug, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alternative
	for rows.Next() {
		var a Alternative
		if err := rows.Scan(&a.Slug, &a.UserAdded); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (db *DB) AddAlternative(ctx context.Context, userID, slug, altSlug string) error {
	_, err := db.write.ExecContext(ctx, `INSERT INTO exercise_alternatives (user_id, slug, alt_slug)
		VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, userID, slug, altSlug)
	return err
}

// RemoveAlternative removes an alternative the user added (seeded ones stay).
func (db *DB) RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM exercise_alternatives
		WHERE user_id = ? AND slug = ? AND alt_slug = ?`, userID, slug, altSlug)
	if err != nil {
		return err
	}
	return mustAffect(res)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/`
Expected: `ok  github.com/LongerHV/onerep/internal/store`

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "feat(store): add exercise catalog with user overrides, seed sync and alternatives"
```

---

### Task 5: Per-exercise settings and training-max log

**Files:**
- Create: `internal/store/user_exercise.go`
- Test: `internal/store/user_exercise_test.go`

**Interfaces:**
- Consumes: Task 3 helpers and `barbell`
- Produces:
  - `store.UserExercise{Slug, EquipmentID string; TrainingMaxKg *float64}`
  - `store.TrainingMaxChange{ID, Slug string; OldKg, NewKg *float64; Source, Note string; CreatedAt time.Time}`
  - `(*DB).UserExercise(ctx, userID, slug) (UserExercise, error)` (returns zero settings if none exist)
  - `(*DB).SetExerciseEquipment(ctx, userID, slug, equipmentID string) error` (another user's profile → `ErrNotFound`)
  - `(*DB).SetTrainingMax(ctx, userID, slug string, newKg *float64, source, note string) error`
  - `(*DB).TrainingMaxHistory(ctx, userID, slug) ([]TrainingMaxChange, error)` (newest first)

- [ ] **Step 1: Write the failing tests**

`internal/store/user_exercise_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
)

func kg(v float64) *float64 { return &v }

func TestTrainingMaxHistory(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")

	if ue, err := db.UserExercise(ctx, u.ID, "squat"); err != nil || ue.TrainingMaxKg != nil || ue.EquipmentID != "" {
		t.Fatalf("empty settings = %+v, %v", ue, err)
	}
	for _, step := range []struct {
		kg     *float64
		source string
	}{{kg(140), "web"}, {kg(140), "web"}, {kg(145), "mcp"}, {nil, "web"}} {
		if err := db.SetTrainingMax(ctx, u.ID, "squat", step.kg, step.source, "note"); err != nil {
			t.Fatal(err)
		}
	}
	if ue, _ := db.UserExercise(ctx, u.ID, "squat"); ue.TrainingMaxKg != nil {
		t.Fatalf("cleared TM still set: %v", *ue.TrainingMaxKg)
	}

	hist, err := db.TrainingMaxHistory(ctx, u.ID, "squat")
	if err != nil {
		t.Fatal(err)
	}
	// Newest first; setting 140 twice records one change.
	if len(hist) != 3 {
		t.Fatalf("history has %d entries, want 3: %+v", len(hist), hist)
	}
	if h := hist[0]; h.NewKg != nil || *h.OldKg != 145 || h.Source != "web" {
		t.Fatalf("clear entry: %+v", h)
	}
	if h := hist[1]; *h.OldKg != 140 || *h.NewKg != 145 || h.Source != "mcp" {
		t.Fatalf("mcp entry: %+v", h)
	}
	if h := hist[2]; h.OldKg != nil || *h.NewKg != 140 {
		t.Fatalf("first entry: %+v", h)
	}
}

func TestSetExerciseEquipment(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	bar, err := db.SaveEquipment(ctx, barbell(alice.ID, "bar", false))
	if err != nil {
		t.Fatal(err)
	}

	if err := db.SetTrainingMax(ctx, alice.ID, "squat", kg(100), "web", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetExerciseEquipment(ctx, alice.ID, "squat", bar.ID); err != nil {
		t.Fatal(err)
	}
	ue, _ := db.UserExercise(ctx, alice.ID, "squat")
	if ue.EquipmentID != bar.ID || *ue.TrainingMaxKg != 100 {
		t.Fatalf("settings = %+v (linking equipment must keep the TM)", ue)
	}

	if err := db.SetExerciseEquipment(ctx, bob.ID, "squat", bar.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("linking another user's equipment must be ErrNotFound, got %v", err)
	}

	// Deleting the profile unlinks it.
	if err := db.DeleteEquipment(ctx, alice.ID, bar.ID); err != nil {
		t.Fatal(err)
	}
	if ue, _ := db.UserExercise(ctx, alice.ID, "squat"); ue.EquipmentID != "" {
		t.Fatalf("deleted equipment still linked: %+v", ue)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL, `db.UserExercise undefined`.

- [ ] **Step 3: Implement**

`internal/store/user_exercise.go`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// UserExercise is a user's settings for one exercise.
type UserExercise struct {
	Slug          string
	EquipmentID   string   // "" = use the default profile for the exercise's kind
	TrainingMaxKg *float64 // nil = not set
}

// TrainingMaxChange is one entry of an exercise's training max history.
type TrainingMaxChange struct {
	ID        string
	Slug      string
	OldKg     *float64
	NewKg     *float64
	Source    string // "web" or "mcp"
	Note      string
	CreatedAt time.Time
}

// UserExercise returns the user's settings for slug (zero settings if none).
func (db *DB) UserExercise(ctx context.Context, userID, slug string) (UserExercise, error) {
	ue := UserExercise{Slug: slug}
	var equipmentID sql.NullString
	var tm sql.NullFloat64
	err := db.read.QueryRowContext(ctx, `SELECT equipment_id, training_max_kg FROM user_exercise
		WHERE user_id = ? AND slug = ?`, userID, slug).Scan(&equipmentID, &tm)
	if errors.Is(err, sql.ErrNoRows) {
		return ue, nil
	}
	if err != nil {
		return UserExercise{}, err
	}
	ue.EquipmentID = equipmentID.String
	if tm.Valid {
		ue.TrainingMaxKg = &tm.Float64
	}
	return ue, nil
}

// SetExerciseEquipment links slug to one of the user's equipment profiles
// ("" removes the link). A profile of another user is ErrNotFound.
func (db *DB) SetExerciseEquipment(ctx context.Context, userID, slug, equipmentID string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		if equipmentID != "" {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM equipment WHERE id = ? AND user_id = ?`,
				equipmentID, userID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			if err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO user_exercise (user_id, slug, equipment_id, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, slug) DO UPDATE SET equipment_id = excluded.equipment_id, updated_at = excluded.updated_at`,
			userID, slug, nullString(equipmentID), formatTime(time.Now()))
		return err
	})
}

// SetTrainingMax sets (or with nil clears) the training max for slug and
// records the change. Setting the current value again records nothing.
func (db *DB) SetTrainingMax(ctx context.Context, userID, slug string, newKg *float64, source, note string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		var old sql.NullFloat64
		err := tx.QueryRowContext(ctx, `SELECT training_max_kg FROM user_exercise WHERE user_id = ? AND slug = ?`,
			userID, slug).Scan(&old)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var oldKg *float64
		if old.Valid {
			oldKg = &old.Float64
		}
		if (oldKg == nil && newKg == nil) || (oldKg != nil && newKg != nil && *oldKg == *newKg) {
			return nil
		}
		now := formatTime(time.Now())
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_exercise (user_id, slug, training_max_kg, updated_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, slug) DO UPDATE SET training_max_kg = excluded.training_max_kg, updated_at = excluded.updated_at`,
			userID, slug, newKg, now); err != nil {
			return err
		}
		id, err := newID()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO training_max_log (id, user_id, slug, old_kg, new_kg, source, note, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, userID, slug, oldKg, newKg, source, note, now)
		return err
	})
}

// TrainingMaxHistory lists training max changes for slug, newest first.
func (db *DB) TrainingMaxHistory(ctx context.Context, userID, slug string) ([]TrainingMaxChange, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT id, slug, old_kg, new_kg, source, note, created_at
		FROM training_max_log WHERE user_id = ? AND slug = ? ORDER BY created_at DESC, id DESC`, userID, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TrainingMaxChange
	for rows.Next() {
		var c TrainingMaxChange
		var oldKg, newKg sql.NullFloat64
		var created string
		if err := rows.Scan(&c.ID, &c.Slug, &oldKg, &newKg, &c.Source, &c.Note, &created); err != nil {
			return nil, err
		}
		if oldKg.Valid {
			c.OldKg = &oldKg.Float64
		}
		if newKg.Valid {
			c.NewKg = &newKg.Float64
		}
		if c.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/store/ && golangci-lint run ./internal/...`
Expected: `ok`, then `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/store
git commit -m "feat(store): add per-exercise equipment link and training max history"
```

---

### Task 6: Exercise vocabulary and weight-list parsing

**Files:**
- Create: `internal/exercise/vocab.go`, `internal/exercise/errors.go`, `internal/exercise/weights.go`
- Test: `internal/exercise/weights_test.go`

**Interfaces:**
- Consumes: nothing
- Produces: `exercise.Muscles`, `exercise.Measurements` (and unexported `isMuscle`, `isMeasurement`); `exercise.FieldErrors map[string]string` (an error type); `exercise.ErrNoTrainingMax`; `ParseWeights(string) ([]float64, error)`, `FormatWeights([]float64) string`, `FormatNumber(float64) string`, `ParsePlatePairs(string) (map[string]int, error)`, `FormatPlatePairs(map[string]int) string`

- [ ] **Step 1: Write the failing tests**

`internal/exercise/weights_test.go`:
```go
package exercise

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestParseWeights(t *testing.T) {
	cases := map[string][]float64{
		"2-10/2, 12.5":     {2, 4, 6, 8, 10, 12.5},
		"25, 20, 20, 1.25": {1.25, 20, 25},
		"1.25-5/1.25":      {1.25, 2.5, 3.75, 5},
		" ":                nil,
		"5-6/2":            {5},
	}
	for in, want := range cases {
		got, err := ParseWeights(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("ParseWeights(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"abc", "-5", "0", "2-10", "10-2/2", "2-10/0", "1-100000/0.01"} {
		if _, err := ParseWeights(bad); err == nil {
			t.Errorf("ParseWeights(%q) should fail", bad)
		}
	}
}

func TestFormatWeightsRoundTrips(t *testing.T) {
	for _, in := range []string{"2-50/2", "1.25, 2.5, 5, 10, 15, 20, 25", "5-100/5, 110, 120.5"} {
		vs, err := ParseWeights(in)
		if err != nil {
			t.Fatal(err)
		}
		again, err := ParseWeights(FormatWeights(vs))
		if err != nil || !slices.Equal(again, vs) {
			t.Errorf("%q: round trip via %q gave %v", in, FormatWeights(vs), again)
		}
	}
	if got := FormatWeights([]float64{2, 4, 6, 8}); got != "2-8/2" {
		t.Errorf("FormatWeights = %q, want 2-8/2", got)
	}
}

func TestPlatePairs(t *testing.T) {
	got, err := ParsePlatePairs("1.25:2, 2.50:4")
	if err != nil || !maps.Equal(got, map[string]int{"1.25": 2, "2.5": 4}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if FormatPlatePairs(got) != "1.25:2, 2.5:4" {
		t.Fatalf("format: %q", FormatPlatePairs(got))
	}
	if got, err := ParsePlatePairs(""); err != nil || got != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	if _, err := ParsePlatePairs("1.25"); err == nil || !strings.Contains(err.Error(), "1.25:2") {
		t.Fatalf("missing count should explain the format, got %v", err)
	}
}

func TestFormatWeightsSortsInput(t *testing.T) {
	in := []float64{25, 20, 15, 10, 5, 2.5, 1.25}
	if got := FormatWeights(in); got != "1.25, 2.5, 5-25/5" {
		t.Fatalf("FormatWeights(descending) = %q", got)
	}
	if in[0] != 25 {
		t.Fatal("FormatWeights must not reorder its argument")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/exercise/`
Expected: FAIL, `undefined: ParseWeights`.

- [ ] **Step 3: Implement**

`internal/exercise/vocab.go`:
```go
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
```

`internal/exercise/errors.go`:
```go
package exercise

import (
	"errors"
	"sort"
	"strings"
)

// FieldErrors maps form field names to messages. It is returned for invalid input.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	var parts []string
	for k, v := range f {
		parts = append(parts, k+": "+v)
	}
	sort.Strings(parts)
	return "invalid input: " + strings.Join(parts, "; ")
}

// ErrNoTrainingMax is returned when a calculation needs a training max that is not set.
var ErrNoTrainingMax = errors.New("no training max set")
```

`internal/exercise/weights.go`:
```go
package exercise

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ParseWeights parses a comma-separated weight list where "a-b/s" expands to
// a, a+s, ... up to b. "2-10/2, 12.5" gives [2 4 6 8 10 12.5]. The result is
// sorted and de-duplicated.
func ParseWeights(s string) ([]float64, error) {
	var out []float64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, rest, ok := strings.Cut(part, "-"); ok && lo != "" {
			hi, step, ok := strings.Cut(rest, "/")
			if !ok {
				return nil, fmt.Errorf("%q: a range needs a step, like 2-50/2", part)
			}
			a, err1 := parsePositive(lo)
			b, err2 := parsePositive(hi)
			st, err3 := parsePositive(step)
			if err1 != nil || err2 != nil || err3 != nil || b < a {
				return nil, fmt.Errorf("%q: not a valid range", part)
			}
			if (b-a)/st > 1000 {
				return nil, fmt.Errorf("%q: range has too many values", part)
			}
			for i := 0; ; i++ {
				v := math.Round((a+float64(i)*st)*100) / 100
				if v > b+1e-9 {
					break
				}
				out = append(out, v)
			}
			continue
		}
		v, err := parsePositive(part)
		if err != nil {
			return nil, fmt.Errorf("%q: not a positive number", part)
		}
		out = append(out, v)
	}
	sort.Float64s(out)
	dedup := out[:0]
	for i, v := range out {
		if i == 0 || v != out[i-1] {
			dedup = append(dedup, v)
		}
	}
	return dedup, nil
}

func parsePositive(s string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 || math.IsInf(v, 0) {
		return 0, fmt.Errorf("not a positive number: %q", s)
	}
	return v, nil
}

// FormatWeights is the inverse of ParseWeights: values are sorted and runs of
// three or more equally spaced values are written as ranges.
func FormatWeights(vs []float64) string {
	vs = slices.Sorted(slices.Values(vs))
	var parts []string
	for i := 0; i < len(vs); {
		j := i + 1
		if j < len(vs) {
			step := vs[j] - vs[i]
			for j+1 < len(vs) && math.Abs(vs[j+1]-vs[j]-step) < 1e-9 {
				j++
			}
			if j-i >= 2 {
				parts = append(parts, FormatNumber(vs[i])+"-"+FormatNumber(vs[j])+"/"+FormatNumber(step))
				i = j + 1
				continue
			}
		}
		parts = append(parts, FormatNumber(vs[i]))
		i++
	}
	return strings.Join(parts, ", ")
}

// FormatNumber prints v with at most two decimals and no trailing zeros.
func FormatNumber(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

// ParsePlatePairs parses "1.25:2, 2.5:4" (plate size: number of pairs).
func ParsePlatePairs(s string) (map[string]int, error) {
	out := map[string]int{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		size, count, ok := strings.Cut(part, ":")
		v, err1 := parsePositive(size)
		n, err2 := strconv.Atoi(strings.TrimSpace(count))
		if !ok || err1 != nil || err2 != nil || n < 0 {
			return nil, fmt.Errorf("%q: write plate size and number of pairs, like 1.25:2", part)
		}
		out[FormatNumber(v)] = n
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// FormatPlatePairs is the inverse of ParsePlatePairs.
func FormatPlatePairs(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, _ := strconv.ParseFloat(keys[i], 64)
		b, _ := strconv.ParseFloat(keys[j], 64)
		return a < b
	})
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/exercise/`
Expected: `ok  github.com/LongerHV/onerep/internal/exercise`

- [ ] **Step 5: Commit**

```bash
git add internal/exercise
git commit -m "feat(exercise): add muscle vocabulary and weight list parsing"
```

---

### Task 7: Seed catalog and exercise service

**Files:**
- Create: `internal/exercise/seed/exercises.json`, `internal/exercise/seed.go`, `internal/exercise/service.go`
- Test: `internal/exercise/seed_test.go`, `internal/exercise/service_test.go`

**Interfaces:**
- Consumes: store types and methods from Tasks 3–5; `calc` (Task 1); Task 6
- Produces:
  - `exercise.Seed(ctx, exercise.SeedStore) error` (`SeedStore` = `SyncSeed`)
  - `exercise.Store` interface (every store method above except SyncSeed)
  - `exercise.Service{Store Store}`
  - `exercise.Input{Slug, Name, Measurement, EquipmentKind string; PrimaryMuscles, SecondaryMuscles, Aliases []string}`
  - Methods:
    - Catalog and exercises: `Catalog(ctx, userID, query) ([]store.Exercise, error)`, `Get(ctx, userID, slug)`, `Create(ctx, userID, Input)`, `Update(ctx, userID, Input)`, `Delete(ctx, userID, slug)`
    - Alternatives: `Alternatives(ctx, userID, slug) ([]AlternativeView, error)`, `AddAlternative`, `RemoveAlternative`
    - Per-exercise settings: `Settings(ctx, userID, store.Exercise) (Settings, error)`, `LinkEquipment(ctx, userID, slug, equipmentID)`, `SetTrainingMax(ctx, userID, slug, *float64, source)`, `TrainingMaxHistory`, `PercentOfTM(ctx, store.User, store.Exercise, pct) (calc.Rounded, error)`
    - Equipment: `ListEquipment`, `GetEquipment`, `SaveEquipment(ctx, store.Equipment)`, `DeleteEquipment`, `EnsureStarterEquipment(ctx, store.User) error`
  - `exercise.Settings{store.UserExercise; Equipment *store.Equipment; Linked bool}`; `exercise.AlternativeView{Exercise store.Exercise; UserAdded bool}`; `exercise.StarterEquipment(unit) []store.Equipment`; `exercise.MaxTrainingMaxKg = 1500`

- [ ] **Step 1: Add the seed data**

`internal/exercise/seed/exercises.json` (116 exercises; review the list, since slugs become permanent once plans refer to them):
```json
[
  {"slug": "barbell-back-squat", "name": "Barbell Back Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["adductors", "lower-back"], "aliases": ["squat", "back squat", "high bar squat", "low bar squat"], "alternatives": ["barbell-front-squat", "hack-squat", "leg-press", "smith-machine-squat"]},
  {"slug": "barbell-front-squat", "name": "Barbell Front Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads"], "secondary": ["glutes", "upper-back", "abs"], "aliases": ["front squat"], "alternatives": ["barbell-back-squat", "goblet-squat", "hack-squat"]},
  {"slug": "barbell-box-squat", "name": "Barbell Box Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["hamstrings", "adductors"], "aliases": ["box squat"], "alternatives": ["barbell-back-squat"]},
  {"slug": "barbell-pause-squat", "name": "Barbell Pause Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["adductors", "lower-back"], "aliases": ["pause squat", "paused squat"], "alternatives": ["barbell-back-squat"]},
  {"slug": "smith-machine-squat", "name": "Smith Machine Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": ["smith squat"], "alternatives": ["barbell-back-squat", "hack-squat"]},
  {"slug": "goblet-squat", "name": "Goblet Squat", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["quads", "glutes"], "secondary": ["adductors", "abs"], "aliases": [], "alternatives": ["barbell-front-squat", "leg-press"]},
  {"slug": "hack-squat", "name": "Hack Squat", "measurement": "weight_reps", "equipment": "machine", "primary": ["quads"], "secondary": ["glutes"], "aliases": ["machine hack squat"], "alternatives": ["leg-press", "barbell-back-squat", "pendulum-squat"]},
  {"slug": "pendulum-squat", "name": "Pendulum Squat", "measurement": "weight_reps", "equipment": "machine", "primary": ["quads"], "secondary": ["glutes"], "aliases": [], "alternatives": ["hack-squat", "leg-press"]},
  {"slug": "leg-press", "name": "Leg Press", "measurement": "weight_reps", "equipment": "machine", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": ["45 degree leg press"], "alternatives": ["hack-squat", "barbell-back-squat"]},
  {"slug": "belt-squat", "name": "Belt Squat", "measurement": "weight_reps", "equipment": "machine", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": [], "alternatives": ["leg-press", "hack-squat"]},
  {"slug": "barbell-bulgarian-split-squat", "name": "Barbell Bulgarian Split Squat", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": ["barbell bss"], "alternatives": ["dumbbell-bulgarian-split-squat"]},
  {"slug": "dumbbell-bulgarian-split-squat", "name": "Dumbbell Bulgarian Split Squat", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": ["bulgarian split squat", "bss", "rear foot elevated split squat"], "alternatives": ["dumbbell-walking-lunge", "barbell-bulgarian-split-squat"]},
  {"slug": "dumbbell-walking-lunge", "name": "Dumbbell Walking Lunge", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["quads", "glutes"], "secondary": ["adductors", "hamstrings"], "aliases": ["walking lunge", "lunge"], "alternatives": ["dumbbell-bulgarian-split-squat", "dumbbell-step-up"]},
  {"slug": "barbell-reverse-lunge", "name": "Barbell Reverse Lunge", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["adductors"], "aliases": ["reverse lunge"], "alternatives": ["dumbbell-walking-lunge"]},
  {"slug": "dumbbell-step-up", "name": "Dumbbell Step-Up", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["quads", "glutes"], "secondary": [], "aliases": ["step up"], "alternatives": ["dumbbell-walking-lunge"]},
  {"slug": "leg-extension", "name": "Leg Extension", "measurement": "weight_reps", "equipment": "machine", "primary": ["quads"], "secondary": [], "aliases": ["quad extension"], "alternatives": ["sissy-squat"]},
  {"slug": "sissy-squat", "name": "Sissy Squat", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["quads"], "secondary": [], "aliases": [], "alternatives": ["leg-extension"]},

  {"slug": "barbell-deadlift", "name": "Barbell Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "hamstrings", "lower-back"], "secondary": ["quads", "traps", "upper-back", "forearms"], "aliases": ["deadlift", "conventional deadlift"], "alternatives": ["barbell-sumo-deadlift", "trap-bar-deadlift"]},
  {"slug": "barbell-sumo-deadlift", "name": "Barbell Sumo Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "adductors", "quads"], "secondary": ["hamstrings", "lower-back", "traps"], "aliases": ["sumo deadlift", "sumo"], "alternatives": ["barbell-deadlift", "trap-bar-deadlift"]},
  {"slug": "trap-bar-deadlift", "name": "Trap Bar Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["quads", "glutes"], "secondary": ["hamstrings", "lower-back", "traps"], "aliases": ["hex bar deadlift"], "alternatives": ["barbell-deadlift"]},
  {"slug": "barbell-deficit-deadlift", "name": "Barbell Deficit Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "hamstrings", "lower-back"], "secondary": ["quads", "traps"], "aliases": ["deficit deadlift"], "alternatives": ["barbell-deadlift"]},
  {"slug": "barbell-block-pull", "name": "Barbell Block Pull", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "lower-back"], "secondary": ["hamstrings", "traps", "upper-back"], "aliases": ["rack pull", "block pull"], "alternatives": ["barbell-deadlift"]},
  {"slug": "barbell-romanian-deadlift", "name": "Barbell Romanian Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["hamstrings", "glutes"], "secondary": ["lower-back"], "aliases": ["rdl", "romanian deadlift"], "alternatives": ["dumbbell-romanian-deadlift", "barbell-stiff-leg-deadlift", "barbell-good-morning"]},
  {"slug": "dumbbell-romanian-deadlift", "name": "Dumbbell Romanian Deadlift", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["hamstrings", "glutes"], "secondary": ["lower-back"], "aliases": ["db rdl"], "alternatives": ["barbell-romanian-deadlift"]},
  {"slug": "barbell-stiff-leg-deadlift", "name": "Barbell Stiff-Leg Deadlift", "measurement": "weight_reps", "equipment": "barbell", "primary": ["hamstrings", "glutes"], "secondary": ["lower-back"], "aliases": ["sldl", "stiff leg deadlift"], "alternatives": ["barbell-romanian-deadlift"]},
  {"slug": "barbell-good-morning", "name": "Barbell Good Morning", "measurement": "weight_reps", "equipment": "barbell", "primary": ["hamstrings", "lower-back"], "secondary": ["glutes"], "aliases": ["good morning"], "alternatives": ["barbell-romanian-deadlift", "back-extension"]},
  {"slug": "barbell-hip-thrust", "name": "Barbell Hip Thrust", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes"], "secondary": ["hamstrings"], "aliases": ["hip thrust"], "alternatives": ["machine-hip-thrust", "barbell-glute-bridge"]},
  {"slug": "machine-hip-thrust", "name": "Machine Hip Thrust", "measurement": "weight_reps", "equipment": "machine", "primary": ["glutes"], "secondary": ["hamstrings"], "aliases": [], "alternatives": ["barbell-hip-thrust"]},
  {"slug": "barbell-glute-bridge", "name": "Barbell Glute Bridge", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes"], "secondary": ["hamstrings"], "aliases": ["glute bridge"], "alternatives": ["barbell-hip-thrust"]},
  {"slug": "lying-leg-curl", "name": "Lying Leg Curl", "measurement": "weight_reps", "equipment": "machine", "primary": ["hamstrings"], "secondary": ["calves"], "aliases": ["prone leg curl"], "alternatives": ["seated-leg-curl", "nordic-hamstring-curl"]},
  {"slug": "seated-leg-curl", "name": "Seated Leg Curl", "measurement": "weight_reps", "equipment": "machine", "primary": ["hamstrings"], "secondary": [], "aliases": ["leg curl"], "alternatives": ["lying-leg-curl", "nordic-hamstring-curl"]},
  {"slug": "nordic-hamstring-curl", "name": "Nordic Hamstring Curl", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["hamstrings"], "secondary": [], "aliases": ["nordic curl", "nordics"], "alternatives": ["lying-leg-curl", "seated-leg-curl"]},
  {"slug": "back-extension", "name": "Back Extension", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["lower-back", "glutes"], "secondary": ["hamstrings"], "aliases": ["hyperextension", "45 degree back extension"], "alternatives": ["barbell-good-morning", "reverse-hyperextension"]},
  {"slug": "reverse-hyperextension", "name": "Reverse Hyperextension", "measurement": "weight_reps", "equipment": "machine", "primary": ["glutes", "lower-back"], "secondary": ["hamstrings"], "aliases": ["reverse hyper"], "alternatives": ["back-extension"]},
  {"slug": "hip-adduction-machine", "name": "Hip Adduction Machine", "measurement": "weight_reps", "equipment": "machine", "primary": ["adductors"], "secondary": [], "aliases": ["adductor machine"], "alternatives": ["cable-hip-adduction"]},
  {"slug": "cable-hip-adduction", "name": "Cable Hip Adduction", "measurement": "weight_reps", "equipment": "cable", "primary": ["adductors"], "secondary": [], "aliases": [], "alternatives": ["hip-adduction-machine"]},
  {"slug": "hip-abduction-machine", "name": "Hip Abduction Machine", "measurement": "weight_reps", "equipment": "machine", "primary": ["abductors", "glutes"], "secondary": [], "aliases": ["abductor machine"], "alternatives": ["cable-hip-abduction"]},
  {"slug": "cable-hip-abduction", "name": "Cable Hip Abduction", "measurement": "weight_reps", "equipment": "cable", "primary": ["abductors", "glutes"], "secondary": [], "aliases": [], "alternatives": ["hip-abduction-machine"]},
  {"slug": "cable-pull-through", "name": "Cable Pull-Through", "measurement": "weight_reps", "equipment": "cable", "primary": ["glutes", "hamstrings"], "secondary": [], "aliases": ["pull through"], "alternatives": ["barbell-romanian-deadlift", "barbell-hip-thrust"]},
  {"slug": "standing-calf-raise", "name": "Standing Calf Raise", "measurement": "weight_reps", "equipment": "machine", "primary": ["calves"], "secondary": [], "aliases": ["calf raise"], "alternatives": ["seated-calf-raise", "leg-press-calf-raise"]},
  {"slug": "seated-calf-raise", "name": "Seated Calf Raise", "measurement": "weight_reps", "equipment": "machine", "primary": ["calves"], "secondary": [], "aliases": [], "alternatives": ["standing-calf-raise"]},
  {"slug": "leg-press-calf-raise", "name": "Leg Press Calf Raise", "measurement": "weight_reps", "equipment": "machine", "primary": ["calves"], "secondary": [], "aliases": [], "alternatives": ["standing-calf-raise"]},

  {"slug": "barbell-bench-press", "name": "Barbell Bench Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["bench", "bench press", "flat bench"], "alternatives": ["dumbbell-bench-press", "smith-machine-bench-press", "machine-chest-press"]},
  {"slug": "barbell-pause-bench-press", "name": "Barbell Pause Bench Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["paused bench", "competition bench"], "alternatives": ["barbell-bench-press"]},
  {"slug": "barbell-close-grip-bench-press", "name": "Barbell Close-Grip Bench Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["triceps", "chest"], "secondary": ["front-delts"], "aliases": ["cgbp", "close grip bench"], "alternatives": ["barbell-bench-press", "dip"]},
  {"slug": "barbell-incline-bench-press", "name": "Barbell Incline Bench Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "front-delts"], "secondary": ["triceps"], "aliases": ["incline bench"], "alternatives": ["dumbbell-incline-bench-press", "smith-machine-incline-press"]},
  {"slug": "barbell-floor-press", "name": "Barbell Floor Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["triceps", "chest"], "secondary": ["front-delts"], "aliases": ["floor press"], "alternatives": ["barbell-close-grip-bench-press"]},
  {"slug": "barbell-larsen-press", "name": "Barbell Larsen Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["larsen press"], "alternatives": ["barbell-bench-press"]},
  {"slug": "smith-machine-bench-press", "name": "Smith Machine Bench Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["smith bench"], "alternatives": ["barbell-bench-press", "machine-chest-press"]},
  {"slug": "smith-machine-incline-press", "name": "Smith Machine Incline Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["chest", "front-delts"], "secondary": ["triceps"], "aliases": ["smith incline"], "alternatives": ["barbell-incline-bench-press", "dumbbell-incline-bench-press"]},
  {"slug": "dumbbell-bench-press", "name": "Dumbbell Bench Press", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["db bench"], "alternatives": ["barbell-bench-press", "machine-chest-press"]},
  {"slug": "dumbbell-incline-bench-press", "name": "Dumbbell Incline Bench Press", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["chest", "front-delts"], "secondary": ["triceps"], "aliases": ["incline db press"], "alternatives": ["barbell-incline-bench-press", "smith-machine-incline-press"]},
  {"slug": "machine-chest-press", "name": "Machine Chest Press", "measurement": "weight_reps", "equipment": "machine", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["chest press"], "alternatives": ["dumbbell-bench-press", "barbell-bench-press"]},
  {"slug": "dumbbell-fly", "name": "Dumbbell Fly", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["chest"], "secondary": ["front-delts"], "aliases": ["db flye"], "alternatives": ["cable-fly", "pec-deck"]},
  {"slug": "cable-fly", "name": "Cable Fly", "measurement": "weight_reps", "equipment": "cable", "primary": ["chest"], "secondary": ["front-delts"], "aliases": ["cable crossover", "cable flye"], "alternatives": ["pec-deck", "dumbbell-fly"]},
  {"slug": "pec-deck", "name": "Pec Deck", "measurement": "weight_reps", "equipment": "machine", "primary": ["chest"], "secondary": [], "aliases": ["machine fly"], "alternatives": ["cable-fly", "dumbbell-fly"]},
  {"slug": "push-up", "name": "Push-Up", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["chest", "triceps"], "secondary": ["front-delts", "abs"], "aliases": ["pushup", "press up"], "alternatives": ["dumbbell-bench-press", "dip"]},
  {"slug": "dip", "name": "Dip", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["chest", "triceps"], "secondary": ["front-delts"], "aliases": ["parallel bar dip", "weighted dip"], "alternatives": ["barbell-close-grip-bench-press", "push-up"]},

  {"slug": "barbell-overhead-press", "name": "Barbell Overhead Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["front-delts", "triceps"], "secondary": ["side-delts", "upper-back", "abs"], "aliases": ["ohp", "overhead press", "military press", "press"], "alternatives": ["dumbbell-shoulder-press", "machine-shoulder-press"]},
  {"slug": "barbell-push-press", "name": "Barbell Push Press", "measurement": "weight_reps", "equipment": "barbell", "primary": ["front-delts", "triceps"], "secondary": ["quads", "side-delts"], "aliases": ["push press"], "alternatives": ["barbell-overhead-press"]},
  {"slug": "dumbbell-shoulder-press", "name": "Dumbbell Shoulder Press", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["front-delts", "triceps"], "secondary": ["side-delts"], "aliases": ["db shoulder press", "seated dumbbell press"], "alternatives": ["barbell-overhead-press", "machine-shoulder-press"]},
  {"slug": "machine-shoulder-press", "name": "Machine Shoulder Press", "measurement": "weight_reps", "equipment": "machine", "primary": ["front-delts", "triceps"], "secondary": ["side-delts"], "aliases": ["shoulder press machine"], "alternatives": ["dumbbell-shoulder-press"]},
  {"slug": "dumbbell-lateral-raise", "name": "Dumbbell Lateral Raise", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["side-delts"], "secondary": [], "aliases": ["lateral raise", "side raise"], "alternatives": ["cable-lateral-raise", "machine-lateral-raise"]},
  {"slug": "cable-lateral-raise", "name": "Cable Lateral Raise", "measurement": "weight_reps", "equipment": "cable", "primary": ["side-delts"], "secondary": [], "aliases": [], "alternatives": ["dumbbell-lateral-raise", "machine-lateral-raise"]},
  {"slug": "machine-lateral-raise", "name": "Machine Lateral Raise", "measurement": "weight_reps", "equipment": "machine", "primary": ["side-delts"], "secondary": [], "aliases": [], "alternatives": ["dumbbell-lateral-raise", "cable-lateral-raise"]},
  {"slug": "dumbbell-front-raise", "name": "Dumbbell Front Raise", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["front-delts"], "secondary": [], "aliases": ["front raise"], "alternatives": []},
  {"slug": "dumbbell-rear-delt-fly", "name": "Dumbbell Rear Delt Fly", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["rear-delts"], "secondary": ["upper-back"], "aliases": ["reverse fly", "bent over fly"], "alternatives": ["reverse-pec-deck", "cable-face-pull"]},
  {"slug": "reverse-pec-deck", "name": "Reverse Pec Deck", "measurement": "weight_reps", "equipment": "machine", "primary": ["rear-delts"], "secondary": ["upper-back"], "aliases": ["rear delt machine"], "alternatives": ["dumbbell-rear-delt-fly", "cable-face-pull"]},
  {"slug": "cable-face-pull", "name": "Cable Face Pull", "measurement": "weight_reps", "equipment": "cable", "primary": ["rear-delts", "upper-back"], "secondary": ["traps"], "aliases": ["face pull"], "alternatives": ["reverse-pec-deck", "dumbbell-rear-delt-fly"]},
  {"slug": "barbell-upright-row", "name": "Barbell Upright Row", "measurement": "weight_reps", "equipment": "barbell", "primary": ["side-delts", "traps"], "secondary": ["biceps"], "aliases": ["upright row"], "alternatives": ["cable-lateral-raise"]},
  {"slug": "barbell-shrug", "name": "Barbell Shrug", "measurement": "weight_reps", "equipment": "barbell", "primary": ["traps"], "secondary": ["forearms"], "aliases": ["shrug"], "alternatives": ["dumbbell-shrug"]},
  {"slug": "dumbbell-shrug", "name": "Dumbbell Shrug", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["traps"], "secondary": ["forearms"], "aliases": ["db shrug"], "alternatives": ["barbell-shrug"]},

  {"slug": "pull-up", "name": "Pull-Up", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["lats"], "secondary": ["biceps", "upper-back", "rear-delts"], "aliases": ["pullup", "weighted pull-up"], "alternatives": ["chin-up", "lat-pulldown", "assisted-pull-up"]},
  {"slug": "chin-up", "name": "Chin-Up", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["lats", "biceps"], "secondary": ["upper-back"], "aliases": ["chinup"], "alternatives": ["pull-up", "lat-pulldown"]},
  {"slug": "assisted-pull-up", "name": "Assisted Pull-Up", "measurement": "weight_reps", "equipment": "machine", "primary": ["lats"], "secondary": ["biceps", "upper-back"], "aliases": ["assisted pullup machine"], "alternatives": ["pull-up", "lat-pulldown"]},
  {"slug": "lat-pulldown", "name": "Lat Pulldown", "measurement": "weight_reps", "equipment": "cable", "primary": ["lats"], "secondary": ["biceps", "upper-back"], "aliases": ["pulldown", "wide grip pulldown"], "alternatives": ["pull-up", "close-grip-lat-pulldown"]},
  {"slug": "close-grip-lat-pulldown", "name": "Close-Grip Lat Pulldown", "measurement": "weight_reps", "equipment": "cable", "primary": ["lats"], "secondary": ["biceps", "upper-back"], "aliases": ["v-bar pulldown"], "alternatives": ["lat-pulldown", "chin-up"]},
  {"slug": "straight-arm-pulldown", "name": "Straight-Arm Pulldown", "measurement": "weight_reps", "equipment": "cable", "primary": ["lats"], "secondary": [], "aliases": ["lat prayer", "straight arm pushdown"], "alternatives": ["dumbbell-pullover"]},
  {"slug": "dumbbell-pullover", "name": "Dumbbell Pullover", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["lats"], "secondary": ["chest"], "aliases": ["pullover"], "alternatives": ["straight-arm-pulldown"]},
  {"slug": "barbell-row", "name": "Barbell Row", "measurement": "weight_reps", "equipment": "barbell", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts", "lower-back"], "aliases": ["bent over row", "bb row"], "alternatives": ["pendlay-row", "dumbbell-row", "seated-cable-row", "t-bar-row"]},
  {"slug": "pendlay-row", "name": "Pendlay Row", "measurement": "weight_reps", "equipment": "barbell", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts", "lower-back"], "aliases": [], "alternatives": ["barbell-row"]},
  {"slug": "t-bar-row", "name": "T-Bar Row", "measurement": "weight_reps", "equipment": "barbell", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts"], "aliases": ["landmine row"], "alternatives": ["barbell-row", "chest-supported-row"]},
  {"slug": "dumbbell-row", "name": "Dumbbell Row", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["lats", "upper-back"], "secondary": ["biceps", "rear-delts"], "aliases": ["one arm row", "single arm dumbbell row"], "alternatives": ["barbell-row", "seated-cable-row"]},
  {"slug": "chest-supported-row", "name": "Chest-Supported Row", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["upper-back", "lats"], "secondary": ["rear-delts", "biceps"], "aliases": ["seal row", "incline dumbbell row"], "alternatives": ["machine-row", "t-bar-row"]},
  {"slug": "seated-cable-row", "name": "Seated Cable Row", "measurement": "weight_reps", "equipment": "cable", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts"], "aliases": ["cable row", "low row"], "alternatives": ["machine-row", "dumbbell-row"]},
  {"slug": "machine-row", "name": "Machine Row", "measurement": "weight_reps", "equipment": "machine", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts"], "aliases": ["hammer strength row"], "alternatives": ["seated-cable-row", "chest-supported-row"]},
  {"slug": "inverted-row", "name": "Inverted Row", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["upper-back", "lats"], "secondary": ["biceps", "rear-delts"], "aliases": ["body row", "australian pull-up"], "alternatives": ["seated-cable-row"]},

  {"slug": "barbell-curl", "name": "Barbell Curl", "measurement": "weight_reps", "equipment": "barbell", "primary": ["biceps"], "secondary": ["forearms"], "aliases": ["bb curl", "ez bar curl"], "alternatives": ["dumbbell-curl", "cable-curl"]},
  {"slug": "dumbbell-curl", "name": "Dumbbell Curl", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["biceps"], "secondary": ["forearms"], "aliases": ["db curl", "bicep curl"], "alternatives": ["barbell-curl", "cable-curl"]},
  {"slug": "dumbbell-hammer-curl", "name": "Dumbbell Hammer Curl", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["biceps", "forearms"], "secondary": [], "aliases": ["hammer curl"], "alternatives": ["dumbbell-curl"]},
  {"slug": "dumbbell-incline-curl", "name": "Dumbbell Incline Curl", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["biceps"], "secondary": [], "aliases": ["incline curl"], "alternatives": ["dumbbell-curl"]},
  {"slug": "preacher-curl", "name": "Preacher Curl", "measurement": "weight_reps", "equipment": "machine", "primary": ["biceps"], "secondary": [], "aliases": ["machine preacher curl", "scott curl"], "alternatives": ["barbell-curl", "cable-curl"]},
  {"slug": "cable-curl", "name": "Cable Curl", "measurement": "weight_reps", "equipment": "cable", "primary": ["biceps"], "secondary": ["forearms"], "aliases": [], "alternatives": ["dumbbell-curl", "barbell-curl"]},
  {"slug": "cable-triceps-pushdown", "name": "Cable Triceps Pushdown", "measurement": "weight_reps", "equipment": "cable", "primary": ["triceps"], "secondary": [], "aliases": ["pushdown", "rope pushdown", "tricep pushdown"], "alternatives": ["cable-overhead-triceps-extension", "barbell-skull-crusher"]},
  {"slug": "cable-overhead-triceps-extension", "name": "Cable Overhead Triceps Extension", "measurement": "weight_reps", "equipment": "cable", "primary": ["triceps"], "secondary": [], "aliases": ["overhead extension"], "alternatives": ["dumbbell-overhead-triceps-extension", "cable-triceps-pushdown"]},
  {"slug": "dumbbell-overhead-triceps-extension", "name": "Dumbbell Overhead Triceps Extension", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["triceps"], "secondary": [], "aliases": ["french press"], "alternatives": ["cable-overhead-triceps-extension"]},
  {"slug": "barbell-skull-crusher", "name": "Barbell Skull Crusher", "measurement": "weight_reps", "equipment": "barbell", "primary": ["triceps"], "secondary": [], "aliases": ["skull crusher", "lying triceps extension", "ez bar skull crusher"], "alternatives": ["cable-triceps-pushdown", "dumbbell-overhead-triceps-extension"]},
  {"slug": "barbell-wrist-curl", "name": "Barbell Wrist Curl", "measurement": "weight_reps", "equipment": "barbell", "primary": ["forearms"], "secondary": [], "aliases": ["wrist curl"], "alternatives": []},
  {"slug": "dead-hang", "name": "Dead Hang", "measurement": "time", "equipment": "bodyweight", "primary": ["forearms"], "secondary": ["lats"], "aliases": ["bar hang"], "alternatives": []},

  {"slug": "plank", "name": "Plank", "measurement": "time", "equipment": "bodyweight", "primary": ["abs"], "secondary": ["obliques"], "aliases": ["front plank"], "alternatives": ["ab-wheel-rollout"]},
  {"slug": "side-plank", "name": "Side Plank", "measurement": "time", "equipment": "bodyweight", "primary": ["obliques"], "secondary": ["abs", "abductors"], "aliases": [], "alternatives": []},
  {"slug": "hanging-leg-raise", "name": "Hanging Leg Raise", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["abs"], "secondary": ["obliques"], "aliases": ["leg raise", "hanging knee raise"], "alternatives": ["cable-crunch"]},
  {"slug": "cable-crunch", "name": "Cable Crunch", "measurement": "weight_reps", "equipment": "cable", "primary": ["abs"], "secondary": [], "aliases": ["kneeling cable crunch"], "alternatives": ["hanging-leg-raise", "machine-crunch"]},
  {"slug": "machine-crunch", "name": "Machine Crunch", "measurement": "weight_reps", "equipment": "machine", "primary": ["abs"], "secondary": [], "aliases": ["ab machine"], "alternatives": ["cable-crunch"]},
  {"slug": "ab-wheel-rollout", "name": "Ab Wheel Rollout", "measurement": "bw_reps", "equipment": "bodyweight", "primary": ["abs"], "secondary": ["lats"], "aliases": ["ab rollout"], "alternatives": ["plank"]},
  {"slug": "cable-woodchop", "name": "Cable Woodchop", "measurement": "weight_reps", "equipment": "cable", "primary": ["obliques"], "secondary": ["abs"], "aliases": ["woodchopper"], "alternatives": ["pallof-press"]},
  {"slug": "pallof-press", "name": "Pallof Press", "measurement": "weight_reps", "equipment": "cable", "primary": ["obliques", "abs"], "secondary": [], "aliases": [], "alternatives": ["cable-woodchop"]},
  {"slug": "neck-curl", "name": "Neck Curl", "measurement": "weight_reps", "equipment": "bodyweight", "primary": ["neck"], "secondary": [], "aliases": ["neck flexion"], "alternatives": []},

  {"slug": "barbell-power-clean", "name": "Barbell Power Clean", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "quads", "traps"], "secondary": ["hamstrings", "upper-back"], "aliases": ["power clean", "clean"], "alternatives": ["barbell-hang-clean"]},
  {"slug": "barbell-hang-clean", "name": "Barbell Hang Clean", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "traps"], "secondary": ["quads", "hamstrings", "upper-back"], "aliases": ["hang clean"], "alternatives": ["barbell-power-clean"]},
  {"slug": "barbell-snatch", "name": "Barbell Snatch", "measurement": "weight_reps", "equipment": "barbell", "primary": ["glutes", "quads", "traps"], "secondary": ["hamstrings", "side-delts", "upper-back"], "aliases": ["power snatch"], "alternatives": ["barbell-power-clean"]},
  {"slug": "kettlebell-swing", "name": "Kettlebell Swing", "measurement": "weight_reps", "equipment": "dumbbell", "primary": ["glutes", "hamstrings"], "secondary": ["lower-back", "abs"], "aliases": ["kb swing"], "alternatives": ["cable-pull-through"]},

  {"slug": "rowing-machine", "name": "Rowing Machine", "measurement": "distance_time", "equipment": "machine", "primary": ["upper-back", "quads"], "secondary": ["lats", "glutes", "hamstrings"], "aliases": ["row erg", "concept2", "rower"], "alternatives": ["stationary-bike"]},
  {"slug": "stationary-bike", "name": "Stationary Bike", "measurement": "distance_time", "equipment": "machine", "primary": ["quads"], "secondary": ["glutes", "hamstrings", "calves"], "aliases": ["bike", "assault bike", "cycling"], "alternatives": ["rowing-machine"]},
  {"slug": "running", "name": "Running", "measurement": "distance_time", "equipment": "bodyweight", "primary": ["quads", "calves"], "secondary": ["glutes", "hamstrings"], "aliases": ["run", "treadmill"], "alternatives": ["stationary-bike"]},
  {"slug": "jump-rope", "name": "Jump Rope", "measurement": "time", "equipment": "bodyweight", "primary": ["calves"], "secondary": ["quads"], "aliases": ["skipping"], "alternatives": []}
]
```

- [ ] **Step 2: Write the failing tests**

`internal/exercise/seed_test.go`:
```go
package exercise

import (
	"slices"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

// The seed is data, so it gets the same validation as user input, plus
// referential checks on alternatives.
func TestSeedIsValid(t *testing.T) {
	exercises, alternatives, err := loadSeed()
	if err != nil {
		t.Fatal(err)
	}
	if len(exercises) < 100 {
		t.Fatalf("seed has only %d exercises", len(exercises))
	}
	seen := map[string]bool{}
	for _, e := range exercises {
		if seen[e.Slug] {
			t.Errorf("duplicate slug %s", e.Slug)
		}
		seen[e.Slug] = true
		in := Input{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind,
			PrimaryMuscles: e.PrimaryMuscles, SecondaryMuscles: e.SecondaryMuscles, Aliases: e.Aliases}
		if errs := in.validate(); len(errs) > 0 {
			t.Errorf("%s: %v", e.Slug, errs)
		}
		if !slices.Contains(calc.Kinds, e.EquipmentKind) {
			t.Errorf("%s: equipment %q", e.Slug, e.EquipmentKind)
		}
	}
	for slug, alts := range alternatives {
		for _, alt := range alts {
			if !seen[alt] || alt == slug {
				t.Errorf("%s: alternative %q is not another seeded exercise", slug, alt)
			}
		}
	}
}
```

`internal/exercise/service_test.go`:
```go
package exercise

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

func newService(t *testing.T) (*Service, store.User) {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	u, err := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	return &Service{Store: db}, u
}

func slugsOf(es []store.Exercise) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Slug)
	}
	return out
}

func validInput(slug string) Input {
	return Input{Slug: slug, Name: "Zercher Squat", Measurement: "weight_reps", EquipmentKind: "barbell",
		PrimaryMuscles: []string{"quads"}, SecondaryMuscles: []string{"upper-back"}, Aliases: []string{" zercher ", "", "zercher"}}
}

func TestCatalogSearch(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	got, err := s.Catalog(ctx, u.ID, "Bench DUMBBELL")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(slugsOf(got), []string{"dumbbell-bench-press", "dumbbell-incline-bench-press"}) {
		t.Fatalf("search = %v", slugsOf(got))
	}
	if got, _ := s.Catalog(ctx, u.ID, "rdl"); !slices.Contains(slugsOf(got), "barbell-romanian-deadlift") {
		t.Fatalf("alias search = %v", slugsOf(got))
	}
	if all, _ := s.Catalog(ctx, u.ID, "  "); len(all) < 100 {
		t.Fatalf("empty query returned %d exercises", len(all))
	}
}

func TestCreateAndCustomize(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()

	created, err := s.Create(ctx, u.ID, validInput("zercher-squat"))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Custom() || !slices.Equal(created.Aliases, []string{"zercher"}) {
		t.Fatalf("created = %+v", created)
	}

	var fe FieldErrors
	_, err = s.Create(ctx, u.ID, validInput("barbell-back-squat"))
	if !errors.As(err, &fe) || fe["slug"] == "" {
		t.Fatalf("slug taken by the seed: want slug error, got %v", err)
	}
	bad := validInput("Bad Slug")
	bad.PrimaryMuscles = nil
	bad.SecondaryMuscles = []string{"wings"}
	_, err = s.Create(ctx, u.ID, bad)
	if !errors.As(err, &fe) || fe["slug"] == "" || fe["primary_muscles"] == "" {
		t.Fatalf("invalid input: got %v", err)
	}
	overlap := validInput("overlap")
	overlap.SecondaryMuscles = []string{"quads"}
	if _, err := s.Create(ctx, u.ID, overlap); !errors.As(err, &fe) || fe["secondary_muscles"] == "" {
		t.Fatalf("overlapping muscles: got %v", err)
	}

	// Updating a seeded exercise creates the user's customized copy.
	in := validInput("barbell-back-squat")
	in.Name = "Low Bar Squat"
	updated, err := s.Update(ctx, u.ID, in)
	if err != nil || !updated.Overrides || updated.Name != "Low Bar Squat" {
		t.Fatalf("customize = %+v, %v", updated, err)
	}
	if err := s.Delete(ctx, u.ID, "barbell-back-squat"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, u.ID, "barbell-back-squat"); got.Name != "Barbell Back Squat" {
		t.Fatalf("reset: %q", got.Name)
	}
	if _, err := s.Update(ctx, u.ID, validInput("does-not-exist")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("update unknown: %v", err)
	}
}

func TestAlternatives(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	var fe FieldErrors
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "pull-up"); !errors.As(err, &fe) {
		t.Fatalf("self alternative: %v", err)
	}
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "nope"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown alternative: %v", err)
	}
	if err := s.AddAlternative(ctx, u.ID, "pull-up", "inverted-row"); err != nil {
		t.Fatal(err)
	}
	alts, err := s.Alternatives(ctx, u.ID, "pull-up")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range alts {
		names = append(names, a.Exercise.Name)
	}
	if !slices.Contains(names, "Inverted Row") || !slices.Contains(names, "Chin-Up") {
		t.Fatalf("alternatives = %v", names)
	}
}

func TestEquipmentLookupOrder(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	squat, _ := s.Get(ctx, u.ID, "barbell-back-squat")

	st, err := s.Settings(ctx, u.ID, squat)
	if err != nil || st.Equipment != nil {
		t.Fatalf("no profiles yet: %+v, %v", st, err)
	}

	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Settings(ctx, u.ID, squat)
	if st.Equipment == nil || st.Equipment.Name != "Barbell" || st.Linked {
		t.Fatalf("default for kind: %+v", st)
	}

	home, err := s.SaveEquipment(ctx, store.Equipment{UserID: u.ID, Name: "Home bar", Spec: calc.Equipment{
		Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 15, Plates: []float64{10}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LinkEquipment(ctx, u.ID, squat.Slug, home.ID); err != nil {
		t.Fatal(err)
	}
	st, _ = s.Settings(ctx, u.ID, squat)
	if st.Equipment == nil || st.Equipment.ID != home.ID || !st.Linked {
		t.Fatalf("explicit link: %+v", st)
	}
	if err := s.LinkEquipment(ctx, u.ID, squat.Slug, ""); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.Settings(ctx, u.ID, squat); st.Linked {
		t.Fatalf("unlink: %+v", st)
	}
}

func TestStarterEquipmentOnceAndInUnit(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListEquipment(ctx, u.ID)
	if len(list) != 2 {
		t.Fatalf("starter profiles: %+v", list)
	}
	for _, e := range list {
		if err := s.DeleteEquipment(ctx, u.ID, e.ID); err != nil {
			t.Fatal(err)
		}
	}
	// u is stale (EquipmentInitialized false) like an old session; the store
	// flag still prevents re-creating what the user deleted.
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListEquipment(ctx, u.ID); len(list) != 0 {
		t.Fatalf("starter profiles came back after deletion: %+v", list)
	}

	lb := StarterEquipment(calc.UnitLb)
	if lb[0].Spec.Unit != calc.UnitLb || lb[0].Spec.Config.Bar != 45 || lb[1].Spec.Config.Weights[0] != 5 {
		t.Fatalf("lb starters: %+v", lb)
	}
	for _, e := range append(StarterEquipment(calc.UnitKg), lb...) {
		if err := e.Spec.Validate(); err != nil {
			t.Fatalf("starter %s invalid: %v", e.Name, err)
		}
	}
}

func TestSaveEquipmentValidates(t *testing.T) {
	s, u := newService(t)
	var fe FieldErrors
	_, err := s.SaveEquipment(context.Background(), store.Equipment{UserID: u.ID, Name: " ",
		Spec: calc.Equipment{Kind: calc.KindDumbbell, Unit: calc.UnitKg}})
	if !errors.As(err, &fe) || fe["name"] == "" || fe["config"] == "" {
		t.Fatalf("got %v", err)
	}
}

func TestPercentOfTM(t *testing.T) {
	s, u := newService(t)
	ctx := context.Background()
	squat, _ := s.Get(ctx, u.ID, "barbell-back-squat")

	if _, err := s.PercentOfTM(ctx, u, squat, 0.75); !errors.Is(err, ErrNoTrainingMax) {
		t.Fatalf("without TM: %v", err)
	}
	var fe FieldErrors
	if err := s.SetTrainingMax(ctx, u.ID, squat.Slug, ptr(-1), "web"); !errors.As(err, &fe) {
		t.Fatalf("negative TM: %v", err)
	}
	if err := s.SetTrainingMax(ctx, u.ID, squat.Slug, ptr(140), "web"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureStarterEquipment(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, err := s.PercentOfTM(ctx, u, squat, 0.75)
	if err != nil || got.Kg != 105 || !slices.Equal(got.PerSide, []float64{25, 15, 2.5}) {
		t.Fatalf("75%% of 140 = %+v, %v", got, err)
	}
	if hist, _ := s.TrainingMaxHistory(ctx, u.ID, squat.Slug); len(hist) != 1 {
		t.Fatalf("history: %+v", hist)
	}
}

func ptr(v float64) *float64 { return &v }
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/exercise/`
Expected: FAIL, `undefined: loadSeed`, `undefined: Input`, `undefined: Service`.

- [ ] **Step 4: Implement**

`internal/exercise/seed.go`:
```go
package exercise

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/LongerHV/onerep/internal/store"
)

//go:embed seed/exercises.json
var seedJSON []byte

type seedEntry struct {
	Slug         string   `json:"slug"`
	Name         string   `json:"name"`
	Measurement  string   `json:"measurement"`
	Equipment    string   `json:"equipment"`
	Primary      []string `json:"primary"`
	Secondary    []string `json:"secondary"`
	Aliases      []string `json:"aliases"`
	Alternatives []string `json:"alternatives"`
}

func loadSeed() ([]store.Exercise, map[string][]string, error) {
	var entries []seedEntry
	if err := json.Unmarshal(seedJSON, &entries); err != nil {
		return nil, nil, err
	}
	exercises := make([]store.Exercise, 0, len(entries))
	alternatives := map[string][]string{}
	for _, e := range entries {
		exercises = append(exercises, store.Exercise{
			Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.Equipment,
			PrimaryMuscles: e.Primary, SecondaryMuscles: e.Secondary, Aliases: e.Aliases,
		})
		if len(e.Alternatives) > 0 {
			alternatives[e.Slug] = e.Alternatives
		}
	}
	return exercises, alternatives, nil
}

// SeedStore is what Seed needs. *store.DB implements it.
type SeedStore interface {
	SyncSeed(ctx context.Context, exercises []store.Exercise, alternatives map[string][]string) error
}

// Seed syncs the embedded catalog into the database. Run it at startup.
func Seed(ctx context.Context, db SeedStore) error {
	exercises, alternatives, err := loadSeed()
	if err != nil {
		return err
	}
	return db.SyncSeed(ctx, exercises, alternatives)
}
```

`internal/exercise/service.go`:
```go
package exercise

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
)

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CatalogExercises(ctx context.Context, userID string) ([]store.Exercise, error)
	ExerciseBySlug(ctx context.Context, userID, slug string) (store.Exercise, error)
	SaveUserExercise(ctx context.Context, userID string, e store.Exercise) (store.Exercise, error)
	DeleteUserExercise(ctx context.Context, userID, slug string) error
	Alternatives(ctx context.Context, userID, slug string) ([]store.Alternative, error)
	AddAlternative(ctx context.Context, userID, slug, altSlug string) error
	RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error

	ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error)
	EquipmentByID(ctx context.Context, userID, id string) (store.Equipment, error)
	DefaultEquipment(ctx context.Context, userID, kind string) (store.Equipment, error)
	SaveEquipment(ctx context.Context, e store.Equipment) (store.Equipment, error)
	DeleteEquipment(ctx context.Context, userID, id string) error
	InitStarterEquipment(ctx context.Context, userID string, items []store.Equipment) (bool, error)

	UserExercise(ctx context.Context, userID, slug string) (store.UserExercise, error)
	SetExerciseEquipment(ctx context.Context, userID, slug, equipmentID string) error
	SetTrainingMax(ctx context.Context, userID, slug string, newKg *float64, source, note string) error
	TrainingMaxHistory(ctx context.Context, userID, slug string) ([]store.TrainingMaxChange, error)
}

// Service implements the catalog and equipment rules.
type Service struct {
	Store Store
}

// Input is a user-defined exercise (new, or a customized seeded one).
type Input struct {
	Slug             string
	Name             string
	Measurement      string
	EquipmentKind    string
	PrimaryMuscles   []string
	SecondaryMuscles []string
	Aliases          []string
}

var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func (in Input) validate() FieldErrors {
	errs := FieldErrors{}
	if !slugRE.MatchString(in.Slug) || len(in.Slug) > 64 {
		errs["slug"] = "use lowercase letters, digits and single dashes, like cable-row"
	}
	if name := strings.TrimSpace(in.Name); name == "" || len(name) > 100 {
		errs["name"] = "name is required (at most 100 characters)"
	}
	if !isMeasurement(in.Measurement) {
		errs["measurement"] = "choose how sets are recorded"
	}
	if !slices.Contains(calc.Kinds, in.EquipmentKind) {
		errs["equipment_kind"] = "choose the equipment"
	}
	if len(in.PrimaryMuscles) == 0 {
		errs["primary_muscles"] = "choose at least one primary muscle"
	}
	for _, m := range append(slices.Clone(in.PrimaryMuscles), in.SecondaryMuscles...) {
		if !isMuscle(m) {
			errs["primary_muscles"] = "unknown muscle " + m
		}
	}
	for _, m := range in.SecondaryMuscles {
		if slices.Contains(in.PrimaryMuscles, m) {
			errs["secondary_muscles"] = m + " is already a primary muscle"
		}
	}
	return errs
}

// Catalog returns the user's exercises whose name, slug, alias, muscle or
// equipment contains every word of query.
func (s *Service) Catalog(ctx context.Context, userID, query string) ([]store.Exercise, error) {
	all, err := s.Store.CatalogExercises(ctx, userID)
	if err != nil {
		return nil, err
	}
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return all, nil
	}
	var out []store.Exercise
	for _, e := range all {
		hay := strings.ToLower(strings.Join(append([]string{e.Name, e.Slug, e.EquipmentKind},
			append(append(slices.Clone(e.Aliases), e.PrimaryMuscles...), e.SecondaryMuscles...)...), " "))
		match := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				match = false
				break
			}
		}
		if match {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, userID, slug string) (store.Exercise, error) {
	return s.Store.ExerciseBySlug(ctx, userID, slug)
}

// Create adds a new user exercise. The slug must not be in use.
func (s *Service) Create(ctx context.Context, userID string, in Input) (store.Exercise, error) {
	in = normalize(in)
	errs := in.validate()
	if _, err := s.Store.ExerciseBySlug(ctx, userID, in.Slug); err == nil {
		errs["slug"] = "an exercise with this slug already exists"
	} else if !errors.Is(err, store.ErrNotFound) {
		return store.Exercise{}, err
	}
	if len(errs) > 0 {
		return store.Exercise{}, errs
	}
	return s.Store.SaveUserExercise(ctx, userID, toStore(in))
}

// Update changes an existing exercise. Updating a seeded exercise creates the
// user's customized copy.
func (s *Service) Update(ctx context.Context, userID string, in Input) (store.Exercise, error) {
	in = normalize(in)
	if _, err := s.Store.ExerciseBySlug(ctx, userID, in.Slug); err != nil {
		return store.Exercise{}, err
	}
	if errs := in.validate(); len(errs) > 0 {
		return store.Exercise{}, errs
	}
	return s.Store.SaveUserExercise(ctx, userID, toStore(in))
}

// Delete removes the user's own exercise, or restores a customized seeded one.
func (s *Service) Delete(ctx context.Context, userID, slug string) error {
	return s.Store.DeleteUserExercise(ctx, userID, slug)
}

func normalize(in Input) Input {
	in.Slug = strings.TrimSpace(in.Slug)
	in.Name = strings.TrimSpace(in.Name)
	var aliases []string
	for _, a := range in.Aliases {
		if a = strings.TrimSpace(a); a != "" && !slices.Contains(aliases, a) {
			aliases = append(aliases, a)
		}
	}
	in.Aliases = aliases
	return in
}

func toStore(in Input) store.Exercise {
	return store.Exercise{Slug: in.Slug, Name: in.Name, Measurement: in.Measurement, EquipmentKind: in.EquipmentKind,
		PrimaryMuscles: in.PrimaryMuscles, SecondaryMuscles: in.SecondaryMuscles, Aliases: in.Aliases}
}

// AlternativeView is an alternative with its exercise resolved.
type AlternativeView struct {
	Exercise  store.Exercise
	UserAdded bool
}

// Alternatives lists the alternatives of slug that exist in the user's catalog.
func (s *Service) Alternatives(ctx context.Context, userID, slug string) ([]AlternativeView, error) {
	alts, err := s.Store.Alternatives(ctx, userID, slug)
	if err != nil {
		return nil, err
	}
	var out []AlternativeView
	for _, a := range alts {
		e, err := s.Store.ExerciseBySlug(ctx, userID, a.Slug)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, AlternativeView{Exercise: e, UserAdded: a.UserAdded})
	}
	return out, nil
}

func (s *Service) AddAlternative(ctx context.Context, userID, slug, altSlug string) error {
	if slug == altSlug {
		return FieldErrors{"alternative": "an exercise cannot be its own alternative"}
	}
	for _, sl := range []string{slug, altSlug} {
		if _, err := s.Store.ExerciseBySlug(ctx, userID, sl); err != nil {
			return err
		}
	}
	return s.Store.AddAlternative(ctx, userID, slug, altSlug)
}

func (s *Service) RemoveAlternative(ctx context.Context, userID, slug, altSlug string) error {
	return s.Store.RemoveAlternative(ctx, userID, slug, altSlug)
}

// Settings is a user's view of one exercise's settings.
type Settings struct {
	store.UserExercise
	// Equipment is the profile used for rounding: the linked one, else the
	// default for the exercise's kind; nil if neither exists.
	Equipment *store.Equipment
	// Linked is true when Equipment comes from an explicit link.
	Linked bool
}

func (s *Service) Settings(ctx context.Context, userID string, ex store.Exercise) (Settings, error) {
	ue, err := s.Store.UserExercise(ctx, userID, ex.Slug)
	if err != nil {
		return Settings{}, err
	}
	st := Settings{UserExercise: ue}
	if ue.EquipmentID != "" {
		eq, err := s.Store.EquipmentByID(ctx, userID, ue.EquipmentID)
		if err != nil {
			return Settings{}, err
		}
		st.Equipment, st.Linked = &eq, true
		return st, nil
	}
	eq, err := s.Store.DefaultEquipment(ctx, userID, ex.EquipmentKind)
	if errors.Is(err, store.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return Settings{}, err
	}
	st.Equipment = &eq
	return st, nil
}

// LinkEquipment links slug to a profile; "" goes back to the kind's default.
func (s *Service) LinkEquipment(ctx context.Context, userID, slug, equipmentID string) error {
	if _, err := s.Store.ExerciseBySlug(ctx, userID, slug); err != nil {
		return err
	}
	return s.Store.SetExerciseEquipment(ctx, userID, slug, equipmentID)
}

// MaxTrainingMaxKg bounds training max input; heavier is a typo.
const MaxTrainingMaxKg = 1500

// SetTrainingMax sets (nil clears) the training max of slug.
func (s *Service) SetTrainingMax(ctx context.Context, userID, slug string, kg *float64, source string) error {
	if kg != nil && (*kg <= 0 || *kg > MaxTrainingMaxKg) {
		return FieldErrors{"training_max": "enter a positive weight"}
	}
	if _, err := s.Store.ExerciseBySlug(ctx, userID, slug); err != nil {
		return err
	}
	return s.Store.SetTrainingMax(ctx, userID, slug, kg, source, "")
}

func (s *Service) TrainingMaxHistory(ctx context.Context, userID, slug string) ([]store.TrainingMaxChange, error) {
	return s.Store.TrainingMaxHistory(ctx, userID, slug)
}

// PercentOfTM rounds pct of the training max to the exercise's equipment.
func (s *Service) PercentOfTM(ctx context.Context, user store.User, ex store.Exercise, pct float64) (calc.Rounded, error) {
	st, err := s.Settings(ctx, user.ID, ex)
	if err != nil {
		return calc.Rounded{}, err
	}
	if st.TrainingMaxKg == nil {
		return calc.Rounded{}, ErrNoTrainingMax
	}
	var eq *calc.Equipment
	if st.Equipment != nil {
		eq = &st.Equipment.Spec
	}
	r, _ := calc.ResolveLoad(calc.Load{PctTM: &pct}, 1, calc.LoadContext{TMKg: st.TrainingMaxKg, Equipment: eq, Unit: user.Unit})
	return r, nil
}

// Equipment profiles.

func (s *Service) ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error) {
	return s.Store.ListEquipment(ctx, userID)
}

func (s *Service) GetEquipment(ctx context.Context, userID, id string) (store.Equipment, error) {
	return s.Store.EquipmentByID(ctx, userID, id)
}

// SaveEquipment validates and stores a profile (empty ID creates one).
func (s *Service) SaveEquipment(ctx context.Context, e store.Equipment) (store.Equipment, error) {
	errs := FieldErrors{}
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len(e.Name) > 100 {
		errs["name"] = "name is required (at most 100 characters)"
	}
	if err := e.Spec.Validate(); err != nil {
		errs["config"] = err.Error()
	}
	if len(errs) > 0 {
		return store.Equipment{}, errs
	}
	return s.Store.SaveEquipment(ctx, e)
}

func (s *Service) DeleteEquipment(ctx context.Context, userID, id string) error {
	return s.Store.DeleteEquipment(ctx, userID, id)
}

// EnsureStarterEquipment gives a new user default barbell and dumbbell
// profiles in their unit. It does nothing once done, even if the user later
// deletes the profiles.
func (s *Service) EnsureStarterEquipment(ctx context.Context, user store.User) error {
	if user.EquipmentInitialized {
		return nil
	}
	_, err := s.Store.InitStarterEquipment(ctx, user.ID, StarterEquipment(user.Unit))
	return err
}

// StarterEquipment is the default equipment for a new user (spec §6).
func StarterEquipment(unit string) []store.Equipment {
	bar, plates, dumbbells := 20.0, []float64{25, 20, 15, 10, 5, 2.5, 1.25}, weightRange(2, 50, 2)
	if unit == calc.UnitLb {
		bar, plates, dumbbells = 45, []float64{45, 35, 25, 10, 5, 2.5}, weightRange(5, 100, 5)
	} else {
		unit = calc.UnitKg
	}
	return []store.Equipment{
		{Name: "Barbell", IsDefault: true, Spec: calc.Equipment{Kind: calc.KindBarbell, Unit: unit,
			Config: calc.EquipmentConfig{Bar: bar, Plates: plates}}},
		{Name: "Dumbbells", IsDefault: true, Spec: calc.Equipment{Kind: calc.KindDumbbell, Unit: unit,
			Config: calc.EquipmentConfig{Weights: dumbbells}}},
	}
}

func weightRange(lo, hi, step float64) []float64 {
	var out []float64
	for v := lo; v <= hi; v += step {
		out = append(out, v)
	}
	return out
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/exercise/ && golangci-lint run ./internal/...`
Expected: `ok`, then `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/exercise
git commit -m "feat(exercise): add seeded catalog and catalog, equipment and training max rules"
```

---

### Task 8: Account settings

**Files:**
- Create: `internal/account/account.go`
- Test: `internal/account/account_test.go`

**Interfaces:**
- Consumes: `(*store.DB).UpdateUserSettings` (Task 3)
- Produces: `account.Service{Store account.Store}`, `(*Service).UpdateSettings(ctx, userID, unit string, e1rmWindowDays int) error`, `account.ErrInvalidSettings`

- [ ] **Step 1: Write the failing test**

`internal/account/account_test.go`:
```go
package account

import (
	"context"
	"errors"
	"testing"

	"github.com/LongerHV/onerep/internal/store/storetest"
)

func TestUpdateSettings(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	u, err := db.UpsertOIDCUser(ctx, "iss", "sub", "", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: db}

	if err := s.UpdateSettings(ctx, u.ID, "lb", 60); err != nil {
		t.Fatal(err)
	}
	got, _ := db.UserByID(ctx, u.ID)
	if got.Unit != "lb" || got.E1RMWindowDays != 60 {
		t.Fatalf("settings not saved: %+v", got)
	}
	for _, bad := range []struct {
		unit string
		days int
	}{{"stone", 30}, {"kg", 6}, {"kg", 366}} {
		if err := s.UpdateSettings(ctx, u.ID, bad.unit, bad.days); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%+v: got %v", bad, err)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/account/`
Expected: FAIL, `undefined: Service`.

- [ ] **Step 3: Implement**

`internal/account/account.go`:
```go
// Package account manages a user's own preferences.
package account

import (
	"context"
	"errors"

	"github.com/LongerHV/onerep/internal/calc"
)

// Store is what the service needs. *store.DB implements it.
type Store interface {
	UpdateUserSettings(ctx context.Context, userID, unit string, e1rmWindowDays int) error
}

type Service struct {
	Store Store
}

// ErrInvalidSettings is returned for a unit other than kg/lb or a window outside 7–365 days.
var ErrInvalidSettings = errors.New("unit must be kg or lb and the e1RM window 7 to 365 days")

// UpdateSettings changes the display unit and the e1RM look-back window.
// Stored weights are always kg, so changing the unit converts nothing.
func (s *Service) UpdateSettings(ctx context.Context, userID, unit string, e1rmWindowDays int) error {
	if (unit != calc.UnitKg && unit != calc.UnitLb) || e1rmWindowDays < 7 || e1rmWindowDays > 365 {
		return ErrInvalidSettings
	}
	return s.Store.UpdateUserSettings(ctx, userID, unit, e1rmWindowDays)
}
```

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/account/`
Expected: `ok  github.com/LongerHV/onerep/internal/account`

- [ ] **Step 5: Commit**

```bash
git add internal/account
git commit -m "feat(account): add unit and e1RM window settings"
```

---

### Task 9: Web pages for exercises, equipment and settings

**Files:**
- Create: `internal/web/views/ui.go`, `internal/web/views/models.go`, `internal/web/views/exercises.templ`, `internal/web/views/equipment.templ`, `internal/web/views/settings.templ`
- Replace: `internal/web/views/layout.templ`, `internal/web/views/pages.templ`
- Create: `internal/web/exercises.go`, `internal/web/equipment.go`, `internal/web/settings.go`
- Replace: `internal/web/server.go`, `internal/web/server_test.go`, `cmd/onerep/main.go`
- Create (generated): `internal/web/views/*_templ.go`, `internal/web/static/app.css`
- Test: `internal/web/catalog_test.go`

**Interfaces:**
- Consumes: `exercise.*` (Tasks 6–7), `account.*` (Task 8), `calc.*`
- Produces:
  - `web.Server` gains the fields `Exercises *exercise.Service` and `Account *account.Service`
  - Helpers: `user(r) store.User`, `(*Server).fail(w, r, err)` (`ErrNotFound` → 404), and the `starterEquipment` middleware
  - Routes (all behind RequireUser, CSRF and starterEquipment):
    - Exercises: `GET /exercises` (an `HX-Target: exercise-results` request gets just the list fragment), `GET /exercises/new`, `POST /exercises`, `GET /exercises/{slug}`, `GET /exercises/{slug}/edit`, `POST /exercises/{slug}`, `POST /exercises/{slug}/delete`, `POST /exercises/{slug}/settings`, `GET /exercises/{slug}/calc?pct=`
    - Alternatives: `POST /exercises/{slug}/alternatives`, `POST /exercises/{slug}/alternatives/{alt}/delete`
    - Equipment: `GET /equipment`, `GET /equipment/new?kind=`, `POST /equipment`, `GET /equipment/{id}/edit`, `POST /equipment/{id}`, `POST /equipment/{id}/delete`
    - Settings: `GET /settings`, `POST /settings`
  - `views.CSRF(Page)` component (hidden token field for POST forms), `views.Weight(kg, unit) string`, `views.Label(string) string`
  - Test helpers in package `web`: `newAppDB(t, devUser) (*httptest.Server, *http.Client, *store.DB)`, `session`, `post`, `htmx`

- [ ] **Step 1: Write the failing tests**

Replace `internal/web/server_test.go` with (it now seeds the catalog and wires the new services):
```go
package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

// newApp serves the full router with the given dev user ("" disables the bypass).
func newApp(t *testing.T, devUser string) (*httptest.Server, *http.Client) {
	t.Helper()
	srv, c, _ := newAppDB(t, devUser)
	return srv, c
}

// newAppDB is newApp that also returns the database, with the catalog seeded.
func newAppDB(t *testing.T, devUser string) (*httptest.Server, *http.Client, *store.DB) {
	t.Helper()
	db := storetest.New(t)
	if err := exercise.Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	s := &Server{DB: db, Sessions: &auth.Sessions{Store: db}, DevUser: devUser,
		Exercises: &exercise.Service{Store: db}, Account: &account.Service{Store: db}}
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return srv, client, db
}

func read(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustGet(t *testing.T, c *http.Client, u string) *http.Response {
	t.Helper()
	resp, err := c.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

var csrfInput = regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)

func TestHomeWithDevUser(t *testing.T) {
	srv, c := newApp(t, "alice")
	resp := mustGet(t, c, srv.URL+"/")
	html := read(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(html, "Hi, alice") {
		t.Fatalf("home: %d\n%s", resp.StatusCode, html)
	}
	if !strings.Contains(html, `hx-headers="{&#34;X-CSRF-Token&#34;:`) {
		t.Fatalf("htmx CSRF header not configured:\n%s", html)
	}
	if !strings.Contains(html, `href="https://github.com/LongerHV/onerep"`) {
		t.Fatalf("source code link (AGPL section 13) missing:\n%s", html)
	}
}

func TestLogoutRequiresCSRF(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/"))
	m := csrfInput.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no csrf input in page:\n%s", html)
	}

	resp, err := c.PostForm(srv.URL+"/auth/logout", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("logout without token: %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = c.PostForm(srv.URL+"/auth/logout", url.Values{auth.CSRFField: {m[1]}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/auth/signed-out" {
		t.Fatalf("logout: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestAnonymousIsRedirectedToLogin(t *testing.T) {
	srv, c := newApp(t, "")
	resp := mustGet(t, c, srv.URL+"/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/auth/login") {
		t.Fatalf("got %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestHealthzAndStatic(t *testing.T) {
	srv, c := newApp(t, "")
	resp := mustGet(t, c, srv.URL+"/healthz")
	if body := read(t, resp); resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("healthz: %d %q", resp.StatusCode, body)
	}
	for _, path := range []string{"/static/app.css", "/static/vendor/htmx.min.js"} {
		resp := mustGet(t, c, srv.URL+path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
}

func TestNotFoundPageShowsRequestID(t *testing.T) {
	srv, c := newApp(t, "alice")
	resp := mustGet(t, c, srv.URL+"/nope")
	html := read(t, resp)
	if resp.StatusCode != http.StatusNotFound || !strings.Contains(html, "Request ID") {
		t.Fatalf("got %d\n%s", resp.StatusCode, html)
	}
}

func TestDevLoginOnlyOnAppRoutes(t *testing.T) {
	srv, c := newApp(t, "alice")
	for _, path := range []string{"/healthz", "/static/app.css"} {
		resp := mustGet(t, c, srv.URL+path)
		resp.Body.Close()
		for _, ck := range resp.Cookies() {
			if ck.Name == auth.CookieName {
				t.Fatalf("%s created a dev session", path)
			}
		}
	}
}

func TestUserNameIsHTMLEscaped(t *testing.T) {
	srv, c := newApp(t, `<img src=x onerror=alert(1)>`)
	html := read(t, mustGet(t, c, srv.URL+"/"))
	if strings.Contains(html, "<img src=x") {
		t.Fatalf("user name rendered unescaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("escaped name missing:\n%s", html)
	}
}
```

`internal/web/catalog_test.go`:
```go
package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
)

// session opens the app as the dev user and returns a CSRF token.
func session(t *testing.T, srv *httptest.Server, c *http.Client) string {
	t.Helper()
	m := csrfInput.FindStringSubmatch(read(t, mustGet(t, c, srv.URL+"/")))
	if m == nil {
		t.Fatal("no CSRF token on home page")
	}
	return m[1]
}

func post(t *testing.T, c *http.Client, u, csrf string, form url.Values) (*http.Response, string) {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set(auth.CSRFField, csrf)
	resp, err := c.PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp, read(t, resp)
}

func htmx(t *testing.T, c *http.Client, u, target string) string {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, u, nil)
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Target", target)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d", u, resp.StatusCode)
	}
	return read(t, resp)
}

func TestExerciseSearchFragment(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)

	full := read(t, mustGet(t, c, srv.URL+"/exercises"))
	if !strings.Contains(full, "Barbell Back Squat") || !strings.Contains(full, "<html") {
		t.Fatal("catalog page incomplete")
	}
	frag := htmx(t, c, srv.URL+"/exercises?q=bench+dumbbell", "exercise-results")
	if strings.Contains(frag, "<html") || !strings.Contains(frag, "Dumbbell Bench Press") || strings.Contains(frag, "Barbell Back Squat") {
		t.Fatalf("search fragment:\n%s", frag)
	}
}

func TestCreateCustomizeAndDeleteExercise(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)

	form := url.Values{"name": {"Zercher Squat"}, "slug": {"zercher-squat"}, "measurement": {"weight_reps"},
		"equipment_kind": {"barbell"}, "primary_muscles": {"quads", "glutes"}, "aliases": {"zercher, "}}
	resp, _ := post(t, c, srv.URL+"/exercises", csrf, form)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/exercises/zercher-squat" {
		t.Fatalf("create: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	page := read(t, mustGet(t, c, srv.URL+"/exercises/zercher-squat"))
	if !strings.Contains(page, "Zercher Squat") || !strings.Contains(page, "custom") {
		t.Fatalf("detail page:\n%s", page)
	}

	form.Set("slug", "zercher-squat")
	resp, body := post(t, c, srv.URL+"/exercises", csrf, form)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "already exists") {
		t.Fatalf("duplicate slug: %d", resp.StatusCode)
	}

	// Customizing a seeded exercise, then resetting it.
	form.Set("name", "Squat (low bar)")
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat", csrf, form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("customize: %d", resp.StatusCode)
	}
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, "Reset to default") {
		t.Fatal("customized exercise offers no reset")
	}
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat/delete", csrf, nil)
	if resp.Header.Get("Location") != "/exercises/barbell-back-squat" {
		t.Fatalf("reset redirect: %s", resp.Header.Get("Location"))
	}
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, "Barbell Back Squat") {
		t.Fatal("reset did not restore the seeded exercise")
	}

	resp, _ = post(t, c, srv.URL+"/exercises/zercher-squat/delete", csrf, nil)
	if resp.Header.Get("Location") != "/exercises" {
		t.Fatalf("delete redirect: %s", resp.Header.Get("Location"))
	}
	if resp := mustGet(t, c, srv.URL+"/exercises/zercher-squat"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted exercise: %d", resp.StatusCode)
	}
}

func TestTrainingMaxAndCalculator(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)

	resp, _ := post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"140"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("set TM: %d", resp.StatusCode)
	}
	frag := htmx(t, c, srv.URL+"/exercises/barbell-back-squat/calc?pct=75", "calc-result")
	for _, want := range []string{"75% of 140 kg", "105 kg", "25 · 15 · 2.5 kg on Barbell"} {
		if !strings.Contains(frag, want) {
			t.Fatalf("calculator missing %q:\n%s", want, frag)
		}
	}

	// A decimal comma is accepted.
	post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"142,5"}})
	if page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat")); !strings.Contains(page, `value="142.5"`) {
		t.Fatal("decimal comma TM not saved as 142.5")
	}
	post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"140"}})

	resp, body := post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"training_max": {"heavy"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "enter a number") {
		t.Fatalf("bad TM: %d", resp.StatusCode)
	}

	// Switching to pounds shows the same TM converted; nothing is re-stored.
	resp, _ = post(t, c, srv.URL+"/settings", csrf, url.Values{"unit": {"lb"}, "e1rm_window_days": {"30"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("settings: %d", resp.StatusCode)
	}
	page := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-back-squat"))
	if !strings.Contains(page, `value="308.65"`) || !strings.Contains(page, "Training max (lb)") {
		t.Fatalf("TM not shown in lb:\n%s", page)
	}
	resp, body = post(t, c, srv.URL+"/settings", csrf, url.Values{"unit": {"stone"}, "e1rm_window_days": {"30"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "unit must be") {
		t.Fatalf("invalid settings: %d", resp.StatusCode)
	}
}

func TestAlternativesAddRemove(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	base := srv.URL + "/exercises/pull-up"

	resp, _ := post(t, c, base+"/alternatives", csrf, url.Values{"alternative": {"inverted-row"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	if page := read(t, mustGet(t, c, base)); !strings.Contains(page, "/exercises/pull-up/alternatives/inverted-row/delete") {
		t.Fatal("user-added alternative has no remove button")
	}
	resp, body := post(t, c, base+"/alternatives", csrf, url.Values{"alternative": {"pull-up"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "its own alternative") {
		t.Fatalf("self: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, base+"/alternatives/inverted-row/delete", csrf, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("remove: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, base+"/alternatives/chin-up/delete", csrf, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("removing a seeded alternative: %d", resp.StatusCode)
	}
}

func TestEquipmentPages(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	csrf := session(t, srv, c)

	list := read(t, mustGet(t, c, srv.URL+"/equipment"))
	if !strings.Contains(list, "20 kg bar, plates 1.25, 2.5, 5-25/5 kg") || !strings.Contains(list, "2-50/2 kg") {
		t.Fatalf("starter equipment missing:\n%s", list)
	}

	form := url.Values{"kind": {"dumbbell"}, "name": {"Home DBs"}, "unit": {"kg"}, "weights": {"2-20/2, 22.5"}, "is_default": {"1"}}
	resp, _ := post(t, c, srv.URL+"/equipment", csrf, form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create: %d", resp.StatusCode)
	}
	list = read(t, mustGet(t, c, srv.URL+"/equipment"))
	if !strings.Contains(list, "Home DBs") || !strings.Contains(list, "2-20/2, 22.5 kg") {
		t.Fatalf("new profile missing:\n%s", list)
	}

	form.Set("weights", "2-20")
	resp, body := post(t, c, srv.URL+"/equipment", csrf, form)
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "a range needs a step") {
		t.Fatalf("bad weights: %d", resp.StatusCode)
	}

	// Another user's profile is invisible.
	bob, err := db.UpsertOIDCUser(context.Background(), "iss", "bob", "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := db.SaveEquipment(context.Background(), store.Equipment{UserID: bob.ID, Name: "Bob's bar",
		Spec: calc.Equipment{Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{20}}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp := mustGet(t, c, srv.URL+"/equipment/"+theirs.ID+"/edit"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other user's equipment: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/equipment/"+theirs.ID+"/delete", csrf, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting other user's equipment: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/exercises/barbell-back-squat/settings", csrf, url.Values{"equipment_id": {theirs.ID}})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("linking other user's equipment: %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web/`
Expected: FAIL, `unknown field Exercises in struct literal of type Server`.

- [ ] **Step 3: Write the view helpers and models**

`internal/web/views/ui.go`:
```go
package views

import (
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
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
```

`internal/web/views/models.go`:
```go
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
```

- [ ] **Step 4: Write the templates**

Replace `internal/web/views/layout.templ` with (nav links, the `CSRF` and `fieldError` components):
```templ
package views

import "github.com/LongerHV/onerep/internal/auth"

templ Layout(p Page) {
	<!DOCTYPE html>
	<html lang="en">
		<head>
			<meta charset="utf-8"/>
			<meta name="viewport" content="width=device-width, initial-scale=1"/>
			<title>{ p.Title } · onerep</title>
			<link rel="stylesheet" href="/static/app.css"/>
			<script src="/static/vendor/htmx.min.js" defer></script>
		</head>
		<body
			class="min-h-screen bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100"
			if p.Identity != nil {
				hx-headers={ csrfHeaders(p.Identity.CSRFToken) }
			}
		>
			<header class="border-b border-zinc-200 dark:border-zinc-800">
				<nav class="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-2 px-4 py-3">
					<div class="flex items-center gap-4">
						<a href="/" class="text-lg font-semibold">onerep</a>
						if p.Identity != nil {
							<a href="/exercises" class="text-sm hover:underline">Exercises</a>
							<a href="/equipment" class="text-sm hover:underline">Equipment</a>
							<a href="/settings" class="text-sm hover:underline">Settings</a>
						}
					</div>
					if p.Identity != nil {
						<form method="post" action="/auth/logout" class="flex items-center gap-3 text-sm">
							<span>{ p.Identity.User.Name }</span>
							@CSRF(p)
							<button type="submit" class="rounded px-2 py-1 hover:bg-zinc-200 dark:hover:bg-zinc-800">Log out</button>
						</form>
					}
				</nav>
			</header>
			<main class="mx-auto max-w-3xl px-4 py-6">
				{ children... }
			</main>
			<footer class="mx-auto max-w-3xl px-4 py-6 text-sm text-zinc-500">
				<a href="https://github.com/LongerHV/onerep" class="underline">onerep</a> is free software under the AGPL-3.0.
			</footer>
		</body>
	</html>
}

// CSRF is the hidden token field every POST form needs.
templ CSRF(p Page) {
	if p.Identity != nil {
		<input type="hidden" name={ auth.CSRFField } value={ p.Identity.CSRFToken }/>
	}
}

templ fieldError(errs map[string]string, field string) {
	if msg, ok := errs[field]; ok {
		<p class={ errorText }>{ msg }</p>
	}
}
```

Replace `internal/web/views/pages.templ` with:
```templ
package views

import "strconv"

templ Home(p Page) {
	@Layout(p) {
		<h1 class={ h1 }>Hi, { p.Identity.User.Name }</h1>
		<p class="mt-2 text-zinc-600 dark:text-zinc-400">No training plan yet.</p>
		<ul class="mt-6 list-inside list-disc space-y-1">
			<li><a href="/exercises" class="underline">Browse exercises</a> and set training maxes</li>
			<li><a href="/equipment" class="underline">Set up your equipment</a> so weights round to what you can load</li>
		</ul>
	}
}

templ SignedOut(p Page) {
	@Layout(p) {
		<h1 class={ h1 }>Signed out</h1>
		<p class="mt-2"><a href="/auth/login" class="underline">Sign in again</a></p>
	}
}

templ Error(p Page, status int, message, requestID string) {
	@Layout(p) {
		<h1 class={ h1 }>{ strconv.Itoa(status) }</h1>
		<p class="mt-2">{ message }</p>
		if requestID != "" {
			<p class="mt-4 text-sm text-zinc-500">Request ID: <code>{ requestID }</code></p>
		}
	}
}
```

`internal/web/views/exercises.templ`:
```templ
package views

import (
	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

templ ExerciseList(p Page, query string, exercises []store.Exercise) {
	@Layout(p) {
		<div class="flex items-center justify-between">
			<h1 class={ h1 }>Exercises</h1>
			<a href="/exercises/new" class={ btn }>New exercise</a>
		</div>
		<input
			type="search"
			name="q"
			value={ query }
			placeholder="Search by name, muscle or equipment"
			aria-label="Search exercises"
			class={ input + " mt-4" }
			hx-get="/exercises"
			hx-trigger="input changed delay:200ms, search"
			hx-target="#exercise-results"
			hx-push-url="true"
		/>
		<div id="exercise-results" class="mt-4">
			@ExerciseResults(exercises)
		</div>
	}
}

templ ExerciseResults(exercises []store.Exercise) {
	if len(exercises) == 0 {
		<p class="text-zinc-500">No exercises match.</p>
	}
	<ul class="divide-y divide-zinc-200 dark:divide-zinc-800">
		for _, e := range exercises {
			<li>
				<a href={ templ.URL("/exercises/" + e.Slug) } class="flex items-baseline justify-between gap-4 py-2 hover:bg-zinc-100 dark:hover:bg-zinc-900">
					<span>
						{ e.Name }
						if e.Overrides {
							<span class={ badge }>customized</span>
						} else if e.Custom() {
							<span class={ badge }>custom</span>
						}
					</span>
					<span class="text-right text-sm text-zinc-500">{ Label(e.EquipmentKind) } · { joinLabels(e.PrimaryMuscles) }</span>
				</a>
			</li>
		}
	</ul>
}

templ ExerciseDetailPage(p Page, d ExerciseDetail) {
	@Layout(p) {
		<div class="flex flex-wrap items-start justify-between gap-2">
			<div>
				<h1 class={ h1 }>{ d.Exercise.Name }</h1>
				<p class="text-sm text-zinc-500">
					{ d.Exercise.Slug }
					if d.Exercise.Overrides {
						<span class={ badge }>customized</span>
					} else if d.Exercise.Custom() {
						<span class={ badge }>custom</span>
					}
				</p>
			</div>
			<div class="flex gap-2">
				<a href={ templ.URL("/exercises/" + d.Exercise.Slug + "/edit") } class={ btnSecondary }>
					if d.Exercise.Custom() {
						Edit
					} else {
						Customize
					}
				</a>
				if d.Exercise.Custom() {
					<form method="post" action={ templ.URL("/exercises/" + d.Exercise.Slug + "/delete") }>
						@CSRF(p)
						<button type="submit" class={ btnDanger }>
							if d.Exercise.Overrides {
								Reset to default
							} else {
								Delete
							}
						</button>
					</form>
				}
			</div>
		</div>
		<dl class="mt-4 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-sm">
			<dt class="text-zinc-500">Recorded as</dt>
			<dd>{ MeasurementLabel(d.Exercise.Measurement) }</dd>
			<dt class="text-zinc-500">Equipment</dt>
			<dd>{ Label(d.Exercise.EquipmentKind) }</dd>
			<dt class="text-zinc-500">Primary</dt>
			<dd>{ joinLabels(d.Exercise.PrimaryMuscles) }</dd>
			if len(d.Exercise.SecondaryMuscles) > 0 {
				<dt class="text-zinc-500">Secondary</dt>
				<dd>{ joinLabels(d.Exercise.SecondaryMuscles) }</dd>
			}
			if len(d.Exercise.Aliases) > 0 {
				<dt class="text-zinc-500">Also known as</dt>
				<dd>{ joinLabels(d.Exercise.Aliases) }</dd>
			}
		</dl>
		<h2 class={ h2 }>Your settings</h2>
		<form method="post" action={ templ.URL("/exercises/" + d.Exercise.Slug + "/settings") } class={ card + " mt-2 grid gap-4 sm:grid-cols-2" }>
			@CSRF(p)
			<label class={ label }>
				Equipment for rounding
				<select name="equipment_id" class={ input }>
					<option value="">
						Default for { Label(d.Exercise.EquipmentKind) }
						if d.Settings.Equipment != nil && !d.Settings.Linked {
							({ d.Settings.Equipment.Name })
						} else if d.Settings.Equipment == nil {
							(none, round to 0.5 kg / 1 lb)
						}
					</option>
					for _, e := range d.Equipment {
						<option value={ e.ID } selected?={ d.Settings.EquipmentID == e.ID }>{ e.Name } ({ Label(e.Spec.Kind) })</option>
					}
				</select>
			</label>
			<label class={ label }>
				Training max ({ p.Identity.User.Unit })
				<input type="text" inputmode="decimal" name="training_max" value={ d.TMInput } placeholder="not set" class={ input }/>
				@fieldError(d.Errors, "training_max")
			</label>
			<div class="sm:col-span-2">
				<button type="submit" class={ btn }>Save</button>
			</div>
		</form>
		if d.Settings.TrainingMaxKg != nil {
			<div class={ card + " mt-4" }>
				<label class={ label }>
					Percent of training max
					<input
						type="number"
						name="pct"
						value="75"
						min="1"
						max="150"
						step="0.5"
						class={ input + " max-w-32" }
						hx-get={ "/exercises/" + d.Exercise.Slug + "/calc" }
						hx-trigger="load, input changed delay:200ms"
						hx-target="#calc-result"
					/>
				</label>
				<div id="calc-result" class="mt-2" aria-live="polite"></div>
			</div>
		}
		if len(d.History) > 0 {
			<h2 class={ h2 }>Training max history</h2>
			<table class="mt-2 w-full text-sm">
				<thead class="text-left text-zinc-500">
					<tr><th class="py-1">When</th><th>From</th><th>To</th><th>By</th></tr>
				</thead>
				<tbody>
					for _, h := range d.History {
						<tr class="border-t border-zinc-200 dark:border-zinc-800">
							<td class="py-1">{ h.CreatedAt.Format("2006-01-02 15:04") }</td>
							<td>@optionalWeight(h.OldKg, p.Identity.User.Unit)</td>
							<td>@optionalWeight(h.NewKg, p.Identity.User.Unit)</td>
							<td>{ h.Source }</td>
						</tr>
					}
				</tbody>
			</table>
		}
		<h2 class={ h2 }>Alternatives</h2>
		<p class={ hint }>Exercises to use when the equipment for this one is taken.</p>
		<ul class="mt-2 space-y-1">
			for _, a := range d.Alternatives {
				<li class="flex items-center justify-between gap-2">
					<a href={ templ.URL("/exercises/" + a.Exercise.Slug) } class="underline">{ a.Exercise.Name }</a>
					if a.UserAdded {
						<form method="post" action={ templ.URL("/exercises/" + d.Exercise.Slug + "/alternatives/" + a.Exercise.Slug + "/delete") }>
							@CSRF(p)
							<button type="submit" class="text-sm text-zinc-500 hover:underline">Remove</button>
						</form>
					}
				</li>
			}
		</ul>
		<form method="post" action={ templ.URL("/exercises/" + d.Exercise.Slug + "/alternatives") } class="mt-3 flex gap-2">
			@CSRF(p)
			<select name="alternative" aria-label="Add alternative" class={ input + " mt-0" }>
				for _, c := range d.Candidates {
					<option value={ c.Slug }>{ c.Name }</option>
				}
			</select>
			<button type="submit" class={ btnSecondary }>Add</button>
		</form>
		@fieldError(d.Errors, "alternative")
	}
}

templ optionalWeight(kg *float64, unit string) {
	if kg == nil {
		<span class="text-zinc-500">none</span>
	} else {
		{ Weight(*kg, unit) }
	}
}

templ CalcResultFragment(unit string, r CalcResult) {
	if r.Error != "" {
		<p class={ errorText }>{ r.Error }</p>
	} else {
		<p>{ r.PctText }% of { Weight(r.TMKg, unit) } = <strong>{ Weight(r.Kg, unit) }</strong></p>
		if len(r.PerSide) > 0 {
			<p class="text-sm text-zinc-500">Per side: { plates(r.PerSide) } { r.Equipment.Spec.Unit } on { r.Equipment.Name }</p>
		}
	}
}

templ ExerciseFormPage(p Page, f ExerciseForm) {
	@Layout(p) {
		<h1 class={ h1 }>
			if f.New {
				New exercise
			} else {
				Edit { f.Input.Name }
			}
		</h1>
		<form
			method="post"
			if f.New {
				action="/exercises"
			} else {
				action={ templ.URL("/exercises/" + f.Input.Slug) }
			}
			class="mt-4 space-y-4"
		>
			@CSRF(p)
			<label class={ label }>
				Name
				<input type="text" name="name" value={ f.Input.Name } required class={ input }/>
				@fieldError(f.Errors, "name")
			</label>
			if f.New {
				<label class={ label }>
					Slug
					<input type="text" name="slug" value={ f.Input.Slug } required pattern="[a-z0-9]+(-[a-z0-9]+)*" class={ input }/>
					<p class={ hint }>Permanent ID used by plans, like zercher-squat.</p>
					@fieldError(f.Errors, "slug")
				</label>
			}
			<div class="grid gap-4 sm:grid-cols-2">
				<label class={ label }>
					Recorded as
					<select name="measurement" class={ input }>
						for _, m := range exercise.Measurements {
							<option value={ m } selected?={ f.Input.Measurement == m }>{ MeasurementLabel(m) }</option>
						}
					</select>
					@fieldError(f.Errors, "measurement")
				</label>
				<label class={ label }>
					Equipment
					<select name="equipment_kind" class={ input }>
						for _, k := range calc.Kinds {
							<option value={ k } selected?={ f.Input.EquipmentKind == k }>{ Label(k) }</option>
						}
					</select>
					@fieldError(f.Errors, "equipment_kind")
				</label>
			</div>
			@muscleChoice("Primary muscles", "primary_muscles", f.Input.PrimaryMuscles, f.Errors)
			@muscleChoice("Secondary muscles", "secondary_muscles", f.Input.SecondaryMuscles, f.Errors)
			<label class={ label }>
				Other names
				<input type="text" name="aliases" value={ f.AliasesText } class={ input }/>
				<p class={ hint }>Comma-separated. Used by search.</p>
			</label>
			<div class="flex gap-2">
				<button type="submit" class={ btn }>Save</button>
				if f.New {
					<a href="/exercises" class={ btnSecondary }>Cancel</a>
				} else {
					<a href={ templ.URL("/exercises/" + f.Input.Slug) } class={ btnSecondary }>Cancel</a>
				}
			</div>
		</form>
	}
}

templ muscleChoice(title, name string, selected []string, errs map[string]string) {
	<fieldset>
		<legend class={ label }>{ title }</legend>
		<div class="mt-1 grid grid-cols-2 gap-1 text-sm sm:grid-cols-4">
			for _, m := range exercise.Muscles {
				<label class="flex items-center gap-1">
					<input type="checkbox" name={ name } value={ m } checked?={ has(selected, m) }/>
					{ Label(m) }
				</label>
			}
		</div>
		@fieldError(errs, name)
	</fieldset>
}
```

`internal/web/views/equipment.templ`:
```templ
package views

import (
	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
)

templ EquipmentList(p Page, items []store.Equipment) {
	@Layout(p) {
		<h1 class={ h1 }>Equipment</h1>
		<p class="mt-1 text-sm text-zinc-500">
			Calculated weights are rounded down to what these can load. Exercises use the
			default profile for their kind unless linked to another one.
		</p>
		if len(items) == 0 {
			<p class="mt-4">No equipment yet. Weights round to 0.5 kg / 1 lb.</p>
		}
		<ul class="mt-4 space-y-2">
			for _, e := range items {
				<li class={ card + " flex items-start justify-between gap-4" }>
					<div>
						<p class="font-medium">
							{ e.Name }
							<span class={ badge }>{ Label(e.Spec.Kind) }</span>
							if e.IsDefault {
								<span class={ badge }>default</span>
							}
						</p>
						<p class="text-sm text-zinc-500">{ EquipmentSummary(e) }</p>
					</div>
					<a href={ templ.URL("/equipment/" + e.ID + "/edit") } class={ btnSecondary }>Edit</a>
				</li>
			}
		</ul>
		<h2 class={ h2 }>Add equipment</h2>
		<div class="mt-2 flex flex-wrap gap-2">
			for _, k := range calc.Kinds {
				<a href={ templ.URL("/equipment/new?kind=" + k) } class={ btnSecondary }>{ Label(k) }</a>
			}
		</div>
	}
}

templ EquipmentFormPage(p Page, f EquipmentForm) {
	@Layout(p) {
		<h1 class={ h1 }>
			if f.ID == "" {
				New { Label(f.Kind) }
			} else {
				Edit { f.Name }
			}
		</h1>
		<form
			method="post"
			if f.ID == "" {
				action="/equipment"
			} else {
				action={ templ.URL("/equipment/" + f.ID) }
			}
			class="mt-4 space-y-4"
		>
			@CSRF(p)
			<input type="hidden" name="kind" value={ f.Kind }/>
			<div class="grid gap-4 sm:grid-cols-2">
				<label class={ label }>
					Name
					<input type="text" name="name" value={ f.Name } required class={ input }/>
					@fieldError(f.Errors, "name")
				</label>
				<label class={ label }>
					Unit
					<select name="unit" class={ input }>
						<option value="kg" selected?={ f.Unit == "kg" }>kg</option>
						<option value="lb" selected?={ f.Unit == "lb" }>lb</option>
					</select>
				</label>
			</div>
			switch f.Kind {
				case calc.KindBarbell:
					<div class="grid gap-4 sm:grid-cols-2">
						<label class={ label }>
							Bar weight
							<input type="text" inputmode="decimal" name="bar" value={ f.Bar } class={ input }/>
							@fieldError(f.Errors, "bar")
						</label>
						<label class={ label }>
							Plates
							<input type="text" name="plates" value={ f.Plates } class={ input }/>
							<p class={ hint }>Sizes you have, like 25, 20, 15, 10, 5, 2.5, 1.25</p>
							@fieldError(f.Errors, "plates")
						</label>
					</div>
					<label class={ label }>
						Limited plates (optional)
						<input type="text" name="plate_pairs" value={ f.PlatePairs } class={ input }/>
						<p class={ hint }>Size and number of pairs for sizes you have few of, like 1.25:1, 2.5:2</p>
						@fieldError(f.Errors, "plate_pairs")
					</label>
				case calc.KindDumbbell:
					<label class={ label }>
						Dumbbell weights
						<input type="text" name="weights" value={ f.Weights } class={ input }/>
						<p class={ hint }>Comma-separated; ranges like 2-50/2 mean 2, 4, … 50.</p>
						@fieldError(f.Errors, "weights")
					</label>
				case calc.KindMachine, calc.KindCable:
					<label class={ label }>
						Weight stack
						<input type="text" name="stack" value={ f.Stack } class={ input }/>
						<p class={ hint }>Comma-separated; ranges like 5-100/5 mean 5, 10, … 100.</p>
						@fieldError(f.Errors, "stack")
					</label>
				default:
					<p class={ hint }>Bodyweight loads round to 0.5 kg or 1 lb.</p>
			}
			@fieldError(f.Errors, "config")
			<label class="flex items-center gap-2 text-sm">
				<input type="checkbox" name="is_default" value="1" checked?={ f.IsDefault }/>
				Default for { Label(f.Kind) } exercises
			</label>
			<div class="flex gap-2">
				<button type="submit" class={ btn }>Save</button>
				<a href="/equipment" class={ btnSecondary }>Cancel</a>
			</div>
		</form>
		if f.ID != "" {
			<form method="post" action={ templ.URL("/equipment/" + f.ID + "/delete") } class="mt-8">
				@CSRF(p)
				<button type="submit" class={ btnDanger }>Delete</button>
			</form>
		}
	}
}
```

`internal/web/views/settings.templ`:
```templ
package views

import "strconv"

templ SettingsPage(p Page, saved bool, errMsg string) {
	@Layout(p) {
		<h1 class={ h1 }>Settings</h1>
		if saved {
			<p class="mt-2 text-sm text-green-700 dark:text-green-400">Saved.</p>
		}
		<form method="post" action="/settings" class="mt-4 max-w-sm space-y-4">
			@CSRF(p)
			<label class={ label }>
				Units
				<select name="unit" class={ input }>
					<option value="kg" selected?={ p.Identity.User.Unit == "kg" }>Kilograms (kg)</option>
					<option value="lb" selected?={ p.Identity.User.Unit == "lb" }>Pounds (lb)</option>
				</select>
				<p class={ hint }>Weights are stored exactly; this only changes how they are shown and entered.</p>
			</label>
			<label class={ label }>
				e1RM window (days)
				<input type="number" name="e1rm_window_days" min="7" max="365" value={ strconv.Itoa(p.Identity.User.E1RMWindowDays) } class={ input }/>
				<p class={ hint }>RPE-based weights use your best estimated 1RM from this many recent days.</p>
			</label>
			if errMsg != "" {
				<p class={ errorText }>{ errMsg }</p>
			}
			<button type="submit" class={ btn }>Save</button>
		</form>
	}
}
```

- [ ] **Step 5: Write the handlers and wiring**

Replace `internal/web/server.go` with:
```go
// Package web wires HTTP routes, middleware, and page handlers.
package web

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// Server holds the dependencies of the HTTP handlers.
type Server struct {
	DB       *store.DB
	Sessions *auth.Sessions
	OIDC     *auth.OIDC // nil when only the dev bypass is configured
	DevUser  string     // non-empty enables the dev login bypass

	Exercises *exercise.Service
	Account   *account.Service
}

// Routes returns the application's HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, logRequests, s.recoverer, s.Sessions.Middleware)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})

	r.Get("/healthz", s.healthz)
	r.Handle("/static/*", staticHandler())
	r.Get("/auth/signed-out", func(w http.ResponseWriter, r *http.Request) {
		render(w, r, http.StatusOK, views.SignedOut(page(r, "Signed out")))
	})
	if s.OIDC != nil {
		r.Get("/auth/login", s.OIDC.Login)
		r.Get("/auth/callback", s.OIDC.Callback)
	} else {
		r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		})
	}

	r.Group(func(r chi.Router) {
		if s.DevUser != "" {
			r.Use(auth.DevLogin(s.Sessions, s.DevUser))
		}
		r.Use(auth.RequireUser, auth.CSRF, s.starterEquipment)
		r.Get("/", s.home)
		r.Post("/auth/logout", s.logout)
		s.exerciseRoutes(r)
		s.equipmentRoutes(r)
		r.Get("/settings", s.settings)
		r.Post("/settings", s.saveSettings)
	})
	return r
}

// page builds the common page data for r.
func page(r *http.Request, title string) views.Page {
	p := views.Page{Title: title}
	if id, ok := auth.FromContext(r.Context()); ok {
		p.Identity = &id
	}
	return p
}

func render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		slog.ErrorContext(r.Context(), "render", "err", err, "request_id", middleware.GetReqID(r.Context()))
	}
}

func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	render(w, r, status, views.Error(page(r, http.StatusText(status)), status, message, middleware.GetReqID(r.Context())))
}

// recoverer turns panics into a logged 500 page carrying the request ID.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				slog.ErrorContext(r.Context(), "panic", "value", v, "request_id", middleware.GetReqID(r.Context()))
				s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		slog.InfoContext(r.Context(), "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()))
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.DB.Ping(r.Context()); err != nil {
		slog.ErrorContext(r.Context(), "healthz", "err", err)
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.Home(page(r, "Home")))
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.Sessions.End(w, r); err != nil {
		slog.ErrorContext(r.Context(), "logout", "err", err)
	}
	http.Redirect(w, r, "/auth/signed-out", http.StatusSeeOther)
}

// user returns the signed-in user. Only call it behind auth.RequireUser.
func user(r *http.Request) store.User {
	id, _ := auth.FromContext(r.Context())
	return id.User
}

// fail renders the error page for err: 404 for missing (or other users')
// resources, 500 otherwise.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound, "Not found.")
		return
	}
	slog.ErrorContext(r.Context(), "request failed", "err", err, "request_id", middleware.GetReqID(r.Context()))
	s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
}

// starterEquipment creates a new user's starter equipment on their first request.
func (s *Server) starterEquipment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.Exercises.EnsureStarterEquipment(r.Context(), user(r)); err != nil {
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

`internal/web/exercises.go` (the training max accepts a decimal comma: `142,5`):
```go
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) exerciseRoutes(r chi.Router) {
	r.Get("/exercises", s.exerciseList)
	r.Get("/exercises/new", s.exerciseNew)
	r.Post("/exercises", s.exerciseCreate)
	r.Get("/exercises/{slug}", s.exerciseDetail)
	r.Get("/exercises/{slug}/edit", s.exerciseEdit)
	r.Post("/exercises/{slug}", s.exerciseUpdate)
	r.Post("/exercises/{slug}/delete", s.exerciseDelete)
	r.Post("/exercises/{slug}/settings", s.exerciseSettings)
	r.Get("/exercises/{slug}/calc", s.exerciseCalc)
	r.Post("/exercises/{slug}/alternatives", s.alternativeAdd)
	r.Post("/exercises/{slug}/alternatives/{alt}/delete", s.alternativeRemove)
}

func (s *Server) exerciseList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	list, err := s.Exercises.Catalog(r.Context(), user(r).ID, q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Target") == "exercise-results" {
		render(w, r, http.StatusOK, views.ExerciseResults(list))
		return
	}
	render(w, r, http.StatusOK, views.ExerciseList(page(r, "Exercises"), q, list))
}

// exerciseDetailData loads everything the exercise page shows.
func (s *Server) exerciseDetailData(r *http.Request, slug string) (views.ExerciseDetail, error) {
	ctx, u := r.Context(), user(r)
	ex, err := s.Exercises.Get(ctx, u.ID, slug)
	if err != nil {
		return views.ExerciseDetail{}, err
	}
	d := views.ExerciseDetail{Exercise: ex, Errors: map[string]string{}}
	if d.Settings, err = s.Exercises.Settings(ctx, u.ID, ex); err != nil {
		return d, err
	}
	if d.Equipment, err = s.Exercises.ListEquipment(ctx, u.ID); err != nil {
		return d, err
	}
	if d.Alternatives, err = s.Exercises.Alternatives(ctx, u.ID, slug); err != nil {
		return d, err
	}
	if d.History, err = s.Exercises.TrainingMaxHistory(ctx, u.ID, slug); err != nil {
		return d, err
	}
	catalog, err := s.Exercises.Catalog(ctx, u.ID, "")
	if err != nil {
		return d, err
	}
	taken := map[string]bool{slug: true}
	for _, a := range d.Alternatives {
		taken[a.Exercise.Slug] = true
	}
	for _, c := range catalog {
		if !taken[c.Slug] {
			d.Candidates = append(d.Candidates, c)
		}
	}
	if tm := d.Settings.TrainingMaxKg; tm != nil {
		d.TMInput = exercise.FormatNumber(calc.FromKg(*tm, u.Unit))
	}
	return d, nil
}

func (s *Server) exerciseDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.exerciseDetailData(r, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
}

func (s *Server) exerciseNew(w http.ResponseWriter, r *http.Request) {
	f := views.ExerciseForm{New: true, Input: exercise.Input{Measurement: "weight_reps", EquipmentKind: calc.KindBarbell}}
	render(w, r, http.StatusOK, views.ExerciseFormPage(page(r, "New exercise"), f))
}

func (s *Server) exerciseEdit(w http.ResponseWriter, r *http.Request) {
	ex, err := s.Exercises.Get(r.Context(), user(r).ID, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f := views.ExerciseForm{
		Input: exercise.Input{Slug: ex.Slug, Name: ex.Name, Measurement: ex.Measurement, EquipmentKind: ex.EquipmentKind,
			PrimaryMuscles: ex.PrimaryMuscles, SecondaryMuscles: ex.SecondaryMuscles, Aliases: ex.Aliases},
		AliasesText: strings.Join(ex.Aliases, ", "),
	}
	render(w, r, http.StatusOK, views.ExerciseFormPage(page(r, "Edit "+ex.Name), f))
}

func exerciseInput(r *http.Request, slug string) (exercise.Input, string) {
	_ = r.ParseForm()
	aliases := r.PostForm.Get("aliases")
	return exercise.Input{
		Slug:             slug,
		Name:             r.PostForm.Get("name"),
		Measurement:      r.PostForm.Get("measurement"),
		EquipmentKind:    r.PostForm.Get("equipment_kind"),
		PrimaryMuscles:   r.PostForm["primary_muscles"],
		SecondaryMuscles: r.PostForm["secondary_muscles"],
		Aliases:          strings.Split(aliases, ","),
	}, aliases
}

func (s *Server) exerciseCreate(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	in, aliases := exerciseInput(r, strings.TrimSpace(r.PostForm.Get("slug")))
	ex, err := s.Exercises.Create(r.Context(), user(r).ID, in)
	s.afterExerciseSave(w, r, views.ExerciseForm{New: true, Input: in, AliasesText: aliases}, ex, err)
}

func (s *Server) exerciseUpdate(w http.ResponseWriter, r *http.Request) {
	in, aliases := exerciseInput(r, chi.URLParam(r, "slug"))
	ex, err := s.Exercises.Update(r.Context(), user(r).ID, in)
	s.afterExerciseSave(w, r, views.ExerciseForm{Input: in, AliasesText: aliases}, ex, err)
}

func (s *Server) afterExerciseSave(w http.ResponseWriter, r *http.Request, f views.ExerciseForm, ex store.Exercise, err error) {
	var fe exercise.FieldErrors
	switch {
	case errors.As(err, &fe):
		f.Errors = fe
		render(w, r, http.StatusUnprocessableEntity, views.ExerciseFormPage(page(r, "Exercise"), f))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/exercises/"+ex.Slug, http.StatusSeeOther)
	}
}

func (s *Server) exerciseDelete(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.Exercises.Delete(r.Context(), user(r).ID, slug); err != nil {
		s.fail(w, r, err)
		return
	}
	// A reset seeded exercise still exists; a deleted custom one does not.
	if _, err := s.Exercises.Get(r.Context(), user(r).ID, slug); err == nil {
		http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/exercises", http.StatusSeeOther)
}

func (s *Server) exerciseSettings(w http.ResponseWriter, r *http.Request) {
	ctx, u, slug := r.Context(), user(r), chi.URLParam(r, "slug")
	_ = r.ParseForm()
	tmText := strings.TrimSpace(r.PostForm.Get("training_max"))
	var tm *float64
	errs := map[string]string{}
	if tmText != "" {
		v, err := strconv.ParseFloat(strings.ReplaceAll(tmText, ",", "."), 64)
		if err != nil {
			errs["training_max"] = "enter a number, or leave empty for none"
		} else {
			kg := calc.ToKg(v, u.Unit)
			tm = &kg
		}
	}
	if len(errs) == 0 {
		err := s.Exercises.LinkEquipment(ctx, u.ID, slug, r.PostForm.Get("equipment_id"))
		if err == nil {
			err = s.Exercises.SetTrainingMax(ctx, u.ID, slug, tm, "web")
		}
		var fe exercise.FieldErrors
		switch {
		case errors.As(err, &fe):
			errs = fe
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
			return
		}
	}
	d, err := s.exerciseDetailData(r, slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d.Errors, d.TMInput = errs, tmText
	render(w, r, http.StatusUnprocessableEntity, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
}

func (s *Server) exerciseCalc(w http.ResponseWriter, r *http.Request) {
	ctx, u := r.Context(), user(r)
	ex, err := s.Exercises.Get(ctx, u.ID, chi.URLParam(r, "slug"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pctText := strings.TrimSpace(r.URL.Query().Get("pct"))
	pct, err := strconv.ParseFloat(pctText, 64)
	if err != nil || pct <= 0 || pct > 150 {
		render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, views.CalcResult{Error: "Enter a percentage between 0 and 150."}))
		return
	}
	res, err := s.Exercises.PercentOfTM(ctx, u, ex, pct/100)
	if errors.Is(err, exercise.ErrNoTrainingMax) {
		render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, views.CalcResult{Error: "Set a training max first."}))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	st, err := s.Exercises.Settings(ctx, u.ID, ex)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := views.CalcResult{PctText: exercise.FormatNumber(pct), TMKg: *st.TrainingMaxKg, Kg: res.Kg, PerSide: res.PerSide}
	if st.Equipment != nil {
		out.Equipment = *st.Equipment
	}
	render(w, r, http.StatusOK, views.CalcResultFragment(u.Unit, out))
}

func (s *Server) alternativeAdd(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	err := s.Exercises.AddAlternative(r.Context(), user(r).ID, slug, r.PostFormValue("alternative"))
	var fe exercise.FieldErrors
	if errors.As(err, &fe) {
		d, derr := s.exerciseDetailData(r, slug)
		if derr != nil {
			s.fail(w, r, derr)
			return
		}
		d.Errors = fe
		render(w, r, http.StatusUnprocessableEntity, views.ExerciseDetailPage(page(r, d.Exercise.Name), d))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
}

func (s *Server) alternativeRemove(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if err := s.Exercises.RemoveAlternative(r.Context(), user(r).ID, slug, chi.URLParam(r, "alt")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/exercises/"+slug, http.StatusSeeOther)
}
```

`internal/web/equipment.go`:
```go
package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) equipmentRoutes(r chi.Router) {
	r.Get("/equipment", s.equipmentList)
	r.Get("/equipment/new", s.equipmentNew)
	r.Post("/equipment", s.equipmentSave)
	r.Get("/equipment/{id}/edit", s.equipmentEdit)
	r.Post("/equipment/{id}", s.equipmentSave)
	r.Post("/equipment/{id}/delete", s.equipmentDelete)
}

func (s *Server) equipmentList(w http.ResponseWriter, r *http.Request) {
	items, err := s.Exercises.ListEquipment(r.Context(), user(r).ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.EquipmentList(page(r, "Equipment"), items))
}

func (s *Server) equipmentNew(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	if !slices.Contains(calc.Kinds, kind) {
		s.renderError(w, r, http.StatusNotFound, "Unknown equipment kind.")
		return
	}
	f := views.EquipmentForm{Kind: kind, Unit: user(r).Unit, Name: views.Label(kind)}
	render(w, r, http.StatusOK, views.EquipmentFormPage(page(r, "New equipment"), f))
}

func (s *Server) equipmentEdit(w http.ResponseWriter, r *http.Request) {
	e, err := s.Exercises.GetEquipment(r.Context(), user(r).ID, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := e.Spec.Config
	f := views.EquipmentForm{
		ID: e.ID, Name: e.Name, Kind: e.Spec.Kind, Unit: e.Spec.Unit, IsDefault: e.IsDefault,
		Plates: exercise.FormatWeights(c.Plates), PlatePairs: exercise.FormatPlatePairs(c.PlatePairs),
		Weights: exercise.FormatWeights(c.Weights), Stack: exercise.FormatWeights(c.Stack),
	}
	if e.Spec.Kind == calc.KindBarbell {
		f.Bar = exercise.FormatNumber(c.Bar)
	}
	if len(c.Stack) == 0 && c.Step > 0 {
		// Profiles created with min/step/max are edited as a stack list.
		f.Stack = exercise.FormatNumber(c.Min) + "-" + exercise.FormatNumber(c.Max) + "/" + exercise.FormatNumber(c.Step)
	}
	render(w, r, http.StatusOK, views.EquipmentFormPage(page(r, "Edit "+e.Name), f))
}

// equipmentFromForm parses the form into a profile, collecting parse errors.
func equipmentFromForm(r *http.Request) (views.EquipmentForm, store.Equipment, exercise.FieldErrors) {
	_ = r.ParseForm()
	f := views.EquipmentForm{
		ID:         chi.URLParam(r, "id"),
		Name:       r.PostForm.Get("name"),
		Kind:       r.PostForm.Get("kind"),
		Unit:       r.PostForm.Get("unit"),
		IsDefault:  r.PostForm.Get("is_default") != "",
		Bar:        r.PostForm.Get("bar"),
		Plates:     r.PostForm.Get("plates"),
		PlatePairs: r.PostForm.Get("plate_pairs"),
		Weights:    r.PostForm.Get("weights"),
		Stack:      r.PostForm.Get("stack"),
	}
	e := store.Equipment{ID: f.ID, UserID: user(r).ID, Name: f.Name, IsDefault: f.IsDefault,
		Spec: calc.Equipment{Kind: f.Kind, Unit: f.Unit}}
	errs := exercise.FieldErrors{}
	list := func(field, text string) []float64 {
		vs, err := exercise.ParseWeights(text)
		if err != nil {
			errs[field] = err.Error()
		}
		return vs
	}
	c := &e.Spec.Config
	switch f.Kind {
	case calc.KindBarbell:
		bar, err := strconv.ParseFloat(strings.TrimSpace(f.Bar), 64)
		if err != nil || bar < 0 {
			errs["bar"] = "enter the bar weight"
		}
		c.Bar = bar
		c.Plates = list("plates", f.Plates)
		pairs, err := exercise.ParsePlatePairs(f.PlatePairs)
		if err != nil {
			errs["plate_pairs"] = err.Error()
		}
		c.PlatePairs = pairs
	case calc.KindDumbbell:
		c.Weights = list("weights", f.Weights)
	case calc.KindMachine, calc.KindCable:
		c.Stack = list("stack", f.Stack)
	}
	return f, e, errs
}

func (s *Server) equipmentSave(w http.ResponseWriter, r *http.Request) {
	f, e, errs := equipmentFromForm(r)
	if len(errs) == 0 {
		_, err := s.Exercises.SaveEquipment(r.Context(), e)
		var fe exercise.FieldErrors
		switch {
		case errors.As(err, &fe):
			errs = fe
		case err != nil:
			s.fail(w, r, err)
			return
		default:
			http.Redirect(w, r, "/equipment", http.StatusSeeOther)
			return
		}
	}
	f.Errors = errs
	render(w, r, http.StatusUnprocessableEntity, views.EquipmentFormPage(page(r, "Equipment"), f))
}

func (s *Server) equipmentDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Exercises.DeleteEquipment(r.Context(), user(r).ID, chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/equipment", http.StatusSeeOther)
}
```

`internal/web/settings.go`:
```go
package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.SettingsPage(page(r, "Settings"), r.URL.Query().Has("saved"), ""))
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.PostFormValue("e1rm_window_days"))
	err := s.Account.UpdateSettings(r.Context(), user(r).ID, r.PostFormValue("unit"), days)
	if errors.Is(err, account.ErrInvalidSettings) {
		render(w, r, http.StatusUnprocessableEntity, views.SettingsPage(page(r, "Settings"), false, err.Error()))
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings?saved", http.StatusSeeOther)
}
```

Replace `cmd/onerep/main.go` with (the catalog is seeded on every start, after migrations):
```go
// Command onerep is the onerep server and its maintenance subcommands.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LongerHV/onerep/internal/account"
	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/config"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web"
)

const usage = `usage: onerep [command]

commands:
  serve          run the web server (default)
  migrate        apply database migrations and exit
  backup <path>  write a consistent copy of the database to <path>
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cmd := "serve"
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	setupLogging(cfg)

	switch cmd {
	case "serve":
		return serve(ctx, cfg)
	case "migrate":
		return store.Migrate(cfg.DBPath)
	case "backup":
		if len(args) != 1 {
			return errors.New("backup: expected exactly one destination path")
		}
		// store.Open creates missing files; a backup must never back up an empty new database.
		if _, err := os.Stat(cfg.DBPath); err != nil {
			return fmt.Errorf("backup: database %s: %w", cfg.DBPath, err)
		}
		db, err := store.Open(ctx, cfg.DBPath)
		if err != nil {
			return err
		}
		defer db.Close()
		return db.Backup(ctx, args[0])
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func setupLogging(cfg config.Config) {
	var h slog.Handler
	if cfg.Dev() {
		h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		h = slog.NewJSONHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(h))
}

func serve(ctx context.Context, cfg config.Config) error {
	if cfg.AutoMigrate {
		if err := store.Migrate(cfg.DBPath); err != nil {
			return err
		}
	}
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := exercise.Seed(ctx, db); err != nil {
		return err
	}

	sessions := &auth.Sessions{Store: db, Secure: cfg.SecureCookies()}
	srv := &web.Server{
		DB:        db,
		Sessions:  sessions,
		Exercises: &exercise.Service{Store: db},
		Account:   &account.Service{Store: db},
	}
	if cfg.OIDC.Issuer != "" {
		srv.OIDC, err = auth.NewOIDC(ctx, cfg.OIDC.Issuer, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, cfg.BaseURL, sessions)
		if err != nil {
			return err
		}
	}
	if cfg.DevUser != "" {
		slog.Warn("DEV LOGIN BYPASS ENABLED: every visitor is signed in as " + cfg.DevUser)
		srv.DevUser = cfg.DevUser
	}

	go cleanupSessions(ctx, db)

	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen, "base_url", cfg.BaseURL)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

// cleanupSessions deletes expired auth sessions once an hour.
func cleanupSessions(ctx context.Context, db *store.DB) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := db.DeleteExpiredAuthSessions(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("cleanup sessions", "err", err)
		} else if n > 0 {
			slog.Info("cleanup sessions", "deleted", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
```

- [ ] **Step 6: Generate and run the tests**

Run: `task generate && go vet ./... && go test ./internal/web/ ./cmd/...`
Expected: both `ok`.

- [ ] **Step 7: Smoke test**

```bash
go build -o bin/onerep ./cmd/onerep
export ONEREP_ENV=dev ONEREP_DEV_USER=alice ONEREP_DB=bin/smoke.db ONEREP_LISTEN=127.0.0.1:18090
./bin/onerep serve & sleep 1
for p in / /exercises /exercises/barbell-back-squat /exercises/new /equipment "/equipment/new?kind=barbell" /settings; do
  curl -s -o /dev/null -w "%{http_code} $p\n" -b bin/jar -c bin/jar "http://127.0.0.1:18090$p"
done                                           # → 200 for every path
kill %1; wait
./bin/onerep serve & sleep 1; kill %1; wait    # second start re-syncs the seed without errors
sqlite3 bin/smoke.db "select count(*), sum(hidden) from exercises where user_id is null"   # → 116|0
rm -f bin/smoke.db* bin/jar
```

- [ ] **Step 8: Commit**

```bash
git add internal/web cmd/onerep
git commit -m "feat(web): add exercise catalog, equipment and settings pages"
```

---

### Task 10: Docs and full CI

**Files:**
- Replace: `AGENTS.md`

**Interfaces:**
- Consumes: everything above
- Produces: updated agent guidance (layout, calc parity rule, seed rule, `task test:js`)

- [ ] **Step 1: Update AGENTS.md**

Replace `AGENTS.md` with:
````markdown
# AGENTS.md

Guidance for AI coding agents (and humans) working on onerep. `CLAUDE.md` is a symlink to this file.

## What this is

onerep is a self-hosted gym progress tracker: a single Go binary serving a PWA (templ + htmx + Tailwind), SQLite storage, OIDC login, and an MCP server so an AI can analyse history and draft training plans.

- Design spec: `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. Read it before changing behaviour.
- Implementation plans: `docs/superpowers/plans/`. One plan per milestone.

## Environment

All tools come from the Nix flake. Run everything inside `nix develop` (or `direnv allow`). Don't install tools globally or add a Node toolchain to the build. Node is only there for the JS test runner.

The dev shell sets `CGO_ENABLED=0` and `ONEREP_ENV=dev`.

## Commands

| Task | Command |
|---|---|
| Live-reload dev server, signed in as `$USER`, no IdP needed | `task dev` |
| Local Dex identity provider (alice@example.com / password) | `task dex`, then `task dev:oidc` |
| Regenerate templ components and Tailwind CSS | `task generate` |
| Tests | `task test` (or `go test ./internal/<pkg>/ -run TestName`); browser JS: `task test:js` |
| Lint | `task lint` |
| Everything CI runs | `task ci` |
| New migration after editing `internal/store/schema.sql` | `task migrate:diff NAME=<snake_case>` |
| Container image (local, no push) | `task image` |

## Layout

```
cmd/onerep/            main, config wiring, subcommands (serve, migrate, backup)
internal/config/       env-var configuration
internal/store/        ALL SQL: schema.sql, generated migrations/, repositories
internal/store/storetest/  migrated temp DB for tests
internal/auth/         OIDC, cookie sessions, CSRF, dev login bypass
internal/account/      user preferences (unit, e1RM window)
internal/calc/         pure training math: RTS/e1RM, rounding to equipment, load resolution
internal/exercise/     catalog (+ embedded seed/exercises.json), equipment profiles, TM, alternatives
internal/web/          chi router, handlers, views/ (templ), static/ (embedded), jstest/ (node tests)
testdata/calc_cases.json  shared Go/JS calc test vectors
```

Later milestones add `plan/`, `training/`, `stats/`, and `mcp/` under `internal/`. See the spec, §4.

## Rules

- **Conventional Commits** for every commit and PR title: `feat(auth): ...`, `fix(store): ...`, `test: ...`, `docs: ...`, `chore(ci): ...`, `refactor: ...`. Scope is the package or area. CI checks PR titles.
- **TDD.** Write the failing test first, watch it fail, then implement.
- **SQL only in `internal/store`.** Services depend on small interfaces declared in their own package, which `*store.DB` satisfies.
- **Every user-owned query filters by `user_id`.** Another user's resource is `store.ErrNotFound`, which surfaces as 404, never 403.
- **IDs** are UUIDv7 strings. **Weights** are stored in kg. **Timestamps** are UTC TEXT in `2006-01-02T15:04:05.000Z07:00` format (`store.formatTime`).
- **Schema changes:** edit `internal/store/schema.sql`, run `task migrate:diff`, and commit both files. Never edit a generated migration or `atlas.sum` by hand. Write `NOT NULL` explicitly on TEXT primary keys, because SQLite allows NULL there otherwise.
- **Generated files are committed:** `*_templ.go` and `internal/web/static/app.css`. Run `task generate` after editing `.templ` files or `internal/web/styles/input.css`. CI fails if they're stale.
- **Tests use a real SQLite database** (`storetest.New(t)`). Don't mock the store.
- **Web handlers and MCP tools are thin adapters.** Business rules live in services so the web UI and the AI can't disagree.
- **Calc parity:** `internal/calc` (Go) and `internal/web/static/js/calc.js` implement the same math. Change both together and add a case to `testdata/calc_cases.json`; `task test` and `task test:js` both run it.
- **Seed catalog:** edit `internal/exercise/seed/exercises.json`; slugs are permanent (plans and history refer to them). Removing an entry hides it, never deletes it. `TestSeedIsValid` checks the file.
- **Every non-GET request with a session must carry the CSRF token**: the `X-CSRF-Token` header, set globally for htmx via `hx-headers`, or the `csrf_token` form field.
- **The dev login bypass** (`ONEREP_DEV_USER`) only runs with `ONEREP_ENV=dev`, and only on authenticated app routes.
- **Keep dependencies few.** Ask before adding a Go module or a vendored JS library.
- **Licensing:** onerep is AGPL-3.0-only. Dependencies must use MIT, BSD-2/3-Clause, ISC, 0BSD or Apache-2.0. `task licenses:check` enforces this for Go modules. A vendored file gets its license next to it (`<name>.LICENSE`). Keep the footer link to the source code.
````

- [ ] **Step 2: Full CI**

Commit first (`check:generated` compares against git), then run:

Run: `task ci`
Expected: exit 0. Lint reports `0 issues.`, every Go package is `ok` (account, auth, calc, config, exercise, store, web, cmd/onerep), Node prints `ℹ pass 6`, and `migrate:check` and `licenses:check` pass. No new Go modules are added in this milestone.

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: describe calc parity and seed catalog rules in AGENTS.md"
```

---

## Done when

- `task ci` passes on a clean checkout.
- In `task dev`, the user can:
  - search the catalog
  - open Barbell Back Squat, set a 140 kg training max, and see "75% of 140 kg = 105 kg" with plates 25 · 15 · 2.5
  - switch to lb and see the training max as 308.65
  - create a custom exercise, customize and reset a seeded one, and add and remove alternatives
  - edit the starter profiles and add a dumbbell profile using `2-20/2`
- A second start of the binary re-syncs the seed without errors.

Deliberate changes to the spec, all already in the spec:
- A default equipment profile per kind, and starter profiles.
- The `hidden` column on seeded exercises.
- `equipment_initialized` on users.
