package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

const planDoc = `{"name":"Block","weeks":2,"days":[{"name":"Squat day","groups":[{"exercises":[
	{"slug":"barbell-back-squat","sets":[{"count":3,"reps":5,"load":{"weight":[100,105]}}]}]}]}]}`

func TestPlanTools(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	cs := connect(t, e.url, e.aliceToken)

	var schema struct {
		Schema map[string]any `json:"schema"`
		Guide  string         `json:"guide"`
	}
	call(t, cs, "get_plan_schema", map[string]any{}, &schema)
	if schema.Schema["$id"] == nil || !strings.Contains(schema.Guide, "per week") {
		t.Fatalf("schema tool = %v…, guide %d chars", schema.Schema["$id"], len(schema.Guide))
	}

	var valid struct {
		Valid    bool `json:"valid"`
		Problems []struct {
			Pointer string `json:"pointer"`
		} `json:"problems"`
	}
	call(t, cs, "validate_plan", map[string]any{"doc": map[string]any{"name": "x", "weeks": 0}}, &valid)
	if valid.Valid || len(valid.Problems) == 0 {
		t.Fatalf("invalid doc validated: %+v", valid)
	}
	// A document sent as a JSON string works like an object.
	call(t, cs, "validate_plan", map[string]any{"doc": planDoc}, &valid)
	if !valid.Valid {
		t.Fatalf("string doc: %+v", valid)
	}

	var draft struct {
		PlanID    string `json:"plan_id"`
		VersionID string `json:"version_id"`
		Version   int    `json:"version"`
		Status    string `json:"status"`
		ReviewURL string `json:"review_url"`
	}
	if msg := call(t, cs, "save_plan_draft", map[string]any{"doc": planDoc, "note": "first try"}, &draft); msg != "" {
		t.Fatal(msg)
	}
	if draft.Status != "draft" || draft.Version != 1 ||
		draft.ReviewURL != "http://example.test/plans/"+draft.PlanID+"/versions/"+draft.VersionID+"/compare" {
		t.Fatalf("draft = %+v", draft)
	}
	if msg := call(t, cs, "save_plan_draft", map[string]any{"doc": map[string]any{"name": "x"}}, nil); !strings.Contains(msg, "/weeks") && !strings.Contains(msg, "weeks") {
		t.Fatalf("invalid draft error should list problems: %q", msg)
	}

	// The user activates it in the web UI; the AI then can't overwrite it.
	if _, err := e.Plans().Activate(ctx, e.alice, draft.VersionID); err != nil {
		t.Fatal(err)
	}
	if msg := call(t, cs, "save_plan_draft", map[string]any{"doc": planDoc, "version_id": draft.VersionID}, nil); !strings.Contains(msg, "draft") {
		t.Fatalf("overwriting the active version: %q", msg)
	}
	var next struct {
		VersionID string `json:"version_id"`
		Version   int    `json:"version"`
	}
	call(t, cs, "save_plan_draft", map[string]any{"doc": planDoc, "plan_id": draft.PlanID}, &next)
	if next.Version != 2 {
		t.Fatalf("next version = %+v", next)
	}

	var plans struct {
		Plans []struct {
			ID       string `json:"id"`
			Versions []struct {
				Status string `json:"status"`
				Source string `json:"source"`
			} `json:"versions"`
		} `json:"plans"`
	}
	call(t, cs, "list_plans", map[string]any{}, &plans)
	if len(plans.Plans) != 1 || len(plans.Plans[0].Versions) != 2 || plans.Plans[0].Versions[0].Source != "mcp" {
		t.Fatalf("plans = %+v", plans)
	}

	var got struct {
		Status  string         `json:"status"`
		Doc     map[string]any `json:"doc"`
		Preview []struct {
			Week int `json:"week"`
			Days []struct {
				Name  string   `json:"name"`
				Lines []string `json:"lines"`
			} `json:"days"`
		} `json:"preview"`
	}
	call(t, cs, "get_plan", map[string]any{"plan_id": draft.PlanID}, &got)
	if got.Status != store.PlanActive || got.Doc["name"] != "Block" || len(got.Preview) != 2 ||
		!strings.Contains(strings.Join(got.Preview[1].Days[0].Lines, "\n"), "105") {
		t.Fatalf("get_plan = %+v", got)
	}
	if msg := call(t, cs, "get_plan", map[string]any{}, nil); !strings.Contains(msg, "follow") {
		t.Fatalf("get_plan without a followed plan: %q", msg)
	}
}

// The guide's example is a valid plan, so the prose can't drift from the rules.
func TestPlanFormatExampleIsValid(t *testing.T) {
	e := newEnv(t)
	_, example, ok := strings.Cut(planFormat, "## Example\n\n```json\n")
	example, _, _ = strings.Cut(example, "```")
	if !ok || example == "" {
		t.Fatal("plan_format.md has no example")
	}
	_, ps := e.Plans().Validate(context.Background(), e.alice, []byte(example))
	if ps.HasErrors() { // warnings (pct_tm without a training max) are fine
		t.Fatalf("example problems: %v", ps)
	}
}

// A plan sent as an object is stored in the client's key order, so the
// review page's JSON diff compares like with like.
func TestDraftKeepsKeyOrder(t *testing.T) {
	e := newEnv(t)
	cs := connect(t, e.url, e.aliceToken)
	var draft struct {
		VersionID string `json:"version_id"`
	}
	args := json.RawMessage(`{"doc": ` + planDoc + `}`)
	if msg := call(t, cs, "save_plan_draft", args, &draft); msg != "" {
		t.Fatal(msg)
	}
	v, err := e.Plans().Version(context.Background(), e.alice, draft.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(v.Doc), `{"unit":"kg","name":"Block","weeks":2,"days":[{"name":"Squat day"`) {
		t.Fatalf("stored doc reordered: %.120s", v.Doc)
	}
}

// Plan weights are in the plan's unit, which defaults to the user's. Tool
// weights are kg, so the AI is told to always name the unit, and the example
// does, or a lb user could get 100 lb where the AI meant 100 kg.
func TestGuideAsksForTheUnit(t *testing.T) {
	_, example, _ := strings.Cut(planFormat, "## Example")
	if !strings.Contains(example, `"unit": "kg"`) {
		t.Error("the guide's example doesn't set the unit")
	}
	if !strings.Contains(instructions, `"unit"`) || !strings.Contains(planFormat, `Always set "unit"`) {
		t.Error("the instructions and the guide must tell the AI to set the plan's unit")
	}
}

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
