// Package mcp is onerep's MCP server (spec §12): tools and prompts that let
// an AI assistant read a user's training and write plan drafts, authenticated
// by personal API tokens. Tools are thin adapters over the services.
package mcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/stats"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/training"
)

const instructions = `onerep is the user's gym training log. Weights are always in kilograms (fields ending in _kg); tools that show weights also give the user's preferred unit, so talk to them in it.
You can read their sessions and stats, search and create exercises, set training maxes, and write training plan drafts. You cannot change logged workouts or activate plans: after save_plan_draft, send the user the review_url so they can compare and activate the draft themselves.
Before writing a plan, call get_plan_schema and use exercise slugs from list_exercises.`

type Server struct {
	Users interface {
		UserByID(ctx context.Context, id string) (store.User, error)
	}
	Tokens interface {
		Verify(ctx context.Context, secret string) (store.User, error)
	}
	Exercises *exercise.Service
	Plans     *plan.Service
	Training  *training.Service
	Stats     *stats.Service
	BaseURL   string
}

// Handler serves MCP over stateless Streamable HTTP behind bearer-token auth.
func (s *Server) Handler() http.Handler {
	srv := sdk.NewServer(&sdk.Implementation{Name: "onerep", Version: "1"}, &sdk.ServerOptions{Instructions: instructions})
	s.addExerciseTools(srv)
	s.addPlanTools(srv)
	s.addTrainingTools(srv)
	s.addPrompts(srv)
	h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv },
		// Requests are bearer-authenticated, which already defeats DNS rebinding (a
		// browser can't add the token), and the localhost check would refuse every
		// request from a reverse proxy or tunnel on the same host.
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true, DisableLocalhostProtection: true})
	return auth.RequireBearerToken(s.verify, &auth.RequireBearerTokenOptions{AllowMissingExpiration: true, Scopes: []string{"mcp"}})(h)
}

func (s *Server) verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	u, err := s.Tokens.Verify(ctx, token)
	if errors.Is(err, store.ErrNotFound) {
		return nil, auth.ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	return &auth.TokenInfo{UserID: u.ID, Scopes: []string{"mcp"}}, nil
}

// user is the token's user for a tool call.
func (s *Server) user(ctx context.Context, extra *sdk.RequestExtra) (store.User, error) {
	if extra == nil || extra.TokenInfo == nil || extra.TokenInfo.UserID == "" {
		return store.User{}, errors.New("not authenticated")
	}
	return s.Users.UserByID(ctx, extra.TokenInfo.UserID)
}

// tool registers a typed tool whose handler gets the calling user. Errors are
// turned into messages the AI can act on.
func tool[In, Out any](s *Server, srv *sdk.Server, t *sdk.Tool, h func(ctx context.Context, u store.User, in In) (Out, error)) {
	sdk.AddTool(srv, t, func(ctx context.Context, req *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
		var zero Out
		u, err := s.user(ctx, req.Extra)
		if err != nil {
			return nil, zero, err
		}
		out, err := h(ctx, u, in)
		if err != nil {
			return nil, zero, explain(ctx, t.Name, err)
		}
		return nil, out, nil
	})
}

// explain turns an error into a tool error message.
func explain(ctx context.Context, tool string, err error) error {
	var ps plan.Problems
	var fe exercise.FieldErrors
	var invalid inputError
	switch {
	case errors.As(err, &ps):
		var b strings.Builder
		b.WriteString("the plan has errors (JSON Pointer: message):")
		for _, p := range ps {
			if !p.Warning {
				fmt.Fprintf(&b, "\n%s: %s", p.Pointer, p.Message)
			}
		}
		return errors.New(b.String())
	case errors.As(err, &fe), errors.As(err, &invalid):
		return err
	case errors.Is(err, store.ErrNotFound):
		return errors.New("not found")
	case errors.Is(err, store.ErrNotDraft):
		return errors.New("only draft versions can be changed; pass plan_id to add a new draft version instead")
	default:
		id := fmt.Sprintf("%d", time.Now().UnixNano())
		slog.ErrorContext(ctx, "mcp tool failed", "tool", tool, "err", err, "request_id", id)
		return fmt.Errorf("something went wrong (request %s)", id)
	}
}

// inputError is a bad argument the AI can fix.
type inputError string

func (e inputError) Error() string { return string(e) }

// jsonValue re-decodes v (a struct, raw JSON or bytes) into plain JSON values, so
// outputs whose shape the schema can't describe travel as "any".
func jsonValue(raw []byte) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// docBytes accepts a plan document as a JSON object or a JSON string.
func docBytes(doc any) ([]byte, error) {
	switch d := doc.(type) {
	case nil:
		return nil, inputError("doc is required")
	case string:
		return []byte(d), nil
	default:
		return json.Marshal(d)
	}
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }
