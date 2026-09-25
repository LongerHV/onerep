# Plan Form Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the plan page's text JSON editor with a form generated from the plan schema by json-editor 2.17.2, with a plain JSON view, per-week fields, server problems marked on fields, and a theme matching the app.

**Architecture:** `plan.EditorSchema` derives the form's schema from the untouched contract: per-week rewrites, a titled `load` choice, an overlay of UI hints, the user's catalog as the exercise list, `$defs` renamed to `definitions`. The browser side has three files:
- `plan-form-core.js`: pure logic, Node-tested.
- `plan-form-theme.js`: the json-editor theme, custom editors and header templates.
- `plan-form.js`: setup, the Form/JSON switch and problem marking.

The `#doc` textarea stays the form field, so saving, the preview and the no-JS path don't change.

**Tech Stack:** Go, templ, htmx, Tailwind v4, vanilla JS, and vendored `@json-editor/json-editor` 2.17.2 (MIT, UMD bundle loaded with a `<script>` tag, which sets `window.JSONEditor`). `vanilla-jsoneditor` is removed.

**Spec:** `docs/superpowers/specs/2026-09-25-plan-form-editor-design.md`

**Deliberate differences from the spec** (found while reading json-editor's source):
- **`EditorSchema` takes an options struct** `{Catalog, DocSlugs, Unit}`. The user's unit becomes `unit`'s default, so a new plan's form doesn't silently pick "kg" for an lb user.
- **`$defs` is always renamed to `definitions`,** and `$schema`/`$id` are dropped. The loader has no `$defs` handling, and it resolves refs against `$id` (which would try to fetch `https://onerep.local/…`). This settles probe item 1 by construction; the probe still confirms that refs resolve.
- **Every delete asks for confirmation,** set lines included. json-editor's `prompt_before_delete` is global only.
- **Group and exercise headers come from a small custom template engine** (`"@group"`, `"@slot"`, …) in `plan-form-core.js`. json-editor's default engine can't express "Group 2 · superset" or show an exercise's name instead of its slug.
- **No "More" section:** `alternatives` is a collapsed list, and `notes` is a small textarea after the sets. json-editor can't group sibling properties into one collapsible section.
- **The probe runs as Task 2,** after the editor schema exists, so it tests the real schema instead of a mock. It still comes before any browser code.

## Global Constraints

- The contract `internal/plan/plan.schema.json` does not change. `/schema/plan.json`, MCP and server validation keep using it.
- The server's validator is the only validator: json-editor runs with `show_errors: "never"`.
- Per-week kinds: `integer` (`rest_s`, `duration_s`), `count` (`count`; an empty week is `null`), `reps` (number, `lo-hi` or `AMRAP`), `number` (`weight`), `rpe` (select, 6–10 in 0.5 steps), `percent` (`pct_tm`, `drop_pct`; typed 75, stored 0.75).
- **Per-week output:** a single value when every week is the same, otherwise an array of exactly `weeks` entries. An empty field produces no key.
- **Form output:** pretty JSON (2-space indents) in keys of the contract's order, cleaned (no `""`, no empty `alternatives` or `only_weeks`, no empty `load`, no `"kind": "working"`).
- **Switching JSON → Form** is refused, with a one-line reason, when `sameDoc(original, formOutput, unit)` is false.
- **Theme:** classes duplicate `internal/web/views/ui.go`'s strings; grid sizes come from a literal lookup table; touch targets are at least 40px on phones; `dark:` variants follow `prefers-color-scheme`; json-editor's CSS is not loaded (`disable_theme_rules`).
- **Vendoring:** `internal/web/static/vendor/json-editor/jsoneditor.js` 2.17.2 with `LICENSE` (MIT) next to it. The user approved the library in brainstorming.
- **Commits:** Conventional Commits, ending with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` and the session's `Claude-Session:` line.

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **A plan the form can't represent** (an unknown key, or hand-written JSON in an odd shape) opens in the JSON view with nothing lost, and switching refuses instead of rewriting it. Tests: "sameDoc detects unknown keys and changed values" (Task 3); e2e step 6 (Task 6).
2. **Shrinking `weeks`** trims per-week arrays, but must not silently widen a day restricted to a now-missing week: `only_weeks` keeps out-of-range entries, so the server flags them. Tests: "week set keeps weeks beyond the plan" and "per-week resize" (Task 3).
3. **Percent float noise:** 82.5 must store exactly 0.825, and 0.825 must show "82.5". Test: "percent round-trips without float noise" (Task 3).
4. **An lb user's plan without `unit`:** the form defaults to lb, and opening the plan isn't treated as a lossy change. Tests: `TestEditorSchemaUsesUsersUnit` (Task 1); "sameDoc fills the unit" (Task 3).
5. **A plan using an exercise no longer in the catalog** (hidden seed or deleted custom) still opens in the form with that exercise selected, and saving keeps it. Test: `TestEditorSchemaKeepsDocSlugs` (Task 1).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/plan/editor.go` (new) | `EditorOptions`, `EditorSchema`, `DocSlugs`, `CheckAgainst` |
| `internal/plan/editor.overlay.json` (new, embedded) | UI hints keyed by JSON Pointer |
| `internal/plan/editor_test.go` (new) | drift, relaxation, catalog and unit tests |
| `internal/mcp/plans_test.go` | the guide's example fits the editor schema |
| `internal/web/static/vendor/json-editor/{jsoneditor.js,LICENSE}` (new) | vendored library |
| `internal/web/static/js/plan-form-core.js` (new) | pure logic |
| `internal/web/jstest/plan-form.test.mjs` (new) | Node tests |
| `internal/web/static/js/plan-form-theme.js` (new) | theme, per-week and week-set editors, template engine, `register` |
| `internal/web/static/js/plan-form.js` (new; replaces `plan-editor.js`) | setup, switch, problems, focus |
| `internal/web/plans.go`, `views/plans.templ` | editor schema, switch markup, `hx-sync`, problems JSON, problem buttons |
| `internal/web/views/layout.templ`, `static/js/sw.js`, `styles/input.css` | head scripts, shell files, Tailwind `@source` |
| `internal/web/plans_test.go` | editor page and preview tests |
| `test/e2e/harness.mjs` (new), `companion.mjs`, `plan-editor.mjs` (new), `Taskfile.yml` | shared browser harness; the plan editor check |
| `AGENTS.md` | layout and rules |

---

### Task 1: The editor schema

**Files:**
- Create: `internal/plan/editor.go`, `internal/plan/editor.overlay.json`, `internal/plan/editor_test.go`
- Modify: `internal/mcp/plans_test.go`

**Interfaces:**
- Consumes: `schemaJSON` (embedded contract in `validate.go`), `StarterTemplate()`, `store.Exercise`, `github.com/santhosh-tekuri/jsonschema/v6`.
- Produces:
  ```go
  type EditorOptions struct { Catalog []store.Exercise; DocSlugs []string; Unit string }
  func EditorSchema(o EditorOptions) ([]byte, error)
  func DocSlugs(raw []byte) []string                // slugs and alternatives used by a document, best effort
  func CheckAgainst(schema, doc []byte) error         // validates doc against a schema (tests)
  var perWeekKinds map[string]string                   // contract pointer → per-week kind
  ```
  The editor schema's shape, relied on by Task 5:
  - root has `definitions`
  - per-week nodes are `{"format": "per-week", "options": {"perWeek": {"kind": k}}}`
  - `definitions.load` is `{"title": "Load", "oneOf": [None, Weight, % of TM, RPE, Drop %]}`
  - `definitions.slug` has `enum` and `options.enum_titles`
  - `properties.unit.default` is the user's unit
  - `only_weeks` has `"format": "week-set"`
  - headers use `"@day"`, `"@group"`, `"@slot"`, `"@set"`

- [ ] **Step 1: Write the failing tests**

`internal/plan/editor_test.go`:

```go
package plan

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/store"
)

func contract(t *testing.T) map[string]any {
	t.Helper()
	var s map[string]any
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func editorSchema(t *testing.T, o EditorOptions) map[string]any {
	t.Helper()
	raw, err := EditorSchema(o)
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

var testCatalog = []store.Exercise{
	{Slug: "barbell-back-squat", Name: "Barbell Back Squat"},
	{Slug: "barbell-bench-press", Name: "Barbell Bench Press"},
	{Slug: "my-zercher", Name: "Zercher Squat", UserID: "u"},
}

// Every overlay pointer must exist in the contract, so a renamed field fails
// here instead of silently losing its UI hints.
func TestEditorOverlayPointsIntoContract(t *testing.T) {
	var overlay map[string]json.RawMessage
	if err := json.Unmarshal(editorOverlay, &overlay); err != nil {
		t.Fatal(err)
	}
	c := contract(t)
	for ptr := range overlay {
		if _, err := lookup(c, ptr); err != nil {
			t.Errorf("overlay pointer %q: %v", ptr, err)
		}
		if strings.HasPrefix(ptr, "/$defs/load/") {
			t.Errorf("overlay pointer %q targets inside load, which EditorSchema rebuilds", ptr)
		}
	}
}

// Every per-week value in the contract (a oneOf with an array branch, or a
// $ref to seconds) is rewritten, and every rewrite points at one.
func TestPerWeekFieldsAreAllRewritten(t *testing.T) {
	found := map[string]bool{}
	var walk func(ptr string, v any)
	walk = func(ptr string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if isPerWeek(m) {
			found[ptr] = true
		}
		for k, child := range m {
			walk(ptr+"/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(k), child)
		}
	}
	walk("", contract(t))
	delete(found, "/$defs/seconds") // the definition itself, not a use of it
	for ptr := range found {
		if _, ok := perWeekKinds[ptr]; !ok {
			t.Errorf("per-week value %s has no kind in perWeekKinds", ptr)
		}
	}
	for ptr := range perWeekKinds {
		if !found[ptr] {
			t.Errorf("perWeekKinds has %s, which is not a per-week value in the contract", ptr)
		}
	}
}

// The rewrites only relax or retitle: documents valid under the contract are
// valid under the editor schema.
func TestEditorSchemaAcceptsContractDocs(t *testing.T) {
	golden, err := os.ReadFile("testdata/upper_lower.json")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EditorSchema(EditorOptions{Catalog: testCatalog, DocSlugs: append(DocSlugs(golden), DocSlugs(StarterTemplate())...), Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	for name, doc := range map[string][]byte{"starter": StarterTemplate(), "golden": golden} {
		if err := CheckAgainst(schemaJSON, doc); err != nil {
			t.Fatalf("%s is not valid under the contract: %v", name, err)
		}
		if err := CheckAgainst(raw, doc); err != nil {
			t.Errorf("%s is valid under the contract but not the editor schema: %v", name, err)
		}
	}
}

func TestEditorSchemaShape(t *testing.T) {
	s := editorSchema(t, EditorOptions{Catalog: testCatalog, Unit: "kg"})
	if _, ok := s["$defs"]; ok || s["definitions"] == nil || s["$id"] != nil || s["$schema"] != nil {
		t.Fatal("editor schema must use definitions and drop $id/$schema")
	}
	if b, _ := json.Marshal(s); strings.Contains(string(b), "#/$defs/") {
		t.Fatal("a $ref still points at $defs")
	}
	count, _ := lookup(s, "/definitions/setLine/properties/count")
	if count["format"] != "per-week" || count["options"].(map[string]any)["perWeek"].(map[string]any)["kind"] != "count" {
		t.Errorf("count = %v", count)
	}
	setLine, _ := lookup(s, "/definitions/setLine")
	if _, ok := setLine["oneOf"]; ok {
		t.Error("the set line's reps-or-duration oneOf must be dropped")
	}
	load, _ := lookup(s, "/definitions/load")
	var titles []string
	for _, b := range load["oneOf"].([]any) {
		titles = append(titles, b.(map[string]any)["title"].(string))
	}
	if !slices.Equal(titles, []string{"None", "Weight", "% of TM", "RPE", "Drop %"}) {
		t.Errorf("load branches = %v", titles)
	}
	weeks, _ := lookup(s, "/definitions/day/properties/only_weeks")
	if weeks["format"] != "week-set" {
		t.Errorf("only_weeks = %v", weeks)
	}
}

func TestEditorSchemaCatalog(t *testing.T) {
	s := editorSchema(t, EditorOptions{Catalog: testCatalog, Unit: "kg"})
	slug, _ := lookup(s, "/definitions/slug")
	enum := slug["enum"].([]any)
	titles := slug["options"].(map[string]any)["enum_titles"].([]any)
	if len(enum) != 3 || len(titles) != 3 || enum[0] != "barbell-back-squat" || titles[2] != "Zercher Squat" {
		t.Fatalf("enum %v, titles %v (sorted by name, custom included)", enum, titles)
	}
}

func TestEditorSchemaKeepsDocSlugs(t *testing.T) {
	s := editorSchema(t, EditorOptions{Catalog: testCatalog, DocSlugs: []string{"hidden-old-lift", "barbell-back-squat"}, Unit: "kg"})
	slug, _ := lookup(s, "/definitions/slug")
	enum := slug["enum"].([]any)
	if len(enum) != 4 || enum[3] != "hidden-old-lift" {
		t.Fatalf("enum = %v, want the catalog plus the document's missing slug", enum)
	}
	if title := slug["options"].(map[string]any)["enum_titles"].([]any)[3]; title != "hidden-old-lift (not in your catalog)" {
		t.Fatalf("title = %v", title)
	}
}

func TestEditorSchemaUsesUsersUnit(t *testing.T) {
	s := editorSchema(t, EditorOptions{Catalog: testCatalog, Unit: "lb"})
	unit, _ := lookup(s, "/properties/unit")
	if unit["default"] != "lb" {
		t.Fatalf("unit = %v", unit)
	}
}

func TestDocSlugs(t *testing.T) {
	got := DocSlugs([]byte(`{"days":[{"groups":[{"exercises":[{"slug":"a","alternatives":["b","a"]},{"slug":"c"}]}]}]}`))
	if !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("slugs = %v", got)
	}
	if DocSlugs([]byte(`not json`)) != nil {
		t.Fatal("unparsable documents have no slugs")
	}
}
```

Append to `internal/mcp/plans_test.go`:

```go
// The AI guide's example also fits the form's schema, so the form can open
// anything the AI is taught to write.
func TestGuideExampleFitsTheEditorSchema(t *testing.T) {
	_, example, _ := strings.Cut(planFormat, "## Example\n\n```json\n")
	example, _, _ = strings.Cut(example, "```")
	raw, err := plan.EditorSchema(plan.EditorOptions{DocSlugs: plan.DocSlugs([]byte(example)), Unit: "kg"})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.CheckAgainst(raw, []byte(example)); err != nil {
		t.Fatal(err)
	}
}
```

(Import `github.com/LongerHV/onerep/internal/plan` in `plans_test.go`.)

Run: `nix develop --command go test ./internal/plan/ ./internal/mcp/`
Expected: FAIL to compile: `undefined: EditorSchema` (and `lookup`, `isPerWeek`, `editorOverlay`, `perWeekKinds`, `DocSlugs`, `CheckAgainst`).

- [ ] **Step 2: Write the overlay**

`internal/plan/editor.overlay.json`:

```json
{
  "": {"title": "Plan", "format": "grid", "options": {"disable_collapse": true}},
  "/properties/name": {"title": "Name", "options": {"grid_columns": 6}},
  "/properties/unit": {"title": "Unit", "options": {"grid_columns": 3}},
  "/properties/weeks": {"title": "Weeks", "options": {"grid_columns": 3}},
  "/properties/days": {"title": "Days", "format": "tabs-top", "options": {"grid_columns": 12}},
  "/$defs/day": {"title": "Day", "headerTemplate": "@day", "options": {"disable_collapse": true}},
  "/$defs/day/properties/name": {"title": "Day name"},
  "/$defs/day/properties/only_weeks": {"title": "Only in weeks", "format": "week-set"},
  "/$defs/day/properties/groups": {"title": "Groups"},
  "/$defs/group": {"title": "Group", "headerTemplate": "@group"},
  "/$defs/group/properties/rest_s": {"title": "Rest (s)"},
  "/$defs/group/properties/exercises": {"title": "Exercises"},
  "/$defs/slot": {"title": "Exercise", "headerTemplate": "@slot"},
  "/$defs/slot/properties/alternatives": {"title": "Alternatives", "options": {"collapsed": true}},
  "/$defs/slot/properties/notes": {"title": "Notes", "format": "textarea"},
  "/$defs/slot/properties/sets": {"title": "Sets"},
  "/$defs/setLine": {"title": "Set line", "format": "grid", "headerTemplate": "@set"},
  "/$defs/setLine/properties/kind": {"title": "Kind", "options": {"grid_columns": 2}},
  "/$defs/setLine/properties/count": {"title": "Sets", "options": {"grid_columns": 2}},
  "/$defs/setLine/properties/reps": {"title": "Reps", "options": {"grid_columns": 2}},
  "/$defs/setLine/properties/duration_s": {"title": "Duration (s)", "options": {"grid_columns": 2}},
  "/$defs/setLine/properties/load": {"options": {"grid_columns": 3}},
  "/$defs/setLine/properties/rpe": {"title": "RPE", "options": {"grid_columns": 1}},
  "/$defs/slug": {"title": "Exercise"}
}
```

- [ ] **Step 3: Implement**

`internal/plan/editor.go`:

```go
package plan

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/LongerHV/onerep/internal/store"
)

// The plan editor's form is generated by json-editor from an editor schema
// derived here from the contract (plan.schema.json), which itself never
// carries UI keywords: the server validates against it and the AI reads it.

//go:embed editor.overlay.json
var editorOverlay []byte

// perWeekKinds maps every per-week value in the contract to how the form
// edits it (see plan-form-core.js).
var perWeekKinds = map[string]string{
	"/$defs/group/properties/rest_s":       "integer",
	"/$defs/setLine/properties/count":      "count",
	"/$defs/setLine/properties/reps":       "reps",
	"/$defs/setLine/properties/duration_s": "integer",
	"/$defs/setLine/properties/rpe":        "rpe",
	"/$defs/load/properties/weight":        "number",
	"/$defs/load/properties/pct_tm":        "percent",
	"/$defs/load/properties/rpe":           "rpe",
	"/$defs/load/properties/drop_pct":      "percent",
}

// EditorOptions are the user-specific parts of the editor schema.
type EditorOptions struct {
	Catalog  []store.Exercise // the user's catalog, for the exercise select
	DocSlugs []string         // slugs the document uses (kept even when not in the catalog)
	Unit     string           // the user's unit, the default for a plan's unit
}

// EditorSchema builds the plan editor's schema: per-week values become the
// custom per-week field, load becomes a titled choice, the set line's
// reps-or-duration rule is left to the server, the overlay adds titles and
// layout, the catalog becomes the exercise select, and $defs is renamed to
// definitions (json-editor doesn't resolve $defs).
func EditorSchema(o EditorOptions) ([]byte, error) {
	var s map[string]any
	if err := json.Unmarshal(schemaJSON, &s); err != nil {
		return nil, err
	}
	delete(s, "$schema")
	delete(s, "$id")

	for ptr, kind := range perWeekKinds {
		node, err := lookup(s, ptr)
		if err != nil {
			return nil, err
		}
		repl := map[string]any{"format": "per-week", "options": map[string]any{"perWeek": map[string]any{"kind": kind}}}
		if d, ok := node["description"]; ok {
			repl["description"] = d
		}
		clear(node)
		for k, v := range repl {
			node[k] = v
		}
	}

	setLine, err := lookup(s, "/$defs/setLine")
	if err != nil {
		return nil, err
	}
	delete(setLine, "oneOf")

	load, err := lookup(s, "/$defs/load")
	if err != nil {
		return nil, err
	}
	props := load["properties"].(map[string]any)
	branch := func(title, key string) map[string]any {
		return map[string]any{"title": title, "type": "object", "additionalProperties": false,
			"required": []any{key}, "properties": map[string]any{key: props[key]}}
	}
	defs := s["$defs"].(map[string]any)
	defs["load"] = map[string]any{"title": "Load", "description": load["description"], "oneOf": []any{
		map[string]any{"title": "None", "type": "object", "additionalProperties": false, "properties": map[string]any{}},
		branch("Weight", "weight"), branch("% of TM", "pct_tm"), branch("RPE", "rpe"), branch("Drop %", "drop_pct"),
	}}

	var overlay map[string]map[string]any
	if err := json.Unmarshal(editorOverlay, &overlay); err != nil {
		return nil, err
	}
	for ptr, add := range overlay {
		node, err := lookup(s, ptr)
		if err != nil {
			return nil, fmt.Errorf("overlay: %w", err)
		}
		deepMerge(node, add)
	}

	unit, _ := lookup(s, "/properties/unit")
	if o.Unit == "kg" || o.Unit == "lb" {
		unit["default"] = o.Unit
	}
	unit["title"] = "Unit"

	slug, _ := lookup(s, "/$defs/slug")
	sorted := slices.Clone(o.Catalog)
	slices.SortStableFunc(sorted, func(a, b store.Exercise) int { return strings.Compare(a.Name, b.Name) })
	var enum, titles []any
	known := map[string]bool{}
	for _, e := range sorted {
		enum, titles = append(enum, e.Slug), append(titles, e.Name)
		known[e.Slug] = true
	}
	for _, used := range o.DocSlugs {
		if !known[used] {
			enum, titles = append(enum, used), append(titles, used+" (not in your catalog)")
			known[used] = true
		}
	}
	if len(enum) > 0 {
		slug["enum"] = enum
		opts, _ := slug["options"].(map[string]any)
		if opts == nil {
			opts = map[string]any{}
		}
		opts["enum_titles"] = titles
		slug["options"] = opts
	}

	s["definitions"] = s["$defs"]
	delete(s, "$defs")
	renameRefs(s)
	return json.Marshal(s)
}

// isPerWeek reports whether a contract node is a per-week value: a oneOf with
// an array branch, or a reference to the per-week seconds definition.
func isPerWeek(m map[string]any) bool {
	if m["$ref"] == "#/$defs/seconds" {
		return true
	}
	branches, _ := m["oneOf"].([]any)
	for _, b := range branches {
		if bm, ok := b.(map[string]any); ok && bm["type"] == "array" {
			return true
		}
	}
	return false
}

// lookup returns the object at a JSON Pointer.
func lookup(root map[string]any, ptr string) (map[string]any, error) {
	cur := root
	if ptr == "" {
		return cur, nil
	}
	for _, part := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		part = strings.NewReplacer("~1", "/", "~0", "~").Replace(part)
		next, ok := cur[part].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("no object at %s (missing %q)", ptr, part)
		}
		cur = next
	}
	return cur, nil
}

// deepMerge copies add into dst, merging nested objects.
func deepMerge(dst, add map[string]any) {
	for k, v := range add {
		if vm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				deepMerge(dm, vm)
				continue
			}
		}
		dst[k] = v
	}
}

// renameRefs rewrites every "#/$defs/..." reference to "#/definitions/...".
func renameRefs(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if ref, ok := child.(string); ok && k == "$ref" {
				t[k] = strings.Replace(ref, "#/$defs/", "#/definitions/", 1)
			} else {
				renameRefs(child)
			}
		}
	case []any:
		for _, child := range t {
			renameRefs(child)
		}
	}
}

// DocSlugs returns the exercise slugs a document uses (slots and
// alternatives), in order of first use. Unparsable documents have none.
func DocSlugs(raw []byte) []string {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(v any) {
		if s, ok := v.(string); ok && s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case map[string]any:
			add(t["slug"])
			if alts, ok := t["alternatives"].([]any); ok {
				for _, a := range alts {
					add(a)
				}
			}
			for _, k := range []string{"days", "groups", "exercises"} {
				walk(t[k])
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(doc)
	return out
}

// CheckAgainst validates doc against schema (used by tests to show the
// editor schema only relaxes the contract).
func CheckAgainst(schema, doc []byte) error {
	sch, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("mem://schema.json", sch); err != nil {
		return err
	}
	compiled, err := c.Compile("mem://schema.json")
	if err != nil {
		return err
	}
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return err
	}
	return compiled.Validate(v)
}
```

Run: `nix develop --command bash -c 'gofmt -l internal/plan; go test ./internal/plan/ ./internal/mcp/'`
Expected: PASS. If `CheckAgainst` rejects `format: "per-week"` or `"week-set"` as unknown formats, it means the compiler asserts formats. Keep it from asserting (by default santhosh v6 only asserts formats when asked). That isn't a ruling.

- [ ] **Step 4: Commit**

```bash
git add internal/plan internal/mcp/plans_test.go
git commit -m "feat(plan): derive the plan editor's schema from the contract"
```

---

### Task 2: Vendor json-editor and probe it

**Files:**
- Create: `internal/web/static/vendor/json-editor/jsoneditor.js`, `internal/web/static/vendor/json-editor/LICENSE`
- Throwaway: `<scratchpad>/probe/` (not committed)

**Interfaces:**
- Consumes: `plan.EditorSchema` (Task 1).
- Produces: a ledger note with the probe's answers. If any answer contradicts Tasks 3–5, record a `Ruling:` and adjust those tasks' code before writing it.

- [ ] **Step 1: Vendor the library**

```bash
cd "$(git rev-parse --show-toplevel)"
tmp=$(mktemp -d -p "$SCRATCH")
curl -fsSL https://registry.npmjs.org/@json-editor/json-editor/-/json-editor-2.17.2.tgz | tar -xz -C "$tmp"
mkdir -p internal/web/static/vendor/json-editor
cp "$tmp/package/dist/jsoneditor.js" internal/web/static/vendor/json-editor/jsoneditor.js
cp "$tmp/package/LICENSE" internal/web/static/vendor/json-editor/LICENSE
head -3 internal/web/static/vendor/json-editor/LICENSE; grep -c "" internal/web/static/vendor/json-editor/jsoneditor.js
```

Expected: the LICENSE is MIT; the bundle is about 537 KB. (`dist/jsoneditor.js.LICENSE.txt` lists the bundled third-party notices. Copy it next to the bundle as `jsoneditor.js.LICENSE.txt`, because the bundle's first line points at it.)

- [ ] **Step 2: Probe with the real editor schema**

Write the editor schema for the seeded catalog and the starter template to a file with a throwaway Go test run from the scratchpad, or with `go run` of a tiny program in the scratchpad that imports `internal/plan` (module-internal import works from a directory inside the module, so put the program under a git-ignored path such as `.superpowers/probe/`).

Then write `probe.html`, which loads `/static/vendor/json-editor/jsoneditor.js`, a minimal theme (a subclass of `JSONEditor.AbstractTheme` that only adds a class) and a stub per-week editor. Open it in headless Chromium with the e2e CDP helper, or the Chrome DevTools MCP if it's available. Serve the repository's static directory with `python -m http.server` or a tiny Go file server from the scratchpad.

Check and record in the ledger:
1. The schema loads with no network requests other than the bundle (`definitions` refs resolve), and `editor.getValue()` after `setValue(starter)` is structurally equal to the starter (ignoring key order, `""`/empty values and `kind: "working"`).
2. The `load` switcher shows None, Weight, % of TM, RPE and Drop %; a set line without `load` selects None; picking "% of TM" gives `{pct_tm: …}`.
3. A resolver `schema => schema.format === "per-week" && "perweek"` registered with `JSONEditor.defaults.resolvers.unshift` is used. `this.options.perWeek.kind` is visible in the editor. `jsoneditor.watch("root.weeks", cb)` fires when weeks changes. `getEditor("root.weeks")` is available when a per-week editor's `setValue` first runs.
4. `editor.showValidationErrors([{path: "root.days.0.name", property: "", message: "x"}])` calls the theme's `addInputError` for that input. The next change event with `show_errors: "never"` clears it.
5. `format: "grid"` calls `setGridColumnSize(el, n)`. `format: "tabs-top"` uses `getTopTabHolder`, `getTopTab`, `addTopTab`, `markTabActive` and `markTabInactive`. A custom template engine registered as `JSONEditor.defaults.templates.onerep` and selected with `template: "onerep"` compiles `headerTemplate` strings.

Also note which keys json-editor adds by itself (for example `kind: "working"` from `default`, or `name: ""`), because `clean` in Task 3 must remove them.

Expected: all five hold. If one fails in a way that needs a different design (not just different code), stop and report to the user. That is the spec's §11 rule.

- [ ] **Step 3: Commit the vendored files**

```bash
git add internal/web/static/vendor/json-editor
git commit -m "chore(web): vendor json-editor 2.17.2 (MIT)"
```

---

### Task 3: Pure form logic

**Files:**
- Create: `internal/web/static/js/plan-form-core.js`, `internal/web/jstest/plan-form.test.mjs`

**Interfaces:**
- Produces (ES module, no DOM):
  ```js
  export const KINDS                                    // ["integer","count","reps","number","rpe","percent"]
  export function parseInput(kind, text)                // → {value} | {error}; "" → {value: undefined} (count in a week: null)
  export function formatValue(kind, value)              // → string for an input
  export function perWeekFrom(value, weeks)             // → {vary, values}
  export function perWeekValue(state)                   // → value | array | undefined
  export function resize(state, weeks)                  // → state with exactly `weeks` values when varying
  export function weekSetFrom(value)                    // → sorted unique ints (out-of-range kept)
  export function weekSetValue(weeks)                   // → array | undefined
  export function clean(doc)                            // → document without empty optional values
  export function sameDoc(a, b, unit)                   // → boolean (key order ignored, unit filled)
  export function pointerToPath(pointer)                // "/days/0/name" → "root.days.0.name"
  export function nearestPath(path, has)                // → {path, week} walking up to an existing editor
  export function problemsToErrors(problems, has)       // → [{path, property, message}]
  export function headerText(template, vars, names)     // "@day" | "@group" | "@slot" | "@set" | other
  export function slugNames(schema)                     // editor schema → {slug: name}
  ```

- [ ] **Step 1: Write the failing tests**

`internal/web/jstest/plan-form.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import * as f from "../static/js/plan-form-core.js";

test("parse and format each kind", () => {
  assert.deepEqual(f.parseInput("integer", "90"), { value: 90 });
  assert.ok(f.parseInput("integer", "1.5").error);
  assert.deepEqual(f.parseInput("count", ""), { value: undefined });
  assert.deepEqual(f.parseInput("reps", "6-10"), { value: "6-10" });
  assert.deepEqual(f.parseInput("reps", "amrap"), { value: "AMRAP" });
  assert.deepEqual(f.parseInput("reps", "5"), { value: 5 });
  assert.ok(f.parseInput("reps", "10-6x").error);
  assert.deepEqual(f.parseInput("number", "102,5"), { value: 102.5 });
  assert.deepEqual(f.parseInput("rpe", "8.5"), { value: 8.5 });
  assert.equal(f.formatValue("reps", "AMRAP"), "AMRAP");
  assert.equal(f.formatValue("number", undefined), "");
});

test("percent round-trips without float noise", () => {
  assert.deepEqual(f.parseInput("percent", "82.5"), { value: 0.825 });
  assert.deepEqual(f.parseInput("percent", "75"), { value: 0.75 });
  assert.equal(f.formatValue("percent", 0.825), "82.5");
  assert.equal(f.formatValue("percent", 0.7), "70");
});

test("per-week state from a single value and from an array", () => {
  assert.deepEqual(f.perWeekFrom(5, 4), { vary: false, values: [5] });
  assert.deepEqual(f.perWeekFrom([3, 3, 4, 2], 4), { vary: true, values: [3, 3, 4, 2] });
  assert.deepEqual(f.perWeekFrom(undefined, 4), { vary: false, values: [undefined] });
});

test("per-week value collapses equal weeks and keeps count skips", () => {
  assert.equal(f.perWeekValue({ vary: true, values: [5, 5, 5] }), 5);
  assert.deepEqual(f.perWeekValue({ vary: true, values: [3, null, 3] }), [3, null, 3]);
  assert.equal(f.perWeekValue({ vary: false, values: [undefined] }), undefined);
  assert.equal(f.perWeekValue({ vary: true, values: [undefined, undefined] }), undefined);
});

test("per-week resize copies the last week and trims", () => {
  assert.deepEqual(f.resize({ vary: true, values: [3, 4] }, 4), { vary: true, values: [3, 4, 4, 4] });
  assert.deepEqual(f.resize({ vary: true, values: [3, 4, 5] }, 2), { vary: true, values: [3, 4] });
  assert.deepEqual(f.resize({ vary: false, values: [5] }, 6), { vary: false, values: [5] });
});

test("week set keeps weeks beyond the plan", () => {
  assert.deepEqual(f.weekSetFrom([5, 2, 2]), [2, 5]);
  assert.deepEqual(f.weekSetValue([2, 5]), [2, 5]);
  assert.equal(f.weekSetValue([]), undefined);
});

test("clean drops empty optional values", () => {
  const doc = { name: "P", weeks: 1, days: [{ name: "D", only_weeks: [], groups: [{ exercises: [
    { slug: "a", alternatives: [], notes: "", sets: [{ kind: "working", count: 3, reps: 5, load: {} }] }] }] }] };
  assert.deepEqual(f.clean(doc), { name: "P", weeks: 1, days: [{ name: "D", groups: [{ exercises: [
    { slug: "a", sets: [{ count: 3, reps: 5 }] }] }] }] });
  assert.deepEqual(f.clean({ days: [{ groups: [] }] }), { days: [{ groups: [] }] }, "an empty day is meaningful");
});

test("sameDoc ignores key order, fills the unit, and detects real changes", () => {
  const a = { name: "P", weeks: 1, days: [] };
  assert.ok(f.sameDoc(a, { days: [], weeks: 1, name: "P", unit: "lb" }, "lb"), "sameDoc fills the unit");
  assert.ok(!f.sameDoc(a, { days: [], weeks: 1, name: "P", unit: "kg" }, "lb"));
  assert.ok(!f.sameDoc({ ...a, foo: 1 }, a, "kg"), "sameDoc detects unknown keys");
  assert.ok(!f.sameDoc({ ...a, weeks: 2 }, a, "kg"), "sameDoc detects changed values");
  assert.ok(f.sameDoc({ ...a, days: [{ name: "D", groups: [{ exercises: [{ slug: "x", sets: [{ kind: "working", count: 1, reps: 1 }] }] }] }] },
    { ...a, days: [{ name: "D", groups: [{ exercises: [{ slug: "x", sets: [{ count: 1, reps: 1 }] }] }] }] }, "kg"));
});

test("pointer to path, walking up to an existing editor", () => {
  assert.equal(f.pointerToPath("/days/0/groups/1/exercises/0/sets/2/count"), "root.days.0.groups.1.exercises.0.sets.2.count");
  assert.equal(f.pointerToPath(""), "root");
  assert.equal(f.pointerToPath("/a~1b/c~0d"), "root.a/b.c~d");
  const editors = new Set(["root", "root.days.0.groups.0.exercises.0.sets.0.count", "root.days.0.groups.0.exercises.0.sets.0.load"]);
  const has = (p) => editors.has(p);
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.exercises.0.sets.0.count.2", has), { path: "root.days.0.groups.0.exercises.0.sets.0.count", week: 3 });
  assert.deepEqual(f.nearestPath("root.days.0.groups.0.exercises.0.sets.0.load.pct_tm", has), { path: "root.days.0.groups.0.exercises.0.sets.0.load", week: null });
  assert.deepEqual(f.nearestPath("root.nowhere.9", has), { path: "root", week: null });
});

test("problems become json-editor errors", () => {
  const has = (p) => p === "root" || p === "root.weeks" || p === "root.days.0.groups.0.rest_s";
  const errs = f.problemsToErrors([
    { pointer: "/weeks", message: "must be at least 1" },
    { pointer: "/days/0/groups/0/rest_s/1", message: "too long", warning: true },
    { pointer: "", message: "no days in week 3" },
  ], has);
  assert.deepEqual(errs, [
    { path: "root.weeks", property: "onerep", message: "must be at least 1" },
    { path: "root.days.0.groups.0.rest_s", property: "onerep", message: "Warning: W2: too long" },
    { path: "root", property: "onerep", message: "no days in week 3" },
  ]);
  assert.deepEqual(f.problemsToErrors(null, has), []);
});

test("header templates", () => {
  const names = { "barbell-back-squat": "Barbell Back Squat" };
  assert.equal(f.headerText("@day", { i1: 2, self: { name: "Lower" } }, names), "Lower");
  assert.equal(f.headerText("@day", { i1: 2, self: {} }, names), "Day 2");
  assert.equal(f.headerText("@group", { i1: 1, self: { exercises: [{}, {}] } }, names), "Group 1 · superset");
  assert.equal(f.headerText("@group", { i1: 3, self: { exercises: [{}] } }, names), "Group 3");
  assert.equal(f.headerText("@slot", { i1: 1, self: { slug: "barbell-back-squat" } }, names), "Barbell Back Squat");
  assert.equal(f.headerText("@set", { i1: 2, self: {} }, names), "Set line 2");
  assert.equal(f.headerText("Plan", {}, names), "Plan");
});

test("slug names from the editor schema", () => {
  const schema = { definitions: { slug: { enum: ["a", "b"], options: { enum_titles: ["A", "B"] } } } };
  assert.deepEqual(f.slugNames(schema), { a: "A", b: "B" });
  assert.deepEqual(f.slugNames({}), {});
});
```

Run: `nix develop --command task test:js`
Expected: FAIL, `Cannot find module …/plan-form-core.js`.

- [ ] **Step 2: Implement**

`internal/web/static/js/plan-form-core.js`:

```js
// Plan form logic, free of the DOM and json-editor so Node can test it
// (internal/web/jstest/plan-form.test.mjs). plan-form-theme.js renders the
// per-week and week-set fields with it; plan-form.js uses the rest.

export const KINDS = ["integer", "count", "reps", "number", "rpe", "percent"];

const num = (text) => Number(String(text).trim().replace(",", "."));
// round4 avoids float noise: 82.5 / 100 is 0.8250000000000001 otherwise.
const round4 = (v) => Math.round(v * 10000) / 10000;

// parseInput reads one input of a per-week field. An empty input is
// undefined ("not set"); callers turn it into null for a skipped count week.
export function parseInput(kind, text) {
  const t = String(text ?? "").trim();
  if (t === "") return { value: undefined };
  switch (kind) {
    case "integer":
    case "count": {
      const v = num(t);
      return Number.isInteger(v) && v >= 0 ? { value: v } : { error: "enter a whole number" };
    }
    case "reps": {
      if (/^amrap$/i.test(t)) return { value: "AMRAP" };
      if (/^[1-9][0-9]?-[1-9][0-9]?$/.test(t)) return { value: t };
      const v = num(t);
      return Number.isInteger(v) && v >= 1 ? { value: v } : { error: "enter reps like 5, 6-10 or AMRAP" };
    }
    case "percent": {
      const v = num(t);
      return Number.isFinite(v) && v > 0 ? { value: round4(v / 100) } : { error: "enter a percentage like 75" };
    }
    default: {
      const v = num(t);
      return Number.isFinite(v) ? { value: v } : { error: "enter a number" };
    }
  }
}

export function formatValue(kind, value) {
  if (value === undefined || value === null) return "";
  if (kind === "percent" && typeof value === "number") return String(round4(value * 100));
  return String(value);
}

// perWeekFrom turns a stored value into the field's state.
export function perWeekFrom(value, weeks) {
  if (Array.isArray(value)) return resize({ vary: true, values: value.slice() }, weeks);
  return { vary: false, values: [value] };
}

// perWeekValue is what the field stores: nothing when empty, one value when
// every week is the same, otherwise one entry per week.
export function perWeekValue(state) {
  const vals = state.vary ? state.values : state.values.slice(0, 1);
  if (vals.every((v) => v === undefined || v === null)) return undefined;
  if (vals.every((v) => v === vals[0])) return vals[0];
  return vals.map((v) => (v === undefined ? null : v));
}

// resize fits a varying field to the plan's weeks: new weeks copy the last one.
export function resize(state, weeks) {
  if (!state.vary) return state;
  const values = state.values.slice(0, weeks);
  while (values.length < weeks) values.push(values.length ? values[values.length - 1] : undefined);
  return { vary: true, values };
}

// weekSetFrom reads only_weeks. Weeks beyond the plan are kept, so shrinking
// a plan never quietly turns "week 5 only" into "every week"; the server
// reports them instead.
export function weekSetFrom(value) {
  return Array.isArray(value) ? [...new Set(value.filter(Number.isInteger))].sort((a, b) => a - b) : [];
}

export function weekSetValue(weeks) {
  return weeks.length ? weekSetFrom(weeks) : undefined;
}

const isPlainObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);

// clean removes values the form produces for untouched optional fields.
export function clean(doc) {
  if (Array.isArray(doc)) return doc.map(clean);
  if (!isPlainObject(doc)) return doc;
  const out = {};
  for (const [k, v] of Object.entries(doc)) {
    if (v === undefined || v === "") continue;
    if ((k === "alternatives" || k === "only_weeks") && Array.isArray(v) && v.length === 0) continue;
    if (k === "load" && isPlainObject(v) && Object.keys(v).length === 0) continue;
    if (k === "kind" && v === "working") continue;
    out[k] = clean(v);
  }
  return out;
}

function canonical(v) {
  if (Array.isArray(v)) return v.map(canonical);
  if (!isPlainObject(v)) return v;
  return Object.fromEntries(Object.keys(v).sort().map((k) => [k, canonical(v[k])]));
}

// sameDoc reports whether two documents mean the same plan: key order is
// ignored, cleaned values are ignored, and a missing unit is the user's.
export function sameDoc(a, b, unit) {
  const norm = (d) => canonical(clean(isPlainObject(d) && !d.unit ? { ...d, unit } : d));
  return JSON.stringify(norm(a)) === JSON.stringify(norm(b));
}

export function pointerToPath(pointer) {
  if (!pointer) return "root";
  return "root." + pointer.slice(1).split("/").map((p) => p.replace(/~1/g, "/").replace(/~0/g, "~")).join(".");
}

// nearestPath walks up from path to the closest path that has an editor. A
// trailing week index on a per-week value becomes week (1-based).
export function nearestPath(path, has) {
  const parts = path.split(".");
  let week = null;
  while (parts.length > 1 && !has(parts.join("."))) {
    const last = parts.pop();
    week = parts.length > 0 && /^\d+$/.test(last) && week === null && has(parts.join(".")) ? Number(last) + 1 : null;
  }
  return { path: parts.join("."), week };
}

// problemsToErrors turns the server's problems into json-editor errors.
// Warnings are marked with a "Warning: " prefix, which the theme styles amber.
export function problemsToErrors(problems, has) {
  return (problems || []).map((p) => {
    const { path, week } = nearestPath(pointerToPath(p.pointer), has);
    const text = (week ? `W${week}: ` : "") + p.message;
    return { path, property: "onerep", message: p.warning ? `Warning: ${text}` : text };
  });
}

// headerText renders the "@…" header templates of the editor schema.
export function headerText(template, vars, names) {
  const self = vars?.self || {};
  switch (template) {
    case "@day": return self.name || `Day ${vars.i1}`;
    case "@group": return `Group ${vars.i1}` + ((self.exercises || []).length > 1 ? " · superset" : "");
    case "@slot": return names[self.slug] || self.slug || `Exercise ${vars.i1}`;
    case "@set": return `Set line ${vars.i1}`;
    default: return template;
  }
}

export function slugNames(schema) {
  const slug = schema?.definitions?.slug;
  const out = {};
  (slug?.enum || []).forEach((s, i) => { out[s] = slug.options?.enum_titles?.[i] || s; });
  return out;
}
```

Run: `nix develop --command task test:js`
Expected: PASS. If `nearestPath`'s week logic fails the "count.2" case, fix the function, not the test: only a numeric segment removed directly above an existing editor is a week.

- [ ] **Step 3: Commit**

```bash
git add internal/web/static/js/plan-form-core.js internal/web/jstest/plan-form.test.mjs
git commit -m "feat(web): add the plan form's pure logic"
```

---

### Task 4: Server side of the editor page

The page keeps working with only the textarea after this task. The form arrives in Task 5.

**Files:**
- Modify: `internal/web/plans.go` (`editor` and its callers, `planPreview`), `internal/web/views/plans.templ`, `internal/web/views/layout.templ`, `internal/web/static/js/sw.js`, `internal/web/styles/input.css`
- Delete: `internal/web/static/vendor/jsoneditor/` and `internal/web/static/js/plan-editor.js`
- Test: `internal/web/plans_test.go`

**Interfaces:**
- Consumes: `plan.EditorSchema`, `plan.EditorOptions`, `plan.DocSlugs` (Task 1).
- Produces, for Task 5:
  - `#doc` textarea with `hx-sync="this:replace"`
  - `<div data-view-switch hidden>` with `<button type="button" data-view="form">` and `<button type="button" data-view="json">`
  - `<p data-view-note hidden>`
  - `<div id="doc-form" hidden>`
  - `#plan-schema` holding the editor schema
  - in the preview: `#plan-problems` (JSON `[{pointer, message, warning}]` or `null`), and each problem with a pointer as `<button type="button" data-pointer="…">`

- [ ] **Step 1: Write the failing tests**

Append to `internal/web/plans_test.go` (reuse its helpers for the dev user, CSRF and posting; read the file's existing tests for their names):

```go
func TestPlanEditorUsesTheEditorSchema(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	for _, want := range []string{`"format":"per-week"`, `"definitions"`, `data-view-switch`, `id="doc-form"`, `hx-sync="this:replace"`,
		`Barbell Back Squat`} {
		if !strings.Contains(html, want) {
			t.Errorf("editor page lacks %q", want)
		}
	}
	if strings.Contains(html, "jse-theme-dark") || strings.Contains(html, "plan-editor.js") {
		t.Error("the old JSON editor is still loaded")
	}
}

func TestPlanPreviewCarriesProblemsJSON(t *testing.T) {
	srv, c := newApp(t, "alice")
	page := read(t, mustGet(t, c, srv.URL+"/plans/new"))
	csrf := csrfInput.FindStringSubmatch(page)[1]
	resp, err := c.PostForm(srv.URL+"/plans/preview", url.Values{"doc": {`{"name":"x","weeks":0,"days":[]}`}, "csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	html := read(t, resp)
	if !strings.Contains(html, `id="plan-problems"`) || !strings.Contains(html, `data-pointer="/weeks"`) {
		t.Fatalf("preview = %s", html)
	}
}
```

(Add `"net/url"` and `"strings"` to the imports if they're missing.)

Run: `nix develop --command go test ./internal/web/ -run 'PlanEditorUses|PlanPreviewCarries'`
Expected: FAIL: the page embeds the contract and has no switch; the preview has no problems JSON.

- [ ] **Step 2: Implement**

`internal/web/plans.go`: `editor` builds the editor schema for the user and the document:

```go
func (s *Server) editor(r *http.Request, title, planID, doc string, problems plan.Problems) (views.PlanEditor, error) {
	u := user(r)
	catalog, err := s.Exercises.Catalog(r.Context(), u.ID, "")
	if err != nil {
		return views.PlanEditor{}, err
	}
	schema, err := plan.EditorSchema(plan.EditorOptions{Catalog: testCatalog, DocSlugs: plan.DocSlugs([]byte(doc)), Unit: u.Unit})
	if err != nil {
		return views.PlanEditor{}, err
	}
	return views.PlanEditor{PlanID: planID, Title: title, Doc: doc, Schema: string(schema), Errors: problems}, nil
}
```

Update every caller (`grep -n "s.editor(" internal/web/plans.go`) to pass `r` and handle the error with `s.fail(w, r, err)`.

`internal/web/views/plans.templ` `PlanEditorPage`:
- Replace the intro paragraph with: `Edit the plan in the form, or switch to JSON. <a href="/schema/plan.json" …>Schema</a>.`
- Above the textarea, add:

```templ
			<div class="mb-2 flex flex-wrap items-center gap-2">
				<div data-view-switch hidden class="inline-flex overflow-hidden rounded border border-zinc-300 text-sm dark:border-zinc-700">
					<button type="button" data-view="form" class="px-3 py-1.5 aria-pressed:bg-zinc-900 aria-pressed:text-white dark:aria-pressed:bg-zinc-100 dark:aria-pressed:text-zinc-900">Form</button>
					<button type="button" data-view="json" class="px-3 py-1.5 aria-pressed:bg-zinc-900 aria-pressed:text-white dark:aria-pressed:bg-zinc-100 dark:aria-pressed:text-zinc-900">JSON</button>
				</div>
				<p data-view-note hidden class="text-sm text-amber-700 dark:text-amber-400"></p>
			</div>
```

- Add `hx-sync="this:replace"` to the textarea.
- Replace `<div id="doc-editor" class="h-[70vh]" hidden></div>` with `<div id="doc-form" hidden></div>`.

`PlanProblems`: for a problem with a pointer, render the pointer as a button:

```templ
					if pr.Pointer != "" {
						<button type="button" data-pointer={ pr.Pointer } class="font-mono underline">{ pr.Pointer }</button>:
					}
```

`PlanPreview`: add `@templ.JSONScript("plan-problems", ps)` before `@PlanProblems(ps)`.

`internal/web/views/layout.templ`: remove the `jse-theme-dark.css` link, and replace `<script type="module" src="/static/js/plan-editor.js"></script>` with `<script type="module" src="/static/js/plan-form.js"></script>`.

Create a minimal `internal/web/static/js/plan-form.js` so the page loads without errors until Task 5:

```js
// Plan form editor: set up in the next commit; until then the textarea is the editor.
```

`internal/web/static/js/sw.js` `SHELL_FILES`: remove `"/static/vendor/jsoneditor/jse-theme-dark.css"` and `"/static/js/plan-editor.js"`; add `"/static/js/plan-form.js"`, `"/static/js/plan-form-core.js"` and `"/static/js/plan-form-theme.js"`. Create `plan-form-theme.js` as an empty module (`export {};`) so the shell's install doesn't 404. Task 5 fills it in.

`internal/web/styles/input.css`: after `@source "../views";`, add `@source "../static/js/plan-form-theme.js";`.

Delete the old editor: `git rm -r internal/web/static/vendor/jsoneditor internal/web/static/js/plan-editor.js`.

Run: `nix develop --command bash -c 'task generate && go test ./internal/web/ && task test:js'`
Expected: PASS, including `TestServiceWorkerAndOfflinePage` and the earlier plan tests.

- [ ] **Step 3: Commit**

```bash
git add -A internal/web
git commit -m "feat(web): serve the plan editor schema and problems for the form; drop the text JSON editor"
```

---

### Task 5: The form in the browser

**Files:**
- Modify: `internal/web/static/js/plan-form-theme.js` (was empty), `internal/web/static/js/plan-form.js` (was a stub)

**Interfaces:**
- Consumes: `window.JSONEditor` (Task 2), `plan-form-core.js` (Task 3), the markup and IDs of Task 4, and the editor schema shape of Task 1.
- Produces: `register(JSONEditor)` (theme `onerep`, editors `perweek` and `weekset`, template engine `onerep`) and `setNames(names)`. Also `#doc-form.je-ready` when the form is up, and `[data-per-week="<path>"]` on every per-week field container (used by Task 6).

- [ ] **Step 1: The theme and custom editors**

`internal/web/static/js/plan-form-theme.js`:

```js
// json-editor theme, custom editors and header templates for the plan form.
// Classes mirror internal/web/views/ui.go (keep them in sync); every class is
// a literal string so Tailwind's scanner (styles/input.css @source) sees it.
import * as core from "./plan-form-core.js";

const cls = {
  input: "block w-full rounded border border-zinc-300 bg-white px-2 py-1.5 text-sm min-h-10 sm:min-h-0 dark:border-zinc-700 dark:bg-zinc-900",
  invalid: ["border-red-500", "dark:border-red-500"],
  warning: ["border-amber-500", "dark:border-amber-500"],
  label: "block text-sm font-medium",
  hint: "mt-1 text-xs text-zinc-500",
  error: "mt-1 text-sm text-red-600 dark:text-red-400",
  warn: "mt-1 text-sm text-amber-700 dark:text-amber-400",
  button: "inline-flex min-h-10 items-center rounded border border-zinc-300 px-2 py-1 text-xs hover:bg-zinc-100 sm:min-h-0 dark:border-zinc-700 dark:hover:bg-zinc-800",
  danger: "inline-flex min-h-10 items-center rounded border border-red-300 px-2 py-1 text-xs text-red-700 hover:bg-red-50 sm:min-h-0 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950",
  card: "mt-2 rounded border border-zinc-200 p-3 dark:border-zinc-800",
  header: "text-sm font-semibold",
  tabs: "flex gap-1 overflow-x-auto border-b border-zinc-200 dark:border-zinc-800",
  tab: "cursor-pointer whitespace-nowrap border-b-2 px-3 py-2 text-sm",
  tabActive: ["border-zinc-900", "font-medium", "dark:border-zinc-100"],
  tabInactive: ["border-transparent", "text-zinc-500"],
  control: "mb-2",
  weeks: "flex flex-wrap gap-1",
  weekInput: "w-16 rounded border border-zinc-300 bg-white px-1 py-1 text-sm min-h-10 sm:min-h-0 dark:border-zinc-700 dark:bg-zinc-900",
  check: "inline-flex items-center gap-1 text-xs text-zinc-600 dark:text-zinc-400",
};
// json-editor grid sizes 1-12; full width on phones.
const cols = ["", "sm:w-1/12", "sm:w-2/12", "sm:w-3/12", "sm:w-4/12", "sm:w-5/12", "sm:w-6/12",
  "sm:w-7/12", "sm:w-8/12", "sm:w-9/12", "sm:w-10/12", "sm:w-11/12", "sm:w-full"];
const add = (el, c) => { el.classList.add(...(Array.isArray(c) ? c : c.split(" "))); return el; };

let names = {};
export function setNames(n) { names = n; }

let registered = false;
export function register(JSONEditor) {
  if (registered) return;
  registered = true;

  class OnerepTheme extends JSONEditor.AbstractTheme {
    constructor(jsoneditor) { super(jsoneditor, { disable_theme_rules: true }); }
    getFormInputLabel(text, req) { const l = super.getFormInputLabel(text, req); l.className = cls.label; return l; }
    getFormInputField(type) { return add(super.getFormInputField(type), cls.input); }
    getSelectInput(options, multiple) { return add(super.getSelectInput(options, multiple), cls.input); }
    getSwitcher(options) { return add(super.getSwitcher(options), cls.input); }
    getTextareaInput() { const t = add(super.getTextareaInput(), cls.input); t.rows = 2; return t; }
    getFormControl(label, input, description, infoText, formName) {
      return add(super.getFormControl(label, input, description, infoText, formName), cls.control);
    }
    getHeader(text, pathDepth) { return add(super.getHeader(text, pathDepth), cls.header); }
    getDescription(text) { return add(super.getDescription(text), cls.hint); }
    getIndentedPanel() { return add(document.createElement("div"), cls.card); }
    getTopIndentedPanel() { return add(document.createElement("div"), "mt-2"); }
    getGridContainer() { return document.createElement("div"); }
    getGridRow() { return add(document.createElement("div"), "-mx-1 flex flex-wrap"); }
    getGridColumn() { return add(document.createElement("div"), "w-full px-1"); }
    setGridColumnSize(el, size) { if (cols[size]) add(el, cols[size]); }
    getButtonHolder() { return add(document.createElement("span"), "inline-flex flex-wrap gap-1"); }
    getButton(text, icon, title) {
      const b = super.getButton(text, icon, title);
      const danger = /^(delete|remove)/i.test(title || text || "");
      return add(b, danger ? cls.danger : cls.button);
    }
    getErrorMessage(text) { const p = add(document.createElement("p"), cls.error); p.textContent = text; return p; }
    addInputError(input, text) {
      const warning = text.startsWith("Warning: ");
      const group = input.controlgroup || input.parentNode;
      if (!input.errmsg && group) { input.errmsg = document.createElement("p"); group.appendChild(input.errmsg); }
      if (input.errmsg) {
        input.errmsg.className = warning ? cls.warn : cls.error;
        input.errmsg.setAttribute("role", "alert");
        input.errmsg.textContent = text.replace(/^Warning: /, "").replace(/\.$/, "");
        input.errmsg.hidden = false;
      }
      input.classList.remove(...cls.invalid, ...cls.warning);
      input.classList.add(...(warning ? cls.warning : cls.invalid));
    }
    removeInputError(input) {
      if (input.errmsg) input.errmsg.hidden = true;
      input.classList.remove(...cls.invalid, ...cls.warning);
    }
    getTopTabHolder(propertyName) {
      const holder = document.createElement("div");
      const strip = add(document.createElement("div"), cls.tabs);
      strip.setAttribute("role", "tablist");
      const content = document.createElement("div");
      if (propertyName) content.id = propertyName;
      holder.append(strip, content);
      return holder;
    }
    getTopTabContentHolder(holder) { return holder.children[1]; }
    getTopTab(span, tabId) {
      const t = add(document.createElement("div"), cls.tab);
      t.id = tabId;
      t.setAttribute("role", "tab");
      t.appendChild(span);
      return t;
    }
    addTopTab(holder, tab) { holder.children[0].appendChild(tab); }
    markTabActive(row) {
      row.tab.classList.remove(...cls.tabInactive);
      row.tab.classList.add(...cls.tabActive);
      row.tab.setAttribute("aria-selected", "true");
      (row.rowPane || row.container).style.display = "";
    }
    markTabInactive(row) {
      row.tab.classList.remove(...cls.tabActive);
      row.tab.classList.add(...cls.tabInactive);
      row.tab.setAttribute("aria-selected", "false");
      (row.rowPane || row.container).style.display = "none";
    }
  }

  // Shared by the per-week and week-set fields.
  class WeeksAware extends JSONEditor.AbstractEditor {
    weeks() {
      const w = this.jsoneditor.getEditor("root.weeks")?.getValue();
      return Number.isInteger(w) && w >= 1 && w <= 52 ? w : 1;
    }
    watchWeeks() {
      this.onWeeks = () => this.weeksChanged();
      this.jsoneditor.watch("root.weeks", this.onWeeks);
    }
    commit(value) {
      const changed = JSON.stringify(value) !== JSON.stringify(this.value);
      this.value = value;
      if (changed) { this.is_dirty = true; this.onChange(true); }
    }
    showValidationErrors(errors) {
      const msgs = errors.filter((e) => e.path === this.path && e.property === "onerep").map((e) => e.message);
      const warning = msgs.length > 0 && msgs.every((m) => m.startsWith("Warning: "));
      this.errmsg.className = warning ? cls.warn : cls.error;
      this.errmsg.textContent = msgs.map((m) => m.replace(/^Warning: /, "")).join(". ");
      this.errmsg.hidden = msgs.length === 0;
    }
    destroy() {
      if (this.onWeeks) this.jsoneditor.unwatch("root.weeks", this.onWeeks);
      this.control?.remove();
      super.destroy();
    }
    getNumColumns() { return 2; }
  }

  class PerWeekEditor extends WeeksAware {
    build() {
      this.kind = this.options.perWeek?.kind || "number";
      this.state = { vary: false, values: [undefined] };
      this.control = document.createElement("div");
      this.control.dataset.perWeek = this.path;
      const label = this.theme.getFormInputLabel(this.getTitle(), this.isRequired());
      const toggle = add(document.createElement("label"), cls.check);
      this.varyBox = document.createElement("input");
      this.varyBox.type = "checkbox";
      this.varyBox.addEventListener("change", () => {
        const first = this.state.values[0];
        this.state = this.varyBox.checked
          ? { vary: true, values: Array(this.weeks()).fill(first) }
          : { vary: false, values: [first] };
        this.render();
        this.commit(core.perWeekValue(this.state));
      });
      toggle.append(this.varyBox, document.createTextNode("vary by week"));
      this.inputs = add(document.createElement("div"), cls.weeks);
      this.errmsg = add(document.createElement("p"), cls.error);
      this.errmsg.hidden = true;
      this.control.append(label, this.inputs, toggle, this.errmsg);
      if (this.schema.description) this.control.title = this.schema.description;
      this.container.appendChild(this.control);
      this.watchWeeks();
      this.render();
    }
    weeksChanged() {
      if (!this.state.vary) return;
      this.state = core.resize(this.state, this.weeks());
      this.render();
      this.commit(core.perWeekValue(this.state));
    }
    setValue(value, initial) {
      this.state = core.perWeekFrom(value, this.weeks());
      this.value = core.perWeekValue(this.state);
      if (this.inputs) this.render();
      if (!initial) this.is_dirty = true;
      this.onChange(false);
    }
    getValue() { return this.value; }
    field(i) {
      const v = this.state.values[i];
      let el;
      if (this.kind === "rpe") {
        el = add(document.createElement("select"), this.state.vary ? cls.weekInput : cls.input);
        el.append(new Option("", ""));
        for (let r = 6; r <= 10; r += 0.5) el.append(new Option(String(r), String(r)));
      } else {
        el = add(document.createElement("input"), this.state.vary ? cls.weekInput : cls.input);
        el.inputMode = this.kind === "reps" ? "text" : "decimal";
        if (this.kind === "percent") el.placeholder = "%";
      }
      el.value = core.formatValue(this.kind, v);
      el.setAttribute("aria-label", this.state.vary ? `${this.getTitle()} week ${i + 1}` : this.getTitle());
      el.addEventListener("change", () => {
        const r = core.parseInput(this.kind, el.value);
        if (r.error) {
          this.errmsg.textContent = r.error;
          this.errmsg.hidden = false;
          el.classList.add(...cls.invalid);
          return;
        }
        el.classList.remove(...cls.invalid);
        this.errmsg.hidden = true;
        const value = r.value === undefined && this.kind === "count" && this.state.vary ? null : r.value;
        this.state.values[i] = value;
        this.commit(core.perWeekValue(this.state));
      });
      if (!this.state.vary) return el;
      const wrap = add(document.createElement("label"), "flex flex-col text-xs text-zinc-500");
      wrap.append(document.createTextNode(`W${i + 1}`), el);
      return wrap;
    }
    render() {
      this.varyBox.checked = this.state.vary;
      const n = this.state.vary ? this.state.values.length : 1;
      this.inputs.replaceChildren(...Array.from({ length: n }, (_, i) => this.field(i)));
    }
  }

  class WeekSetEditor extends WeeksAware {
    build() {
      this.selected = [];
      this.control = document.createElement("div");
      this.control.dataset.weekSet = this.path;
      this.control.append(this.theme.getFormInputLabel(this.getTitle(), false));
      this.boxes = add(document.createElement("div"), cls.weeks);
      this.errmsg = add(document.createElement("p"), cls.error);
      this.errmsg.hidden = true;
      this.control.append(this.boxes, this.errmsg);
      this.container.appendChild(this.control);
      this.watchWeeks();
      this.render();
    }
    weeksChanged() { this.render(); }
    setValue(value, initial) {
      this.selected = core.weekSetFrom(value);
      this.value = core.weekSetValue(this.selected);
      if (this.boxes) this.render();
      if (!initial) this.is_dirty = true;
      this.onChange(false);
    }
    getValue() { return this.value; }
    render() {
      const n = this.weeks();
      const weeks = [...new Set([...Array.from({ length: n }, (_, i) => i + 1), ...this.selected])].sort((a, b) => a - b);
      this.boxes.replaceChildren(...weeks.map((w) => {
        const l = add(document.createElement("label"), cls.check);
        const box = document.createElement("input");
        box.type = "checkbox";
        box.checked = this.selected.includes(w);
        box.addEventListener("change", () => {
          this.selected = box.checked ? [...this.selected, w] : this.selected.filter((x) => x !== w);
          this.selected = core.weekSetFrom(this.selected);
          this.commit(core.weekSetValue(this.selected));
        });
        l.append(box, document.createTextNode(w > n ? `W${w} (not in plan)` : `W${w}`));
        return l;
      }));
    }
  }

  JSONEditor.defaults.themes.onerep = OnerepTheme;
  JSONEditor.defaults.editors.perweek = PerWeekEditor;
  JSONEditor.defaults.editors.weekset = WeekSetEditor;
  JSONEditor.defaults.resolvers.unshift((schema) =>
    schema.format === "per-week" ? "perweek" : schema.format === "week-set" ? "weekset" : undefined);
  JSONEditor.defaults.templates.onerep = () => ({ compile: (t) => (vars) => core.headerText(t, vars, names) });
}
```

(If the probe in Task 2 showed different method names or arguments, the ledger's `Ruling:` lines say what to change here. Apply them.)

- [ ] **Step 2: Setup, switch, problems**

`internal/web/static/js/plan-form.js`:

```js
// Plan editor (docs/superpowers/specs/2026-09-25-plan-form-editor-design.md):
// a form generated by json-editor (vendored UMD bundle, loaded on first use)
// from the editor schema in #plan-schema, plus a plain JSON view. The #doc
// textarea stays the form field: every form change is written back and
// announced with "doc-changed", so the preview and saving work as before,
// and without JS the textarea is the editor. Loaded once from the layout and
// set up via htmx.onLoad; a restored page is set up again.
import * as core from "./plan-form-core.js";
import { register, setNames } from "./plan-form-theme.js";

const VIEW_KEY = "onerep.planEditorView";
let lib;
const started = new WeakSet();

function loadLibrary() {
  lib ??= new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = "/static/vendor/json-editor/jsoneditor.js";
    s.onload = () => { register(window.JSONEditor); resolve(window.JSONEditor); };
    s.onerror = () => { lib = undefined; reject(new Error("json-editor failed to load")); };
    document.head.append(s);
  });
  return lib;
}

const storedView = () => { try { return localStorage.getItem(VIEW_KEY) || "form"; } catch { return "form"; } };
const storeView = (v) => { try { localStorage.setItem(VIEW_KEY, v); } catch { /* per-browser convenience only */ } };

async function start(textarea) {
  const holder = document.getElementById("doc-form");
  const schemaEl = document.getElementById("plan-schema");
  const switcher = document.querySelector("[data-view-switch]");
  const note = document.querySelector("[data-view-note]");
  const preview = document.getElementById("plan-preview");
  if (!holder || !schemaEl || !switcher) return;
  let JSONEditor;
  try {
    JSONEditor = await loadLibrary();
  } catch (err) {
    console.error("plan form unavailable, using the JSON text", err);
    return;
  }
  const schema = JSON.parse(schemaEl.textContent);
  const unit = schema.properties?.unit?.default || "kg";
  setNames(core.slugNames(schema));
  const editor = new JSONEditor(holder, {
    schema, theme: "onerep", template: "onerep", iconlib: null, show_errors: "never",
    disable_edit_json: true, disable_properties: true, disable_array_delete_all_rows: true,
    disable_array_delete_last_row: true, remove_empty_properties: true, prompt_before_delete: true,
    no_additional_properties: true,
  });
  await editor.promise;

  let view = "json";
  let loading = false;
  const has = (p) => !!editor.getEditor(p);

  const writeBack = () => {
    if (loading || view !== "form") return;
    textarea.value = JSON.stringify(core.clean(editor.getValue()), null, 2);
    textarea.dispatchEvent(new Event("doc-changed", { bubbles: true }));
  };
  // loadForm puts the JSON into the form, or says why it can't.
  const loadForm = () => {
    let doc;
    try { doc = JSON.parse(textarea.value); } catch { return "The JSON doesn't parse, so it stays in the JSON view."; }
    loading = true;
    editor.setValue(doc);
    const back = core.clean(editor.getValue());
    loading = false;
    return core.sameDoc(doc, back, unit) ? null
      : "The form can't show everything in this plan (for example keys it doesn't know), so it stays in the JSON view.";
  };
  const show = (v, reason) => {
    view = v;
    holder.hidden = v !== "form";
    textarea.hidden = v === "form";
    for (const b of switcher.querySelectorAll("[data-view]")) b.setAttribute("aria-pressed", String(b.dataset.view === v));
    note.textContent = reason || "";
    note.hidden = !reason;
  };
  const markProblems = () => {
    if (view !== "form") return;
    const el = document.getElementById("plan-problems");
    editor.showValidationErrors(core.problemsToErrors(el ? JSON.parse(el.textContent) : [], has));
  };

  editor.on("change", writeBack);
  switcher.addEventListener("click", (e) => {
    const v = e.target.closest("[data-view]")?.dataset.view;
    if (!v || v === view) return;
    const reason = v === "form" ? loadForm() : null;
    if (reason) return show("json", reason);
    storeView(v);
    show(v);
    if (v === "form") markProblems();
  });
  const onSwap = () => markProblems();
  const onFocus = (e) => {
    const b = e.target.closest("[data-pointer]");
    if (b && view === "form") focusPath(editor, core.nearestPath(core.pointerToPath(b.dataset.pointer), has).path);
  };
  preview?.addEventListener("htmx:afterSwap", onSwap);
  preview?.addEventListener("click", onFocus);
  textarea.addEventListener("htmx:beforeCleanupElement", () => {
    preview?.removeEventListener("htmx:afterSwap", onSwap);
    preview?.removeEventListener("click", onFocus);
    editor.destroy();
  }, { once: true });

  switcher.hidden = false;
  if (storedView() === "form") {
    const reason = loadForm();
    show(reason ? "json" : "form", reason);
  } else {
    show("json");
  }
}

// focusPath opens the day tab and collapsed sections leading to path, then
// focuses the field.
function focusPath(editor, path) {
  const parts = path.split(".");
  for (let i = 2; i <= parts.length; i++) {
    const parent = editor.getEditor(parts.slice(0, i - 1).join("."));
    const index = Number(parts[i - 1]);
    const row = Number.isInteger(index) ? parent?.rows?.[index] : null;
    if (row?.tab) row.tab.click();
    const ed = editor.getEditor(parts.slice(0, i).join("."));
    if (ed?.collapsed && ed.toggle_button) ed.toggle_button.click();
  }
  const target = editor.getEditor(path);
  const el = target?.input || target?.container?.querySelector("input, select, textarea");
  el?.scrollIntoView({ block: "center" });
  el?.focus();
}

htmx.onLoad((root) => {
  const textarea = root.id === "doc" ? root : root.querySelector?.("#doc");
  if (textarea && !started.has(textarea)) {
    started.add(textarea);
    start(textarea);
  }
});
```

- [ ] **Step 3: Check it in a browser and fix what you see**

Run `task generate`, then start the app (`task dev`) and open `/plans/new` in headless Chromium, using the e2e harness or the Chrome DevTools MCP. Check:
- the starter template in the form
- days as tabs
- the Upper day's bench set lines, one row each at 1024 px width
- "vary by week" on the count with W1–W4
- the load switcher on "% of TM" with 75/80/85/65
- no console errors, and the textarea updating on each change (switch to JSON to look)

Take screenshots at 1280 and 390 px, in light and dark (`Emulation.setEmulatedMedia`). Fix layout problems in the theme's classes; record anything that changes this plan's code as a `Ruling:`.

Run: `nix develop --command bash -c 'task generate && task test:js && go test ./internal/web/'`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/web
git commit -m "feat(web): edit plans in a generated form with per-week fields and a JSON view"
```

---

### Task 6: Browser check, docs

**Files:**
- Create: `test/e2e/harness.mjs`, `test/e2e/plan-editor.mjs`
- Modify: `test/e2e/companion.mjs` (use the harness), `Taskfile.yml` (`e2e` runs both), `AGENTS.md`

**Interfaces:**
- Consumes: `#doc-form.je-ready`, `[data-view="form"|"json"]`, `[data-view-note]`, `[data-per-week="…"]`, `input[name="root[weeks]"]`, and the "Save as draft" button.

- [ ] **Step 1: Extract the harness**

Move `startServer`, `stopServer`, `waitFor`, `browser`, `check`, `sleep` and the temp-dir/port setup from the top of `test/e2e/companion.mjs` into `test/e2e/harness.mjs`, exported unchanged:

```js
export function setup(name) // → { dir, BASE, startServer, stopServer, waitFor, browser, check, results, sleep, finish }
```

`finish(b)` closes the browser, stops the server, removes the dir, prints `N/M checks passed` and exits with the right code. `companion.mjs` imports `setup("companion")` and keeps its scenario unchanged.

Run: `nix develop --command task e2e`
Expected: `21/21 checks passed` (nothing lost in the move).

- [ ] **Step 2: Write the plan editor check**

`test/e2e/plan-editor.mjs`:

```js
// End-to-end check of the plan form editor: the starter template in the form,
// adding a set line, a per-week field following the plan's weeks, a server
// problem marked on its field, a draft round-tripping, and the JSON view
// refusing a document the form can't show.
import { setup } from "./harness.mjs";

const h = setup("plan-editor");
let b;
try {
  await h.startServer();
  b = await h.browser();
  const { send, evaluate } = b;
  const go = async (path) => {
    await send("Page.navigate", { url: h.BASE + path });
    await h.waitFor(() => evaluate("document.readyState === 'complete'"), `load of ${path}`);
  };
  const doc = () => evaluate("JSON.parse(document.getElementById('doc').value)");
  const setInput = (selector, value) => evaluate(`(() => { const el = document.querySelector(${JSON.stringify(selector)});
    el.value = ${JSON.stringify(value)}; el.dispatchEvent(new Event('change', {bubbles: true})); el.dispatchEvent(new Event('input', {bubbles: true})); })()`);

  await go("/plans/new");
  await h.waitFor(() => evaluate("!!document.querySelector('#doc-form.je-ready') && !document.getElementById('doc-form').hidden"), "the form");
  h.check("the starter template opens in the form",
    await evaluate(`document.querySelector('#doc-form input[name="root[name]"]').value === 'Upper/Lower starter'`));

  const setsBefore = (await doc()).days[0].groups[0].exercises[0].sets.length;
  await evaluate(`[...document.querySelectorAll('#doc-form button')].find(b => /^Add Set line/.test(b.textContent.trim())).click()`);
  await h.waitFor(async () => (await doc()).days[0].groups[0].exercises[0].sets.length === setsBefore + 1, "a new set line").catch(() => {});
  h.check("adding a set line updates the JSON", (await doc()).days[0].groups[0].exercises[0].sets.length === setsBefore + 1);

  const rest = "[data-per-week='root.days.0.groups.0.rest_s']";
  await evaluate(`document.querySelector("${rest} input[type=checkbox]").click()`);
  await setInput(`input[name="root[weeks]"]`, "5");
  await h.waitFor(async () => (await evaluate(`document.querySelectorAll("${rest} input:not([type=checkbox])").length`)) === 5, "five week inputs").catch(() => {});
  h.check("a varying field follows the plan's weeks", (await evaluate(`document.querySelectorAll("${rest} input:not([type=checkbox])").length`)) === 5);
  await setInput(`${rest} input:not([type=checkbox])`, "99999");
  await h.waitFor(async () => (await evaluate(`document.querySelector("${rest} p")?.textContent || ''`)).length > 0, "the rest problem").catch(() => {});
  h.check("a server problem is marked on its field", /W1|3600|maximum/i.test(await evaluate(`document.querySelector("${rest} p")?.textContent || ''`)));

  await setInput(`${rest} input:not([type=checkbox])`, "180");
  await h.waitFor(async () => !(await evaluate(`document.getElementById('plan-preview').textContent`)).includes("3600"), "a clean preview").catch(() => {});
  const saved = await doc();
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Save as draft').click()`);
  await h.waitFor(() => evaluate("/^\\/plans\\/[0-9a-f-]{36}$/.test(location.pathname)"), "the plan page");
  const edit = await evaluate(`[...document.querySelectorAll('a')].find(a => /\\/edit\\?from=/.test(a.getAttribute('href') || ''))?.getAttribute('href')`);
  await go(edit);
  await h.waitFor(() => evaluate("!!document.querySelector('#doc-form.je-ready')"), "the form again");
  const norm = (d) => JSON.stringify(d, (k, v) => (v && typeof v === "object" && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort()) : v));
  h.check("a saved draft reopens with the same plan", norm(await doc()) === norm({ unit: "kg", ...saved }));

  await evaluate(`document.querySelector('[data-view="json"]').click()`);
  await evaluate(`(() => { const t = document.getElementById('doc'); const d = JSON.parse(t.value); d.surprise = 1; t.value = JSON.stringify(d); })()`);
  await evaluate(`document.querySelector('[data-view="form"]').click()`);
  h.check("the form refuses a plan it can't show",
    await evaluate(`!document.getElementById('doc').hidden && !document.querySelector('[data-view-note]').hidden`));
} catch (err) {
  h.check("scenario ran to the end", false, err.stack || String(err));
} finally {
  await h.finish(b);
}
```

`Taskfile.yml` `e2e`: run `node test/e2e/companion.mjs && node test/e2e/plan-editor.mjs` with the same environment.

Run: `nix develop --command task e2e`
Expected: `21/21` and `6/6 checks passed`. If the form's JSON after reopening differs only in key order or cleaned values, the comparison is wrong, not the form: compare with `sameDoc` semantics. Otherwise debug the form.

- [ ] **Step 3: Docs**

`AGENTS.md`:
- Layout: extend the `internal/plan/` line with `editor schema for the form (editor.go + editor.overlay.json)`. Add `static/js/plan-form*.js: plan editor form (json-editor, vendored) and its pure logic` under `internal/web/`, and `test/e2e/plan-editor.mjs` next to the e2e line.
- Rules, in the **Plan documents** rule, add: "The editor's form uses a schema derived by `plan.EditorSchema` (UI hints in `editor.overlay.json`, never in the contract). A new per-week field needs a kind in `perWeekKinds`; `TestPerWeekFieldsAreAllRewritten` and `TestEditorOverlayPointsIntoContract` fail when the two drift."

- [ ] **Step 4: Run CI and commit**

Run: `nix develop --command task ci && nix develop --command task e2e`
Expected: both succeed.

```bash
git add test/e2e Taskfile.yml AGENTS.md
git commit -m "test(e2e): check the plan form editor; document it"
```
