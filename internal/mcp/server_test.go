package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
	"github.com/LongerHV/onerep/internal/training"
)

type env struct {
	db           *store.DB
	url          string
	alice, bob   store.User
	aliceToken   string
	bobToken     string
	tokens       *auth.Tokens
	aliceTokenID string
}

func newEnv(t *testing.T) env {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := exercise.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	ex := &exercise.Service{Store: db}
	plans := &plan.Service{Store: db, Exercises: ex, History: db}
	tokens := &auth.Tokens{Store: db}
	s := &Server{Users: db, Tokens: tokens, Exercises: ex, Plans: plans,
		Training: &training.Service{Store: db, Plans: plans, Exercises: ex},
		Stats:    &stats.Service{Store: db, Exercises: ex}, BaseURL: "http://example.test"}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	e := env{db: db, url: srv.URL, tokens: tokens}
	e.alice, _ = db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	e.bob, _ = db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	var tok store.APIToken
	e.aliceToken, tok, _ = tokens.Create(ctx, e.alice.ID, "test")
	e.aliceTokenID = tok.ID
	e.bobToken, _, _ = tokens.Create(ctx, e.bob.ID, "test")
	return e
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// connect opens an MCP client session to the server as the token's user.
func connect(t *testing.T, url, token string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: url, HTTPClient: &http.Client{Transport: bearer{token}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

// call runs a tool and decodes its structured output into out. It fails the
// test on a tool error unless wantErr is set; it returns the error text.
func call(t *testing.T, cs *sdk.ClientSession, tool string, args any, out any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		return res.Content[0].(*sdk.TextContent).Text
	}
	if out != nil {
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatalf("%s output %s: %v", tool, b, err)
		}
	}
	return ""
}

func TestMCPAuth(t *testing.T) {
	e := newEnv(t)
	post := func(auth string) int {
		req, _ := http.NewRequest(http.MethodPost, e.url, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for name, h := range map[string]string{"none": "", "no Bearer prefix": e.aliceToken, "unknown": "Bearer onerep_nope",
		"session cookie style": "Basic abc"} {
		if code := post(h); code != http.StatusUnauthorized {
			t.Errorf("%s: %d, want 401", name, code)
		}
	}
	cs := connect(t, e.url, e.aliceToken)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_exercises", "create_exercise", "set_training_max", "get_plan_schema", "get_plan",
		"list_plans", "validate_plan", "save_plan_draft"} {
		if !names[want] {
			t.Errorf("missing tool %s", want)
		}
	}
	for name := range names {
		if strings.Contains(name, "activate") || strings.Contains(name, "delete") || strings.Contains(name, "log_set") {
			t.Errorf("the AI must not be able to %s", name)
		}
	}
	if err := e.tokens.Revoke(context.Background(), e.alice.ID, e.aliceTokenID); err != nil {
		t.Fatal(err)
	}
	if code := post("Bearer " + e.aliceToken); code != http.StatusUnauthorized {
		t.Fatalf("revoked token: %d", code)
	}
}

func TestExerciseTools(t *testing.T) {
	e := newEnv(t)
	cs := connect(t, e.url, e.aliceToken)
	var list struct {
		Exercises []struct {
			Slug           string   `json:"slug"`
			PrimaryMuscles []string `json:"primary_muscles"`
		} `json:"exercises"`
		Muscles []string `json:"muscles"`
	}
	call(t, cs, "list_exercises", map[string]any{"query": "squat", "muscle": "quads"}, &list)
	if len(list.Exercises) < 3 || len(list.Muscles) != 20 {
		t.Fatalf("list = %+v", list)
	}
	for _, x := range list.Exercises {
		if !strings.Contains(x.Slug, "squat") {
			t.Errorf("%s doesn't match the query", x.Slug)
		}
	}
	var created struct {
		Slug   string `json:"slug"`
		Custom bool   `json:"custom"`
	}
	if msg := call(t, cs, "create_exercise", map[string]any{"slug": "zercher-squat", "name": "Zercher Squat", "measurement": "weight_reps",
		"equipment_kind": "barbell", "primary_muscles": []string{"quads"}}, &created); msg != "" || !created.Custom {
		t.Fatalf("create = %+v %s", created, msg)
	}
	if msg := call(t, cs, "create_exercise", map[string]any{"slug": "zercher-squat", "name": "Again", "measurement": "weight_reps",
		"equipment_kind": "barbell", "primary_muscles": []string{"quads"}}, nil); !strings.Contains(msg, "already exists") {
		t.Fatalf("duplicate slug: %q", msg)
	}
	var tm struct {
		TrainingMaxKg *float64 `json:"training_max_kg"`
		PreviousKg    *float64 `json:"previous_kg"`
	}
	call(t, cs, "set_training_max", map[string]any{"slug": "barbell-back-squat", "training_max_kg": 140}, &tm)
	if tm.TrainingMaxKg == nil || *tm.TrainingMaxKg != 140 || tm.PreviousKg != nil {
		t.Fatalf("tm = %+v", tm)
	}
	hist, _ := (&exercise.Service{Store: e.db}).TrainingMaxHistory(context.Background(), e.alice.ID, "barbell-back-squat")
	if len(hist) != 1 || hist[0].Source != "mcp" {
		t.Fatalf("TM history = %+v, want one mcp entry", hist)
	}
	if msg := call(t, cs, "set_training_max", map[string]any{"slug": "barbell-back-squat", "training_max_kg": -5}, nil); msg == "" {
		t.Fatal("a negative training max must be refused")
	}
}

// Plans is a plan service over the env's database, for acting as the user in the web UI.
func (e env) Plans() *plan.Service {
	return &plan.Service{Store: e.db, Exercises: &exercise.Service{Store: e.db}, History: e.db}
}

// Behind a reverse proxy or tunnel on the same host, requests arrive on
// loopback with the public Host header. Bearer auth already stops DNS
// rebinding (a browser can't add the token), so they must be served.
func TestMCPBehindLocalProxy(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest(http.MethodPost, e.url, strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	req.Host = "onerep.example.com"
	req.Header.Set("Authorization", "Bearer "+e.aliceToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxied request = %d, want 200", resp.StatusCode)
	}
}
