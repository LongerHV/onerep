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

var editorCatalog = []store.Exercise{
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
	raw, err := EditorSchema(EditorOptions{Catalog: editorCatalog, DocSlugs: append(DocSlugs(golden), DocSlugs(StarterTemplate())...), Unit: "kg"})
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
	s := editorSchema(t, EditorOptions{Catalog: editorCatalog, Unit: "kg"})
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
	s := editorSchema(t, EditorOptions{Catalog: editorCatalog, Unit: "kg"})
	slug, _ := lookup(s, "/definitions/slug")
	enum := slug["enum"].([]any)
	titles := slug["options"].(map[string]any)["enum_titles"].([]any)
	if len(enum) != 3 || len(titles) != 3 || enum[0] != "barbell-back-squat" || titles[2] != "Zercher Squat" {
		t.Fatalf("enum %v, titles %v (sorted by name, custom included)", enum, titles)
	}
}

func TestEditorSchemaKeepsDocSlugs(t *testing.T) {
	s := editorSchema(t, EditorOptions{Catalog: editorCatalog, DocSlugs: []string{"hidden-old-lift", "barbell-back-squat"}, Unit: "kg"})
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
	s := editorSchema(t, EditorOptions{Catalog: editorCatalog, Unit: "lb"})
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
