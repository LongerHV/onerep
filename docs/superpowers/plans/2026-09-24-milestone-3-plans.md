# onerep Milestone 3: Plans — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Training plans as JSON documents, validated in three layers and expanded week by week with resolved weights. They're edited in a schema-aware editor with a live preview, saved as versions (drafts or active), compared and activated. The user follows one plan with a cursor (next day, skip, choose day, restart), and the home page shows the next workout.

**Architecture:** `internal/plan` is the core:
- the JSON Schema (embedded, also served at `/schema/plan.json`) and the Go types
- validation: syntax, then schema via `santhosh-tekuri/jsonschema/v6`, then semantic checks against the user's catalog
- `Expand` and `Resolve`, text descriptions and a line diff
- a `Service` over the store and the exercise service

`internal/store` adds the `plans`, `plan_versions` and `active_plan` tables. `internal/web` adds the plan pages and the home page. The editor is `vanilla-jsoneditor` (vendored) mounted over a plain textarea, which keeps working without JS. A body-size limit protects every authenticated POST.

**Tech Stack:** unchanged, plus `github.com/santhosh-tekuri/jsonschema/v6` v6.0.3 (Apache-2.0; pulls in `golang.org/x/text`, BSD-3) and `vanilla-jsoneditor` 3.13.0 (ISC, vendored `standalone.min.js` and dark theme CSS). Both were approved by the user.

**Spec:** `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. This plan implements:
- §5 plans, plan_versions and active_plan
- §6 format, rules, validation and resolution (e1RM comes from history in milestone 4; until then RPE loads show "pick weight")
- §8 active plan and cursor, except finishing a session, which is milestone 4
- §10 editor, preview, versions and diffs; MCP drafts are milestone 6
- §16 errors as JSON pointers

## Global Constraints

Everything from milestones 1 and 2 still applies. The commit trailer for agent-authored commits is `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. In addition:

- The schema (`internal/plan/plan.schema.json`) is the contract for the editor, the server and (later) the AI. Values marked "per week" are a single value or an array with exactly `weeks` entries.
- Validation layers (spec §6):
  1. syntax, reported with line and column
  2. the JSON Schema
  3. semantic checks: per-week array lengths, `only_weeks` within range, ranges not going down, no `drop_pct` on an exercise's first set line, every week has a day, slugs exist, a total of at most 20,000 sets

  Warnings (a hidden exercise, `pct_tm` without a training max, RPE loads that can't be resolved) never block saving.
- Problems are `{pointer, message, warning}` with JSON Pointers into the document.
- Stored documents are compacted. A document without `"unit"` gets the user's unit added when saved, so absolute weights keep their meaning.
- Versions: every save creates one. Saving as active supersedes the current active version. Any draft or superseded version can be activated (so rolling back works). Only drafts can be deleted.
- Cursor (spec §8): `(week, day)`, where day indexes the days that apply to that week. After the last week, the plan is complete at `(weeks+1, 0)`. Activating a version keeps the cursor if that day still exists (or the plan was complete); otherwise it resets to `(1, 0)`, and the comparison page warns about that beforehand.
- `rest_s` defaults to 120 seconds. A per-week `count` of null or 0 skips that set line for the week.
- RPE loads use the lower bound of a rep range, and AMRAP sets with an RPE load stay unresolved. `drop_pct` is taken from the previous set's resolved weight.
- Authenticated request bodies are capped at 1 MB (413 with a message).

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **A plan that multiplies out to millions of sets** (every schema maximum at once: 52 weeks × 14 days × 30 groups × 6 exercises × 20 lines × 20 sets). It must be rejected before anything expands it. Test: `TestHugePlanIsRejectedQuickly` (Task 1).
2. **Changing unit after saving a plan without `"unit"`.** 100 kg must not become 100 lb. Test: `TestSavedPlanKeepsItsUnit` (Task 4).
3. **A followed plan that refers to a custom exercise the user later deleted.** The home page must still render, falling back to the slug as the name. Test: `TestPlanSurvivesDeletedCustomExercise` (Task 4).
4. **Hand-edited "go to day" values** (`1:-1`, `0:0`, `x`, empty) must be rejected, not stored. Test: `TestChooseRejectsNegativeDay` (Task 6).
5. **A pasted multi-megabyte document on the live preview** (which runs on every keystroke) must be refused cheaply. Test: `TestOversizedPlanDocumentIsRefused` (Task 6).

Also covered: other users' plans and versions return 404, a version URL under the wrong plan returns 404, the schema is public, and the editor keeps the typed text after errors.

## File Map

```
internal/plan/plan.schema.json           JSON Schema (draft 2020-12)
internal/plan/doc.go                     Doc/Day/Group/Slot/SetLine, PerWeek[T], Reps
internal/plan/validate.go                Validate, Problems, schema error reporting, semantic checks, MaxTotalSets
internal/plan/expand.go                  DaysForWeek, Expand, Resolve, ExpandedDay/PrescribedSet
internal/plan/describe.go                NextPosition, ValidPosition, GroupSets, Prescription, DayLines
internal/plan/diff.go                    DiffLines (LCS), Changed
internal/plan/service.go                 Service: create/save/versions/activate/follow/cursor/preview/compare
internal/plan/templates/starter.json     document for "New plan"
internal/plan/testdata/upper_lower.json(+.golden.json)
internal/plan/*_test.go
internal/store/schema.sql                + plans, plan_versions, active_plan
internal/store/migrations/*_plans.{up,down}.sql, atlas.sum   generated
internal/store/plans.go(+_test)
internal/web/static/vendor/jsoneditor/{standalone.min.js,jse-theme-dark.css,LICENSE}   vendored
internal/web/static/js/plan-editor.js
internal/web/views/plans.templ, pages.templ (Home), models.go, ui.go
internal/web/plans.go, server.go, server_test.go, plans_test.go
cmd/onerep/main.go, AGENTS.md, go.mod, go.sum
```

---

### Task 1: Plan documents, validation, and expansion

**Files:**
- Create: `internal/plan/plan.schema.json`, `internal/plan/doc.go`, `internal/plan/validate.go`, `internal/plan/expand.go`
- Create: `internal/plan/testdata/upper_lower.json`
- Create (generated): `internal/plan/testdata/upper_lower.golden.json`
- Test: `internal/plan/validate_test.go`, `internal/plan/expand_test.go`

**Interfaces:**
- Consumes: `calc.ToKg`, `calc.ResolveLoad`, `calc.DropLoad`, `calc.LoadContext` (milestone 2)
- Produces:
  - Document types: `plan.Doc{Name, Unit string; Weeks int; Days []Day}`, `Day{Name; OnlyWeeks []int; Groups []Group}`, `Group{RestS PerWeek[int]; Exercises []Slot}`, `Slot{Slug; Alternatives []string; Notes; Sets []SetLine}`, `SetLine{Kind; Count PerWeek[*int]; Reps PerWeek[Reps]; DurationS PerWeek[int]; RPE PerWeek[float64]; Load *LoadPrescription}`, `LoadPrescription{Weight, PctTM, RPE, DropPct PerWeek[float64]}`
  - Per-week and reps values: `PerWeek[T]{Values []T; List bool}` with `Set()` and `At(week)`; `Reps{Min, Max int; AMRAP bool}` with `String()`
  - Validation: `plan.Schema() []byte`, `plan.Validate(raw []byte, lookup plan.Lookup) (Doc, Problems)`, `plan.Lookup` interface (`Exercise(slug) (exists, hidden bool)`, `HasTrainingMax(slug) bool`), `plan.Problem{Pointer, Message string; Warning bool}`, `plan.Problems` (an error, with `HasErrors()`), `plan.MaxTotalSets = 20000`
  - Expansion: `plan.DefaultRestS = 120`, `plan.DaysForWeek(doc, week) []int`, `plan.Expand(doc, week, day int, defaultUnit string) (ExpandedDay, bool)`, `plan.Resolve(*ExpandedDay, func(slug string) calc.LoadContext)`
  - Expanded types: `ExpandedDay{Week, Day int; Name string; Groups []ExpandedGroup}`, `ExpandedGroup{RestS int; Exercises []ExpandedSlot}`, `ExpandedSlot{Slug, Name string; Alternatives []string; Notes string; Sets []PrescribedSet}`, `PrescribedSet{Kind string; Reps Reps; DurationS int; TargetRPE float64; LoadKind string; LoadValue float64; Kg *float64; PerSide []float64}`
  - Load kinds: `plan.LoadWeight|LoadPctTM|LoadRPE|LoadDropPct`
  - Test helpers: `testCatalog` (a fake `Lookup`), `readFixture`, `exampleDoc`, `withSet`, `problemsAt`, `ptr`

- [ ] **Step 1: Add the JSON Schema dependency**

Run: `go get github.com/santhosh-tekuri/jsonschema/v6@v6.0.3`

- [ ] **Step 2: Write the schema and the example plan**

`internal/plan/plan.schema.json`:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://onerep.local/schema/plan.json",
  "title": "onerep training plan",
  "description": "A block of training weeks. Any value marked 'per week' may be a single value or an array with exactly one entry per week (entry i applies to week i+1).",
  "type": "object",
  "required": ["name", "weeks", "days"],
  "additionalProperties": false,
  "properties": {
    "name": {"type": "string", "minLength": 1, "maxLength": 100},
    "unit": {"description": "Unit of absolute weights in this plan. Defaults to the user's unit.", "enum": ["kg", "lb"]},
    "weeks": {"description": "Number of weeks in the block.", "type": "integer", "minimum": 1, "maximum": 52},
    "days": {
      "description": "Training days in the order they are done each week.",
      "type": "array", "minItems": 1, "maxItems": 14,
      "items": {"$ref": "#/$defs/day"}
    }
  },
  "$defs": {
    "day": {
      "type": "object",
      "required": ["name", "groups"],
      "additionalProperties": false,
      "properties": {
        "name": {"type": "string", "minLength": 1, "maxLength": 60},
        "only_weeks": {
          "description": "Restrict this day to these weeks (1-based).",
          "type": "array", "minItems": 1, "uniqueItems": true,
          "items": {"type": "integer", "minimum": 1, "maximum": 52}
        },
        "groups": {"type": "array", "maxItems": 30, "items": {"$ref": "#/$defs/group"}}
      }
    },
    "group": {
      "description": "Exercises done together. More than one exercise is a superset: A1, B1, A2, B2, ...",
      "type": "object",
      "required": ["exercises"],
      "additionalProperties": false,
      "properties": {
        "rest_s": {"description": "Rest in seconds after each round (per week). Default 120.", "$ref": "#/$defs/seconds"},
        "exercises": {"type": "array", "minItems": 1, "maxItems": 6, "items": {"$ref": "#/$defs/slot"}}
      }
    },
    "slot": {
      "type": "object",
      "required": ["slug", "sets"],
      "additionalProperties": false,
      "properties": {
        "slug": {"$ref": "#/$defs/slug"},
        "alternatives": {"type": "array", "uniqueItems": true, "items": {"$ref": "#/$defs/slug"}},
        "notes": {"type": "string", "maxLength": 500},
        "sets": {"type": "array", "minItems": 1, "maxItems": 20, "items": {"$ref": "#/$defs/setLine"}}
      }
    },
    "setLine": {
      "description": "One or more identical sets. Give reps or duration_s, not both.",
      "type": "object",
      "required": ["count"],
      "additionalProperties": false,
      "oneOf": [{"required": ["reps"]}, {"required": ["duration_s"]}],
      "properties": {
        "kind": {"enum": ["warmup", "working", "drop", "amrap"], "default": "working"},
        "count": {
          "description": "Number of sets (per week). In a per-week array, null or 0 skips the line that week.",
          "oneOf": [
            {"$ref": "#/$defs/count"},
            {"type": "array", "minItems": 1, "items": {"oneOf": [{"$ref": "#/$defs/count"}, {"type": "null"}]}}
          ]
        },
        "reps": {
          "description": "Reps (per week): a number, a range like \"6-10\", or \"AMRAP\".",
          "oneOf": [{"$ref": "#/$defs/reps"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/reps"}}]
        },
        "duration_s": {"description": "Duration in seconds for timed sets (per week).", "$ref": "#/$defs/seconds"},
        "rpe": {
          "description": "Informational RPE target (per week), shown alongside weight or pct_tm loads.",
          "oneOf": [{"$ref": "#/$defs/rpe"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/rpe"}}]
        },
        "load": {"$ref": "#/$defs/load"}
      }
    },
    "load": {
      "description": "How the weight is chosen. Exactly one key.",
      "type": "object",
      "minProperties": 1,
      "maxProperties": 1,
      "additionalProperties": false,
      "properties": {
        "weight": {
          "description": "Absolute weight in the plan's unit (per week).",
          "oneOf": [{"$ref": "#/$defs/weight"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/weight"}}]
        },
        "pct_tm": {
          "description": "Fraction of the exercise's training max, e.g. 0.75 (per week).",
          "oneOf": [{"$ref": "#/$defs/fraction"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/fraction"}}]
        },
        "rpe": {
          "description": "Target RPE (per week); the weight comes from the recent estimated 1RM.",
          "oneOf": [{"$ref": "#/$defs/rpe"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/rpe"}}]
        },
        "drop_pct": {
          "description": "Fraction below the previous set's weight, e.g. 0.2 (per week). Not allowed on an exercise's first set line.",
          "oneOf": [{"$ref": "#/$defs/drop"}, {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/drop"}}]
        }
      }
    },
    "count": {"type": "integer", "minimum": 0, "maximum": 20},
    "reps": {
      "oneOf": [
        {"type": "integer", "minimum": 1, "maximum": 100},
        {"type": "string", "pattern": "^([1-9][0-9]?-[1-9][0-9]?|AMRAP)$"}
      ]
    },
    "rpe": {"type": "number", "minimum": 6, "maximum": 10, "multipleOf": 0.5},
    "seconds": {
      "oneOf": [
        {"type": "integer", "minimum": 0, "maximum": 3600},
        {"type": "array", "minItems": 1, "items": {"type": "integer", "minimum": 0, "maximum": 3600}}
      ]
    },
    "weight": {"type": "number", "exclusiveMinimum": 0, "maximum": 1000},
    "fraction": {"type": "number", "exclusiveMinimum": 0, "maximum": 1.5},
    "drop": {"type": "number", "exclusiveMinimum": 0, "exclusiveMaximum": 1},
    "slug": {"type": "string", "pattern": "^[a-z0-9]+(-[a-z0-9]+)*$", "maxLength": 64}
  }
}
```

`internal/plan/testdata/upper_lower.json` (uses seed slugs; exercises per-week arrays, `only_weeks`, supersets, drop sets, ranges, AMRAP and timed sets):
```json
{
  "name": "Upper/Lower RPE block",
  "unit": "kg",
  "weeks": 4,
  "days": [
    {
      "name": "Upper A",
      "groups": [
        {"rest_s": 180, "exercises": [
          {"slug": "barbell-bench-press",
           "alternatives": ["dumbbell-bench-press"],
           "notes": "pause first rep",
           "sets": [
             {"kind": "warmup", "count": 2, "reps": 5, "load": {"pct_tm": 0.5}},
             {"count": [3, 3, 4, 2], "reps": 5, "load": {"rpe": [7, 8, 9, 6]}},
             {"kind": "drop", "count": [1, 1, 1, null], "reps": 10, "load": {"drop_pct": 0.2}}
           ]}]},
        {"rest_s": [90, 90, 90, 60], "exercises": [
          {"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10", "rpe": 8}]},
          {"slug": "cable-lateral-raise", "sets": [{"count": 3, "reps": 15, "load": {"weight": 7.5}}]}
        ]}
      ]
    },
    {
      "name": "Lower A",
      "groups": [
        {"exercises": [
          {"slug": "barbell-back-squat", "sets": [{"count": [5, 4, 3, 2], "reps": [5, 4, 3, 5], "load": {"pct_tm": [0.75, 0.8, 0.85, 0.6]}}]}
        ]},
        {"exercises": [{"slug": "plank", "sets": [{"count": 3, "duration_s": [45, 50, 55, 30]}]}]}
      ]
    },
    {"name": "Deload extra", "only_weeks": [4], "groups": [
      {"exercises": [{"slug": "dumbbell-curl", "sets": [{"count": 2, "reps": "AMRAP", "load": {"weight": 10}}]}]}
    ]}
  ]
}
```

- [ ] **Step 3: Write the failing tests**

`internal/plan/validate_test.go`:
```go
package plan

import (
	"os"
	"strings"
	"testing"
	"time"
)

// fakeLookup knows a fixed testCatalog: slug -> hidden, plus which have a TM.
type fakeLookup struct {
	exercises map[string]bool
	tms       map[string]bool
}

func (f fakeLookup) Exercise(slug string) (bool, bool) {
	hidden, ok := f.exercises[slug]
	return ok, hidden
}
func (f fakeLookup) HasTrainingMax(slug string) bool { return f.tms[slug] }

var testCatalog = fakeLookup{
	exercises: map[string]bool{
		"barbell-bench-press": false, "dumbbell-bench-press": false, "pull-up": false,
		"cable-lateral-raise": false, "barbell-back-squat": false, "plank": false,
		"dumbbell-curl": false, "old-press": true,
	},
	tms: map[string]bool{"barbell-back-squat": true},
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestValidExampleHasOnlyWarnings(t *testing.T) {
	doc, ps := Validate(readFixture(t, "upper_lower.json"), testCatalog)
	if ps.HasErrors() {
		t.Fatalf("unexpected errors: %v", ps)
	}
	if doc.Weeks != 4 || len(doc.Days) != 3 {
		t.Fatalf("decoded doc: %+v", doc)
	}
	// Bench has pct_tm warm-ups but no training max in this testCatalog.
	if len(ps) != 1 || ps[0].Pointer != "/days/0/groups/0/exercises/0/sets/0/load/pct_tm" || !ps[0].Warning {
		t.Fatalf("problems = %+v", ps)
	}
}

func TestSyntaxErrorsGiveLineAndColumn(t *testing.T) {
	_, ps := Validate([]byte("{\n  \"name\": \"x\",\n  \"weeks\": ,\n}"), testCatalog)
	if len(ps) != 1 || !strings.Contains(ps[0].Message, "line 3") {
		t.Fatalf("problems = %+v", ps)
	}
}

// problemsAt returns the messages reported at pointer.
func problemsAt(ps Problems, pointer string) []string {
	var out []string
	for _, p := range ps {
		if p.Pointer == pointer {
			out = append(out, p.Message)
		}
	}
	return out
}

const minimal = `{"name": "x", "weeks": 2, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "pull-up", "sets": [%s]}]}]}]}`

func withSet(set string) []byte { return []byte(strings.Replace(minimal, "%s", set, 1)) }

func TestSchemaErrorsPointAtTheProblem(t *testing.T) {
	const setPtr = "/days/0/groups/0/exercises/0/sets/0"
	cases := []struct {
		name, doc, pointer, message string
	}{
		{"missing name", `{"weeks": 1, "days": []}`, "", "missing property 'name'"},
		{"zero weeks", `{"name": "x", "weeks": 0, "days": [{"name": "A", "groups": []}]}`, "/weeks", "minimum"},
		{"bad reps string", string(withSet(`{"count": 3, "reps": "5-"}`)), setPtr + "/reps", "does not match pattern"},
		{"two load keys", string(withSet(`{"count": 3, "reps": 5, "load": {"weight": 10, "rpe": 8}}`)), setPtr + "/load", "maxProperties"},
		{"reps and duration", string(withSet(`{"count": 3, "reps": 5, "duration_s": 30}`)), setPtr, "give reps or duration_s, not both"},
		{"neither reps nor duration", string(withSet(`{"count": 3}`)), setPtr, "give reps (or duration_s for timed sets)"},
		{"unknown field", string(withSet(`{"count": 3, "reps": 5, "tempo": "3010"}`)), setPtr, "'tempo' not allowed"},
		{"wrong type inside a per-week array", string(withSet(`{"count": [3, "x"], "reps": 5}`)), setPtr + "/count/1", "expected integer or null"},
		{"rpe off the half steps", string(withSet(`{"count": 3, "reps": 5, "load": {"rpe": 8.25}}`)), setPtr + "/load/rpe", "multipleOf"},
		{"bad slug", `{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "Pull Up", "sets": [{"count": 1, "reps": 1}]}]}]}]}`, "/days/0/groups/0/exercises/0/slug", "does not match pattern"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ps := Validate([]byte(c.doc), testCatalog)
			if !ps.HasErrors() {
				t.Fatalf("no errors: %+v", ps)
			}
			for _, msg := range problemsAt(ps, c.pointer) {
				if strings.Contains(msg, c.message) {
					return
				}
			}
			t.Fatalf("want %q at %q, got %+v", c.message, c.pointer, ps)
		})
	}
}

func TestSemanticChecks(t *testing.T) {
	const setPtr = "/days/0/groups/0/exercises/0/sets/0"
	cases := []struct {
		name, doc, pointer, message string
		warning                     bool
	}{
		{"per-week array too short", string(withSet(`{"count": [3], "reps": 5}`)), setPtr + "/count", "has 1 values but the plan has 2 weeks", false},
		{"range going down", string(withSet(`{"count": 3, "reps": "10-6"}`)), setPtr + "/reps", "range 10-6 goes down", false},
		{"drop on first line", string(withSet(`{"count": 1, "reps": 10, "load": {"drop_pct": 0.2}}`)), setPtr + "/load/drop_pct", "needs a previous set line", false},
		{"AMRAP with RPE load", string(withSet(`{"count": 1, "reps": "AMRAP", "load": {"rpe": 8}}`)), setPtr + "/load/rpe", "need 1-12 reps", true},
		{"pct_tm without TM", string(withSet(`{"count": 1, "reps": 5, "load": {"pct_tm": 0.7}}`)), setPtr + "/load/pct_tm", "no training max", true},
		{"unknown exercise", `{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "nope", "sets": [{"count": 1, "reps": 1}]}]}]}]}`, "/days/0/groups/0/exercises/0/slug", `unknown exercise "nope"`, false},
		{"hidden exercise", `{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "old-press", "sets": [{"count": 1, "reps": 1}]}]}]}]}`, "/days/0/groups/0/exercises/0/slug", "no longer in the catalog", true},
		{"week without days", `{"name": "x", "weeks": 2, "days": [{"name": "A", "only_weeks": [1], "groups": []}]}`, "/days", "week 2 has no training days", false},
		{"only_weeks past the end", `{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": []}, {"name": "B", "only_weeks": [3], "groups": []}]}`, "/days/1/only_weeks/0", "past the plan's 1 weeks", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ps := Validate([]byte(c.doc), testCatalog)
			for _, p := range ps {
				if p.Pointer == c.pointer && strings.Contains(p.Message, c.message) && p.Warning == c.warning {
					if !c.warning && !ps.HasErrors() {
						t.Fatal("HasErrors is false")
					}
					return
				}
			}
			t.Fatalf("want %q (warning=%v) at %q, got %+v", c.message, c.warning, c.pointer, ps)
		})
	}
}

// The schema caps each dimension, but they multiply; the preview expands the
// whole plan on every keystroke, so the total is bounded too.
func TestHugePlanIsRejectedQuickly(t *testing.T) {
	line := `{"count": 20, "reps": 5}`
	slot := `{"slug": "pull-up", "sets": [` + strings.TrimSuffix(strings.Repeat(line+",", 20), ",") + `]}`
	group := `{"exercises": [` + strings.TrimSuffix(strings.Repeat(slot+",", 6), ",") + `]}`
	day := `{"name": "D", "groups": [` + strings.TrimSuffix(strings.Repeat(group+",", 30), ",") + `]}`
	doc := `{"name": "huge", "weeks": 52, "days": [` + strings.TrimSuffix(strings.Repeat(day+",", 14), ",") + `]}`
	start := time.Now()
	_, ps := Validate([]byte(doc), testCatalog)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("validation took %v", d)
	}
	if len(problemsAt(ps, "")) == 0 || !strings.Contains(problemsAt(ps, "")[0], "sets in total") {
		t.Fatalf("problems = %v", ps)
	}
}
```

`internal/plan/expand_test.go`:
```go
package plan

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"slices"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

var update = flag.Bool("update", false, "rewrite golden files")

func exampleDoc(t *testing.T) Doc {
	t.Helper()
	doc, ps := Validate(readFixture(t, "upper_lower.json"), testCatalog)
	if ps.HasErrors() {
		t.Fatal(ps)
	}
	return doc
}

func TestDaysForWeek(t *testing.T) {
	doc := exampleDoc(t)
	if got := DaysForWeek(doc, 1); !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("week 1 days = %v", got)
	}
	if got := DaysForWeek(doc, 4); !slices.Equal(got, []int{0, 1, 2}) {
		t.Fatalf("week 4 days = %v", got)
	}
}

func TestExpandPicksPerWeekValues(t *testing.T) {
	doc := exampleDoc(t)
	w3, ok := Expand(doc, 3, 0, calc.UnitKg)
	if !ok {
		t.Fatal("week 3 day 0 missing")
	}
	bench := w3.Groups[0].Exercises[0]
	// 2 warm-ups + 4 working sets at RPE 9 + 1 drop set.
	if len(bench.Sets) != 7 || bench.Sets[2].TargetRPE != 9 || bench.Sets[6].Kind != "drop" {
		t.Fatalf("week 3 bench = %+v", bench.Sets)
	}
	if bench.Name != "barbell-bench-press" || bench.Notes != "pause first rep" {
		t.Fatalf("slot fields: %+v", bench)
	}

	w4, _ := Expand(doc, 4, 0, calc.UnitKg)
	bench4 := w4.Groups[0].Exercises[0]
	if len(bench4.Sets) != 4 || bench4.Sets[3].Kind != "working" { // null count drops the drop set
		t.Fatalf("week 4 bench = %+v", bench4.Sets)
	}
	if w4.Groups[1].RestS != 60 || w3.Groups[0].RestS != 180 {
		t.Fatalf("rest = %d / %d", w4.Groups[1].RestS, w3.Groups[0].RestS)
	}

	lower, _ := Expand(doc, 2, 1, calc.UnitKg)
	if lower.Groups[0].RestS != DefaultRestS || lower.Groups[1].Exercises[0].Sets[0].DurationS != 50 {
		t.Fatalf("lower day week 2 = %+v", lower)
	}
	if _, ok := Expand(doc, 1, 2, calc.UnitKg); ok {
		t.Fatal("the deload day must not exist in week 1")
	}
	if _, ok := Expand(doc, 5, 0, calc.UnitKg); ok {
		t.Fatal("week 5 must not exist")
	}
}

func TestExpandConvertsPlanUnit(t *testing.T) {
	raw := []byte(`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 1, "reps": 5, "load": {"weight": 45}}]}]}]}]}`)
	doc, ps := Validate(raw, testCatalog)
	if ps.HasErrors() {
		t.Fatal(ps)
	}
	day, _ := Expand(doc, 1, 0, calc.UnitLb) // the user's unit applies when the plan has none
	if got := day.Groups[0].Exercises[0].Sets[0].LoadValue; got != 45*calc.KgPerLb {
		t.Fatalf("45 lb = %v kg", got)
	}
}

func ptr(v float64) *float64 { return &v }

func TestResolve(t *testing.T) {
	doc := exampleDoc(t)
	bar := &calc.Equipment{Kind: calc.KindBarbell, Unit: calc.UnitKg, Config: calc.EquipmentConfig{Bar: 20, Plates: []float64{25, 20, 15, 10, 5, 2.5, 1.25}}}
	ctx := map[string]calc.LoadContext{
		"barbell-bench-press": {TMKg: ptr(100), E1RMKg: ptr(110), Equipment: bar, Unit: calc.UnitKg},
		"barbell-back-squat":  {TMKg: ptr(140), Equipment: bar, Unit: calc.UnitKg},
	}
	lookup := func(slug string) calc.LoadContext {
		if c, ok := ctx[slug]; ok {
			return c
		}
		return calc.LoadContext{Unit: calc.UnitKg}
	}

	day, _ := Expand(doc, 2, 0, calc.UnitKg)
	Resolve(&day, lookup)
	bench := day.Groups[0].Exercises[0].Sets
	kgs := func(sets []PrescribedSet) []float64 {
		var out []float64
		for _, s := range sets {
			if s.Kg == nil {
				out = append(out, -1)
			} else {
				out = append(out, *s.Kg)
			}
		}
		return out
	}
	// warm-up 50% of 100 = 50; 5 @ RPE 8 = 0.811 * 110 = 89.2 -> 87.5; drop 20% of 87.5 = 70.
	if got := kgs(bench); !slices.Equal(got, []float64{50, 50, 87.5, 87.5, 87.5, 70}) {
		t.Fatalf("bench kg = %v", got)
	}
	if !slices.Equal(bench[2].PerSide, []float64{25, 5, 2.5, 1.25}) {
		t.Fatalf("plates = %v", bench[2].PerSide)
	}
	// pull-ups have no load; lateral raises are an absolute 7.5 kg.
	if got := kgs(day.Groups[1].Exercises[0].Sets); !slices.Equal(got, []float64{-1, -1, -1}) {
		t.Fatalf("pull-up kg = %v", got)
	}
	if got := kgs(day.Groups[1].Exercises[1].Sets); !slices.Equal(got, []float64{7.5, 7.5, 7.5}) {
		t.Fatalf("raise kg = %v", got)
	}

	// Without an e1RM the RPE sets, and the drop set after them, are left open.
	delete(ctx, "barbell-bench-press")
	Resolve(&day, lookup)
	if got := kgs(day.Groups[0].Exercises[0].Sets); !slices.Equal(got, []float64{-1, -1, -1, -1, -1, -1}) {
		t.Fatalf("unresolved bench kg = %v", got)
	}

	// AMRAP sets with an absolute weight still resolve.
	deload, _ := Expand(doc, 4, 2, calc.UnitKg)
	Resolve(&deload, lookup)
	if got := kgs(deload.Groups[0].Exercises[0].Sets); !slices.Equal(got, []float64{10, 10}) {
		t.Fatalf("deload curls = %v", got)
	}
}

// The golden file pins the full expansion of the example plan.
func TestExpandGolden(t *testing.T) {
	doc := exampleDoc(t)
	var weeks [][]ExpandedDay
	for w := 1; w <= doc.Weeks; w++ {
		var days []ExpandedDay
		for d := range DaysForWeek(doc, w) {
			day, _ := Expand(doc, w, d, calc.UnitKg)
			days = append(days, day)
		}
		weeks = append(weeks, days)
	}
	got, err := json.MarshalIndent(weeks, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/upper_lower.golden.json"
	if *update {
		if err := os.WriteFile(path, append(got, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), got) {
		t.Fatalf("expansion differs from %s; run go test ./internal/plan/ -update and review the diff", path)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail**

Run: `go test ./internal/plan/`
Expected: FAIL, build errors such as `undefined: Validate`, `undefined: Problems`, `undefined: Expand`.

- [ ] **Step 5: Implement**

`internal/plan/doc.go`:
```go
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
```

`internal/plan/validate.go`: schema errors are reported from the library's `Causes` tree with English messages. Inside `oneOf`/`anyOf` only the branches that got past the type check are kept, so `"5-"` reports the pattern failure rather than "want integer". The reps/duration rule gets its own wording.
```go
package plan

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed plan.schema.json
var schemaJSON []byte

// Schema returns the plan JSON Schema (published at /schema/plan.json).
func Schema() []byte { return schemaJSON }

// schemaID is the $id in plan.schema.json.
const schemaID = "https://onerep.local/schema/plan.json"

var compiled = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaID, doc); err != nil {
		return nil, err
	}
	return c.Compile(schemaID)
})

// Problem is a validation finding located by a JSON Pointer into the document.
type Problem struct {
	Pointer string `json:"pointer"`
	Message string `json:"message"`
	Warning bool   `json:"warning,omitempty"`
}

// Problems is returned as an error when a document has errors.
type Problems []Problem

func (ps Problems) HasErrors() bool {
	return slices.ContainsFunc(ps, func(p Problem) bool { return !p.Warning })
}

func (ps Problems) Error() string {
	var parts []string
	for _, p := range ps {
		if !p.Warning {
			parts = append(parts, p.Pointer+": "+p.Message)
		}
	}
	return "invalid plan: " + strings.Join(parts, "; ")
}

// Lookup answers the questions validation asks about the user's catalog.
type Lookup interface {
	// Exercise reports whether slug exists for the user and whether it is hidden.
	Exercise(slug string) (exists, hidden bool)
	HasTrainingMax(slug string) bool
}

// Validate checks raw against the JSON Schema, then the rules the schema
// cannot express. The Doc is only meaningful when there are no errors.
func Validate(raw []byte, lookup Lookup) (Doc, Problems) {
	var doc Doc
	var syntax *json.SyntaxError
	if err := json.Unmarshal(raw, new(any)); errors.As(err, &syntax) {
		line, col := position(raw, syntax.Offset)
		return doc, Problems{{Pointer: "", Message: fmt.Sprintf("invalid JSON at line %d, column %d: %s", line, col, syntax.Error())}}
	} else if err != nil {
		return doc, Problems{{Pointer: "", Message: "invalid JSON: " + err.Error()}}
	}
	if ps := schemaProblems(raw); len(ps) > 0 {
		return doc, ps
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, Problems{{Pointer: "", Message: err.Error()}}
	}
	return doc, semanticProblems(doc, lookup)
}

func position(raw []byte, offset int64) (line, col int) {
	line, col = 1, 1
	for i := int64(0); i < offset-1 && i < int64(len(raw)); i++ {
		if raw[i] == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}

func schemaProblems(raw []byte) Problems {
	sch, err := compiled()
	if err != nil {
		return Problems{{Message: "plan schema failed to compile: " + err.Error()}}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return Problems{{Message: "invalid JSON: " + err.Error()}}
	}
	var ve *jsonschema.ValidationError
	if err := sch.Validate(inst); !errors.As(err, &ve) {
		return nil
	}
	var out Problems
	seen := map[string]bool{}
	add := func(e *jsonschema.ValidationError, msg string) {
		ptr := pointer(e.InstanceLocation)
		if key := ptr + "\x00" + msg; !seen[key] {
			seen[key] = true
			out = append(out, Problem{Pointer: ptr, Message: msg})
		}
	}
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if msg, ok := friendlier(e); ok {
			add(e, msg)
			return
		}
		causes := e.Causes
		switch e.ErrorKind.(type) {
		case *kind.OneOf, *kind.AnyOf:
			// A value that fails every alternative: report the alternatives
			// that got past the type check, or else which types are allowed.
			var kept []*jsonschema.ValidationError
			var wants []string
			for _, c := range causes {
				if w, ok := typeMismatch(c, len(e.InstanceLocation)); ok {
					wants = append(wants, w...)
				} else {
					kept = append(kept, c)
				}
			}
			if len(kept) == 0 && len(wants) > 0 {
				add(e, "expected "+strings.Join(slices.Compact(slices.Sorted(slices.Values(wants))), " or "))
				return
			}
			causes = kept
		}
		if len(causes) == 0 {
			add(e, e.ErrorKind.LocalizedString(printer))
			return
		}
		for _, c := range causes {
			walk(c)
		}
	}
	walk(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

var printer = message.NewPrinter(language.English)

// typeMismatch reports whether e (a oneOf branch) failed only because the
// value at depth has the wrong type, and which types it wanted.
func typeMismatch(e *jsonschema.ValidationError, depth int) ([]string, bool) {
	if len(e.InstanceLocation) != depth {
		return nil, false
	}
	if t, ok := e.ErrorKind.(*kind.Type); ok {
		return t.Want, true
	}
	if len(e.Causes) == 0 {
		return nil, false
	}
	var wants []string
	for _, c := range e.Causes {
		w, ok := typeMismatch(c, depth)
		if !ok {
			return nil, false
		}
		wants = append(wants, w...)
	}
	return wants, true
}

// pointer turns an instance location into a JSON Pointer.
func pointer(loc []string) string {
	var b strings.Builder
	for _, tok := range loc {
		b.WriteString("/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(tok))
	}
	return b.String()
}

func semanticProblems(doc Doc, lookup Lookup) Problems {
	if n := totalSets(doc); n > MaxTotalSets {
		return Problems{{Message: fmt.Sprintf("the plan has %d sets in total; at most %d are allowed", n, MaxTotalSets)}}
	}
	var ps Problems
	errorf := func(ptr, format string, args ...any) {
		ps = append(ps, Problem{Pointer: ptr, Message: fmt.Sprintf(format, args...)})
	}
	warnf := func(ptr, format string, args ...any) {
		ps = append(ps, Problem{Pointer: ptr, Message: fmt.Sprintf(format, args...), Warning: true})
	}
	perWeek := func(ptr string, n int, list bool) {
		if list && n != doc.Weeks {
			errorf(ptr, "has %d values but the plan has %d weeks", n, doc.Weeks)
		}
	}
	checkSlug := func(ptr, slug string) bool {
		exists, hidden := lookup.Exercise(slug)
		switch {
		case !exists:
			errorf(ptr, "unknown exercise %q", slug)
		case hidden:
			warnf(ptr, "%q is no longer in the catalog", slug)
		}
		return exists
	}

	for w := 1; w <= doc.Weeks; w++ {
		if len(DaysForWeek(doc, w)) == 0 {
			errorf("/days", "week %d has no training days (check only_weeks)", w)
		}
	}
	for di, day := range doc.Days {
		dp := "/days/" + strconv.Itoa(di)
		for i, w := range day.OnlyWeeks {
			if w > doc.Weeks {
				errorf(dp+"/only_weeks/"+strconv.Itoa(i), "week %d is past the plan's %d weeks", w, doc.Weeks)
			}
		}
		for gi, g := range day.Groups {
			gp := dp + "/groups/" + strconv.Itoa(gi)
			perWeek(gp+"/rest_s", len(g.RestS.Values), g.RestS.List)
			for si, slot := range g.Exercises {
				sp := gp + "/exercises/" + strconv.Itoa(si)
				known := checkSlug(sp+"/slug", slot.Slug)
				for ai, alt := range slot.Alternatives {
					checkSlug(sp+"/alternatives/"+strconv.Itoa(ai), alt)
				}
				for li, line := range slot.Sets {
					lp := sp + "/sets/" + strconv.Itoa(li)
					perWeek(lp+"/count", len(line.Count.Values), line.Count.List)
					perWeek(lp+"/reps", len(line.Reps.Values), line.Reps.List)
					perWeek(lp+"/duration_s", len(line.DurationS.Values), line.DurationS.List)
					perWeek(lp+"/rpe", len(line.RPE.Values), line.RPE.List)
					for ri, r := range line.Reps.Values {
						if !r.AMRAP && r.Min > r.Max {
							ptr := lp + "/reps"
							if line.Reps.List {
								ptr += "/" + strconv.Itoa(ri)
							}
							errorf(ptr, "range %s goes down", r)
						}
					}
					if line.Load == nil {
						continue
					}
					l := line.Load
					for name, v := range map[string]PerWeek[float64]{"weight": l.Weight, "pct_tm": l.PctTM, "rpe": l.RPE, "drop_pct": l.DropPct} {
						perWeek(lp+"/load/"+name, len(v.Values), v.List)
					}
					if l.DropPct.Set() && li == 0 {
						errorf(lp+"/load/drop_pct", "a drop set needs a previous set line to drop from")
					}
					if l.PctTM.Set() && known && !lookup.HasTrainingMax(slot.Slug) {
						warnf(lp+"/load/pct_tm", "no training max set for %q, so these weights will be left for you to pick", slot.Slug)
					}
					if l.RPE.Set() && !rpeResolvable(line) {
						warnf(lp+"/load/rpe", "RPE loads need 1-12 reps; the weight will be left for you to pick")
					}
				}
			}
		}
	}
	return ps
}

func rpeResolvable(line SetLine) bool {
	for _, r := range line.Reps.Values {
		if r.AMRAP || r.Min < 1 || r.Min > 12 {
			return false
		}
	}
	return line.Reps.Set()
}

// friendlier replaces library wording for rules people break most often.
func friendlier(e *jsonschema.ValidationError) (string, bool) {
	if _, ok := e.ErrorKind.(*kind.OneOf); !ok || !strings.HasSuffix(e.SchemaURL, "#/$defs/setLine") {
		return "", false
	}
	if len(e.Causes) == 0 { // both alternatives matched
		return "give reps or duration_s, not both", true
	}
	return "give reps (or duration_s for timed sets)", true
}

// MaxTotalSets bounds the sets of a whole plan (a year of 5 days a week with
// 25 sets a day is about 6500). Every preview expands the whole plan.
const MaxTotalSets = 20000

// totalSets counts the sets of every week without expanding them.
func totalSets(doc Doc) int {
	n := 0
	for w := 1; w <= doc.Weeks; w++ {
		for _, di := range DaysForWeek(doc, w) {
			for _, g := range doc.Days[di].Groups {
				for _, slot := range g.Exercises {
					for _, line := range slot.Sets {
						if c := line.Count.At(w); c != nil {
							n += *c
						}
					}
				}
			}
		}
	}
	return n
}
```

`internal/plan/expand.go`:
```go
package plan

import (
	"slices"

	"github.com/LongerHV/onerep/internal/calc"
)

// DefaultRestS is the rest after a group's round when rest_s is not given.
const DefaultRestS = 120

// ExpandedDay is one concrete training day: per-week values picked, set
// counts expanded, weights in kg. It is what a session snapshots (spec §6).
type ExpandedDay struct {
	Week   int             `json:"week"`
	Day    int             `json:"day"` // index among the days of this week
	Name   string          `json:"name"`
	Groups []ExpandedGroup `json:"groups"`
}

type ExpandedGroup struct {
	RestS     int            `json:"rest_s"`
	Exercises []ExpandedSlot `json:"exercises"`
}

type ExpandedSlot struct {
	Slug         string          `json:"slug"`
	Name         string          `json:"name"` // filled in by the caller when known
	Alternatives []string        `json:"alternatives,omitempty"`
	Notes        string          `json:"notes,omitempty"`
	Sets         []PrescribedSet `json:"sets"`
}

// Load kinds of a prescribed set.
const (
	LoadNone    = ""
	LoadWeight  = "weight"
	LoadPctTM   = "pct_tm"
	LoadRPE     = "rpe"
	LoadDropPct = "drop_pct"
)

// PrescribedSet is one set as planned, plus its resolved weight.
type PrescribedSet struct {
	Kind      string    `json:"kind"`
	Reps      Reps      `json:"reps,omitzero"` // zero for timed sets
	DurationS int       `json:"duration_s,omitempty"`
	TargetRPE float64   `json:"target_rpe,omitempty"` // 0 = none
	LoadKind  string    `json:"load_kind,omitempty"`
	LoadValue float64   `json:"load_value,omitempty"` // kg for weight, fraction for pct_tm/drop_pct, RPE for rpe
	Kg        *float64  `json:"kg,omitempty"`         // resolved weight; nil = the lifter picks
	PerSide   []float64 `json:"per_side,omitempty"`   // barbell plates per side, in the equipment's unit
}

// DaysForWeek returns the indexes into doc.Days that apply to week.
func DaysForWeek(doc Doc, week int) []int {
	var out []int
	for i, d := range doc.Days {
		if len(d.OnlyWeeks) == 0 || slices.Contains(d.OnlyWeeks, week) {
			out = append(out, i)
		}
	}
	return out
}

// Expand returns day index day (among the days of week) of week. ok is false
// when that day does not exist. Absolute weights are converted from the
// plan's unit (defaultUnit when the plan has none) to kg.
func Expand(doc Doc, week, day int, defaultUnit string) (ExpandedDay, bool) {
	days := DaysForWeek(doc, week)
	if week < 1 || week > doc.Weeks || day < 0 || day >= len(days) {
		return ExpandedDay{}, false
	}
	unit := doc.Unit
	if unit == "" {
		unit = defaultUnit
	}
	src := doc.Days[days[day]]
	out := ExpandedDay{Week: week, Day: day, Name: src.Name}
	for _, g := range src.Groups {
		rest := DefaultRestS
		if g.RestS.Set() {
			rest = g.RestS.At(week)
		}
		eg := ExpandedGroup{RestS: rest}
		for _, slot := range g.Exercises {
			es := ExpandedSlot{Slug: slot.Slug, Name: slot.Slug, Alternatives: slot.Alternatives, Notes: slot.Notes}
			for _, line := range slot.Sets {
				count := line.Count.At(week)
				if count == nil {
					continue
				}
				ps := PrescribedSet{Kind: line.Kind, Reps: line.Reps.At(week), DurationS: line.DurationS.At(week), TargetRPE: line.RPE.At(week)}
				if ps.Kind == "" {
					ps.Kind = "working"
				}
				if l := line.Load; l != nil {
					switch {
					case l.Weight.Set():
						ps.LoadKind, ps.LoadValue = LoadWeight, calc.ToKg(l.Weight.At(week), unit)
					case l.PctTM.Set():
						ps.LoadKind, ps.LoadValue = LoadPctTM, l.PctTM.At(week)
					case l.RPE.Set():
						ps.LoadKind, ps.LoadValue = LoadRPE, l.RPE.At(week)
						ps.TargetRPE = ps.LoadValue
					case l.DropPct.Set():
						ps.LoadKind, ps.LoadValue = LoadDropPct, l.DropPct.At(week)
					}
				}
				for range *count {
					es.Sets = append(es.Sets, ps)
				}
			}
			if len(es.Sets) > 0 {
				eg.Exercises = append(eg.Exercises, es)
			}
		}
		if len(eg.Exercises) > 0 {
			out.Groups = append(out.Groups, eg)
		}
	}
	return out, true
}

// Resolve fills in Kg and PerSide for every set it can (spec §6):
// weight as given, pct_tm from the training max, rpe from the e1RM (lower
// bound of a rep range), drop_pct from the previous set's resolved weight.
// ctxFor supplies what the lifter knows about each exercise.
func Resolve(day *ExpandedDay, ctxFor func(slug string) calc.LoadContext) {
	for gi := range day.Groups {
		for si := range day.Groups[gi].Exercises {
			slot := &day.Groups[gi].Exercises[si]
			ctx := ctxFor(slot.Slug)
			var prev *float64
			for i := range slot.Sets {
				s := &slot.Sets[i]
				s.Kg, s.PerSide = nil, nil
				var r calc.Rounded
				ok := false
				switch s.LoadKind {
				case LoadWeight:
					r, ok = calc.ResolveLoad(calc.Load{Weight: &s.LoadValue}, s.Reps.Min, ctx)
				case LoadPctTM:
					r, ok = calc.ResolveLoad(calc.Load{PctTM: &s.LoadValue}, s.Reps.Min, ctx)
				case LoadRPE:
					if !s.Reps.AMRAP {
						r, ok = calc.ResolveLoad(calc.Load{RPE: &s.LoadValue}, s.Reps.Min, ctx)
					}
				case LoadDropPct:
					if prev != nil {
						r, ok = calc.DropLoad(*prev, s.LoadValue, ctx), true
					}
				}
				if ok {
					kg := r.Kg
					s.Kg, s.PerSide = &kg, r.PerSide
				}
				prev = s.Kg
			}
		}
	}
}
```

- [ ] **Step 6: Generate the golden file and review it**

Run: `go mod tidy && go test ./internal/plan/ -run Golden -update && go test ./internal/plan/`
Expected: `ok`. Then check what the golden file pinned:

```bash
jq -c '.[0][0].groups[0].exercises[0].sets | map([.kind, .reps, .load_kind, .load_value, .target_rpe])' internal/plan/testdata/upper_lower.golden.json
# → [["warmup",5,"pct_tm",0.5,null],["warmup",5,"pct_tm",0.5,null],["working",5,"rpe",7,7],["working",5,"rpe",7,7],["working",5,"rpe",7,7],["drop",10,"drop_pct",0.2,null]]
jq -c '.[3] | map(.name)' internal/plan/testdata/upper_lower.golden.json
# → ["Upper A","Lower A","Deload extra"]
jq -c '.[3][1].groups[1].exercises[0].sets[0]' internal/plan/testdata/upper_lower.golden.json
# → {"kind":"working","duration_s":30}
```

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/plan
git commit -m "feat(plan): add plan documents with schema validation and per-week expansion"
```

---

### Task 2: Cursor positions, set descriptions, and line diff

**Files:**
- Create: `internal/plan/describe.go`, `internal/plan/diff.go`
- Test: `internal/plan/describe_test.go`

**Interfaces:**
- Consumes: Task 1
- Produces: `plan.NextPosition(doc, week, day) (int, int)`, `plan.ValidPosition(doc, week, day) bool`, `plan.SetGroup{Count int; Set PrescribedSet}`, `plan.GroupSets([]PrescribedSet) []SetGroup`, `plan.Prescription(SetGroup, unit) string` (e.g. `"3 × 5 @ RPE 8"`, `"2 × 5 warmup @ 50% TM"`, `"3 × 45s"`), `plan.DayLines(ExpandedDay, unit) []string`, `plan.DiffLine{Op byte; Text string}`, `plan.DiffLines(a, b []string) []DiffLine`, `plan.Changed([]DiffLine) bool`

- [ ] **Step 1: Write the failing tests**

`internal/plan/describe_test.go`:
```go
package plan

import (
	"slices"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/calc"
)

func TestNextPosition(t *testing.T) {
	doc := exampleDoc(t) // weeks 1-3: 2 days, week 4: 3 days
	steps := [][2]int{{1, 0}, {1, 1}, {2, 0}, {2, 1}, {3, 0}, {3, 1}, {4, 0}, {4, 1}, {4, 2}, {5, 0}}
	w, d := 1, 0
	for i, want := range steps[1:] {
		w, d = NextPosition(doc, w, d)
		if [2]int{w, d} != want {
			t.Fatalf("step %d: got (%d, %d), want %v", i+1, w, d, want)
		}
	}
	if ValidPosition(doc, 5, 0) || ValidPosition(doc, 1, 2) || !ValidPosition(doc, 4, 2) {
		t.Fatal("ValidPosition wrong")
	}
}

func TestPrescriptionText(t *testing.T) {
	doc := exampleDoc(t)
	day, _ := Expand(doc, 1, 0, calc.UnitKg)
	day.Groups[0].Exercises[0].Name = "Bench"
	got := DayLines(day, calc.UnitLb)
	want := []string{
		"Group 1, rest 180s",
		"  Bench",
		"    2 × 5 warmup @ 50% TM",
		"    3 × 5 @ RPE 7",
		"    1 × 10 drop -20%",
		"Group 2, rest 90s",
		"  pull-up",
		"    3 × 6-10 (RPE 8)",
		"  cable-lateral-raise",
		"    3 × 15 @ 16.53 lb",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
	plank, _ := Expand(doc, 1, 1, calc.UnitKg)
	if lines := DayLines(plank, calc.UnitKg); lines[len(lines)-1] != "    3 × 45s" {
		t.Fatalf("timed sets: %q", lines[len(lines)-1])
	}
}

func TestGroupSetsKeepsDifferentWeightsApart(t *testing.T) {
	a, b := 100.0, 102.5
	sets := []PrescribedSet{{Kind: "working", Kg: &a}, {Kind: "working", Kg: &a}, {Kind: "working", Kg: &b}, {Kind: "working"}}
	if got := GroupSets(sets); len(got) != 3 || got[0].Count != 2 {
		t.Fatalf("groups = %+v", got)
	}
}

func TestDiffLines(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"a", "c", "d", "e"}
	got := DiffLines(a, b)
	want := []DiffLine{{' ', "a"}, {'-', "b"}, {' ', "c"}, {' ', "d"}, {'+', "e"}}
	if !slices.Equal(got, want) || !Changed(got) || Changed(DiffLines(a, a)) {
		t.Fatalf("diff = %q", got)
	}
	big := make([]string, 3000)
	for i := range big {
		big[i] = strings.Repeat("x", i%7)
	}
	if got := DiffLines(big, big[1:]); len(got) != len(big)+len(big)-1 {
		t.Fatalf("oversized inputs should be shown as replaced, got %d lines", len(got))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/plan/`
Expected: FAIL, `undefined: NextPosition`, `undefined: DayLines`, `undefined: DiffLines`.

- [ ] **Step 3: Implement**

`internal/plan/describe.go`:
```go
package plan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
)

// NextPosition is the cursor position after finishing or skipping (week, day):
// the next day of the week, else the first day of the next week. Past the last
// week the plan is complete (week = doc.Weeks+1, day 0).
func NextPosition(doc Doc, week, day int) (int, int) {
	if day+1 < len(DaysForWeek(doc, week)) {
		return week, day + 1
	}
	return week + 1, 0
}

// ValidPosition reports whether (week, day) is a training day of doc.
func ValidPosition(doc Doc, week, day int) bool {
	return week >= 1 && week <= doc.Weeks && day >= 0 && day < len(DaysForWeek(doc, week))
}

// SetGroup is a run of identical consecutive sets.
type SetGroup struct {
	Count int
	Set   PrescribedSet
}

// GroupSets collapses identical consecutive sets (resolved weight included).
func GroupSets(sets []PrescribedSet) []SetGroup {
	var out []SetGroup
	for _, s := range sets {
		if n := len(out); n > 0 && sameSet(out[n-1].Set, s) {
			out[n-1].Count++
			continue
		}
		out = append(out, SetGroup{Count: 1, Set: s})
	}
	return out
}

func sameSet(a, b PrescribedSet) bool {
	return a.Kind == b.Kind && a.Reps == b.Reps && a.DurationS == b.DurationS && a.TargetRPE == b.TargetRPE &&
		a.LoadKind == b.LoadKind && a.LoadValue == b.LoadValue && ((a.Kg == nil) == (b.Kg == nil)) &&
		(a.Kg == nil || *a.Kg == *b.Kg)
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Prescription describes a set group without its resolved weight, like
// "3 × 5 @ RPE 8" or "2 × 5 warmup @ 50% TM". Weights are shown in unit.
func Prescription(g SetGroup, unit string) string {
	s := g.Set
	var b strings.Builder
	switch {
	case s.DurationS > 0:
		fmt.Fprintf(&b, "%d × %ds", g.Count, s.DurationS)
	default:
		fmt.Fprintf(&b, "%d × %s", g.Count, s.Reps)
	}
	if s.Kind != "working" {
		b.WriteString(" " + s.Kind)
	}
	switch s.LoadKind {
	case LoadWeight:
		fmt.Fprintf(&b, " @ %s %s", num(round2(calc.FromKg(s.LoadValue, unit))), unit)
	case LoadPctTM:
		fmt.Fprintf(&b, " @ %s%% TM", num(round2(s.LoadValue*100)))
	case LoadRPE:
		fmt.Fprintf(&b, " @ RPE %s", num(s.LoadValue))
	case LoadDropPct:
		fmt.Fprintf(&b, " -%s%%", num(round2(s.LoadValue*100)))
	}
	if s.TargetRPE > 0 && s.LoadKind != LoadRPE {
		fmt.Fprintf(&b, " (RPE %s)", num(s.TargetRPE))
	}
	return b.String()
}

func round2(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }

// DayLines describes a day's prescription as text lines, one per exercise
// and set group, for comparing versions.
func DayLines(day ExpandedDay, unit string) []string {
	var out []string
	for gi, g := range day.Groups {
		out = append(out, fmt.Sprintf("Group %d, rest %ds", gi+1, g.RestS))
		for _, slot := range g.Exercises {
			out = append(out, "  "+slot.Name)
			for _, sg := range GroupSets(stripKg(slot.Sets)) {
				out = append(out, "    "+Prescription(sg, unit))
			}
		}
	}
	return out
}

func stripKg(sets []PrescribedSet) []PrescribedSet {
	out := make([]PrescribedSet, len(sets))
	for i, s := range sets {
		s.Kg, s.PerSide = nil, nil
		out[i] = s
	}
	return out
}
```

`internal/plan/diff.go`:
```go
package plan

// DiffLine is one line of a line diff: Op is ' ' (same), '-' (removed) or '+' (added).
type DiffLine struct {
	Op   byte
	Text string
}

// maxDiffCells bounds the LCS table; larger inputs are shown as replaced.
const maxDiffCells = 4_000_000

// DiffLines returns a minimal line diff turning a into b (LCS based).
func DiffLines(a, b []string) []DiffLine {
	if len(a)*len(b) > maxDiffCells {
		var out []DiffLine
		for _, s := range a {
			out = append(out, DiffLine{'-', s})
		}
		for _, s := range b {
			out = append(out, DiffLine{'+', s})
		}
		return out
	}
	// lcs[i][j] = LCS length of a[i:] and b[j:].
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []DiffLine
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, DiffLine{' ', a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{'-', a[i]})
			i++
		default:
			out = append(out, DiffLine{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, DiffLine{'-', a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, DiffLine{'+', b[j]})
	}
	return out
}

// Changed reports whether a diff has any added or removed lines.
func Changed(d []DiffLine) bool {
	for _, l := range d {
		if l.Op != ' ' {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/plan/`
Expected: `ok  github.com/LongerHV/onerep/internal/plan`

- [ ] **Step 5: Commit**

```bash
git add internal/plan
git commit -m "feat(plan): add cursor movement, set descriptions and line diff"
```

---

### Task 3: Plan storage

**Files:**
- Replace: `internal/store/schema.sql`
- Create (generated): `internal/store/migrations/<timestamp>_plans.{up,down}.sql`, updated `atlas.sum`
- Create: `internal/store/plans.go`
- Test: `internal/store/plans_test.go`

**Interfaces:**
- Consumes: store helpers (`tx`, `newID`, `mustAffect`, `formatTime`, `parseTime`, `newUser`)
- Produces:
  - Statuses: `store.PlanDraft|PlanActive|PlanSuperseded`
  - Types: `store.Plan{ID, UserID, Name string; Archived bool; CreatedAt}`, `store.PlanVersion{ID, PlanID string; Version int; Doc []byte; Status, Source, Note string; CreatedAt}`, `store.ActivePlan{PlanID string; Week, Day int}`
  - Plans and versions: `CreatePlan(ctx, userID, name, doc, status, source, note) (Plan, PlanVersion, error)`, `SavePlanVersion(ctx, userID, planID, name, doc, status, source, note) (PlanVersion, error)`, `ListPlans`, `PlanByID`, `SetPlanArchived`, `PlanVersions` (newest first), `PlanVersionByID`, `ActivePlanVersion`, `ActivateVersion`, `DeleteDraft`
  - Cursor: `ActivePlan(ctx, userID) (ActivePlan, error)`, `SetActivePlan(ctx, userID, ActivePlan) error` (another user's plan → `ErrNotFound`), `ClearActivePlan`

- [ ] **Step 1: Extend the schema and generate the migration**

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

CREATE TABLE plans (
  id         TEXT    NOT NULL PRIMARY KEY,
  user_id    TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name       TEXT    NOT NULL,
  archived   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL
);

CREATE INDEX plans_user_id ON plans (user_id);

-- Every save creates a version. Active and superseded versions are immutable;
-- at most one version of a plan is active.
CREATE TABLE plan_versions (
  id         TEXT    NOT NULL PRIMARY KEY,
  plan_id    TEXT    NOT NULL REFERENCES plans (id) ON DELETE CASCADE,
  version    INTEGER NOT NULL,
  doc        TEXT    NOT NULL, -- the plan as authored (JSON)
  status     TEXT    NOT NULL CHECK (status IN ('draft', 'active', 'superseded')),
  source     TEXT    NOT NULL CHECK (source IN ('web', 'mcp')),
  note       TEXT    NOT NULL DEFAULT '',
  created_at TEXT    NOT NULL,
  UNIQUE (plan_id, version)
);

CREATE UNIQUE INDEX plan_versions_one_active ON plan_versions (plan_id) WHERE status = 'active';

-- The plan a user is following and the next day to train (spec §8).
-- cursor_week past the plan's last week means the block is complete.
CREATE TABLE active_plan (
  user_id     TEXT    NOT NULL PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  plan_id     TEXT    NOT NULL REFERENCES plans (id) ON DELETE CASCADE,
  cursor_week INTEGER NOT NULL,
  cursor_day  INTEGER NOT NULL,
  updated_at  TEXT    NOT NULL
);
```

Run: `task migrate:diff NAME=plans && task migrate:check`
Expected: a new `<timestamp>_plans.up.sql` creating the three tables and the partial unique index `plan_versions_one_active`; `migrate:check` exits 0.

- [ ] **Step 2: Write the failing tests**

`internal/store/plans_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
)

func TestPlanVersions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	p, v1, err := db.CreatePlan(ctx, alice.ID, "PPL", []byte(`{"v":1}`), PlanActive, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if v1.Version != 1 || v1.Status != PlanActive {
		t.Fatalf("v1 = %+v", v1)
	}
	v2, err := db.SavePlanVersion(ctx, alice.ID, p.ID, "PPL v2", []byte(`{"v":2}`), PlanDraft, "mcp", "harder")
	if err != nil || v2.Version != 2 {
		t.Fatalf("v2 = %+v, %v", v2, err)
	}
	if got, _ := db.PlanByID(ctx, alice.ID, p.ID); got.Name != "PPL v2" {
		t.Fatalf("plan not renamed: %q", got.Name)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v1.ID {
		t.Fatal("a draft must not replace the active version")
	}

	if err := db.ActivateVersion(ctx, alice.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	vs, err := db.PlanVersions(ctx, alice.ID, p.ID)
	if err != nil || len(vs) != 2 || vs[0].Status != PlanActive || vs[1].Status != PlanSuperseded || string(vs[0].Doc) != `{"v":2}` {
		t.Fatalf("versions = %+v, %v", vs, err)
	}

	// Saving straight to active supersedes the current one.
	v3, err := db.SavePlanVersion(ctx, alice.ID, p.ID, "PPL", []byte(`{"v":3}`), PlanActive, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v3.ID || active.Version != 3 {
		t.Fatalf("active = %+v", active)
	}

	// Rolling back to an older version is allowed.
	if err := db.ActivateVersion(ctx, alice.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	if active, _ := db.ActivePlanVersion(ctx, alice.ID, p.ID); active.ID != v1.ID {
		t.Fatal("rollback failed")
	}

	// Isolation.
	if _, err := db.PlanVersionByID(ctx, bob.ID, v1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob reads alice's version: %v", err)
	}
	if _, err := db.SavePlanVersion(ctx, bob.ID, p.ID, "x", []byte(`{}`), PlanDraft, "web", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob saves into alice's plan: %v", err)
	}
	if err := db.ActivateVersion(ctx, bob.ID, v2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob activates alice's version: %v", err)
	}
	if vs, _ := db.PlanVersions(ctx, bob.ID, p.ID); len(vs) != 0 {
		t.Fatal("bob lists alice's versions")
	}
}

func TestDeleteDraftOnly(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	p, v1, _ := db.CreatePlan(ctx, u.ID, "P", []byte(`{}`), PlanActive, "web", "")
	draft, _ := db.SavePlanVersion(ctx, u.ID, p.ID, "P", []byte(`{}`), PlanDraft, "web", "")
	if err := db.DeleteDraft(ctx, u.ID, v1.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleting the active version: %v", err)
	}
	if err := db.DeleteDraft(ctx, u.ID, draft.ID); err != nil {
		t.Fatal(err)
	}
	// A deleted draft's number is reused by the next version.
	next, _ := db.SavePlanVersion(ctx, u.ID, p.ID, "P", []byte(`{}`), PlanDraft, "web", "")
	if next.Version != 2 {
		t.Fatalf("next version = %d", next.Version)
	}
}

func TestActivePlanCursor(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	p, _, _ := db.CreatePlan(ctx, alice.ID, "P", []byte(`{}`), PlanActive, "web", "")

	if _, err := db.ActivePlan(ctx, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no active plan yet: %v", err)
	}
	if err := db.SetActivePlan(ctx, alice.ID, ActivePlan{PlanID: p.ID, Week: 1, Day: 0}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetActivePlan(ctx, alice.ID, ActivePlan{PlanID: p.ID, Week: 2, Day: 1}); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.ActivePlan(ctx, alice.ID); a != (ActivePlan{PlanID: p.ID, Week: 2, Day: 1}) {
		t.Fatalf("cursor = %+v", a)
	}
	if err := db.SetActivePlan(ctx, bob.ID, ActivePlan{PlanID: p.ID, Week: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob follows alice's plan: %v", err)
	}
	if err := db.ClearActivePlan(ctx, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ActivePlan(ctx, alice.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("cleared plan still active")
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL, `db.CreatePlan undefined`, `undefined: PlanActive`.

- [ ] **Step 4: Implement**

`internal/store/plans.go`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Plan statuses of a version.
const (
	PlanDraft      = "draft"
	PlanActive     = "active"
	PlanSuperseded = "superseded"
)

type Plan struct {
	ID        string
	UserID    string
	Name      string
	Archived  bool
	CreatedAt time.Time
}

// PlanVersion is one saved state of a plan document.
type PlanVersion struct {
	ID        string
	PlanID    string
	Version   int
	Doc       []byte
	Status    string // PlanDraft, PlanActive or PlanSuperseded
	Source    string // "web" or "mcp"
	Note      string
	CreatedAt time.Time
}

// ActivePlan is the plan a user follows and the next day to train.
type ActivePlan struct {
	PlanID string
	Week   int // 1-based; past the plan's last week = complete
	Day    int // index among the days of Week
}

const planColumns = `id, user_id, name, archived, created_at`

func scanPlan(row interface{ Scan(...any) error }) (Plan, error) {
	var p Plan
	var created string
	err := row.Scan(&p.ID, &p.UserID, &p.Name, &p.Archived, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	p.CreatedAt, err = parseTime(created)
	return p, err
}

const versionColumns = `v.id, v.plan_id, v.version, v.doc, v.status, v.source, v.note, v.created_at`

func scanVersion(row interface{ Scan(...any) error }) (PlanVersion, error) {
	var v PlanVersion
	var doc, created string
	err := row.Scan(&v.ID, &v.PlanID, &v.Version, &doc, &v.Status, &v.Source, &v.Note, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanVersion{}, ErrNotFound
	}
	if err != nil {
		return PlanVersion{}, err
	}
	v.Doc = []byte(doc)
	v.CreatedAt, err = parseTime(created)
	return v, err
}

// CreatePlan creates a plan with doc as version 1.
func (db *DB) CreatePlan(ctx context.Context, userID, name string, doc []byte, status, source, note string) (Plan, PlanVersion, error) {
	var p Plan
	var v PlanVersion
	err := db.tx(ctx, func(tx *sql.Tx) error {
		id, err := newID()
		if err != nil {
			return err
		}
		p = Plan{ID: id, UserID: userID, Name: name, CreatedAt: time.Now().UTC()}
		if _, err := tx.ExecContext(ctx, `INSERT INTO plans (`+planColumns+`) VALUES (?, ?, ?, 0, ?)`,
			p.ID, userID, name, formatTime(p.CreatedAt)); err != nil {
			return err
		}
		v, err = insertVersion(ctx, tx, p.ID, doc, status, source, note)
		return err
	})
	return p, v, err
}

// SavePlanVersion adds the next version of the user's plan and renames the
// plan to name. Saving as active supersedes the current active version.
func (db *DB) SavePlanVersion(ctx context.Context, userID, planID, name string, doc []byte, status, source, note string) (PlanVersion, error) {
	var v PlanVersion
	err := db.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE plans SET name = ? WHERE id = ? AND user_id = ?`, name, planID, userID)
		if err != nil {
			return err
		}
		if err := mustAffect(res); err != nil {
			return err
		}
		v, err = insertVersion(ctx, tx, planID, doc, status, source, note)
		return err
	})
	return v, err
}

func insertVersion(ctx context.Context, tx *sql.Tx, planID string, doc []byte, status, source, note string) (PlanVersion, error) {
	if status == PlanActive {
		if _, err := tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'superseded'
			WHERE plan_id = ? AND status = 'active'`, planID); err != nil {
			return PlanVersion{}, err
		}
	}
	id, err := newID()
	if err != nil {
		return PlanVersion{}, err
	}
	v := PlanVersion{ID: id, PlanID: planID, Doc: doc, Status: status, Source: source, Note: note, CreatedAt: time.Now().UTC()}
	err = tx.QueryRowContext(ctx, `INSERT INTO plan_versions (id, plan_id, version, doc, status, source, note, created_at)
		VALUES (?, ?, (SELECT coalesce(max(version), 0) + 1 FROM plan_versions WHERE plan_id = ?), ?, ?, ?, ?, ?)
		RETURNING version`,
		v.ID, planID, planID, string(doc), status, source, note, formatTime(v.CreatedAt)).Scan(&v.Version)
	return v, err
}

// ListPlans returns the user's plans, unarchived first, by name.
func (db *DB) ListPlans(ctx context.Context, userID string) ([]Plan, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+planColumns+` FROM plans WHERE user_id = ?
		ORDER BY archived, name COLLATE NOCASE`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Plan
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (db *DB) PlanByID(ctx context.Context, userID, id string) (Plan, error) {
	return scanPlan(db.read.QueryRowContext(ctx, `SELECT `+planColumns+` FROM plans WHERE user_id = ? AND id = ?`, userID, id))
}

func (db *DB) SetPlanArchived(ctx context.Context, userID, id string, archived bool) error {
	res, err := db.write.ExecContext(ctx, `UPDATE plans SET archived = ? WHERE user_id = ? AND id = ?`, archived, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// PlanVersions lists a plan's versions, newest first.
func (db *DB) PlanVersions(ctx context.Context, userID, planID string) ([]PlanVersion, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.plan_id = ? ORDER BY v.version DESC`, userID, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlanVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (db *DB) PlanVersionByID(ctx context.Context, userID, versionID string) (PlanVersion, error) {
	return scanVersion(db.read.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.id = ?`, userID, versionID))
}

// ActivePlanVersion returns the plan's active version, or ErrNotFound.
func (db *DB) ActivePlanVersion(ctx context.Context, userID, planID string) (PlanVersion, error) {
	return scanVersion(db.read.QueryRowContext(ctx, `SELECT `+versionColumns+` FROM plan_versions v
		JOIN plans p ON p.id = v.plan_id WHERE p.user_id = ? AND v.plan_id = ? AND v.status = 'active'`, userID, planID))
}

// ActivateVersion makes a draft (or an older superseded version) the plan's
// active version.
func (db *DB) ActivateVersion(ctx context.Context, userID, versionID string) error {
	return db.tx(ctx, func(tx *sql.Tx) error {
		var planID string
		err := tx.QueryRowContext(ctx, `SELECT v.plan_id FROM plan_versions v JOIN plans p ON p.id = v.plan_id
			WHERE p.user_id = ? AND v.id = ?`, userID, versionID).Scan(&planID)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'superseded'
			WHERE plan_id = ? AND status = 'active' AND id != ?`, planID, versionID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE plan_versions SET status = 'active' WHERE id = ?`, versionID)
		return err
	})
}

// DeleteDraft deletes a draft version. Other versions are ErrNotFound.
func (db *DB) DeleteDraft(ctx context.Context, userID, versionID string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM plan_versions WHERE id = ? AND status = 'draft'
		AND plan_id IN (SELECT id FROM plans WHERE user_id = ?)`, versionID, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// ActivePlan returns the plan the user follows, or ErrNotFound.
func (db *DB) ActivePlan(ctx context.Context, userID string) (ActivePlan, error) {
	var a ActivePlan
	err := db.read.QueryRowContext(ctx, `SELECT plan_id, cursor_week, cursor_day FROM active_plan WHERE user_id = ?`,
		userID).Scan(&a.PlanID, &a.Week, &a.Day)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivePlan{}, ErrNotFound
	}
	return a, err
}

// SetActivePlan makes the user follow planID at (week, day).
func (db *DB) SetActivePlan(ctx context.Context, userID string, a ActivePlan) error {
	res, err := db.write.ExecContext(ctx, `INSERT INTO active_plan (user_id, plan_id, cursor_week, cursor_day, updated_at)
		SELECT ?, id, ?, ?, ? FROM plans WHERE id = ? AND user_id = ?
		ON CONFLICT (user_id) DO UPDATE SET plan_id = excluded.plan_id, cursor_week = excluded.cursor_week,
			cursor_day = excluded.cursor_day, updated_at = excluded.updated_at`,
		userID, a.Week, a.Day, formatTime(time.Now()), a.PlanID, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

func (db *DB) ClearActivePlan(ctx context.Context, userID string) error {
	_, err := db.write.ExecContext(ctx, `DELETE FROM active_plan WHERE user_id = ?`, userID)
	return err
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/store/ && golangci-lint run ./internal/...`
Expected: `ok`, `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/store
git commit -m "feat(store): add plans, versions and the active plan cursor"
```

---

### Task 4: Plan service

**Files:**
- Create: `internal/plan/templates/starter.json`, `internal/plan/service.go`
- Test: `internal/plan/service_test.go`

**Interfaces:**
- Consumes: store (Task 3), `exercise.Service.Get` and `Settings` (milestone 2), Tasks 1–2
- Produces:
  - Setup: `plan.Store` and `plan.Exercises` interfaces, `plan.Service{Store, Exercises}`, `plan.SaveDraft|SaveActivate`
  - Errors: `plan.ErrNoActiveVersion`, `plan.ErrNoSuchDay`
  - Helpers: `plan.StarterTemplate() []byte`, `plan.Pretty([]byte) string`, `plan.Decode([]byte) (Doc, error)`
  - Methods:
    - Documents and versions: `Validate(ctx, user, raw) (Doc, Problems)`, `Create(ctx, user, raw, status, source, note) (store.Plan, store.PlanVersion, error)`, `Save(ctx, user, planID, raw, status, source, note) (store.PlanVersion, bool /*cursor reset*/, error)`, `Plans(ctx, user) ([]PlanSummary, error)`, `Plan(ctx, user, planID) (store.Plan, []store.PlanVersion, error)`, `Version(ctx, user, versionID)`, `Activate(ctx, user, versionID) (bool, error)`, `Discard`, `Compare(ctx, user, versionID) (Comparison, error)`
    - Following: `Follow`, `Unfollow`, `Archive(ctx, user, planID, archived)`
    - Days and cursor: `Day(ctx, user, doc, week, day) (ExpandedDay, bool)`, `Preview(ctx, user, doc) [][]ExpandedDay`, `Next(ctx, user) (*Next, error)`, `Skip`, `Choose(ctx, user, week, day)`, `Restart`
  - `plan.PlanSummary{store.Plan; Active *store.PlanVersion; Drafts int; Following bool}`, `plan.Next{Plan; Version; Doc; Week, Day int; Complete bool; Today ExpandedDay}`, `plan.Comparison{Base *store.PlanVersion; Target store.PlanVersion; JSON []DiffLine; Days []DayChange; ResetsCursor bool}`, `plan.DayChange{Week int; Name string; Lines []DiffLine}`

- [ ] **Step 1: Add the starter template**

`internal/plan/templates/starter.json`:
```json
{
  "name": "Upper/Lower starter",
  "weeks": 4,
  "days": [
    {
      "name": "Upper",
      "groups": [
        {"rest_s": 180, "exercises": [
          {"slug": "barbell-bench-press", "alternatives": ["dumbbell-bench-press"], "sets": [
            {"kind": "warmup", "count": 2, "reps": 5, "load": {"pct_tm": 0.5}},
            {"count": [3, 3, 4, 2], "reps": 5, "load": {"pct_tm": [0.75, 0.8, 0.85, 0.65]}}
          ]}
        ]},
        {"rest_s": 120, "exercises": [
          {"slug": "barbell-row", "alternatives": ["seated-cable-row"], "sets": [{"count": 3, "reps": "8-10", "rpe": 8}]}
        ]},
        {"rest_s": 90, "exercises": [
          {"slug": "pull-up", "alternatives": ["lat-pulldown"], "sets": [{"count": 3, "reps": "6-10", "rpe": 8}]},
          {"slug": "dumbbell-lateral-raise", "sets": [{"count": 3, "reps": "12-15"}]}
        ]}
      ]
    },
    {
      "name": "Lower",
      "groups": [
        {"rest_s": 180, "exercises": [
          {"slug": "barbell-back-squat", "alternatives": ["hack-squat"], "sets": [
            {"kind": "warmup", "count": 2, "reps": 5, "load": {"pct_tm": 0.5}},
            {"count": [3, 3, 4, 2], "reps": 5, "load": {"pct_tm": [0.75, 0.8, 0.85, 0.65]}}
          ]}
        ]},
        {"rest_s": 150, "exercises": [
          {"slug": "barbell-romanian-deadlift", "sets": [{"count": 3, "reps": 8, "rpe": 7}]}
        ]},
        {"rest_s": 60, "exercises": [
          {"slug": "standing-calf-raise", "sets": [{"count": 3, "reps": "10-15"}]},
          {"slug": "plank", "sets": [{"count": 3, "duration_s": 45}]}
        ]}
      ]
    }
  ]
}
```

- [ ] **Step 2: Write the failing tests**

`internal/plan/service_test.go`:
```go
package plan

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc   *Service
	ex    *exercise.Service
	alice store.User
	bob   store.User
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := exercise.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	ex := &exercise.Service{Store: db}
	alice, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	if err := ex.EnsureStarterEquipment(ctx, alice); err != nil {
		t.Fatal(err)
	}
	return env{svc: &Service{Store: db, Exercises: ex}, ex: ex, alice: alice, bob: bob}
}

func TestStarterTemplateIsValid(t *testing.T) {
	e := newEnv(t)
	_, ps := e.svc.Validate(context.Background(), e.alice, StarterTemplate())
	if ps.HasErrors() {
		t.Fatalf("starter template: %v", ps)
	}
}

// weeksDoc is a small plan with two days a week for n weeks.
func weeksDoc(n int) []byte {
	return []byte(strings.NewReplacer("N", string(rune('0'+n))).Replace(`{"name": "Test", "weeks": N, "days": [
		{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5, "load": {"pct_tm": 0.75}}]}]}]},
		{"name": "B", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10"}]}]}]}]}`))
}

func TestFollowAndMoveThroughPlan(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	p, draft, err := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Follow(ctx, e.alice, p.ID); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("following a draft-only plan: %v", err)
	}
	if _, err := e.svc.Activate(ctx, e.alice, draft.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Follow(ctx, e.bob, p.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob follows alice's plan: %v", err)
	}
	if err := e.svc.Follow(ctx, e.alice, p.ID); err != nil {
		t.Fatal(err)
	}

	tm := 140.0
	if err := e.ex.SetTrainingMax(ctx, e.alice.ID, "barbell-back-squat", &tm, "web"); err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Next(ctx, e.alice)
	if err != nil || n == nil || n.Week != 1 || n.Day != 0 || n.Today.Name != "A" {
		t.Fatalf("next = %+v, %v", n, err)
	}
	squat := n.Today.Groups[0].Exercises[0]
	if squat.Name != "Barbell Back Squat" || squat.Sets[0].Kg == nil || *squat.Sets[0].Kg != 105 {
		t.Fatalf("resolved squat = %+v", squat)
	}

	for _, want := range [][2]int{{1, 1}, {2, 0}, {2, 1}} {
		if err := e.svc.Skip(ctx, e.alice); err != nil {
			t.Fatal(err)
		}
		n, _ = e.svc.Next(ctx, e.alice)
		if [2]int{n.Week, n.Day} != want {
			t.Fatalf("after skip: (%d, %d), want %v", n.Week, n.Day, want)
		}
	}
	_ = e.svc.Skip(ctx, e.alice)
	if n, _ = e.svc.Next(ctx, e.alice); !n.Complete {
		t.Fatalf("plan should be complete: %+v", n)
	}
	if err := e.svc.Skip(ctx, e.alice); err != nil {
		t.Fatal("skipping a complete plan must be harmless")
	}
	if err := e.svc.Restart(ctx, e.alice); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Choose(ctx, e.alice, 3, 0); !errors.Is(err, ErrNoSuchDay) {
		t.Fatalf("choosing week 3 of 2: %v", err)
	}
	if err := e.svc.Choose(ctx, e.alice, 2, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n.Week != 2 || n.Day != 1 {
		t.Fatalf("chosen day: (%d, %d)", n.Week, n.Day)
	}

	// A new version with the same days keeps the cursor...
	if _, reset, err := e.svc.Save(ctx, e.alice, p.ID, weeksDoc(3), SaveActivate, "web", ""); err != nil || reset {
		t.Fatalf("save 3 weeks: reset=%v err=%v", reset, err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n.Week != 2 || n.Day != 1 {
		t.Fatalf("cursor moved: (%d, %d)", n.Week, n.Day)
	}
	// ...one without the current day resets it, and the comparison warns first.
	draft1, _, err := e.svc.Save(ctx, e.alice, p.ID, weeksDoc(1), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := e.svc.Compare(ctx, e.alice, draft1.ID)
	if err != nil || !cmp.ResetsCursor || cmp.Base == nil || !Changed(cmp.JSON) || len(cmp.Days) != 4 {
		t.Fatalf("comparison = %+v, %v", cmp, err)
	}
	reset, err := e.svc.Activate(ctx, e.alice, draft1.ID)
	if err != nil || !reset {
		t.Fatalf("activate 1-week version: reset=%v err=%v", reset, err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n.Week != 1 || n.Day != 0 {
		t.Fatalf("cursor not reset: (%d, %d)", n.Week, n.Day)
	}

	// Archiving the followed plan stops following it.
	if err := e.svc.Archive(ctx, e.alice, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if n, _ = e.svc.Next(ctx, e.alice); n != nil {
		t.Fatal("archived plan still followed")
	}
}

func TestInvalidDocumentIsNotSaved(t *testing.T) {
	e := newEnv(t)
	_, _, err := e.svc.Create(context.Background(), e.alice, []byte(`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "nope", "sets": [{"count": 1, "reps": 1}]}]}]}]}`), SaveActivate, "web", "")
	var ps Problems
	if !errors.As(err, &ps) || ps[0].Pointer != "/days/0/groups/0/exercises/0/slug" {
		t.Fatalf("got %v", err)
	}
	plans, _ := e.svc.Plans(context.Background(), e.alice)
	if len(plans) != 0 {
		t.Fatalf("invalid plan was stored: %+v", plans)
	}
}

func TestPlansSummary(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, _, _ := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveActivate, "web", "")
	_, _, _ = e.svc.Save(ctx, e.alice, p.ID, weeksDoc(2), SaveDraft, "mcp", "")
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	list, err := e.svc.Plans(ctx, e.alice)
	if err != nil || len(list) != 1 || list[0].Active == nil || list[0].Drafts != 1 || !list[0].Following || list[0].Name != "Test" {
		t.Fatalf("plans = %+v, %v", list, err)
	}
	if other, _ := e.svc.Plans(ctx, e.bob); len(other) != 0 {
		t.Fatal("bob sees alice's plans")
	}
}

// Absolute weights mean what the lifter meant when saving: a plan without
// "unit" is pinned to the user's unit at save time.
func TestSavedPlanKeepsItsUnit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	raw := []byte(`{"name": "Fixed", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "dumbbell-curl", "sets": [{"count": 1, "reps": 10, "load": {"weight": 20}}]}]}]}]}`)
	p, v, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if doc, _ := Decode(v.Doc); doc.Unit != "kg" {
		t.Fatalf("stored unit = %q", doc.Unit)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	lbUser := e.alice
	lbUser.Unit = "lb"
	n, err := e.svc.Next(ctx, lbUser)
	if err != nil {
		t.Fatal(err)
	}
	// 20 kg dumbbells (the kg starter set has 20), not 20 lb.
	if kg := n.Today.Groups[0].Exercises[0].Sets[0].Kg; kg == nil || *kg != 20 {
		t.Fatalf("resolved = %v", kg)
	}
}

func TestPlanSurvivesDeletedCustomExercise(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	in := exercise.Input{Slug: "zercher-squat", Name: "Zercher Squat", Measurement: "weight_reps", EquipmentKind: "barbell", PrimaryMuscles: []string{"quads"}}
	if _, err := e.ex.Create(ctx, e.alice.ID, in); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"name": "Z", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "zercher-squat", "sets": [{"count": 3, "reps": 5}]}]}]}]}`)
	p, _, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	if err := e.ex.Delete(ctx, e.alice.ID, "zercher-squat"); err != nil {
		t.Fatal(err)
	}
	n, err := e.svc.Next(ctx, e.alice)
	if err != nil || n.Today.Groups[0].Exercises[0].Name != "zercher-squat" {
		t.Fatalf("next = %+v, %v", n, err)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/plan/`
Expected: FAIL, `undefined: Service`, `undefined: StarterTemplate`.

- [ ] **Step 4: Implement**

`internal/plan/service.go`:
```go
package plan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

//go:embed templates/starter.json
var starterTemplate []byte

// StarterTemplate is the document a new plan starts from.
func StarterTemplate() []byte { return starterTemplate }

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CreatePlan(ctx context.Context, userID, name string, doc []byte, status, source, note string) (store.Plan, store.PlanVersion, error)
	SavePlanVersion(ctx context.Context, userID, planID, name string, doc []byte, status, source, note string) (store.PlanVersion, error)
	ListPlans(ctx context.Context, userID string) ([]store.Plan, error)
	PlanByID(ctx context.Context, userID, id string) (store.Plan, error)
	SetPlanArchived(ctx context.Context, userID, id string, archived bool) error
	PlanVersions(ctx context.Context, userID, planID string) ([]store.PlanVersion, error)
	PlanVersionByID(ctx context.Context, userID, versionID string) (store.PlanVersion, error)
	ActivePlanVersion(ctx context.Context, userID, planID string) (store.PlanVersion, error)
	ActivateVersion(ctx context.Context, userID, versionID string) error
	DeleteDraft(ctx context.Context, userID, versionID string) error
	ActivePlan(ctx context.Context, userID string) (store.ActivePlan, error)
	SetActivePlan(ctx context.Context, userID string, a store.ActivePlan) error
	ClearActivePlan(ctx context.Context, userID string) error
}

// Exercises is what plans need from the catalog. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
	Settings(ctx context.Context, userID string, ex store.Exercise) (exercise.Settings, error)
}

// Service implements plan rules on top of the store.
type Service struct {
	Store     Store
	Exercises Exercises
}

// ErrNoActiveVersion is returned when following a plan that has only drafts.
var ErrNoActiveVersion = errors.New("the plan has no active version yet")

// Save statuses.
const (
	SaveDraft    = store.PlanDraft
	SaveActivate = store.PlanActive
)

// catalog answers validation and resolution questions for one user, caching
// lookups for the duration of a request.
type catalog struct {
	ctx   context.Context
	svc   *Service
	user  store.User
	cache map[string]*slugInfo
}

type slugInfo struct {
	ex       store.Exercise
	exists   bool
	settings exercise.Settings
}

func (s *Service) catalogFor(ctx context.Context, user store.User) *catalog {
	return &catalog{ctx: ctx, svc: s, user: user, cache: map[string]*slugInfo{}}
}

func (c *catalog) info(slug string) *slugInfo {
	if i, ok := c.cache[slug]; ok {
		return i
	}
	i := &slugInfo{}
	if ex, err := c.svc.Exercises.Get(c.ctx, c.user.ID, slug); err == nil {
		i.ex, i.exists = ex, true
		if st, err := c.svc.Exercises.Settings(c.ctx, c.user.ID, ex); err == nil {
			i.settings = st
		}
	}
	c.cache[slug] = i
	return i
}

func (c *catalog) Exercise(slug string) (bool, bool) {
	i := c.info(slug)
	return i.exists, i.ex.Hidden
}

func (c *catalog) HasTrainingMax(slug string) bool { return c.info(slug).settings.TrainingMaxKg != nil }

func (c *catalog) loadContext(slug string) calc.LoadContext {
	st := c.info(slug).settings
	ctx := calc.LoadContext{TMKg: st.TrainingMaxKg, Unit: c.user.Unit}
	if st.Equipment != nil {
		ctx.Equipment = &st.Equipment.Spec
	}
	return ctx
}

// Validate checks a document against the schema and the user's catalog.
func (s *Service) Validate(ctx context.Context, user store.User, raw []byte) (Doc, Problems) {
	return Validate(raw, s.catalogFor(ctx, user))
}

// Create stores a new plan from raw. With status SaveActivate it becomes the
// plan's active version. Invalid documents return Problems.
func (s *Service) Create(ctx context.Context, user store.User, raw []byte, status, source, note string) (store.Plan, store.PlanVersion, error) {
	doc, ps := s.Validate(ctx, user, raw)
	if ps.HasErrors() {
		return store.Plan{}, store.PlanVersion{}, ps
	}
	return s.Store.CreatePlan(ctx, user.ID, doc.Name, pinUnit(raw, doc, user.Unit), status, source, note)
}

// Save stores raw as the next version of planID. Activating a new version of
// the followed plan keeps the cursor when that day still exists (spec §8).
func (s *Service) Save(ctx context.Context, user store.User, planID string, raw []byte, status, source, note string) (store.PlanVersion, bool, error) {
	doc, ps := s.Validate(ctx, user, raw)
	if ps.HasErrors() {
		return store.PlanVersion{}, false, ps
	}
	v, err := s.Store.SavePlanVersion(ctx, user.ID, planID, doc.Name, pinUnit(raw, doc, user.Unit), status, source, note)
	if err != nil || status != SaveActivate {
		return v, false, err
	}
	reset, err := s.fitCursor(ctx, user, planID, doc)
	return v, reset, err
}

// compact stores documents without insignificant whitespace but keeps key order.
func compact(raw []byte) []byte {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return raw
	}
	return b.Bytes()
}

// pinUnit compacts raw and, when the plan has no "unit", adds the user's so
// its absolute weights keep their meaning if the user later changes unit.
func pinUnit(raw []byte, doc Doc, unit string) []byte {
	c := compact(raw)
	if doc.Unit != "" || len(c) < 2 || c[0] != '{' {
		return c
	}
	field, _ := json.Marshal(unit)
	out := append([]byte(`{"unit":`), field...)
	if c[1] != '}' {
		out = append(out, ',')
	}
	return append(out, c[1:]...)
}

// Pretty indents a stored document for editing.
func Pretty(raw []byte) string {
	var b bytes.Buffer
	if err := json.Indent(&b, raw, "", "  "); err != nil {
		return string(raw)
	}
	return b.String()
}

// Decode parses a stored (already validated) document.
func Decode(raw []byte) (Doc, error) {
	var doc Doc
	err := json.Unmarshal(raw, &doc)
	return doc, err
}

// keepsCursor reports whether a cursor at (week, day) still makes sense in
// doc: a training day, or the "complete" position just past the last week.
func keepsCursor(doc Doc, week, day int) bool {
	return ValidPosition(doc, week, day) || (week == doc.Weeks+1 && day == 0)
}

// fitCursor resets the cursor of a followed plan whose day no longer exists.
// It reports whether it reset.
func (s *Service) fitCursor(ctx context.Context, user store.User, planID string, doc Doc) (bool, error) {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && a.PlanID != planID) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if keepsCursor(doc, a.Week, a.Day) {
		return false, nil
	}
	return true, s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: 1, Day: 0})
}

// PlanSummary is a plan as listed.
type PlanSummary struct {
	store.Plan
	Active    *store.PlanVersion // nil if the plan has only drafts
	Drafts    int
	Following bool
}

func (s *Service) Plans(ctx context.Context, user store.User) ([]PlanSummary, error) {
	plans, err := s.Store.ListPlans(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	following, err := s.Store.ActivePlan(ctx, user.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	out := make([]PlanSummary, 0, len(plans))
	for _, p := range plans {
		versions, err := s.Store.PlanVersions(ctx, user.ID, p.ID)
		if err != nil {
			return nil, err
		}
		sum := PlanSummary{Plan: p, Following: following.PlanID == p.ID}
		for i, v := range versions {
			switch v.Status {
			case store.PlanActive:
				sum.Active = &versions[i]
			case store.PlanDraft:
				sum.Drafts++
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

// Plan returns a plan with its versions, newest first.
func (s *Service) Plan(ctx context.Context, user store.User, planID string) (store.Plan, []store.PlanVersion, error) {
	p, err := s.Store.PlanByID(ctx, user.ID, planID)
	if err != nil {
		return store.Plan{}, nil, err
	}
	vs, err := s.Store.PlanVersions(ctx, user.ID, planID)
	return p, vs, err
}

// Version returns a version of one of the user's plans.
func (s *Service) Version(ctx context.Context, user store.User, versionID string) (store.PlanVersion, error) {
	return s.Store.PlanVersionByID(ctx, user.ID, versionID)
}

// Activate makes a version active. It reports whether the cursor of the
// followed plan had to be reset because its day no longer exists.
func (s *Service) Activate(ctx context.Context, user store.User, versionID string) (bool, error) {
	v, err := s.Store.PlanVersionByID(ctx, user.ID, versionID)
	if err != nil {
		return false, err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return false, err
	}
	if err := s.Store.ActivateVersion(ctx, user.ID, versionID); err != nil {
		return false, err
	}
	return s.fitCursor(ctx, user, v.PlanID, doc)
}

// Discard deletes a draft.
func (s *Service) Discard(ctx context.Context, user store.User, versionID string) error {
	return s.Store.DeleteDraft(ctx, user.ID, versionID)
}

// Follow makes planID the user's plan, starting at week 1, day 1.
func (s *Service) Follow(ctx context.Context, user store.User, planID string) error {
	if _, err := s.Store.ActivePlanVersion(ctx, user.ID, planID); errors.Is(err, store.ErrNotFound) {
		if _, perr := s.Store.PlanByID(ctx, user.ID, planID); perr != nil {
			return perr
		}
		return ErrNoActiveVersion
	} else if err != nil {
		return err
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: 1, Day: 0})
}

func (s *Service) Unfollow(ctx context.Context, user store.User) error {
	return s.Store.ClearActivePlan(ctx, user.ID)
}

// Archive hides or restores a plan. Archiving the followed plan stops following it.
func (s *Service) Archive(ctx context.Context, user store.User, planID string, archived bool) error {
	if err := s.Store.SetPlanArchived(ctx, user.ID, planID, archived); err != nil {
		return err
	}
	if a, err := s.Store.ActivePlan(ctx, user.ID); archived && err == nil && a.PlanID == planID {
		return s.Store.ClearActivePlan(ctx, user.ID)
	}
	return nil
}

// Day expands and resolves (week, day) of doc for the user, with exercise
// names filled in. ok is false when the day does not exist.
func (s *Service) Day(ctx context.Context, user store.User, doc Doc, week, day int) (ExpandedDay, bool) {
	return s.catalogFor(ctx, user).day(doc, week, day)
}

func (c *catalog) day(doc Doc, week, day int) (ExpandedDay, bool) {
	d, ok := Expand(doc, week, day, c.user.Unit)
	if !ok {
		return d, false
	}
	for gi := range d.Groups {
		for si := range d.Groups[gi].Exercises {
			slot := &d.Groups[gi].Exercises[si]
			if i := c.info(slot.Slug); i.exists {
				slot.Name = i.ex.Name
			}
		}
	}
	Resolve(&d, c.loadContext)
	return d, true
}

// Preview expands and resolves every day of every week.
func (s *Service) Preview(ctx context.Context, user store.User, doc Doc) [][]ExpandedDay {
	c := s.catalogFor(ctx, user)
	weeks := make([][]ExpandedDay, 0, doc.Weeks)
	for w := 1; w <= doc.Weeks; w++ {
		var days []ExpandedDay
		for d := range DaysForWeek(doc, w) {
			day, _ := c.day(doc, w, d)
			days = append(days, day)
		}
		weeks = append(weeks, days)
	}
	return weeks
}

// Next is where the user is in the followed plan.
type Next struct {
	Plan     store.Plan
	Version  store.PlanVersion
	Doc      Doc
	Week     int
	Day      int
	Complete bool        // past the last week
	Today    ExpandedDay // the next training day (zero when complete)
}

// Next returns the user's position in their plan, or nil if they follow none.
func (s *Service) Next(ctx context.Context, user store.User) (*Next, error) {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p, err := s.Store.PlanByID(ctx, user.ID, a.PlanID)
	if err != nil {
		return nil, err
	}
	v, err := s.Store.ActivePlanVersion(ctx, user.ID, a.PlanID)
	if err != nil {
		return nil, err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return nil, err
	}
	n := &Next{Plan: p, Version: v, Doc: doc, Week: a.Week, Day: a.Day}
	if a.Week > doc.Weeks {
		n.Complete = true
		return n, nil
	}
	if n.Today, _ = s.Day(ctx, user, doc, a.Week, a.Day); n.Today.Name == "" {
		// The stored position no longer exists; start the block over.
		n.Week, n.Day = 1, 0
		n.Today, _ = s.Day(ctx, user, doc, 1, 0)
	}
	return n, nil
}

// Skip moves the cursor past the next day without training it.
func (s *Service) Skip(ctx context.Context, user store.User) error {
	n, err := s.Next(ctx, user)
	if err != nil || n == nil || n.Complete {
		return err
	}
	w, d := NextPosition(n.Doc, n.Week, n.Day)
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: n.Plan.ID, Week: w, Day: d})
}

// ErrNoSuchDay is returned when choosing a day the plan does not have.
var ErrNoSuchDay = errors.New("the plan has no such day")

// Choose moves the cursor to (week, day).
func (s *Service) Choose(ctx context.Context, user store.User, week, day int) error {
	n, err := s.Next(ctx, user)
	if err != nil {
		return err
	}
	if n == nil || !ValidPosition(n.Doc, week, day) {
		return ErrNoSuchDay
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: n.Plan.ID, Week: week, Day: day})
}

// Restart moves the cursor back to week 1, day 1.
func (s *Service) Restart(ctx context.Context, user store.User) error {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if err != nil {
		return err
	}
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: a.PlanID, Week: 1, Day: 0})
}

// Comparison is a version compared with the plan's active version.
type Comparison struct {
	Base   *store.PlanVersion // nil when the plan has no active version
	Target store.PlanVersion
	JSON   []DiffLine
	Days   []DayChange // only days whose prescription changed
	// ResetsCursor is true when activating Target would move the followed
	// plan back to week 1, day 1.
	ResetsCursor bool
}

// DayChange is the prescription diff of one (week, day name).
type DayChange struct {
	Week  int
	Name  string
	Lines []DiffLine
}

// Compare diffs a version against the plan's active version.
func (s *Service) Compare(ctx context.Context, user store.User, versionID string) (Comparison, error) {
	target, err := s.Store.PlanVersionByID(ctx, user.ID, versionID)
	if err != nil {
		return Comparison{}, err
	}
	cmp := Comparison{Target: target}
	var baseDoc Doc
	var baseRaw []byte
	if base, err := s.Store.ActivePlanVersion(ctx, user.ID, target.PlanID); err == nil {
		cmp.Base, baseRaw = &base, base.Doc
		if baseDoc, err = Decode(base.Doc); err != nil {
			return Comparison{}, err
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		return Comparison{}, err
	}
	targetDoc, err := Decode(target.Doc)
	if err != nil {
		return Comparison{}, err
	}
	cmp.JSON = DiffLines(lines(baseRaw), lines(target.Doc))

	c := s.catalogFor(ctx, user)
	for w := 1; w <= max(baseDoc.Weeks, targetDoc.Weeks); w++ {
		names := dayNames(baseDoc, w)
		for _, n := range dayNames(targetDoc, w) {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
		for _, name := range names {
			a := dayLinesByName(c, baseDoc, w, name)
			b := dayLinesByName(c, targetDoc, w, name)
			if d := DiffLines(a, b); Changed(d) {
				cmp.Days = append(cmp.Days, DayChange{Week: w, Name: name, Lines: d})
			}
		}
	}

	if a, err := s.Store.ActivePlan(ctx, user.ID); err == nil && a.PlanID == target.PlanID {
		cmp.ResetsCursor = !keepsCursor(targetDoc, a.Week, a.Day)
	}
	return cmp, nil
}

func lines(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	return strings.Split(Pretty(raw), "\n")
}

func dayNames(doc Doc, week int) []string {
	var out []string
	if week > doc.Weeks {
		return nil
	}
	for _, i := range DaysForWeek(doc, week) {
		out = append(out, doc.Days[i].Name)
	}
	return out
}

func dayLinesByName(c *catalog, doc Doc, week int, name string) []string {
	for d, i := range DaysForWeek(doc, week) {
		if week <= doc.Weeks && doc.Days[i].Name == name {
			day, _ := c.day(doc, week, d)
			return DayLines(day, c.user.Unit)
		}
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/plan/ && golangci-lint run ./internal/...`
Expected: `ok`, `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/plan
git commit -m "feat(plan): add plan service with versions, following, cursor, preview and comparison"
```

---

### Task 5: Vendored editor and editor script

**Files:**
- Create (vendored): `internal/web/static/vendor/jsoneditor/standalone.min.js`, `jse-theme-dark.css`, `LICENSE`
- Create: `internal/web/static/js/plan-editor.js`

**Interfaces:**
- Consumes: nothing
- Produces: `/static/js/plan-editor.js`. It upgrades `<textarea id="doc">` when `#doc-editor` and `<script id="plan-schema" type="application/json">` exist on the page, copies edits back to the textarea, and fires a bubbling `doc-changed` event on it.

This task has no automated test of its own. Task 6 step 7 checks the editor mounts in headless Chromium.

- [ ] **Step 1: Vendor vanilla-jsoneditor 3.13.0**

```bash
mkdir -p internal/web/static/vendor/jsoneditor
cd internal/web/static/vendor/jsoneditor
curl -fsSL -o standalone.min.js  https://cdn.jsdelivr.net/npm/vanilla-jsoneditor@3.13.0/standalone.min.js
curl -fsSL -o jse-theme-dark.css https://cdn.jsdelivr.net/npm/vanilla-jsoneditor@3.13.0/themes/jse-theme-dark.css
curl -fsSL -o LICENSE            https://cdn.jsdelivr.net/npm/vanilla-jsoneditor@3.13.0/LICENSE.md
sha256sum standalone.min.js jse-theme-dark.css LICENSE
cd -
```

Expected checksums:
```
f713438db1a33e8f99f879087ed59b8d7862ec4b2e778c04a6f2292abbc0cd61  standalone.min.js
b88bd4a28ad06c7647567fba5e25d290c6b60e87991a8bfb915e20161d9948a9  jse-theme-dark.css
fd50f5abb7eeb614c8d1293b9f239a5238d7536e3a9a9a8929a082c82d12b3fd  LICENSE
```

- [ ] **Step 2: Write the editor script**

`internal/web/static/js/plan-editor.js`:
```javascript
// Upgrades the plan editor's <textarea id="doc"> to vanilla-jsoneditor with
// schema validation. The textarea stays the form field: every change is
// copied back and announced with a "doc-changed" event, which htmx uses to
// refresh the server-side preview. Without JS the textarea works on its own.
import { createAjvValidator, createJSONEditor } from "/static/vendor/jsoneditor/standalone.min.js";

const textarea = document.getElementById("doc");
const holder = document.getElementById("doc-editor");
const schemaEl = document.getElementById("plan-schema");

if (textarea && holder && schemaEl) {
  // The editor's validator (Ajv) predates draft 2020-12; the schema only uses
  // keywords both understand, so drop the $schema marker.
  const { $schema: _draft, ...schema } = JSON.parse(schemaEl.textContent);
  if (window.matchMedia("(prefers-color-scheme: dark)").matches) {
    holder.classList.add("jse-theme-dark");
  }
  textarea.hidden = true;
  holder.hidden = false;
  createJSONEditor({
    target: holder,
    props: {
      content: { text: textarea.value },
      mode: "text",
      validator: createAjvValidator({ schema }),
      onChange(content) {
        textarea.value = content.text !== undefined ? content.text : JSON.stringify(content.json, null, 2);
        textarea.dispatchEvent(new Event("doc-changed", { bubbles: true }));
      },
    },
  });
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/vendor/jsoneditor internal/web/static/js/plan-editor.js
git commit -m "feat(web): vendor vanilla-jsoneditor and add the plan editor script"
```

---

### Task 6: Plan pages and the home page

**Files:**
- Create: `internal/web/views/plans.templ`, `internal/web/plans.go`
- Replace: `internal/web/views/pages.templ`, `internal/web/views/models.go`, `internal/web/views/ui.go`, `internal/web/server.go`, `internal/web/server_test.go`, `cmd/onerep/main.go`
- Create (generated): `internal/web/views/*_templ.go`, `internal/web/static/app.css`
- Test: `internal/web/plans_test.go`

**Interfaces:**
- Consumes: `plan.Service` (Task 4), editor script (Task 5)
- Produces:
  - `web.Server.Plans *plan.Service`, and the `limitBody` middleware (1 MB, 413) on authenticated routes
  - Public route: `GET /schema/plan.json`
  - Plan routes: `GET /plans`, `GET /plans/new`, `POST /plans` (`doc`, `action=draft|activate`), `POST /plans/preview` (fragment), `GET /plans/{id}`, `GET /plans/{id}/edit[?from=vid]`, `POST /plans/{id}`, `POST /plans/{id}/follow`, `POST /plans/{id}/archive` (`archived=1|0`)
  - Version routes: `GET /plans/{id}/versions/{vid}`, `GET …/compare`, `POST …/activate`, `POST …/discard`
  - Cursor routes: `POST /plan/skip`, `POST /plan/choose` (`position=week:day`), `POST /plan/restart`, `POST /plan/unfollow`
  - Views: `views.Home(Page, HomePage)`, `views.PlanDay(Page, plan.ExpandedDay)` (reused by milestone 4), `views.DayOptions(plan.Doc)`

- [ ] **Step 1: Write the failing tests**

Replace `internal/web/server_test.go` with (it wires the plan service):
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
	"github.com/LongerHV/onerep/internal/plan"
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
	s.Plans = &plan.Service{Store: db, Exercises: s.Exercises}
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

`internal/web/plans_test.go`:
```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
)

var starter = string(plan.StarterTemplate())

func planID(t *testing.T, resp *http.Response) string {
	t.Helper()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/plans/") {
		t.Fatalf("expected redirect to a plan, got %d %q", resp.StatusCode, loc)
	}
	return strings.TrimSuffix(strings.TrimPrefix(loc, "/plans/"), "?reset")
}

func TestPlanEditorPage(t *testing.T) {
	srv, c := newApp(t, "alice")
	session(t, srv, c)
	html := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	if !strings.Contains(html, "Upper/Lower starter") || !strings.Contains(html, `hx-post="/plans/preview"`) {
		t.Fatal("editor misses the template or the preview hook")
	}
	m := regexp.MustCompile(`(?s)<script id="plan-schema" type="application/json">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil || !json.Valid([]byte(m[1])) {
		t.Fatalf("schema is not embedded as parseable JSON:\n%s", html)
	}
}

func TestPlanPreview(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	_, frag := post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {starter}})
	if strings.Contains(frag, "<html") || !strings.Contains(frag, "Week 4") || !strings.Contains(frag, "Barbell Bench Press") ||
		!strings.Contains(frag, "3 × 5 @ 75% TM") {
		t.Fatalf("preview:\n%s", frag)
	}
	_, frag = post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {`{"name": "x", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [{"slug": "nope", "sets": [{"count": 1, "reps": 1}]}]}]}]}`}})
	if !strings.Contains(frag, "/days/0/groups/0/exercises/0/slug") || !strings.Contains(frag, "unknown exercise") || strings.Contains(frag, "Week 1") {
		t.Fatalf("error preview:\n%s", frag)
	}
}

func TestFollowPlanAndMoveThroughDays(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	id := planID(t, resp)

	resp, _ = post(t, c, srv.URL+"/plans/"+id+"/follow", csrf, nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("follow: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/exercises/barbell-bench-press/settings", csrf, url.Values{"training_max": {"120"}})
	home := read(t, mustGet(t, c, srv.URL+"/"))
	for _, want := range []string{"Next: Upper", "week 1 of 4", "2 × 5 warmup @ 50% TM", "60 kg", "3 × 5 @ 75% TM", "90 kg"} {
		if !strings.Contains(home, want) {
			t.Fatalf("home misses %q:\n%s", want, home)
		}
	}
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "Next: Lower") {
		t.Fatal("skip did not move to Lower")
	}
	post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"4:0"}})
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 4 of 4") || !strings.Contains(home, "Next: Upper") {
		t.Fatal("choose did not move to week 4")
	}
	resp, _ = post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"9:0"}})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("choosing a missing day: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	post(t, c, srv.URL+"/plan/skip", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "You finished all 4 weeks") {
		t.Fatal("plan should be complete")
	}
	post(t, c, srv.URL+"/plan/restart", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 1 of 4") {
		t.Fatal("restart did not go back to week 1")
	}
}

func TestReviewDraftThatResetsCursor(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	id := planID(t, resp)
	post(t, c, srv.URL+"/plans/"+id+"/follow", csrf, nil)
	post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {"4:1"}})

	shorter := strings.Replace(starter, `"weeks": 4`, `"weeks": 1`, 1)
	shorter = regexp.MustCompile(`\[(\d+(?:\.\d+)?), [^\]]*\]`).ReplaceAllString(shorter, "$1") // per-week arrays -> week 1 value
	resp, _ = post(t, c, srv.URL+"/plans/"+id, csrf, url.Values{"doc": {shorter}, "action": {"draft"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft: %d", resp.StatusCode)
	}
	detail := read(t, mustGet(t, c, srv.URL+"/plans/"+id))
	m := regexp.MustCompile(`/plans/` + id + `/versions/([0-9a-f-]+)/compare`).FindStringSubmatch(detail)
	if m == nil || !strings.Contains(detail, "Review") {
		t.Fatalf("draft not offered for review:\n%s", detail)
	}
	cmp := read(t, mustGet(t, c, srv.URL+"/plans/"+id+"/versions/"+m[1]+"/compare"))
	if !strings.Contains(cmp, "starts the plan over at week 1") || !strings.Contains(cmp, "Week 2 · Upper") {
		t.Fatalf("comparison:\n%s", cmp)
	}
	resp, _ = post(t, c, srv.URL+"/plans/"+id+"/versions/"+m[1]+"/activate", csrf, nil)
	if resp.Header.Get("Location") != "/plans/"+id+"?reset" {
		t.Fatalf("activate redirect: %s", resp.Header.Get("Location"))
	}
	if page := read(t, mustGet(t, c, srv.URL+"/plans/"+id+"?reset")); !strings.Contains(page, "starts over at week 1") {
		t.Fatal("reset notice missing")
	}
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "week 1 of 1") {
		t.Fatal("cursor not reset")
	}
}

func TestInvalidPlanKeepsTheText(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	bad := `{"name": "Mine", "weeks": 0, "days": []}`
	resp, body := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {bad}, "action": {"activate"}})
	if resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "/weeks") ||
		!strings.Contains(body, `{&#34;name&#34;: &#34;Mine&#34;`) {
		t.Fatalf("got %d:\n%s", resp.StatusCode, body)
	}
}

func TestPlansAreIsolatedAndSchemaIsPublic(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	session(t, srv, c)
	ctx := context.Background()
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	svc := &plan.Service{Store: db, Exercises: &exercise.Service{Store: db}}
	p, v, err := svc.Create(ctx, bob, plan.StarterTemplate(), plan.SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/plans/" + p.ID, "/plans/" + p.ID + "/edit", "/plans/" + p.ID + "/versions/" + v.ID} {
		if resp := mustGet(t, c, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}

	anon, ac := newApp(t, "")
	resp := mustGet(t, ac, anon.URL+"/schema/plan.json")
	if body := read(t, resp); resp.StatusCode != http.StatusOK || !json.Valid([]byte(body)) || resp.Header.Get("Content-Type") != "application/schema+json" {
		t.Fatalf("schema: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
}

func TestChooseRejectsNegativeDay(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, _ := post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	post(t, c, srv.URL+"/plans/"+planID(t, resp)+"/follow", csrf, nil)
	for _, pos := range []string{"1:-1", "0:0", "x", ""} {
		if resp, _ := post(t, c, srv.URL+"/plan/choose", csrf, url.Values{"position": {pos}}); resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("position %q: %d", pos, resp.StatusCode)
		}
	}
}

func TestOversizedPlanDocumentIsRefused(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	big := `{"name": "` + strings.Repeat("x", 1<<20) + `"}`
	resp, body := post(t, c, srv.URL+"/plans/preview", csrf, url.Values{"doc": {big}})
	if !strings.Contains(body, "too large") {
		t.Fatalf("preview of a 1 MB document: %d\n%.300s", resp.StatusCode, body)
	}
	resp, body = post(t, c, srv.URL+"/plans", csrf, url.Values{"doc": {big}, "action": {"draft"}})
	if resp.StatusCode != http.StatusRequestEntityTooLarge || !strings.Contains(body, "too large") {
		t.Fatalf("saving a 1 MB document: %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web/`
Expected: FAIL, `s.Plans undefined (type *Server has no field or method Plans)`.

- [ ] **Step 3: Views**

Replace `internal/web/views/models.go` with:
```go
package views

import (
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
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
	Notice string
}
```

Replace `internal/web/views/ui.go` with:
```go
package views

import (
	"strconv"
	"strings"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
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

// DayOption is one entry of the "go to day" select.
type DayOption struct {
	Week, Day    int
	Value, Label string
}

// DayOptions lists every training day of a plan.
func DayOptions(doc plan.Doc) []DayOption {
	var out []DayOption
	for w := 1; w <= doc.Weeks; w++ {
		for d, i := range plan.DaysForWeek(doc, w) {
			out = append(out, DayOption{Week: w, Day: d, Value: strconv.Itoa(w) + ":" + strconv.Itoa(d),
				Label: "Week " + strconv.Itoa(w) + " · " + doc.Days[i].Name})
		}
	}
	return out
}
```

Replace `internal/web/views/pages.templ` with (the home page shows the next workout):
```templ
package views

import "strconv"

templ Home(p Page, h HomePage) {
	@Layout(p) {
		<h1 class={ h1 }>Hi, { p.Identity.User.Name }</h1>
		if h.Notice != "" {
			<p class="mt-4 rounded bg-amber-100 p-2 text-sm dark:bg-amber-950">{ h.Notice }</p>
		}
		switch {
			case h.Next == nil:
				<p class="mt-2 text-zinc-600 dark:text-zinc-400">
					You are not following a plan. <a href="/plans" class="underline">Choose or create one</a>.
				</p>
				<ul class="mt-6 list-inside list-disc space-y-1">
					<li><a href="/exercises" class="underline">Browse exercises</a> and set training maxes</li>
					<li><a href="/equipment" class="underline">Set up your equipment</a> so weights round to what you can load</li>
				</ul>
			case h.Next.Complete:
				<div class={ card + " mt-4" }>
					<p>
						You finished all { strconv.Itoa(h.Next.Doc.Weeks) } weeks of
						<a href={ templ.URL("/plans/" + h.Next.Plan.ID) } class="underline">{ h.Next.Plan.Name }</a>.
					</p>
					<p class="mt-1 text-sm text-zinc-500">Raise your training maxes and run it again, or ask your AI assistant to draft the next block.</p>
					<form method="post" action="/plan/restart" class="mt-3">
						@CSRF(p)
						<button type="submit" class={ btn }>Start the block again</button>
					</form>
				</div>
			default:
				<div class="mt-4">
					<p class="text-sm text-zinc-500">
						<a href={ templ.URL("/plans/" + h.Next.Plan.ID) } class="underline">{ h.Next.Plan.Name }</a>
						· week { strconv.Itoa(h.Next.Week) } of { strconv.Itoa(h.Next.Doc.Weeks) }
					</p>
					<h2 class="text-xl font-semibold">Next: { h.Next.Today.Name }</h2>
					@PlanDay(p, h.Next.Today)
					<div class="mt-3 flex flex-wrap items-center gap-2">
						<form method="post" action="/plan/skip">
							@CSRF(p)
							<button type="submit" class={ btnSecondary }>Skip this day</button>
						</form>
						<form method="post" action="/plan/choose" class="flex gap-2">
							@CSRF(p)
							<select name="position" aria-label="Train another day" class={ input + " mt-0" }>
								for _, o := range DayOptions(h.Next.Doc) {
									<option value={ o.Value } selected?={ o.Week == h.Next.Week && o.Day == h.Next.Day }>{ o.Label }</option>
								}
							</select>
							<button type="submit" class={ btnSecondary }>Go to day</button>
						</form>
					</div>
				</div>
		}
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

`internal/web/views/plans.templ`: the schema is embedded with `templ.JSONScript`, since browsers don't decode entities inside `<script>`:
```templ
package views

import (
	"encoding/json"
	"strconv"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

templ PlansPage(p Page, plans []plan.PlanSummary) {
	@Layout(p) {
		<div class="flex items-center justify-between">
			<h1 class={ h1 }>Plans</h1>
			<a href="/plans/new" class={ btn }>New plan</a>
		</div>
		if len(plans) == 0 {
			<p class="mt-4 text-zinc-500">No plans yet. Start from the template, or ask your AI assistant to draft one.</p>
		}
		<ul class="mt-4 space-y-2">
			for _, s := range plans {
				<li class={ card + " flex items-center justify-between gap-4" }>
					<div>
						<a href={ templ.URL("/plans/" + s.ID) } class="font-medium underline">{ s.Name }</a>
						if s.Following {
							<span class={ badge }>following</span>
						}
						if s.Archived {
							<span class={ badge }>archived</span>
						}
						<p class="text-sm text-zinc-500">
							if s.Active != nil {
								Version { strconv.Itoa(s.Active.Version) } active
							} else {
								No active version
							}
							if s.Drafts > 0 {
								· { strconv.Itoa(s.Drafts) } draft(s) to review
							}
						</p>
					</div>
				</li>
			}
		</ul>
	}
}

templ PlanDetailPage(p Page, d PlanPage) {
	@Layout(p) {
		<div class="flex flex-wrap items-start justify-between gap-2">
			<div>
				<h1 class={ h1 }>{ d.Plan.Name }</h1>
				if d.Following {
					<p class="text-sm text-zinc-500">You are following this plan.</p>
				}
			</div>
			<div class="flex flex-wrap gap-2">
				<a href={ templ.URL("/plans/" + d.Plan.ID + "/edit") } class={ btnSecondary }>Edit</a>
				if !d.Following && !d.Plan.Archived {
					<form method="post" action={ templ.URL("/plans/" + d.Plan.ID + "/follow") }>
						@CSRF(p)
						<button type="submit" class={ btn }>Follow this plan</button>
					</form>
				}
				<form method="post" action={ templ.URL("/plans/" + d.Plan.ID + "/archive") }>
					@CSRF(p)
					if d.Plan.Archived {
						<input type="hidden" name="archived" value="0"/>
						<button type="submit" class={ btnSecondary }>Restore</button>
					} else {
						<input type="hidden" name="archived" value="1"/>
						<button type="submit" class={ btnSecondary }>Archive</button>
					}
				</form>
			</div>
		</div>
		if d.Notice != "" {
			<p class="mt-4 rounded bg-amber-100 p-2 text-sm dark:bg-amber-950">{ d.Notice }</p>
		}
		<h2 class={ h2 }>Versions</h2>
		<table class="mt-2 w-full text-sm">
			<thead class="text-left text-zinc-500">
				<tr><th class="py-1">Version</th><th>Status</th><th>By</th><th>Saved</th><th></th></tr>
			</thead>
			<tbody>
				for _, v := range d.Versions {
					<tr class="border-t border-zinc-200 dark:border-zinc-800">
						<td class="py-1">
							<a href={ templ.URL("/plans/" + d.Plan.ID + "/versions/" + v.ID) } class="underline">v{ strconv.Itoa(v.Version) }</a>
							if v.Note != "" {
								<span class="text-zinc-500">{ v.Note }</span>
							}
						</td>
						<td>{ v.Status }</td>
						<td>{ v.Source }</td>
						<td>{ v.CreatedAt.Format("2006-01-02 15:04") }</td>
						<td class="text-right">
							if v.Status != store.PlanActive {
								<a href={ templ.URL("/plans/" + d.Plan.ID + "/versions/" + v.ID + "/compare") } class="underline">
									if v.Status == store.PlanDraft {
										Review
									} else {
										Compare
									}
								</a>
							}
						</td>
					</tr>
				}
			</tbody>
		</table>
	}
}

templ PlanEditorPage(p Page, e PlanEditor) {
	@Layout(p) {
		<link rel="stylesheet" href="/static/vendor/jsoneditor/jse-theme-dark.css"/>
		<h1 class={ h1 }>{ e.Title }</h1>
		<p class="mt-1 text-sm text-zinc-500">
			The plan is a JSON document. <a href="/schema/plan.json" class="underline">Schema</a>:
			values marked "per week" can be one value or an array with one entry per week.
		</p>
		<form
			method="post"
			if e.PlanID == "" {
				action="/plans"
			} else {
				action={ templ.URL("/plans/" + e.PlanID) }
			}
			class="mt-4 grid gap-4 lg:grid-cols-2"
		>
			@CSRF(p)
			<div>
				<textarea
					id="doc"
					name="doc"
					rows="30"
					spellcheck="false"
					aria-label="Plan document (JSON)"
					class={ input + " font-mono text-xs" }
					hx-post="/plans/preview"
					hx-trigger="load, keyup changed delay:500ms, doc-changed delay:500ms"
					hx-target="#plan-preview"
				>{ e.Doc }</textarea>
				<div id="doc-editor" class="h-[70vh]" hidden></div>
				<div class="mt-2 flex gap-2">
					<button type="submit" name="action" value="draft" class={ btnSecondary }>Save as draft</button>
					<button type="submit" name="action" value="activate" class={ btn }>Save and activate</button>
				</div>
			</div>
			<div id="plan-preview" aria-live="polite">
				@PlanProblems(e.Errors)
			</div>
		</form>
		@templ.JSONScript("plan-schema", json.RawMessage(e.Schema))
		<script type="module" src="/static/js/plan-editor.js"></script>
	}
}

templ PlanProblems(ps plan.Problems) {
	if len(ps) > 0 {
		<ul class="space-y-1 text-sm">
			for _, pr := range ps {
				<li
					if pr.Warning {
						class="rounded bg-amber-100 px-2 py-1 dark:bg-amber-950"
					} else {
						class="rounded bg-red-100 px-2 py-1 dark:bg-red-950"
					}
				>
					if pr.Pointer != "" {
						<code>{ pr.Pointer }</code>:
					}
					{ pr.Message }
				</li>
			}
		</ul>
	}
}

// PlanPreview is the editor's live preview: problems, then every week.
templ PlanPreview(p Page, ps plan.Problems, weeks [][]plan.ExpandedDay) {
	@PlanProblems(ps)
	if !ps.HasErrors() {
		@PlanWeeks(p, weeks)
	}
}

templ PlanWeeks(p Page, weeks [][]plan.ExpandedDay) {
	for wi, days := range weeks {
		<details class="mt-2" open?={ wi == 0 }>
			<summary class="cursor-pointer font-medium">Week { strconv.Itoa(wi + 1) }</summary>
			for _, d := range days {
				@PlanDay(p, d)
			}
		</details>
	}
}

templ PlanDay(p Page, d plan.ExpandedDay) {
	<div class={ card + " mt-2" }>
		<h3 class="font-medium">{ d.Name }</h3>
		for gi, g := range d.Groups {
			<div class="mt-2">
				<p class="text-xs text-zinc-500">
					if len(g.Exercises) > 1 {
						Superset,
					}
					rest { strconv.Itoa(g.RestS) }s
				</p>
				for _, slot := range g.Exercises {
					<div class="mt-1">
						<p>
							<a href={ templ.URL("/exercises/" + slot.Slug) } class="underline">{ slot.Name }</a>
							if slot.Notes != "" {
								<span class="text-sm text-zinc-500">{ slot.Notes }</span>
							}
						</p>
						<ul class="ml-4 text-sm">
							for _, sg := range plan.GroupSets(slot.Sets) {
								<li>
									{ plan.Prescription(sg, p.Identity.User.Unit) }
									if sg.Set.Kg != nil {
										→ <strong>{ Weight(*sg.Set.Kg, p.Identity.User.Unit) }</strong>
										if len(sg.Set.PerSide) > 0 {
											<span class="text-zinc-500">(per side { plates(sg.Set.PerSide) })</span>
										}
									} else if sg.Set.LoadKind != "" {
										<span class="text-zinc-500">→ pick weight</span>
									}
								</li>
							}
						</ul>
					</div>
				}
			</div>
			if gi < len(d.Groups)-1 {
				<hr class="mt-2 border-zinc-200 dark:border-zinc-800"/>
			}
		}
	</div>
}

templ PlanVersionView(p Page, d PlanVersionPage) {
	@Layout(p) {
		<p class="text-sm"><a href={ templ.URL("/plans/" + d.Plan.ID) } class="underline">{ d.Plan.Name }</a></p>
		<h1 class={ h1 }>Version { strconv.Itoa(d.Version.Version) } <span class={ badge }>{ d.Version.Status }</span></h1>
		<div class="mt-2 flex gap-2">
			<a href={ templ.URL("/plans/" + d.Plan.ID + "/edit?from=" + d.Version.ID) } class={ btnSecondary }>Edit from this version</a>
			if d.Version.Status != store.PlanActive {
				<a href={ templ.URL("/plans/" + d.Plan.ID + "/versions/" + d.Version.ID + "/compare") } class={ btnSecondary }>Compare with active</a>
			}
		</div>
		@PlanWeeks(p, d.Weeks)
		<details class="mt-4">
			<summary class="cursor-pointer text-sm text-zinc-500">JSON</summary>
			<pre class="mt-2 overflow-x-auto text-xs">{ d.JSON }</pre>
		</details>
	}
}

templ PlanComparePage(p Page, pl store.Plan, c plan.Comparison) {
	@Layout(p) {
		<p class="text-sm"><a href={ templ.URL("/plans/" + pl.ID) } class="underline">{ pl.Name }</a></p>
		<h1 class={ h1 }>
			Version { strconv.Itoa(c.Target.Version) }
			if c.Base != nil {
				compared with active version { strconv.Itoa(c.Base.Version) }
			}
		</h1>
		if c.Target.Note != "" {
			<p class="mt-1">{ c.Target.Note }</p>
		}
		if c.ResetsCursor {
			<p class="mt-4 rounded bg-amber-100 p-2 text-sm dark:bg-amber-950">
				Your current day does not exist in this version: activating it starts the plan over at week 1, day 1.
			</p>
		}
		<div class="mt-4 flex gap-2">
			<form method="post" action={ templ.URL("/plans/" + pl.ID + "/versions/" + c.Target.ID + "/activate") }>
				@CSRF(p)
				<button type="submit" class={ btn }>Activate</button>
			</form>
			if c.Target.Status == store.PlanDraft {
				<form method="post" action={ templ.URL("/plans/" + pl.ID + "/versions/" + c.Target.ID + "/discard") }>
					@CSRF(p)
					<button type="submit" class={ btnDanger }>Discard draft</button>
				</form>
			}
		</div>
		<h2 class={ h2 }>What changes, week by week</h2>
		if len(c.Days) == 0 {
			<p class="mt-2 text-zinc-500">No change to any prescribed set.</p>
		}
		for _, dc := range c.Days {
			<h3 class="mt-4 font-medium">Week { strconv.Itoa(dc.Week) } · { dc.Name }</h3>
			@diffLines(dc.Lines)
		}
		<details class="mt-6">
			<summary class="cursor-pointer text-sm text-zinc-500">JSON diff</summary>
			@diffLines(c.JSON)
		</details>
	}
}

templ diffLines(lines []plan.DiffLine) {
	<pre class="mt-1 overflow-x-auto text-xs">
		for _, l := range lines {
			switch l.Op {
				case '+':
					<span class="block bg-green-100 dark:bg-green-950">+ { l.Text }</span>
				case '-':
					<span class="block bg-red-100 dark:bg-red-950">- { l.Text }</span>
				default:
					<span class="block">{ "  " + l.Text }</span>
			}
		}
	</pre>
}
```

- [ ] **Step 4: Handlers and wiring**

`internal/web/plans.go`:
```go
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// cursorResetNotice is shown when a new version no longer has the user's day.
const cursorResetNotice = "Your current day does not exist in the new version, so the plan starts over at week 1, day 1."

func (s *Server) planRoutes(r chi.Router) {
	r.Get("/plans", s.planList)
	r.Get("/plans/new", s.planNew)
	r.Post("/plans", s.planCreate)
	r.Post("/plans/preview", s.planPreview)
	r.Get("/plans/{id}", s.planDetail)
	r.Get("/plans/{id}/edit", s.planEdit)
	r.Post("/plans/{id}", s.planSave)
	r.Post("/plans/{id}/follow", s.planFollow)
	r.Post("/plans/{id}/archive", s.planArchive)
	r.Get("/plans/{id}/versions/{vid}", s.planVersion)
	r.Get("/plans/{id}/versions/{vid}/compare", s.planCompare)
	r.Post("/plans/{id}/versions/{vid}/activate", s.planActivate)
	r.Post("/plans/{id}/versions/{vid}/discard", s.planDiscard)
	r.Post("/plan/skip", s.planSkip)
	r.Post("/plan/choose", s.planChoose)
	r.Post("/plan/restart", s.planRestart)
	r.Post("/plan/unfollow", s.planUnfollow)
}

// planSchema serves the plan JSON Schema; it is public so tools can fetch it.
func planSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/schema+json")
	_, _ = w.Write(plan.Schema())
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	next, err := s.Plans.Next(r.Context(), user(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.Home(page(r, "Home"), views.HomePage{Next: next}))
}

func (s *Server) planList(w http.ResponseWriter, r *http.Request) {
	plans, err := s.Plans.Plans(r.Context(), user(r))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.PlansPage(page(r, "Plans"), plans))
}

func (s *Server) editor(title, planID, doc string, problems plan.Problems) views.PlanEditor {
	return views.PlanEditor{PlanID: planID, Title: title, Doc: doc, Schema: string(plan.Schema()), Errors: problems}
}

func (s *Server) planNew(w http.ResponseWriter, r *http.Request) {
	e := s.editor("New plan", "", plan.Pretty(plan.StarterTemplate()), nil)
	render(w, r, http.StatusOK, views.PlanEditorPage(page(r, "New plan"), e))
}

func saveStatus(r *http.Request) string {
	if r.PostFormValue("action") == "activate" {
		return plan.SaveActivate
	}
	return plan.SaveDraft
}

func (s *Server) planCreate(w http.ResponseWriter, r *http.Request) {
	doc := r.PostFormValue("doc")
	p, _, err := s.Plans.Create(r.Context(), user(r), []byte(doc), saveStatus(r), "web", "")
	var ps plan.Problems
	switch {
	case errors.As(err, &ps):
		render(w, r, http.StatusUnprocessableEntity, views.PlanEditorPage(page(r, "New plan"), s.editor("New plan", "", doc, ps)))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/plans/"+p.ID, http.StatusSeeOther)
	}
}

func (s *Server) planPreview(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	doc, ps := s.Plans.Validate(r.Context(), u, []byte(r.PostFormValue("doc")))
	var weeks [][]plan.ExpandedDay
	if !ps.HasErrors() {
		weeks = s.Plans.Preview(r.Context(), u, doc)
	}
	render(w, r, http.StatusOK, views.PlanPreview(page(r, ""), ps, weeks))
}

func (s *Server) planDetail(w http.ResponseWriter, r *http.Request) {
	ctx, u := r.Context(), user(r)
	p, versions, err := s.Plans.Plan(ctx, u, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	next, err := s.Plans.Next(ctx, u)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := views.PlanPage{Plan: p, Versions: versions, Following: next != nil && next.Plan.ID == p.ID}
	if r.URL.Query().Has("reset") {
		d.Notice = cursorResetNotice
	}
	render(w, r, http.StatusOK, views.PlanDetailPage(page(r, p.Name), d))
}

// planEdit opens the editor on ?from=<version>, else the active version,
// else the newest version.
func (s *Server) planEdit(w http.ResponseWriter, r *http.Request) {
	p, versions, err := s.Plans.Plan(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	from := r.URL.Query().Get("from")
	var src *store.PlanVersion
	for i, v := range versions {
		if (from != "" && v.ID == from) || (from == "" && v.Status == store.PlanActive) {
			src = &versions[i]
		}
	}
	if src == nil && from == "" && len(versions) > 0 {
		src = &versions[0]
	}
	if src == nil {
		s.renderError(w, r, http.StatusNotFound, "Version not found.")
		return
	}
	e := s.editor("Edit "+p.Name, p.ID, plan.Pretty(src.Doc), nil)
	render(w, r, http.StatusOK, views.PlanEditorPage(page(r, "Edit "+p.Name), e))
}

func (s *Server) planSave(w http.ResponseWriter, r *http.Request) {
	ctx, u, id := r.Context(), user(r), chi.URLParam(r, "id")
	doc := r.PostFormValue("doc")
	_, reset, err := s.Plans.Save(ctx, u, id, []byte(doc), saveStatus(r), "web", "")
	var ps plan.Problems
	switch {
	case errors.As(err, &ps):
		p, _, perr := s.Plans.Plan(ctx, u, id)
		if perr != nil {
			s.fail(w, r, perr)
			return
		}
		e := s.editor("Edit "+p.Name, id, doc, ps)
		render(w, r, http.StatusUnprocessableEntity, views.PlanEditorPage(page(r, "Edit "+p.Name), e))
	case err != nil:
		s.fail(w, r, err)
	default:
		s.redirectToPlan(w, r, id, reset)
	}
}

func (s *Server) redirectToPlan(w http.ResponseWriter, r *http.Request, id string, reset bool) {
	target := "/plans/" + id
	if reset {
		target += "?reset"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// version loads {vid} and checks that it belongs to plan {id}.
func (s *Server) version(r *http.Request) (store.PlanVersion, error) {
	v, err := s.Plans.Version(r.Context(), user(r), chi.URLParam(r, "vid"))
	if err == nil && v.PlanID != chi.URLParam(r, "id") {
		return store.PlanVersion{}, store.ErrNotFound
	}
	return v, err
}

func (s *Server) planVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, _, err := s.Plans.Plan(r.Context(), user(r), v.PlanID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	doc, err := plan.Decode(v.Doc)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := views.PlanVersionPage{Plan: p, Version: v, Weeks: s.Plans.Preview(r.Context(), user(r), doc), JSON: plan.Pretty(v.Doc)}
	render(w, r, http.StatusOK, views.PlanVersionView(page(r, p.Name), d))
}

func (s *Server) planCompare(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p, _, err := s.Plans.Plan(r.Context(), user(r), v.PlanID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cmp, err := s.Plans.Compare(r.Context(), user(r), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.PlanComparePage(page(r, p.Name), p, cmp))
}

func (s *Server) planActivate(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	reset, err := s.Plans.Activate(r.Context(), user(r), v.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.redirectToPlan(w, r, v.PlanID, reset)
}

func (s *Server) planDiscard(w http.ResponseWriter, r *http.Request) {
	v, err := s.version(r)
	if err == nil {
		err = s.Plans.Discard(r.Context(), user(r), v.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans/"+v.PlanID, http.StatusSeeOther)
}

func (s *Server) planFollow(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.Plans.Follow(r.Context(), user(r), id)
	if errors.Is(err, plan.ErrNoActiveVersion) {
		s.renderError(w, r, http.StatusConflict, "Activate a version of this plan before following it.")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planArchive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.Plans.Archive(r.Context(), user(r), id, r.PostFormValue("archived") == "1"); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans/"+id, http.StatusSeeOther)
}

func (s *Server) planSkip(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Skip(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planChoose(w http.ResponseWriter, r *http.Request) {
	ws, ds, _ := strings.Cut(r.PostFormValue("position"), ":")
	week, err1 := strconv.Atoi(ws)
	day, err2 := strconv.Atoi(ds)
	err := errors.Join(err1, err2)
	if err == nil {
		err = s.Plans.Choose(r.Context(), user(r), week, day)
	}
	if err != nil {
		s.renderError(w, r, http.StatusUnprocessableEntity, "That day is not in your plan.")
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Restart(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) planUnfollow(w http.ResponseWriter, r *http.Request) {
	if err := s.Plans.Unfollow(r.Context(), user(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/plans", http.StatusSeeOther)
}
```

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
	"github.com/LongerHV/onerep/internal/plan"
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
	Plans     *plan.Service
}

// Routes returns the application's HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, logRequests, s.recoverer, s.Sessions.Middleware)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})

	r.Get("/healthz", s.healthz)
	r.Get("/schema/plan.json", planSchema)
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
		r.Use(auth.RequireUser, s.limitBody, auth.CSRF, s.starterEquipment)
		r.Get("/", s.home)
		r.Post("/auth/logout", s.logout)
		s.exerciseRoutes(r)
		s.planRoutes(r)
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

// maxBodyBytes bounds request bodies; the largest legitimate one is a plan document.
const maxBodyBytes = 1 << 20

// limitBody refuses oversized requests before anything reads them. Form posts
// are parsed here so the limit applies before the CSRF check reads the token.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		if r.Method == http.MethodPost {
			var tooLarge *http.MaxBytesError
			if err := r.ParseForm(); errors.As(err, &tooLarge) {
				s.renderError(w, r, http.StatusRequestEntityTooLarge, "The request is too large (at most 1 MB).")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
```

Replace `cmd/onerep/main.go` with:
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
	"github.com/LongerHV/onerep/internal/plan"
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
	srv.Plans = &plan.Service{Store: db, Exercises: srv.Exercises}
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

- [ ] **Step 5: Generate and run the tests**

Run: `task generate && go vet ./... && go test ./internal/web/ ./cmd/...`
Expected: both `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/web cmd/onerep
git commit -m "feat(web): add plan editor, versions, comparison and next workout on the home page"
```

- [ ] **Step 7: Check the editor in a real browser**

```bash
go build -o bin/onerep ./cmd/onerep
export ONEREP_ENV=dev ONEREP_DEV_USER=alice ONEREP_DB=bin/h.db ONEREP_LISTEN=127.0.0.1:18095
./bin/onerep serve & sleep 1
nix run nixpkgs#chromium -- --headless=new --no-sandbox --disable-gpu --virtual-time-budget=8000 \
  --dump-dom http://127.0.0.1:18095/plans/new > bin/dom.html 2> bin/chrome.err
kill %1; wait
for k in jse-main cm-editor "Week 1" "Barbell Bench Press" 'textarea id="doc"[^>]*hidden'; do
  printf "%-32s %s\n" "$k" "$(grep -cE "$k" bin/dom.html)"
done
rm -f bin/h.db* bin/dom.html bin/chrome.err
```

Expected: every count is at least 1. That shows the editor mounted (`jse-main`, `cm-editor`), the textarea is hidden, and the htmx preview rendered week 1 with exercise names.

---

### Task 7: Docs and full CI

**Files:**
- Replace: `AGENTS.md`

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
internal/plan/         plan JSON Schema + validation, per-week expansion, load resolution, versions, cursor, diffs
internal/web/          chi router, handlers, views/ (templ), static/ (embedded), jstest/ (node tests)
testdata/calc_cases.json  shared Go/JS calc test vectors
```

Later milestones add `training/`, `stats/`, and `mcp/` under `internal/`. See the spec, §4.

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
- **Plan documents:** `internal/plan/plan.schema.json` is the contract for the editor, the server and the AI. Change the schema, the Go types in `internal/plan/doc.go` and the semantic checks together; `internal/plan/testdata/*.golden.json` pins expansion (`go test ./internal/plan/ -update` rewrites it, so review the diff).
- **Every non-GET request with a session must carry the CSRF token**: the `X-CSRF-Token` header, set globally for htmx via `hx-headers`, or the `csrf_token` form field.
- **The dev login bypass** (`ONEREP_DEV_USER`) only runs with `ONEREP_ENV=dev`, and only on authenticated app routes.
- **Keep dependencies few.** Ask before adding a Go module or a vendored JS library.
- **Licensing:** onerep is AGPL-3.0-only. Dependencies must use MIT, BSD-2/3-Clause, ISC, 0BSD or Apache-2.0. `task licenses:check` enforces this for Go modules. A vendored file gets its license next to it (`<name>.LICENSE`, or `LICENSE` inside its own vendor directory). Keep the footer link to the source code.
````

- [ ] **Step 2: Full CI**

Commit first (`check:generated` compares against git), then:

Run: `task ci`
Expected: exit 0. All nine Go packages are `ok` (account, auth, calc, config, exercise, plan, store, web, cmd/onerep), Node prints `ℹ pass 7`, lint reports `0 issues.`, and `migrate:check` and `licenses:check` pass (jsonschema is Apache-2.0, x/text is BSD-3-Clause).

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: describe plan documents in AGENTS.md"
```

---

## Done when

- `task ci` passes on a clean checkout.
- In `task dev`, the user can:
  - open Plans → New plan and see the starter template in the JSON editor with a live preview
  - save and activate it, follow it, set a bench training max of 120 kg, and see on the home page "Next: Upper" with warm-ups at 60 kg and working sets at 90 kg
  - skip days, go to a chosen day, finish the block and restart it
  - save a shorter draft, review the comparison (with its warning that the plan starts over), and activate it
