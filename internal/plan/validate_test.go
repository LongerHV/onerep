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
