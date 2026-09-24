# onerep Milestone 6: MCP — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an AI assistant work with a user's training through MCP:
- Users create and revoke personal API tokens at `/settings/tokens`.
- `/mcp` serves the spec's twelve tools and two prompts over Streamable HTTP, acting as the token's user.
- The AI can read history and stats, manage exercises and training maxes, and write plan **drafts** that the user reviews and activates in the web UI.

**Architecture:**
- **`internal/store`:** gains `api_tokens` (SHA-256 hashes, revocable, `last_used_at` updated at most once a minute), a filtered session search, and an in-place update for draft plan versions.
- **`internal/auth`:** a `Tokens` service creates tokens (shown once), lists, revokes and verifies them.
- **Services** gain what the tools need:
  - `plan.Service.SaveDraft`: a new plan, a new version, or replacing an existing draft; never activates.
  - `training.Service.Sessions`: date and exercise filters.
  - `stats.Point` carries the best set's weight, reps and RPE.
- **New `internal/mcp` package:** builds one `go-sdk` server with typed tools that are thin adapters over the services. It is wrapped in the SDK's `RequireBearerToken` middleware with our verifier, runs stateless with JSON responses, and is mounted at `/mcp` outside the cookie session, CSRF and layout middleware. Tools read the user from the request's `TokenInfo`.

**Tech Stack:** adds `github.com/modelcontextprotocol/go-sdk` v1.8.0, which spec §3 names for MCP. Its licence is MIT moving to Apache-2.0; its dependencies (`google/jsonschema-go`, `golang-jwt/jwt`, `segmentio/encoding`, `yosida95/uritemplate`, `golang.org/x/*`) are MIT, BSD or Apache-2.0. `task licenses:check` must pass.

**Spec:** `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. This plan implements:
- §5 `api_tokens`
- §11 API tokens (`/settings/tokens`, shown once, SHA-256, Bearer)
- §12 the MCP server: all twelve tools, both prompts, and no way to change logged sessions or sets or to activate plans
- §10 MCP drafts replacing a draft's document in place
- §16 validation problems as tool errors
- §17 in-process tests with the go-sdk client covering each tool, including user isolation

**Deliberate differences from and clarifications of spec §12:**
- **Stateless Streamable HTTP with JSON responses.** No server-to-client requests are needed, so a restart never strands a client's session. Each request is authenticated on its own.
- **Weights are in kg** in every tool's input and output (field names end in `_kg`). Outputs that show weights also carry the user's preferred `unit`, so the AI can talk in it.
- **The "diff page URL"** returned by `save_plan_draft` is the existing comparison page `/plans/{id}/versions/{vid}/compare` (milestone 3 named it "compare").
- **`get_plan` previews** are readable lines per week and day (the web preview's `plan.DayLines`), not the raw expanded structure. The authored document comes back as JSON.
- **`list_exercises`** uses the catalog's word search (slug, name, aliases, equipment, muscles) plus an exact `muscle` filter, and returns the muscle vocabulary.
- **`get_exercise_stats`** "recent best sets" are the best set (by e1RM) of each of the last 10 workouts with that exercise, newest first.
- **`set_training_max`** takes `training_max_kg` (`null` clears it) and has no note (the history table never shows notes).
- **`get_weekly_muscle_volume`** defaults to the last 12 ISO weeks and accepts at most 104.
- **Token scope** is always `mcp`, the only scope the spec defines.

## Global Constraints

Everything from milestones 1–5 applies. The commit trailer is `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` plus the session's `Claude-Session:` line. In addition:

- **Tokens:**
  - The format is `onerep_` followed by 43 base64url characters (32 random bytes).
  - Only `auth.HashToken(token)` (SHA-256 hex) is stored.
  - A revoked or unknown token is 401.
  - Token names are 1–100 characters after trimming.
  - The token appears in exactly one response: the page right after creation, sent with `Cache-Control: no-store` and `hx-history="false"` so htmx never stores it.
- **`/mcp`:**
  - It needs a Bearer token and has no cookie session, CSRF or layout.
  - Bodies are at most 1 MB (`maxBodyBytes`).
  - Every tool acts as the token's user. Another user's plan, version, session or custom exercise is "not found".
- **The AI cannot change logged sessions or sets, cannot activate or discard plan versions, and cannot delete anything.** No tool may call `training.Service.SaveSet`, `DeleteSet`, `SetNotes`, `Finish`, `Delete` or `ApplyOps`, or `plan.Service.Activate`, `Discard`, `Follow` or `Archive`.
- **`save_plan_draft`** creates or replaces **draft** versions only, with `source = "mcp"`. Replacing a non-draft version is an error.
- **Errors:** validation problems come back as tool errors (`IsError: true`) whose text lists `pointer: message` lines. Unexpected errors are logged and reported as "something went wrong (request <id>)".
- **Dates** in tool inputs are `YYYY-MM-DD`, interpreted in UTC, with inclusive ranges. Outputs use RFC 3339 UTC timestamps.

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **One user's token reading or writing another user's data.** Every tool with an id or slug argument must answer "not found" for another user's resources. Test: `TestToolsAreIsolatedPerUser` (Task 5).
2. **A revoked token, or one sent without the `Bearer` prefix,** must get 401 without running a tool. A revoked token must stop working immediately. Test: `TestMCPAuth` (Task 4).
3. **An AI "updating" the active version** through `save_plan_draft` with its version id must fail and leave the active version unchanged. Test: `TestSaveDraftNeverTouchesActiveVersions` (Task 3), plus the MCP-level check in `TestPlanTools` (Task 4).
4. **The token leaking after creation:** the tokens list and later page loads must never show it again, and the creation page must not be cacheable. Test: `TestTokensPage` (Task 2).
5. **A plan document sent as a JSON string instead of an object** (common with some clients) must validate and save the same way. Test: `TestPlanTools` (Task 4).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/store/schema.sql`, `internal/store/migrations/*_api_tokens.{up,down}.sql` | `api_tokens` table |
| `internal/store/api_tokens.go` (new), `api_tokens_test.go` | token rows |
| `internal/store/sessions.go`, `sessions_test.go` | `SearchSessions`, `SessionSummary.Slugs` |
| `internal/store/plans.go`, `plans_test.go` | `UpdateDraftVersion`, `ErrNotDraft` |
| `internal/auth/tokens.go` (new), `tokens_test.go` | `Tokens` service |
| `internal/web/tokens.go` (new), `views/tokens.templ` (new), `views/settings.templ`, `server.go`, `tokens_test.go` | `/settings/tokens` pages; `Server.BaseURL`, `Server.Tokens`, `Server.MCP` |
| `internal/plan/service.go`, `service_test.go` | `SaveDraft` |
| `internal/training/history.go`, `service.go`, `service_test.go` | `Sessions(filter)` |
| `internal/stats/stats.go`, `stats_test.go` | `Point.WeightKg/Reps/RPE` |
| `internal/mcp/server.go` (new) | server, auth, helpers |
| `internal/mcp/exercises.go`, `plans.go`, `training.go`, `prompts.go` (new) | tools and prompts |
| `internal/mcp/plan_format.md` (new, embedded) | prose guide to the plan format for `get_plan_schema` |
| `internal/mcp/*_test.go` (new) | go-sdk client tests over `httptest` |
| `cmd/onerep/main.go` | wiring |
| `README.md`, `AGENTS.md` | connecting an assistant; layout and rules |

---

### Task 1: API tokens in the store

**Files:**
- Modify: `internal/store/schema.sql`; generate the migration with `task migrate:diff NAME=api_tokens`
- Create: `internal/store/api_tokens.go`, `internal/store/api_tokens_test.go`

**Interfaces:**
- Consumes: `newID`, `formatTime`, `parseTime`, `parseNullTime`, `nullTime`, `mustAffect`, `ErrNotFound`, `db.read`/`db.write`, `userColumns`/`scanUser`. Test helpers `newTestDB`, `newUser`, `t0`.
- Produces:
  ```go
  type APIToken struct { ID, UserID, Name, Scope string; LastUsedAt *time.Time; CreatedAt time.Time; RevokedAt *time.Time }
  func (db *DB) CreateAPIToken(ctx context.Context, userID, name, hash string, at time.Time) (APIToken, error)
  func (db *DB) APITokens(ctx context.Context, userID string) ([]APIToken, error)              // newest first, revoked included
  func (db *DB) RevokeAPIToken(ctx context.Context, userID, id string, at time.Time) error     // ErrNotFound if not the user's; idempotent
  func (db *DB) UserByAPIToken(ctx context.Context, hash string, now time.Time) (User, error) // ErrNotFound if unknown or revoked; touches last_used_at
  ```

- [ ] **Step 1: Schema and migration**

Append to `internal/store/schema.sql`:

```sql
CREATE TABLE api_tokens (
  id           TEXT NOT NULL PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL, -- SHA-256 of the token; the token itself is never stored
  scope        TEXT NOT NULL DEFAULT 'mcp' CHECK (scope IN ('mcp')),
  last_used_at TEXT,
  created_at   TEXT NOT NULL,
  revoked_at   TEXT
);
CREATE UNIQUE INDEX api_tokens_hash ON api_tokens (token_hash);
CREATE INDEX api_tokens_user ON api_tokens (user_id);
```

Run: `nix develop --command task migrate:diff NAME=api_tokens && nix develop --command task migrate:check`
Expected: a new `*_api_tokens.up.sql`/`.down.sql` pair and an updated `atlas.sum`, and the check passes.

- [ ] **Step 2: Write the failing tests**

`internal/store/api_tokens_test.go`:

```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAPITokens(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")

	tok, err := db.CreateAPIToken(ctx, alice.ID, "laptop", "hash-1", t0)
	if err != nil || tok.Name != "laptop" || tok.Scope != "mcp" || tok.LastUsedAt != nil || !tok.CreatedAt.Equal(t0) {
		t.Fatalf("created %+v, %v", tok, err)
	}
	if _, err := db.CreateAPIToken(ctx, alice.ID, "second", "hash-2", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	u, err := db.UserByAPIToken(ctx, "hash-1", t0.Add(time.Minute))
	if err != nil || u.ID != alice.ID {
		t.Fatalf("user by token = %+v, %v", u, err)
	}
	list, _ := db.APITokens(ctx, alice.ID)
	if len(list) != 2 || list[0].Name != "second" || list[1].LastUsedAt == nil || !list[1].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("tokens = %+v", list)
	}
	// Uses within a minute of the last recorded one don't write.
	_, _ = db.UserByAPIToken(ctx, "hash-1", t0.Add(90*time.Second))
	list, _ = db.APITokens(ctx, alice.ID)
	if !list[1].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("last used moved within a minute: %v", list[1].LastUsedAt)
	}
	_, _ = db.UserByAPIToken(ctx, "hash-1", t0.Add(3*time.Minute))
	list, _ = db.APITokens(ctx, alice.ID)
	if !list[1].LastUsedAt.Equal(t0.Add(3 * time.Minute)) {
		t.Fatalf("last used = %v, want +3m", list[1].LastUsedAt)
	}

	if err := db.RevokeAPIToken(ctx, bob.ID, tok.ID, t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob revoking alice's token: %v", err)
	}
	if err := db.RevokeAPIToken(ctx, alice.ID, tok.ID, t0.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.RevokeAPIToken(ctx, alice.ID, tok.ID, t0.Add(5*time.Minute)); err != nil {
		t.Fatalf("revoking twice: %v", err)
	}
	if _, err := db.UserByAPIToken(ctx, "hash-1", t0.Add(6*time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked token still works: %v", err)
	}
	if _, err := db.UserByAPIToken(ctx, "nope", t0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown token: %v", err)
	}
	list, _ = db.APITokens(ctx, alice.ID)
	if list[1].RevokedAt == nil || !list[1].RevokedAt.Equal(t0.Add(4*time.Minute)) {
		t.Fatalf("revoked_at = %v, want the first revocation", list[1].RevokedAt)
	}
	if other, _ := db.APITokens(ctx, bob.ID); len(other) != 0 {
		t.Fatalf("bob sees %d tokens", len(other))
	}
}
```

Run: `nix develop --command go test ./internal/store/ -run APITokens`
Expected: FAIL to compile: `db.CreateAPIToken undefined`.

- [ ] **Step 3: Implement**

`internal/store/api_tokens.go`:

```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// APIToken is a personal access token for the MCP server (spec §11). Only
// its hash is stored.
type APIToken struct {
	ID         string
	UserID     string
	Name       string
	Scope      string
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

func (db *DB) CreateAPIToken(ctx context.Context, userID, name, hash string, at time.Time) (APIToken, error) {
	id, err := newID()
	if err != nil {
		return APIToken{}, err
	}
	t := APIToken{ID: id, UserID: userID, Name: name, Scope: "mcp", CreatedAt: at.UTC()}
	_, err = db.write.ExecContext(ctx, `INSERT INTO api_tokens (id, user_id, name, token_hash, scope, created_at)
		VALUES (?, ?, ?, ?, 'mcp', ?)`, id, userID, name, hash, formatTime(at))
	return t, err
}

// APITokens lists the user's tokens, newest first, revoked ones included.
func (db *DB) APITokens(ctx context.Context, userID string) ([]APIToken, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT id, user_id, name, scope, last_used_at, created_at, revoked_at
		FROM api_tokens WHERE user_id = ? ORDER BY created_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIToken
	for rows.Next() {
		var t APIToken
		var used, revoked sql.NullString
		var created string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.Scope, &used, &created, &revoked); err != nil {
			return nil, err
		}
		if t.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if t.LastUsedAt, err = parseNullTime(used); err != nil {
			return nil, err
		}
		if t.RevokedAt, err = parseNullTime(revoked); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeAPIToken revokes one of the user's tokens. Revoking twice keeps the
// first revocation time.
func (db *DB) RevokeAPIToken(ctx context.Context, userID, id string, at time.Time) error {
	res, err := db.write.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = coalesce(revoked_at, ?)
		WHERE id = ? AND user_id = ?`, formatTime(at), id, userID)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

// UserByAPIToken returns the owner of an unrevoked token and records the use
// (at most once a minute, to keep MCP calls from writing on every request).
func (db *DB) UserByAPIToken(ctx context.Context, hash string, now time.Time) (User, error) {
	var tokenID string
	var used sql.NullString
	err := db.read.QueryRowContext(ctx, `SELECT id, last_used_at FROM api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL`, hash).Scan(&tokenID, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	u, err := db.UserByTokenID(ctx, tokenID)
	if err != nil {
		return User{}, err
	}
	last, err := parseNullTime(used)
	if err != nil {
		return User{}, err
	}
	if last == nil || now.Sub(*last) >= time.Minute {
		if _, err := db.write.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, formatTime(now), tokenID); err != nil {
			return User{}, err
		}
	}
	return u, nil
}

// UserByTokenID returns the owner of a token.
func (db *DB) UserByTokenID(ctx context.Context, tokenID string) (User, error) {
	u, err := scanUser(db.read.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users
		WHERE id = (SELECT user_id FROM api_tokens WHERE id = ?)`, tokenID))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}
```

Check the helper names first: `grep -n "func parseNullTime\|func mustAffect\|userColumns\|func scanUser" internal/store/*.go`. If `userColumns` in `users.go` is written without table qualifiers, the subquery form above works unchanged.

Run: `nix develop --command go test ./internal/store/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/store
git commit -m "feat(store): add API tokens"
```

---

### Task 2: Token service and the tokens page

**Files:**
- Create: `internal/auth/tokens.go`, `internal/auth/tokens_test.go`
- Create: `internal/web/tokens.go`, `internal/web/views/tokens.templ`, `internal/web/tokens_test.go`
- Modify: `internal/web/server.go` (`Server.BaseURL`, `Server.Tokens`, the routes), `internal/web/server_test.go` (`newAppDB`), `internal/web/views/settings.templ` (link), `cmd/onerep/main.go`

**Interfaces:**
- Consumes (Task 1): the four store methods. `randomToken()` and `HashToken(string) string` from `internal/auth/sessions.go`.
- Produces:
  ```go
  const TokenPrefix = "onerep_"
  var ErrTokenName = errors.New("give the token a name of at most 100 characters")
  type TokenStore interface { CreateAPIToken(...); APITokens(...); RevokeAPIToken(...); UserByAPIToken(...) } // Task 1 signatures
  type Tokens struct { Store TokenStore; Now func() time.Time }
  func (t *Tokens) Create(ctx context.Context, userID, name string) (secret string, tok store.APIToken, err error)
  func (t *Tokens) List(ctx context.Context, userID string) ([]store.APIToken, error)
  func (t *Tokens) Revoke(ctx context.Context, userID, id string) error
  func (t *Tokens) Verify(ctx context.Context, secret string) (store.User, error) // store.ErrNotFound for unknown/revoked/malformed
  ```
  - `web.Server.BaseURL string`, `web.Server.Tokens *auth.Tokens`
  - `views.TokensPage(p Page, d TokensData)` with `views.TokensData{Tokens []store.APIToken; Secret, MCPURL, Error string}`

- [ ] **Step 1: Write the failing service test**

`internal/auth/tokens_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

func TestTokens(t *testing.T) {
	db := storetest.New(t)
	ctx := context.Background()
	u, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	tokens := &Tokens{Store: db}

	if _, _, err := tokens.Create(ctx, u.ID, "   "); !errors.Is(err, ErrTokenName) {
		t.Fatalf("blank name: %v", err)
	}
	if _, _, err := tokens.Create(ctx, u.ID, strings.Repeat("x", 101)); !errors.Is(err, ErrTokenName) {
		t.Fatalf("long name: %v", err)
	}
	secret, tok, err := tokens.Create(ctx, u.ID, "  Claude  ")
	if err != nil || tok.Name != "Claude" || !strings.HasPrefix(secret, TokenPrefix) || len(secret) != len(TokenPrefix)+43 {
		t.Fatalf("created %q %+v %v", secret, tok, err)
	}
	got, err := tokens.Verify(ctx, secret)
	if err != nil || got.ID != u.ID {
		t.Fatalf("verify = %+v, %v", got, err)
	}
	for _, bad := range []string{"", "onerep_", "not-a-token", secret + "x"} {
		if _, err := tokens.Verify(ctx, bad); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("verify(%q) = %v, want not found", bad, err)
		}
	}
	if err := tokens.Revoke(ctx, u.ID, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Verify(ctx, secret); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("revoked token verifies: %v", err)
	}
}
```

Run: `nix develop --command go test ./internal/auth/ -run TestTokens`
Expected: FAIL to compile: `undefined: Tokens`.

- [ ] **Step 2: Implement the service**

`internal/auth/tokens.go`:

```go
package auth

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LongerHV/onerep/internal/store"
)

// TokenPrefix starts every API token, so leaked tokens are recognisable.
const TokenPrefix = "onerep_"

// ErrTokenName rejects a blank or overlong token name.
var ErrTokenName = errors.New("give the token a name of at most 100 characters")

// TokenStore is the persistence API tokens need. *store.DB implements it.
type TokenStore interface {
	CreateAPIToken(ctx context.Context, userID, name, hash string, at time.Time) (store.APIToken, error)
	APITokens(ctx context.Context, userID string) ([]store.APIToken, error)
	RevokeAPIToken(ctx context.Context, userID, id string, at time.Time) error
	UserByAPIToken(ctx context.Context, hash string, now time.Time) (store.User, error)
}

// Tokens manages personal access tokens for the MCP server (spec §11).
type Tokens struct {
	Store TokenStore
	Now   func() time.Time // defaults to time.Now
}

func (t *Tokens) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Create makes a token and returns its secret, which is never stored and
// can't be shown again.
func (t *Tokens) Create(ctx context.Context, userID, name string) (string, store.APIToken, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", store.APIToken{}, ErrTokenName
	}
	random, err := randomToken()
	if err != nil {
		return "", store.APIToken{}, err
	}
	secret := TokenPrefix + random
	tok, err := t.Store.CreateAPIToken(ctx, userID, name, HashToken(secret), t.now())
	return secret, tok, err
}

func (t *Tokens) List(ctx context.Context, userID string) ([]store.APIToken, error) {
	return t.Store.APITokens(ctx, userID)
}

func (t *Tokens) Revoke(ctx context.Context, userID, id string) error {
	return t.Store.RevokeAPIToken(ctx, userID, id, t.now())
}

// Verify returns the user of an unrevoked token, or store.ErrNotFound.
func (t *Tokens) Verify(ctx context.Context, secret string) (store.User, error) {
	if !strings.HasPrefix(secret, TokenPrefix) || len(secret) <= len(TokenPrefix) {
		return store.User{}, store.ErrNotFound
	}
	return t.Store.UserByAPIToken(ctx, HashToken(secret), t.now())
}
```

Run: `nix develop --command go test ./internal/auth/`
Expected: PASS.

- [ ] **Step 3: Write the failing page test**

`internal/web/tokens_test.go`:

```go
package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var secretRe = regexp.MustCompile(`onerep_[A-Za-z0-9_-]{43}`)

func TestTokensPage(t *testing.T) {
	srv, c := newApp(t, "alice")
	page := read(t, mustGet(t, c, srv.URL+"/settings/tokens"))
	csrf := csrfInput.FindStringSubmatch(page)[1]
	if !strings.Contains(read(t, mustGet(t, c, srv.URL+"/settings")), `href="/settings/tokens"`) {
		t.Error("settings page doesn't link to the tokens page")
	}

	resp, err := c.PostForm(srv.URL+"/settings/tokens", url.Values{"name": {"Claude"}, "csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	created := read(t, resp)
	secret := secretRe.FindString(created)
	if resp.StatusCode != http.StatusOK || secret == "" {
		t.Fatalf("create = %d, secret %q", resp.StatusCode, secret)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || !strings.Contains(created, `hx-history="false"`) {
		t.Error("the page showing a token must not be cached by the browser or htmx")
	}
	if !strings.Contains(created, "http://example.test/mcp") || !strings.Contains(created, "Bearer "+secret) {
		t.Error("the page should show how to connect an assistant")
	}

	list := read(t, mustGet(t, c, srv.URL+"/settings/tokens"))
	if secretRe.MatchString(list) || !strings.Contains(list, "Claude") {
		t.Fatal("the token list must name the token but never show its secret again")
	}
	if resp, _ := c.PostForm(srv.URL+"/settings/tokens", url.Values{"name": {" "}, "csrf_token": {csrf}}); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("blank name = %d", resp.StatusCode)
	}

	id := regexp.MustCompile(`/settings/tokens/([0-9a-f-]{36})/revoke`).FindStringSubmatch(list)[1]
	resp, _ = c.PostForm(srv.URL+"/settings/tokens/"+id+"/revoke", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("revoke = %d", resp.StatusCode)
	}
	if !strings.Contains(read(t, mustGet(t, c, srv.URL+"/settings/tokens")), "revoked") {
		t.Error("a revoked token should say so")
	}
	resp, _ = c.PostForm(srv.URL+"/settings/tokens/00000000-0000-7000-8000-000000000000/revoke", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("revoking an unknown token = %d", resp.StatusCode)
	}
}
```

In `newAppDB` (`server_test.go`), set `BaseURL: "http://example.test"` and `Tokens: &auth.Tokens{Store: db}` on the `Server`.

Run: `nix develop --command go test ./internal/web/ -run TokensPage`
Expected: FAIL: `/settings/tokens` is a 404 (or the test doesn't compile until the `Server` fields exist).

- [ ] **Step 4: Implement the pages**

`internal/web/server.go`: add fields `BaseURL string` and `Tokens *auth.Tokens`. In the app route group, after `r.Post("/settings", s.saveSettings)`, add `s.tokenRoutes(r)`.

`internal/web/tokens.go`:

```go
package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) tokenRoutes(r chi.Router) {
	r.Get("/settings/tokens", s.tokensPage)
	r.Post("/settings/tokens", s.tokenCreate)
	r.Post("/settings/tokens/{id}/revoke", s.tokenRevoke)
}

func (s *Server) tokensData(r *http.Request) (views.TokensData, error) {
	list, err := s.Tokens.List(r.Context(), user(r).ID)
	return views.TokensData{Tokens: list, MCPURL: strings.TrimSuffix(s.BaseURL, "/") + "/mcp"}, err
}

func (s *Server) tokensPage(w http.ResponseWriter, r *http.Request) {
	d, err := s.tokensData(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.TokensPage(page(r, "API tokens"), d))
}

// tokenCreate shows the new token once. The response is never cached, and
// hx-history="false" keeps htmx from saving the page in its history cache.
func (s *Server) tokenCreate(w http.ResponseWriter, r *http.Request) {
	secret, _, err := s.Tokens.Create(r.Context(), user(r).ID, r.PostFormValue("name"))
	d, derr := s.tokensData(r)
	switch {
	case derr != nil:
		s.fail(w, r, derr)
	case errors.Is(err, auth.ErrTokenName):
		d.Error = err.Error()
		render(w, r, http.StatusUnprocessableEntity, views.TokensPage(page(r, "API tokens"), d))
	case err != nil:
		s.fail(w, r, err)
	default:
		d.Secret = secret
		w.Header().Set("Cache-Control", "no-store")
		render(w, r, http.StatusOK, views.TokensPage(page(r, "API tokens"), d))
	}
}

func (s *Server) tokenRevoke(w http.ResponseWriter, r *http.Request) {
	if err := s.Tokens.Revoke(r.Context(), user(r).ID, chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings/tokens", http.StatusSeeOther)
}
```

(Check that the layout middleware keeps a handler's `Cache-Control` header when it wraps the page. If it builds a new response, copy the header across in `layout.go`, and add a line to `TestTokensPage` for an htmx request.)

`internal/web/views/models.go`: add

```go
// TokensData is the API tokens page. Secret is set only right after creation.
type TokensData struct {
	Tokens []store.APIToken
	Secret string
	MCPURL string
	Error  string
}
```

`internal/web/views/tokens.templ`:

```templ
package views

templ TokensPage(p Page, d TokensData) {
	<h1 class={ h1 }>API tokens</h1>
	<p class={ hint }>
		A token lets an AI assistant use onerep as you through MCP: it can read your training and stats, manage exercises and training maxes, and write plan drafts for you to review. It can't change logged workouts or activate plans.
	</p>
	if d.Secret != "" {
		<div hx-history="false" class={ card + " mt-4 border-amber-300 dark:border-amber-800" }>
			<p class="font-medium">Copy your token now. It won't be shown again.</p>
			<pre class="mt-2 overflow-x-auto rounded bg-zinc-100 p-2 text-sm dark:bg-zinc-900"><code>{ d.Secret }</code></pre>
			<p class="mt-3 text-sm">MCP server URL: <code>{ d.MCPURL }</code>, with the header <code>Authorization: Bearer …</code>. For example, in Claude Code:</p>
			<pre class="mt-2 overflow-x-auto rounded bg-zinc-100 p-2 text-sm dark:bg-zinc-900"><code>{ "claude mcp add --transport http onerep " + d.MCPURL + " --header \"Authorization: Bearer " + d.Secret + "\"" }</code></pre>
		</div>
	}
	<form method="post" action="/settings/tokens" class="mt-4 flex flex-wrap items-end gap-2">
		@CSRF(p)
		<label class={ label }>
			Name
			<input type="text" name="name" maxlength="100" required placeholder="e.g. Claude on my laptop" class={ input }/>
		</label>
		<button type="submit" class={ btn }>Create token</button>
	</form>
	if d.Error != "" {
		<p class={ errorText }>{ d.Error }</p>
	}
	if len(d.Tokens) > 0 {
		<table class="mt-6 w-full text-sm">
			<thead class="text-left text-zinc-500">
				<tr><th class="py-1">Name</th><th>Created</th><th>Last used</th><th></th></tr>
			</thead>
			<tbody>
				for _, t := range d.Tokens {
					<tr class="border-t border-zinc-200 dark:border-zinc-800">
						<td class="py-1">{ t.Name }</td>
						<td>{ t.CreatedAt.Format("2006-01-02") }</td>
						<td>
							if t.LastUsedAt != nil {
								{ t.LastUsedAt.Format("2006-01-02 15:04") }
							} else {
								never
							}
						</td>
						<td class="text-right">
							if t.RevokedAt != nil {
								<span class="text-zinc-500">revoked</span>
							} else {
								<form method="post" action={ templ.URL("/settings/tokens/" + t.ID + "/revoke") }>
									@CSRF(p)
									<button type="submit" class="text-sm text-red-700 hover:underline dark:text-red-400">Revoke</button>
								</form>
							}
						</td>
					</tr>
				}
			</tbody>
		</table>
	}
}
```

`internal/web/views/settings.templ`: after the settings form, add

```templ
	<h2 class={ h2 }>AI assistant</h2>
	<p class="mt-2 text-sm"><a href="/settings/tokens" class="underline">Manage API tokens</a> to connect an assistant through MCP.</p>
```

`cmd/onerep/main.go`: set `BaseURL: cfg.BaseURL` and `Tokens: &auth.Tokens{Store: db}` on the server.

Run: `nix develop --command bash -c 'task generate && go test ./internal/web/ ./internal/auth/ ./cmd/...'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth internal/web cmd/onerep/main.go
git commit -m "feat(auth): create, list and revoke API tokens at /settings/tokens"
```

---

### Task 3: Service support for the tools

**Files:**
- Modify: `internal/store/plans.go`, `plans_test.go` (`UpdateDraftVersion`, `ErrNotDraft`)
- Modify: `internal/store/sessions.go`, `sessions_test.go` (`SessionFilter`, `SearchSessions`, `SessionSummary.Slugs`)
- Modify: `internal/plan/service.go`, `service_test.go` (`SaveDraft`)
- Modify: `internal/training/service.go` (Store interface), `history.go`, `service_test.go` (`Sessions`)
- Modify: `internal/stats/stats.go`, `stats_test.go` (`Point` fields)

**Interfaces:**
- Produces:
  ```go
  // store
  var ErrNotDraft = errors.New("only draft versions can be changed")
  func (db *DB) UpdateDraftVersion(ctx context.Context, userID, versionID string, doc []byte, source, note string) (PlanVersion, error)
  type SessionFilter struct { From, To time.Time /* started_at in [From, To); zero = unbounded */; Slug string; Limit int }
  func (db *DB) SearchSessions(ctx context.Context, userID string, f SessionFilter) ([]SessionSummary, error) // newest first; Slugs filled
  // SessionSummary gains: Slugs []string // exercises done, in order (SearchSessions only)
  // plan
  type DraftInput struct { PlanID, VersionID string; Doc []byte; Source, Note string }
  type Draft struct { Plan store.Plan; Version store.PlanVersion; Warnings Problems }
  func (s *Service) SaveDraft(ctx context.Context, user store.User, in DraftInput) (Draft, error)
  // training
  func (s *Service) Sessions(ctx context.Context, user store.User, f store.SessionFilter) ([]store.SessionSummary, error) // Limit defaults to 20, capped at 100
  // stats: Point gains WeightKg float64; Reps int; RPE *float64
  ```

- [ ] **Step 1: Write the failing tests**

Append to `internal/store/plans_test.go`, using its existing helpers to make a user and a plan. Read the top of the file first for their names.

```go
func TestUpdateDraftVersion(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u, other := newUser(t, db, "u"), newUser(t, db, "other")
	p, active, err := db.CreatePlan(ctx, u.ID, "P", []byte(`{"v":1}`), PlanActive, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := db.SavePlanVersion(ctx, u.ID, p.ID, "P", []byte(`{"v":2}`), PlanDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.UpdateDraftVersion(ctx, u.ID, draft.ID, []byte(`{"v":3}`), "mcp", "tweaked")
	if err != nil || string(got.Doc) != `{"v":3}` || got.Source != "mcp" || got.Note != "tweaked" || got.Version != draft.Version || got.Status != PlanDraft {
		t.Fatalf("updated = %+v, %v", got, err)
	}
	if _, err := db.UpdateDraftVersion(ctx, u.ID, active.ID, []byte(`{}`), "mcp", ""); !errors.Is(err, ErrNotDraft) {
		t.Fatalf("updating the active version: %v", err)
	}
	if v, _ := db.PlanVersionByID(ctx, u.ID, active.ID); string(v.Doc) != `{"v":1}` {
		t.Fatalf("active version changed: %s", v.Doc)
	}
	if _, err := db.UpdateDraftVersion(ctx, other.ID, draft.ID, []byte(`{}`), "mcp", ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's draft: %v", err)
	}
}
```

Append to `internal/store/sessions_test.go`:

```go
func TestSearchSessions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	a := newSession(t, db, u.ID, "a")
	_, _ = db.UpsertSet(ctx, u.ID, set(a.ID, "s1", 100, t0), "") // squat
	curl := set(a.ID, "s2", 12, t0.Add(time.Minute))
	curl.Slug, curl.GroupPos = "dumbbell-curl", 1
	_, _ = db.UpsertSet(ctx, u.ID, curl, "")
	b := newSession(t, db, u.ID, "b")
	_, _ = db.UpsertSet(ctx, u.ID, set(b.ID, "s3", 105, t0), "")
	_, _ = db.DeleteSet(ctx, u.ID, b.ID, "s3", t0.Add(time.Minute), "")
	other := newUser(t, db, "other")
	newSession(t, db, other.ID, "theirs")

	all, err := db.SearchSessions(ctx, u.ID, SessionFilter{Limit: 10})
	if err != nil || len(all) != 2 || all[0].Name != "b" {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if got := all[1].Slugs; len(got) != 2 || got[0] != "barbell-back-squat" || got[1] != "dumbbell-curl" || all[1].Sets != 2 {
		t.Fatalf("session a summary = %+v", all[1])
	}
	squat, _ := db.SearchSessions(ctx, u.ID, SessionFilter{Slug: "barbell-back-squat", Limit: 10})
	if len(squat) != 1 || squat[0].Name != "a" {
		t.Fatalf("squat sessions = %+v (deleted sets must not match)", squat)
	}
	future, _ := db.SearchSessions(ctx, u.ID, SessionFilter{From: time.Now().Add(time.Hour), Limit: 10})
	past, _ := db.SearchSessions(ctx, u.ID, SessionFilter{To: time.Now().Add(-time.Hour), Limit: 10})
	if len(future) != 0 || len(past) != 0 {
		t.Fatalf("date filters: future %d, past %d", len(future), len(past))
	}
	one, _ := db.SearchSessions(ctx, u.ID, SessionFilter{Limit: 1})
	if len(one) != 1 {
		t.Fatalf("limit: %d", len(one))
	}
}
```

Append to `internal/plan/service_test.go`, using its existing environment helper (read the file's setup first; it creates a service with a seeded catalog and a user):

```go
func TestSaveDraftNeverTouchesActiveVersions(t *testing.T) {
	// env setup as in the file's other tests: svc *Service, user store.User
	doc := func(name string) []byte {
		return []byte(`{"name":"` + name + `","weeks":1,"days":[{"name":"D","groups":[{"exercises":[{"slug":"barbell-back-squat","sets":[{"count":3,"reps":5}]}]}]}]}`)
	}
	d, err := svc.SaveDraft(ctx, user, DraftInput{Doc: doc("New"), Source: "mcp"})
	if err != nil || d.Version.Status != store.PlanDraft || d.Version.Source != "mcp" || d.Plan.Name != "New" {
		t.Fatalf("new plan draft = %+v, %v", d, err)
	}
	d2, err := svc.SaveDraft(ctx, user, DraftInput{PlanID: d.Plan.ID, Doc: doc("New v2"), Source: "mcp", Note: "heavier"})
	if err != nil || d2.Version.Version != 2 || d2.Version.Status != store.PlanDraft {
		t.Fatalf("second draft = %+v, %v", d2, err)
	}
	d3, err := svc.SaveDraft(ctx, user, DraftInput{VersionID: d2.Version.ID, Doc: doc("New v2b"), Source: "mcp"})
	if err != nil || d3.Version.ID != d2.Version.ID || !strings.Contains(string(d3.Version.Doc), "New v2b") {
		t.Fatalf("replaced draft = %+v, %v", d3, err)
	}
	if _, err := svc.Activate(ctx, user, d3.Version.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SaveDraft(ctx, user, DraftInput{VersionID: d3.Version.ID, Doc: doc("Sneaky"), Source: "mcp"}); !errors.Is(err, store.ErrNotDraft) {
		t.Fatalf("replacing the active version: %v", err)
	}
	var ps Problems
	if _, err := svc.SaveDraft(ctx, user, DraftInput{Doc: []byte(`{"name":"x"}`), Source: "mcp"}); !errors.As(err, &ps) || !ps.HasErrors() {
		t.Fatalf("invalid doc: %v", err)
	}
}
```

Append to `internal/training/service_test.go`:

```go
func TestSessionsFilter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	s, _ := e.svc.StartAdHoc(ctx, e.alice)
	_, _ = e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-00000000002a", s.ID, "barbell-back-squat", 100, 5, 8, time.Now().UTC())})
	got, err := e.svc.Sessions(ctx, e.alice, store.SessionFilter{})
	if err != nil || len(got) != 1 || len(got[0].Slugs) != 1 {
		t.Fatalf("sessions = %+v, %v", got, err)
	}
	if none, _ := e.svc.Sessions(ctx, e.bob, store.SessionFilter{}); len(none) != 0 {
		t.Fatalf("bob sees %d sessions", len(none))
	}
}
```

In `internal/stats/stats_test.go`'s `TestExerciseStats`, add:

```go
	if p := st.Series[0]; p.WeightKg != 100 || p.Reps != 5 || p.RPE == nil || *p.RPE != 8 {
		t.Fatalf("best set of the point = %+v", p)
	}
```

Run: `nix develop --command go test ./internal/store/ ./internal/plan/ ./internal/training/ ./internal/stats/`
Expected: FAIL to compile on the new names.

- [ ] **Step 2: Implement**

`internal/store/plans.go`:

```go
// ErrNotDraft is returned when changing a version that isn't a draft.
var ErrNotDraft = errors.New("only draft versions can be changed")

// UpdateDraftVersion replaces a draft's document (spec §10: drafts are
// mutable, active and superseded versions are not).
func (db *DB) UpdateDraftVersion(ctx context.Context, userID, versionID string, doc []byte, source, note string) (PlanVersion, error) {
	v, err := db.PlanVersionByID(ctx, userID, versionID)
	if err != nil {
		return PlanVersion{}, err
	}
	if v.Status != PlanDraft {
		return PlanVersion{}, ErrNotDraft
	}
	res, err := db.write.ExecContext(ctx, `UPDATE plan_versions SET doc = ?, source = ?, note = ?
		WHERE id = ? AND status = 'draft'`, string(doc), source, note, versionID)
	if err != nil {
		return PlanVersion{}, err
	}
	if err := mustAffect(res); err != nil {
		return PlanVersion{}, ErrNotDraft // activated in between
	}
	v.Doc, v.Source, v.Note = doc, source, note
	return v, nil
}
```

(Match how `insertVersion` binds `doc`: `[]byte` or `string`. Use the same form so the stored type doesn't change.)

`internal/store/sessions.go`: add `Slugs []string` to `SessionSummary` with the comment `// exercises done, in order (SearchSessions only)`, and:

```go
// SessionFilter narrows SearchSessions. Zero times are unbounded.
type SessionFilter struct {
	From, To time.Time // started_at in [From, To)
	Slug     string    // only sessions with a (non-deleted) set of this exercise
	Limit    int
}

// SearchSessions returns the user's sessions matching f, newest first, with
// the exercises each one included.
func (db *DB) SearchSessions(ctx context.Context, userID string, f SessionFilter) ([]SessionSummary, error) {
	from, to := "", "9999"
	if !f.From.IsZero() {
		from = formatTime(f.From)
	}
	if !f.To.IsZero() {
		to = formatTime(f.To)
	}
	rows, err := db.read.QueryContext(ctx, `SELECT `+sessionColumns+`,
		(SELECT count(*) FROM sets WHERE sets.session_id = sessions.id AND deleted_at IS NULL),
		coalesce((SELECT group_concat(slug, ',') FROM (SELECT slug FROM sets
			WHERE sets.session_id = sessions.id AND deleted_at IS NULL
			GROUP BY slug ORDER BY min(group_pos), min(exercise_pos), min(done_at))), '')
		FROM sessions WHERE user_id = ? AND started_at >= ? AND started_at < ?
			AND (? = '' OR EXISTS (SELECT 1 FROM sets WHERE sets.session_id = sessions.id AND slug = ? AND deleted_at IS NULL))
		ORDER BY started_at DESC, id DESC LIMIT ?`, userID, from, to, f.Slug, f.Slug, f.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var sum SessionSummary
		var snapshot, started, updated, slugs string
		var finished sql.NullString
		if err := rows.Scan(&sum.ID, &sum.UserID, &sum.PlanID, &sum.PlanVersionID, &sum.Week, &sum.Day, &sum.Name,
			&snapshot, &started, &finished, &sum.Notes, &updated, &sum.Sets, &slugs); err != nil {
			return nil, err
		}
		sum.Snapshot = []byte(snapshot)
		if sum.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}
		if sum.FinishedAt, err = parseNullTime(finished); err != nil {
			return nil, err
		}
		if sum.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		if slugs != "" {
			sum.Slugs = strings.Split(slugs, ",")
		}
		out = append(out, sum)
	}
	return out, rows.Err()
}
```

(Slugs match `^[a-z0-9-]+$`, so a comma separator is safe. The scan list mirrors `ListSessions`. If the columns differ, copy that function's scan and add `&slugs`.)

`internal/plan/service.go`:

```go
// DraftInput is a plan document saved as a draft: a new plan (no ids), a new
// version of PlanID, or a replacement for the draft VersionID.
type DraftInput struct {
	PlanID, VersionID string
	Doc               []byte
	Source, Note      string
}

// Draft is a saved draft and the warnings its document produced.
type Draft struct {
	Plan     store.Plan
	Version  store.PlanVersion
	Warnings Problems
}

// SaveDraft stores a draft without ever activating it (spec §10, used by MCP).
// Invalid documents return Problems; replacing a non-draft returns store.ErrNotDraft.
func (s *Service) SaveDraft(ctx context.Context, user store.User, in DraftInput) (Draft, error) {
	doc, ps := s.Validate(ctx, user, in.Doc)
	if ps.HasErrors() {
		return Draft{}, ps
	}
	raw := pinUnit(in.Doc, doc, user.Unit)
	var d Draft
	var err error
	switch {
	case in.VersionID != "":
		d.Version, err = s.Store.UpdateDraftVersion(ctx, user.ID, in.VersionID, raw, in.Source, in.Note)
		if err == nil {
			d.Plan, err = s.Store.PlanByID(ctx, user.ID, d.Version.PlanID)
		}
	case in.PlanID != "":
		d.Version, err = s.Store.SavePlanVersion(ctx, user.ID, in.PlanID, doc.Name, raw, SaveDraft, in.Source, in.Note)
		if err == nil {
			d.Plan, err = s.Store.PlanByID(ctx, user.ID, in.PlanID)
		}
	default:
		d.Plan, d.Version, err = s.Store.CreatePlan(ctx, user.ID, doc.Name, raw, SaveDraft, in.Source, in.Note)
	}
	if err != nil {
		return Draft{}, err
	}
	for _, p := range ps {
		if p.Warning {
			d.Warnings = append(d.Warnings, p)
		}
	}
	return d, nil
}
```

Add `UpdateDraftVersion(ctx context.Context, userID, versionID string, doc []byte, source, note string) (store.PlanVersion, error)` to `plan.Store`.

`internal/training/service.go` `Store`: add `SearchSessions(ctx context.Context, userID string, f store.SessionFilter) ([]store.SessionSummary, error)`. `internal/training/history.go`:

```go
// Sessions lists the user's sessions matching f, newest first (limit 20 by default, at most 100).
func (s *Service) Sessions(ctx context.Context, user store.User, f store.SessionFilter) ([]store.SessionSummary, error) {
	if f.Limit <= 0 {
		f.Limit = 20
	}
	f.Limit = min(f.Limit, 100)
	return s.Store.SearchSessions(ctx, user.ID, f)
}
```

`internal/stats/stats.go` `Point`: add `WeightKg float64`, `Reps int` and `RPE *float64`, all set in `ExerciseStats` from the store point.

Run: `nix develop --command go test ./internal/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal
git commit -m "feat(plan): save MCP drafts; search sessions by date and exercise"
```

---

### Task 4: The MCP server, auth, and exercise and plan tools

**Files:**
- `go.mod`/`go.sum`: `go get github.com/modelcontextprotocol/go-sdk@v1.8.0`
- Create: `internal/mcp/server.go`, `internal/mcp/exercises.go`, `internal/mcp/plans.go`, `internal/mcp/plan_format.md`, `internal/mcp/server_test.go`, `internal/mcp/plans_test.go`
- Modify: `internal/web/server.go` (`Server.MCP http.Handler`, mounted at `/mcp`), `cmd/onerep/main.go`

**Interfaces:**
- Consumes: `auth.Tokens.Verify`, `store.DB.UserByID`, `exercise.Service` (`Catalog`, `Get`, `Create`, `Settings`, `SetTrainingMax`), `exercise.Muscles`, `exercise.Input`, `plan.Service` (`Plans`, `Plan`, `Version`, `Next`, `Validate`, `Preview`, `SaveDraft`, `Decode`), `plan.Schema()`, `plan.DayLines(day, unit)`, `stats.Service`, `training.Service`.
- Produces:
  ```go
  package mcp // imports sdk "github.com/modelcontextprotocol/go-sdk/mcp", "github.com/modelcontextprotocol/go-sdk/auth"
  type Server struct {
      Users     interface{ UserByID(ctx context.Context, id string) (store.User, error) }
      Tokens    interface{ Verify(ctx context.Context, secret string) (store.User, error) }
      Exercises *exercise.Service; Plans *plan.Service; Training *training.Service; Stats *stats.Service
      BaseURL   string
  }
  func (s *Server) Handler() http.Handler // bearer auth + stateless Streamable HTTP
  ```
  `web.Server.MCP http.Handler`, mounted with `r.Handle("/mcp", http.MaxBytesHandler(s.MCP, maxBodyBytes))` when non-nil.

- [ ] **Step 1: Add the module and check licences**

Run: `nix develop --command bash -c 'go get github.com/modelcontextprotocol/go-sdk@v1.8.0 && go mod tidy'` (tidy keeps it once the code imports it). Run `task licenses:check` after Step 4.
Expected: passes. If `go-licenses` can't classify the SDK's transitional MIT → Apache-2.0 `LICENSE`, add `--ignore github.com/modelcontextprotocol/go-sdk` to the task with a comment: "MIT/Apache-2.0 per its LICENSE (transitional text go-licenses can't classify); checked by hand at v1.8.0". Record it as a ruling.

- [ ] **Step 2: Write the failing tests**

`internal/mcp/server_test.go`:

```go
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
```

`internal/mcp/plans_test.go`:

```go
package mcp

import (
	"context"
	"strings"
	"testing"

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
```

Add a helper to `server_test.go`: `func (e env) Plans() *plan.Service` that returns `&plan.Service{Store: e.db, Exercises: &exercise.Service{Store: e.db}, History: e.db}`.

Run: `nix develop --command go test ./internal/mcp/`
Expected: FAIL to compile: `undefined: Server`.

- [ ] **Step 3: Implement the server and tools**

`internal/mcp/server.go`:

```go
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
	Users     interface{ UserByID(ctx context.Context, id string) (store.User, error) }
	Tokens    interface{ Verify(ctx context.Context, secret string) (store.User, error) }
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
		&sdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
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
```

(Before relying on `req.Extra.TokenInfo`, confirm that the Streamable handler fills it from the request context in stateless mode: `grep -n "TokenInfo" $(go list -m -f '{{.Dir}}' github.com/modelcontextprotocol/go-sdk)/mcp/streamable.go`. If it doesn't, put the user ID into the context in `verify`'s wrapper instead, and read it from `ctx`. Record a ruling either way.)

`internal/mcp/exercises.go`:

```go
package mcp

import (
	"context"
	"slices"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

type exerciseOut struct {
	Slug             string   `json:"slug"`
	Name             string   `json:"name"`
	Measurement      string   `json:"measurement"`
	EquipmentKind    string   `json:"equipment_kind"`
	PrimaryMuscles   []string `json:"primary_muscles"`
	SecondaryMuscles []string `json:"secondary_muscles"`
	Aliases          []string `json:"aliases"`
	Custom           bool     `json:"custom"`
}

func exerciseOf(e store.Exercise) exerciseOut {
	return exerciseOut{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind,
		PrimaryMuscles: e.PrimaryMuscles, SecondaryMuscles: e.SecondaryMuscles, Aliases: e.Aliases, Custom: e.Custom()}
}

type listExercisesIn struct {
	Query  string `json:"query,omitempty" jsonschema:"words that must all appear in the slug, name, aliases, equipment or muscles"`
	Muscle string `json:"muscle,omitempty" jsonschema:"only exercises that work this muscle (primary or secondary), from the muscles list"`
}

type listExercisesOut struct {
	Exercises []exerciseOut `json:"exercises"`
	Muscles   []string      `json:"muscles" jsonschema:"the muscle vocabulary"`
}

type createExerciseIn struct {
	Slug             string   `json:"slug" jsonschema:"permanent id: lowercase letters, digits and dashes"`
	Name             string   `json:"name"`
	Measurement      string   `json:"measurement" jsonschema:"weight_reps, bw_reps, reps, time or distance_time"`
	EquipmentKind    string   `json:"equipment_kind" jsonschema:"barbell, dumbbell, machine, cable or bodyweight"`
	PrimaryMuscles   []string `json:"primary_muscles"`
	SecondaryMuscles []string `json:"secondary_muscles,omitempty"`
	Aliases          []string `json:"aliases,omitempty"`
}

type setTrainingMaxIn struct {
	Slug          string   `json:"slug"`
	TrainingMaxKg *float64 `json:"training_max_kg" jsonschema:"new training max in kg; null clears it"`
}

type setTrainingMaxOut struct {
	Slug          string   `json:"slug"`
	TrainingMaxKg *float64 `json:"training_max_kg"`
	PreviousKg    *float64 `json:"previous_kg"`
}

func (s *Server) addExerciseTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "list_exercises", Description: "Search the user's exercise catalog (built-in and custom)."},
		func(ctx context.Context, u store.User, in listExercisesIn) (listExercisesOut, error) {
			all, err := s.Exercises.Catalog(ctx, u.ID, in.Query)
			if err != nil {
				return listExercisesOut{}, err
			}
			out := listExercisesOut{Exercises: []exerciseOut{}, Muscles: exercise.Muscles}
			for _, e := range all {
				if in.Muscle == "" || slices.Contains(e.PrimaryMuscles, in.Muscle) || slices.Contains(e.SecondaryMuscles, in.Muscle) {
					out.Exercises = append(out.Exercises, exerciseOf(e))
				}
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "create_exercise", Description: "Create a custom exercise for the user. Check list_exercises first: the catalog is large."},
		func(ctx context.Context, u store.User, in createExerciseIn) (exerciseOut, error) {
			e, err := s.Exercises.Create(ctx, u.ID, exercise.Input{Slug: in.Slug, Name: in.Name, Measurement: in.Measurement,
				EquipmentKind: in.EquipmentKind, PrimaryMuscles: in.PrimaryMuscles, SecondaryMuscles: in.SecondaryMuscles, Aliases: in.Aliases})
			return exerciseOf(e), err
		})
	tool(s, srv, &sdk.Tool{Name: "set_training_max", Description: "Set (or clear) the training max of an exercise, in kg. Plans use it for pct_tm loads; the change is logged as made by the AI."},
		func(ctx context.Context, u store.User, in setTrainingMaxIn) (setTrainingMaxOut, error) {
			ex, err := s.Exercises.Get(ctx, u.ID, in.Slug)
			if err != nil {
				return setTrainingMaxOut{}, err
			}
			before, err := s.Exercises.Settings(ctx, u.ID, ex)
			if err != nil {
				return setTrainingMaxOut{}, err
			}
			if err := s.Exercises.SetTrainingMax(ctx, u.ID, in.Slug, in.TrainingMaxKg, "mcp"); err != nil {
				return setTrainingMaxOut{}, err
			}
			return setTrainingMaxOut{Slug: in.Slug, TrainingMaxKg: in.TrainingMaxKg, PreviousKg: before.TrainingMaxKg}, nil
		})
}
```

`internal/mcp/plans.go`:

```go
package mcp

import (
	"context"
	_ "embed"
	"errors"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

//go:embed plan_format.md
var planFormat string

type schemaOut struct {
	Schema any    `json:"schema" jsonschema:"the plan document JSON Schema"`
	Guide  string `json:"guide" jsonschema:"how plans work, in prose"`
}

type versionOut struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

type planOut struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Archived  bool         `json:"archived"`
	Following bool         `json:"following" jsonschema:"the user follows this plan"`
	Versions  []versionOut `json:"versions" jsonschema:"newest first"`
}

type listPlansOut struct {
	Plans []planOut `json:"plans"`
}

type getPlanIn struct {
	PlanID    string `json:"plan_id,omitempty" jsonschema:"default: the plan the user follows"`
	VersionID string `json:"version_id,omitempty" jsonschema:"default: the plan's active version"`
}

type previewDay struct {
	Name  string   `json:"name"`
	Lines []string `json:"lines"`
}

type previewWeek struct {
	Week int          `json:"week"`
	Days []previewDay `json:"days"`
}

type getPlanOut struct {
	PlanID    string        `json:"plan_id"`
	Name      string        `json:"name"`
	VersionID string        `json:"version_id"`
	Version   int           `json:"version"`
	Status    string        `json:"status"`
	Unit      string        `json:"unit" jsonschema:"the user's preferred unit, used in the preview"`
	Doc       any           `json:"doc" jsonschema:"the authored plan document"`
	Preview   []previewWeek `json:"preview" jsonschema:"every week and day with loads resolved from the user's training maxes, e1RMs and equipment"`
}

type validateIn struct {
	Doc any `json:"doc" jsonschema:"the plan document (a JSON object, or a string containing one)"`
}

type validateOut struct {
	Valid    bool           `json:"valid"`
	Problems []plan.Problem `json:"problems" jsonschema:"errors and warnings with JSON Pointers into the document"`
}

type saveDraftIn struct {
	Doc       any    `json:"doc" jsonschema:"the plan document (a JSON object, or a string containing one)"`
	PlanID    string `json:"plan_id,omitempty" jsonschema:"add a new draft version to this plan"`
	VersionID string `json:"version_id,omitempty" jsonschema:"replace this draft version (only drafts can be replaced)"`
	Note      string `json:"note,omitempty" jsonschema:"what changed and why, shown to the user"`
}

type saveDraftOut struct {
	PlanID    string         `json:"plan_id"`
	VersionID string         `json:"version_id"`
	Version   int            `json:"version"`
	Status    string         `json:"status"`
	ReviewURL string         `json:"review_url" jsonschema:"where the user compares and activates the draft"`
	Warnings  []plan.Problem `json:"warnings"`
}

func versionOf(v store.PlanVersion) versionOut {
	return versionOut{ID: v.ID, Version: v.Version, Status: v.Status, Source: v.Source, Note: v.Note, CreatedAt: ts(v.CreatedAt)}
}

func (s *Server) addPlanTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "get_plan_schema", Description: "The plan document JSON Schema and a guide to the format. Read before writing a plan."},
		func(context.Context, store.User, struct{}) (schemaOut, error) {
			return schemaOut{Schema: jsonValue(plan.Schema()), Guide: planFormat}, nil
		})
	tool(s, srv, &sdk.Tool{Name: "list_plans", Description: "The user's plans with their versions and statuses."},
		func(ctx context.Context, u store.User, _ struct{}) (listPlansOut, error) {
			sums, err := s.Plans.Plans(ctx, u)
			if err != nil {
				return listPlansOut{}, err
			}
			out := listPlansOut{Plans: []planOut{}}
			for _, sum := range sums {
				_, versions, err := s.Plans.Plan(ctx, u, sum.ID)
				if err != nil {
					return listPlansOut{}, err
				}
				p := planOut{ID: sum.ID, Name: sum.Name, Archived: sum.Archived, Following: sum.Following, Versions: []versionOut{}}
				for _, v := range versions {
					p.Versions = append(p.Versions, versionOf(v))
				}
				out.Plans = append(out.Plans, p)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_plan", Description: "A plan version's document and a resolved preview of every week. Defaults to the active version of the plan the user follows."},
		func(ctx context.Context, u store.User, in getPlanIn) (getPlanOut, error) {
			v, err := s.pickVersion(ctx, u, in)
			if err != nil {
				return getPlanOut{}, err
			}
			p, _, err := s.Plans.Plan(ctx, u, v.PlanID)
			if err != nil {
				return getPlanOut{}, err
			}
			doc, err := plan.Decode(v.Doc)
			if err != nil {
				return getPlanOut{}, err
			}
			out := getPlanOut{PlanID: p.ID, Name: p.Name, VersionID: v.ID, Version: v.Version, Status: v.Status, Unit: u.Unit,
				Doc: jsonValue(v.Doc), Preview: []previewWeek{}}
			for w, days := range s.Plans.Preview(ctx, u, doc) {
				week := previewWeek{Week: w + 1, Days: []previewDay{}}
				for _, d := range days {
					week.Days = append(week.Days, previewDay{Name: d.Name, Lines: plan.DayLines(d, u.Unit)})
				}
				out.Preview = append(out.Preview, week)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "validate_plan", Description: "Check a plan document without saving it."},
		func(ctx context.Context, u store.User, in validateIn) (validateOut, error) {
			raw, err := docBytes(in.Doc)
			if err != nil {
				return validateOut{}, err
			}
			_, ps := s.Plans.Validate(ctx, u, raw)
			return validateOut{Valid: !ps.HasErrors(), Problems: append([]plan.Problem{}, ps...)}, nil
		})
	tool(s, srv, &sdk.Tool{Name: "save_plan_draft", Description: "Save a plan document as a draft: a new plan, a new version of plan_id, or a replacement for the draft version_id. Drafts never take effect until the user activates them at review_url."},
		func(ctx context.Context, u store.User, in saveDraftIn) (saveDraftOut, error) {
			raw, err := docBytes(in.Doc)
			if err != nil {
				return saveDraftOut{}, err
			}
			d, err := s.Plans.SaveDraft(ctx, u, plan.DraftInput{PlanID: in.PlanID, VersionID: in.VersionID, Doc: raw, Source: "mcp", Note: in.Note})
			if err != nil {
				return saveDraftOut{}, err
			}
			return saveDraftOut{PlanID: d.Plan.ID, VersionID: d.Version.ID, Version: d.Version.Version, Status: d.Version.Status,
				ReviewURL: strings.TrimSuffix(s.BaseURL, "/") + "/plans/" + d.Plan.ID + "/versions/" + d.Version.ID + "/compare",
				Warnings:  append([]plan.Problem{}, d.Warnings...)}, nil
		})
}

// pickVersion resolves get_plan's arguments to a version of the user's.
func (s *Server) pickVersion(ctx context.Context, u store.User, in getPlanIn) (store.PlanVersion, error) {
	if in.VersionID != "" {
		return s.Plans.Version(ctx, u, in.VersionID)
	}
	planID := in.PlanID
	if planID == "" {
		next, err := s.Plans.Next(ctx, u)
		if err != nil {
			return store.PlanVersion{}, err
		}
		if next == nil {
			return store.PlanVersion{}, inputError("the user doesn't follow a plan; pass plan_id (see list_plans)")
		}
		return next.Version, nil
	}
	_, versions, err := s.Plans.Plan(ctx, u, planID)
	if err != nil {
		return store.PlanVersion{}, err
	}
	for _, v := range versions {
		if v.Status == store.PlanActive {
			return v, nil
		}
	}
	return store.PlanVersion{}, errors.New("the plan has no active version; pass version_id (see list_plans)")
}
```

(The last error should be an `inputError` so the AI sees it. Write it as `inputError("…")`.)

`internal/mcp/plan_format.md` is a prose guide of about 60 lines. Write it from spec §6 and the schema's descriptions. Cover:
- the document outline (name, unit, weeks, days → groups → exercises → sets)
- the "per week" array rule (exactly `weeks` entries; `null`/0 in `count` skips a week)
- `only_weeks`
- reps forms (number, `"lo-hi"`, `"AMRAP"`) vs `duration_s`
- set kinds
- the four load keys and when each resolves (`pct_tm` needs a training max; set one with `set_training_max`; `rpe` needs recent e1RM history; `drop_pct` never on an exercise's first set line)
- informational `rpe`
- supersets and `rest_s`
- `alternatives`
- slugs from `list_exercises`
- validation layers and warnings
- the draft → review → activate flow
- one complete example (the starter template's shape)

The test checks that it mentions "per week".

Stubs so the package compiles: `addTrainingTools` and `addPrompts` are empty until Task 5.

`internal/web/server.go`: add `MCP http.Handler // nil disables /mcp`. In `Routes()`, next to `/healthz`:

```go
	if s.MCP != nil {
		r.Handle("/mcp", http.MaxBytesHandler(s.MCP, maxBodyBytes)) // bearer-token auth, no cookies or CSRF (spec §11)
	}
```

`cmd/onerep/main.go`: after the services, wire `srv.MCP = (&mcp.Server{Users: db, Tokens: srv.Tokens, Exercises: srv.Exercises, Plans: srv.Plans, Training: srv.Training, Stats: srv.Stats, BaseURL: cfg.BaseURL}).Handler()`. Import it as `mcpserver "github.com/LongerHV/onerep/internal/mcp"` if the name clashes.

Run: `nix develop --command go test ./internal/mcp/ ./internal/web/ ./cmd/...`
Expected: PASS. If the SDK rejects an output schema at `AddTool` (it panics at startup), the message names the field. Change that field to `any`, filled with `jsonValue`.

- [ ] **Step 4: Add a web-level test that `/mcp` is mounted and needs a token**

Append to `internal/web/server_test.go`:

```go
func TestMCPIsMountedWithoutCookiesOrCSRF(t *testing.T) {
	db := storetest.New(t)
	called := false
	s := &Server{DB: db, Sessions: &auth.Sessions{Store: db}, MCP: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})}
	srv := httptest.NewServer(s.Routes())
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !called || resp.StatusCode != http.StatusTeapot {
		t.Fatalf("/mcp = %d (handler called %v), want the MCP handler without CSRF", resp.StatusCode, called)
	}
}
```

Run: `nix develop --command bash -c 'go test ./... && task licenses:check && task lint'`
Expected: PASS, `0 issues`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/mcp internal/web cmd/onerep/main.go Taskfile.yml
git commit -m "feat(mcp): serve exercise and plan tools at /mcp with API token auth"
```

---

### Task 5: Training and stats tools, prompts, isolation

**Files:**
- Create: `internal/mcp/training.go`, `internal/mcp/prompts.go`, `internal/mcp/training_test.go`

**Interfaces:**
- Consumes: `training.Service.Sessions`, `training.Service.Session`, `stats.Service.ExerciseStats`, `SessionPRs`, `WeeklyMuscleSets`, and `exercise.Service.Settings`.
- Produces the tools `list_sessions`, `get_session`, `get_exercise_stats` and `get_weekly_muscle_volume`, and the prompts `review_block` and `build_plan`.

- [ ] **Step 1: Write the failing tests**

`internal/mcp/training_test.go`:

```go
package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/store"
)

// logSession stores a finished session with squat sets for user, done at when.
func (e env) logSession(t *testing.T, u store.User, when time.Time, kgs ...float64) store.Session {
	t.Helper()
	ctx := context.Background()
	s, err := e.db.CreateSession(ctx, store.Session{UserID: u.ID, Name: "Squat day", Snapshot: []byte(`{"name":"Squat day","groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	for i, kg := range kgs {
		kg, reps, rpe := kg, 5, 8.0
		e1rm := kg / 0.811 // RTS 5 @ 8
		done := when.Add(time.Duration(i) * time.Minute)
		id := s.ID[:24] + strings.Repeat("0", 11) + string(rune('a'+i))
		if _, err := e.db.UpsertSet(ctx, u.ID, store.Set{ID: id, SessionID: s.ID, Slug: "barbell-back-squat", Kind: "working", SetPos: i,
			WeightKg: &kg, Reps: &reps, RPE: &rpe, E1RMKg: &e1rm, DoneAt: &done, UpdatedAt: done}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.db.FinishSession(ctx, u.ID, s.ID, when.Add(time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTrainingTools(t *testing.T) {
	e := newEnv(t)
	now := time.Now().UTC()
	first := e.logSession(t, e.alice, now.AddDate(0, 0, -8), 100, 100)
	second := e.logSession(t, e.alice, now.AddDate(0, 0, -1), 105, 107.5)
	cs := connect(t, e.url, e.aliceToken)

	var sessions struct {
		Sessions []struct {
			ID        string   `json:"id"`
			Sets      int      `json:"sets"`
			Exercises []string `json:"exercises"`
		} `json:"sessions"`
	}
	call(t, cs, "list_sessions", map[string]any{"exercise": "barbell-back-squat"}, &sessions)
	if len(sessions.Sessions) != 2 || sessions.Sessions[0].ID != second.ID || sessions.Sessions[0].Exercises[0] != "barbell-back-squat" {
		t.Fatalf("sessions = %+v", sessions)
	}
	from := now.AddDate(0, 0, -3).Format("2006-01-02")
	call(t, cs, "list_sessions", map[string]any{"from": from}, &sessions)
	if len(sessions.Sessions) != 1 {
		t.Fatalf("from %s: %d sessions", from, len(sessions.Sessions))
	}
	if msg := call(t, cs, "list_sessions", map[string]any{"from": "last week"}, nil); !strings.Contains(msg, "YYYY-MM-DD") {
		t.Fatalf("bad date: %q", msg)
	}

	var sess struct {
		Unit       string `json:"unit"`
		Prescribed any    `json:"prescribed"`
		Sets       []struct {
			WeightKg *float64 `json:"weight_kg"`
			PR       bool     `json:"pr"`
		} `json:"sets"`
	}
	call(t, cs, "get_session", map[string]any{"id": second.ID}, &sess)
	if sess.Unit != "kg" || sess.Prescribed == nil || len(sess.Sets) != 2 || !sess.Sets[0].PR || !sess.Sets[1].PR {
		t.Fatalf("session = %+v", sess)
	}
	_ = first

	var st struct {
		Unit           string   `json:"unit"`
		TrainingMaxKg  *float64 `json:"training_max_kg"`
		E1RMSeries     []any    `json:"e1rm_series"`
		RepMaxes       []struct {
			Reps     int     `json:"reps"`
			WeightKg float64 `json:"weight_kg"`
		} `json:"rep_maxes"`
		RecentBestSets []struct {
			WeightKg float64 `json:"weight_kg"`
		} `json:"recent_best_sets"`
	}
	call(t, cs, "get_exercise_stats", map[string]any{"slug": "barbell-back-squat"}, &st)
	if len(st.E1RMSeries) != 2 || len(st.RepMaxes) != 1 || st.RepMaxes[0].WeightKg != 107.5 ||
		len(st.RecentBestSets) != 2 || st.RecentBestSets[0].WeightKg != 107.5 {
		t.Fatalf("stats = %+v", st)
	}

	var vol struct {
		Weeks   []string `json:"weeks"`
		Muscles []struct {
			Muscle string    `json:"muscle"`
			Sets   []float64 `json:"sets"`
		} `json:"muscles"`
	}
	call(t, cs, "get_weekly_muscle_volume", map[string]any{}, &vol)
	if len(vol.Weeks) != 12 || len(vol.Muscles) == 0 {
		t.Fatalf("volume = %+v", vol)
	}
	if msg := call(t, cs, "get_weekly_muscle_volume", map[string]any{"from": "2020-01-01", "to": "2026-01-01"}, nil); !strings.Contains(msg, "104") {
		t.Fatalf("too long a range: %q", msg)
	}

	prompts, err := cs.ListPrompts(context.Background(), nil)
	if err != nil || len(prompts.Prompts) != 2 {
		t.Fatalf("prompts = %+v, %v", prompts, err)
	}
	for _, name := range []string{"review_block", "build_plan"} {
		got, err := cs.GetPrompt(context.Background(), &sdk.GetPromptParams{Name: name})
		if err != nil || len(got.Messages) == 0 {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
	}
}

func TestToolsAreIsolatedPerUser(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	aliceSession := e.logSession(t, e.alice, time.Now().UTC().AddDate(0, 0, -1), 100)
	alice := connect(t, e.url, e.aliceToken)
	var draft struct {
		PlanID    string `json:"plan_id"`
		VersionID string `json:"version_id"`
	}
	call(t, alice, "save_plan_draft", map[string]any{"doc": planDoc}, &draft)
	call(t, alice, "create_exercise", map[string]any{"slug": "zercher-squat", "name": "Zercher Squat", "measurement": "weight_reps",
		"equipment_kind": "barbell", "primary_muscles": []string{"quads"}}, nil)

	bob := connect(t, e.url, e.bobToken)
	for tool, args := range map[string]map[string]any{
		"get_session":        {"id": aliceSession.ID},
		"get_plan":           {"plan_id": draft.PlanID},
		"get_plan#version":   {"version_id": draft.VersionID},
		"save_plan_draft":    {"doc": planDoc, "version_id": draft.VersionID},
		"save_plan_draft#pl": {"doc": planDoc, "plan_id": draft.PlanID},
		"get_exercise_stats": {"slug": "zercher-squat"},
		"set_training_max":   {"slug": "zercher-squat", "training_max_kg": 100},
	} {
		name, _, _ := strings.Cut(tool, "#")
		if msg := call(t, bob, name, args, nil); !strings.Contains(msg, "not found") {
			t.Errorf("bob %s(%v) = %q, want not found", name, args, msg)
		}
	}
	var sessions struct {
		Sessions []any `json:"sessions"`
	}
	call(t, bob, "list_sessions", map[string]any{}, &sessions)
	var plans struct {
		Plans []any `json:"plans"`
	}
	call(t, bob, "list_plans", map[string]any{}, &plans)
	var st struct {
		E1RMSeries []any `json:"e1rm_series"`
	}
	call(t, bob, "get_exercise_stats", map[string]any{"slug": "barbell-back-squat"}, &st)
	if len(sessions.Sessions) != 0 || len(plans.Plans) != 0 || len(st.E1RMSeries) != 0 {
		t.Fatalf("bob sees alice's data: %d sessions, %d plans, %d e1RM points", len(sessions.Sessions), len(plans.Plans), len(st.E1RMSeries))
	}
	if v, _ := e.Plans().Version(ctx, e.alice, draft.VersionID); !strings.Contains(string(v.Doc), "Block") {
		t.Fatal("alice's draft was changed")
	}
}
```

(Import `sdk "github.com/modelcontextprotocol/go-sdk/mcp"` in this test file. The set ids built in `logSession` must be valid UUIDs if the store checks them; it doesn't, since the store takes any text, but keep them 36 characters and unique.)

Run: `nix develop --command go test ./internal/mcp/ -run 'Training|Isolated'`
Expected: FAIL: `list_sessions` is an unknown tool.

- [ ] **Step 2: Implement**

`internal/mcp/training.go`:

```go
package mcp

import (
	"context"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/LongerHV/onerep/internal/store"
)

type listSessionsIn struct {
	From     string `json:"from,omitempty" jsonschema:"first day, YYYY-MM-DD (UTC)"`
	To       string `json:"to,omitempty" jsonschema:"last day, YYYY-MM-DD (UTC), inclusive"`
	Exercise string `json:"exercise,omitempty" jsonschema:"only sessions with sets of this exercise slug"`
	Limit    int    `json:"limit,omitempty" jsonschema:"at most this many sessions (default 20, max 100)"`
}

type sessionSummaryOut struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	StartedAt  string   `json:"started_at"`
	FinishedAt string   `json:"finished_at,omitempty"`
	PlanWeek   int      `json:"plan_week,omitempty"`
	PlanDay    int      `json:"plan_day,omitempty" jsonschema:"0-based day index in the plan week"`
	Sets       int      `json:"sets"`
	Exercises  []string `json:"exercises"`
	Notes      string   `json:"notes,omitempty"`
}

type listSessionsOut struct {
	Sessions []sessionSummaryOut `json:"sessions" jsonschema:"newest first"`
}

type idIn struct {
	ID string `json:"id"`
}

type setOut struct {
	ID          string   `json:"id"`
	Slug        string   `json:"slug"`
	GroupPos    int      `json:"group_pos"`
	ExercisePos int      `json:"exercise_pos"`
	SetPos      int      `json:"set_pos"`
	Kind        string   `json:"kind"`
	WeightKg    *float64 `json:"weight_kg"`
	Reps        *int     `json:"reps"`
	RPE         *float64 `json:"rpe"`
	DurationS   *int     `json:"duration_s,omitempty"`
	DistanceM   *float64 `json:"distance_m,omitempty"`
	E1RMKg      *float64 `json:"e1rm_kg,omitempty"`
	DoneAt      string   `json:"done_at,omitempty"`
	PR          bool     `json:"pr" jsonschema:"heavier than every earlier set of this exercise at these reps"`
}

type sessionOut struct {
	sessionSummaryOut
	Unit       string   `json:"unit"`
	Prescribed any      `json:"prescribed" jsonschema:"the planned day as it was when the session started (loads in kg)"`
	SetsDone   []setOut `json:"sets" jsonschema:"what was actually done"`
}

type slugIn struct {
	Slug string `json:"slug"`
}

type pointOut struct {
	Date      string   `json:"date"`
	SessionID string   `json:"session_id"`
	E1RMKg    float64  `json:"e1rm_kg"`
	RPEBased  bool     `json:"rpe_based"`
	WeightKg  float64  `json:"weight_kg"`
	Reps      int      `json:"reps"`
	RPE       *float64 `json:"rpe"`
}

type repMaxOut struct {
	Reps      int     `json:"reps"`
	WeightKg  float64 `json:"weight_kg"`
	Date      string  `json:"date"`
	SessionID string  `json:"session_id"`
}

type exerciseStatsOut struct {
	Slug           string      `json:"slug"`
	Name           string      `json:"name"`
	Unit           string      `json:"unit"`
	TrainingMaxKg  *float64    `json:"training_max_kg"`
	E1RMSeries     []pointOut  `json:"e1rm_series" jsonschema:"best estimated 1RM of each workout, oldest first"`
	RepMaxes       []repMaxOut `json:"rep_maxes" jsonschema:"heaviest working set per rep count, 1-12"`
	RecentBestSets []pointOut  `json:"recent_best_sets" jsonschema:"best set of each of the last 10 workouts, newest first"`
}

type volumeIn struct {
	From string `json:"from,omitempty" jsonschema:"a day in the first ISO week, YYYY-MM-DD (default: 11 weeks before to)"`
	To   string `json:"to,omitempty" jsonschema:"a day in the last ISO week, YYYY-MM-DD (default: today)"`
}

type muscleOut struct {
	Muscle string    `json:"muscle"`
	Sets   []float64 `json:"sets" jsonschema:"hard sets per week, aligned with weeks"`
	Total  float64   `json:"total"`
}

type volumeOut struct {
	Weeks   []string    `json:"weeks" jsonschema:"ISO weeks like 2026-W39, oldest first"`
	Muscles []muscleOut `json:"muscles" jsonschema:"most volume first; a hard set counts 1 for each primary muscle and 0.5 for each secondary"`
}

// day parses a YYYY-MM-DD tool argument (UTC).
func day(name, v string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, inputError(name + " must be a date like 2026-09-24 (YYYY-MM-DD)")
	}
	return t, nil
}

func summaryOf(s store.SessionSummary) sessionSummaryOut {
	out := sessionSummaryOut{ID: s.ID, Name: s.Name, StartedAt: ts(s.StartedAt), PlanWeek: s.Week, PlanDay: s.Day,
		Sets: s.Sets, Exercises: append([]string{}, s.Slugs...), Notes: s.Notes}
	if s.FinishedAt != nil {
		out.FinishedAt = ts(*s.FinishedAt)
	}
	return out
}

func pointOf(p stats.Point) pointOut {
	return pointOut{Date: ts(p.DoneAt), SessionID: p.SessionID, E1RMKg: p.E1RMKg, RPEBased: p.RPEBased,
		WeightKg: p.WeightKg, Reps: p.Reps, RPE: p.RPE}
}

func (s *Server) addTrainingTools(srv *sdk.Server) {
	tool(s, srv, &sdk.Tool{Name: "list_sessions", Description: "The user's workouts, newest first, optionally by date range and exercise."},
		func(ctx context.Context, u store.User, in listSessionsIn) (listSessionsOut, error) {
			f := store.SessionFilter{Slug: in.Exercise, Limit: in.Limit}
			var err error
			if in.From != "" {
				if f.From, err = day("from", in.From); err != nil {
					return listSessionsOut{}, err
				}
			}
			if in.To != "" {
				if f.To, err = day("to", in.To); err != nil {
					return listSessionsOut{}, err
				}
				f.To = f.To.AddDate(0, 0, 1)
			}
			list, err := s.Training.Sessions(ctx, u, f)
			if err != nil {
				return listSessionsOut{}, err
			}
			out := listSessionsOut{Sessions: []sessionSummaryOut{}}
			for _, sum := range list {
				out.Sessions = append(out.Sessions, summaryOf(sum))
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_session", Description: "One workout: what was prescribed and every set actually done, with PRs marked."},
		func(ctx context.Context, u store.User, in idIn) (sessionOut, error) {
			sess, sets, err := s.Training.Session(ctx, u, in.ID)
			if err != nil {
				return sessionOut{}, err
			}
			prs, err := s.Stats.SessionPRs(ctx, u, sess.ID)
			if err != nil {
				return sessionOut{}, err
			}
			out := sessionOut{sessionSummaryOut: summaryOf(store.SessionSummary{Session: sess, Sets: len(sets)}), Unit: u.Unit,
				Prescribed: jsonValue(sess.Snapshot), SetsDone: []setOut{}}
			seen := map[string]bool{}
			for _, x := range sets {
				if !seen[x.Slug] {
					seen[x.Slug] = true
					out.Exercises = append(out.Exercises, x.Slug)
				}
				so := setOut{ID: x.ID, Slug: x.Slug, GroupPos: x.GroupPos, ExercisePos: x.ExercisePos, SetPos: x.SetPos, Kind: x.Kind,
					WeightKg: x.WeightKg, Reps: x.Reps, RPE: x.RPE, DurationS: x.DurationS, DistanceM: x.DistanceM, E1RMKg: x.E1RMKg, PR: prs[x.ID]}
				if x.DoneAt != nil {
					so.DoneAt = ts(*x.DoneAt)
				}
				out.SetsDone = append(out.SetsDone, so)
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_exercise_stats", Description: "Progress on one exercise: e1RM over time, rep maxes, recent best sets and the current training max."},
		func(ctx context.Context, u store.User, in slugIn) (exerciseStatsOut, error) {
			st, err := s.Stats.ExerciseStats(ctx, u, in.Slug)
			if err != nil {
				return exerciseStatsOut{}, err
			}
			settings, err := s.Exercises.Settings(ctx, u.ID, st.Exercise)
			if err != nil {
				return exerciseStatsOut{}, err
			}
			out := exerciseStatsOut{Slug: st.Exercise.Slug, Name: st.Exercise.Name, Unit: u.Unit, TrainingMaxKg: settings.TrainingMaxKg,
				E1RMSeries: []pointOut{}, RepMaxes: []repMaxOut{}, RecentBestSets: []pointOut{}}
			for _, p := range st.Series {
				out.E1RMSeries = append(out.E1RMSeries, pointOf(p))
			}
			for i := len(st.Series) - 1; i >= 0 && len(out.RecentBestSets) < 10; i-- {
				out.RecentBestSets = append(out.RecentBestSets, pointOf(st.Series[i]))
			}
			for _, m := range st.RepMaxes {
				out.RepMaxes = append(out.RepMaxes, repMaxOut{Reps: m.Reps, WeightKg: m.WeightKg, Date: ts(m.DoneAt), SessionID: m.SessionID})
			}
			return out, nil
		})
	tool(s, srv, &sdk.Tool{Name: "get_weekly_muscle_volume", Description: "Hard sets per muscle per ISO week (working, drop and AMRAP sets at RPE 7+ or without RPE). Default: the last 12 weeks; at most 104."},
		func(ctx context.Context, u store.User, in volumeIn) (volumeOut, error) {
			to := time.Now().UTC()
			var err error
			if in.To != "" {
				if to, err = day("to", in.To); err != nil {
					return volumeOut{}, err
				}
			}
			from := to.AddDate(0, 0, -7*11)
			if in.From != "" {
				if from, err = day("from", in.From); err != nil {
					return volumeOut{}, err
				}
			}
			if to.Before(from) || to.Sub(from) > 104*7*24*time.Hour {
				return volumeOut{}, inputError("from must be before to, and the range at most 104 weeks")
			}
			mw, err := s.Stats.WeeklyMuscleSets(ctx, u, from, to)
			if err != nil {
				return volumeOut{}, err
			}
			out := volumeOut{Weeks: []string{}, Muscles: []muscleOut{}}
			for _, w := range mw.Weeks {
				out.Weeks = append(out.Weeks, w.Label)
			}
			for _, m := range mw.Muscles {
				out.Muscles = append(out.Muscles, muscleOut{Muscle: m.Muscle, Sets: m.Sets, Total: m.Total})
			}
			return out, nil
		})
}
```

(Import `internal/stats` for `stats.Point`.)

`internal/mcp/prompts.go`:

```go
package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const reviewBlock = `Review my latest completed training block in onerep and propose the next one.
1. Call get_plan (the plan I follow) and list_sessions for the block's weeks.
2. For the main lifts, call get_exercise_stats; look at e1RM trend, rep maxes, and how actual sets compared with what was prescribed (get_session).
3. Call get_weekly_muscle_volume for the block to check volume per muscle.
4. Summarise what went well, what stalled, and why, then propose changes (loads, volume, exercise swaps).
5. Write the next block with get_plan_schema's format, check it with validate_plan, and save it with save_plan_draft (plan_id of my plan). Update training maxes with set_training_max only if I agree.
6. Give me the review_url so I can compare and activate it.`

const buildPlan = `Help me build a new training plan in onerep.
1. Ask about my goals, how many days a week I train and for how long, my experience, injuries, and my equipment.
2. Look at my recent training (list_sessions, get_exercise_stats for main lifts) and weekly volume (get_weekly_muscle_volume).
3. Read get_plan_schema, and pick exercises with list_exercises (create_exercise only if nothing fits).
4. Draft the plan, check it with validate_plan, fix every error, and save it with save_plan_draft.
5. If loads use pct_tm, suggest training maxes and set them with set_training_max once I agree.
6. Give me the review_url so I can review and activate the plan.`

func (s *Server) addPrompts(srv *sdk.Server) {
	add := func(name, description, text string) {
		srv.AddPrompt(&sdk.Prompt{Name: name, Description: description},
			func(context.Context, *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
				return &sdk.GetPromptResult{Description: description, Messages: []*sdk.PromptMessage{
					{Role: "user", Content: &sdk.TextContent{Text: text}}}}, nil
			})
	}
	add("review_block", "Analyse the latest completed block and propose the next as a draft.", reviewBlock)
	add("build_plan", "Gather goals, schedule and equipment, then draft a plan.", buildPlan)
}
```

Remove the Task 4 stubs.

Run: `nix develop --command go test ./internal/mcp/`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp
git commit -m "feat(mcp): add session, stats and volume tools and the review and build prompts"
```

---

### Task 6: Docs and a live check

**Files:**
- Modify: `README.md`, `AGENTS.md`

- [ ] **Step 1: Check it live**

Start `task dev` in the background, create a token through the tokens page (dev login, form POST), then talk to `/mcp` with curl:

```bash
curl -s -X POST localhost:8080/mcp -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"1"}}}'
curl -s -X POST localhost:8080/mcp -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_exercises","arguments":{"query":"bench"}}}'
```

Expected: an `initialize` result naming `onerep`, then a JSON list of bench exercises. Without the header, 401.

- [ ] **Step 2: Docs**

`README.md`: add a section "Connecting an AI assistant (MCP)" covering:
- create a token at Settings → API tokens
- the server URL `<ONEREP_BASE_URL>/mcp` with an `Authorization: Bearer` header
- the Claude Code command
- what the assistant can and cannot do
- the two prompts

`AGENTS.md`:
- Layout: add `internal/mcp/            MCP server (go-sdk): tools and prompts over the services, bearer-token auth` and extend `internal/auth/` with "API tokens". Replace "Milestone 6 adds `mcp/`…" with nothing.
- Rules: add **MCP tools are thin adapters too.** Every tool acts as the token's user, answers "not found" for anything else, uses kg in and out, and never changes logged sessions or sets or activates plans. Adding a tool means a test in `internal/mcp` that calls it through the go-sdk client, including another user's token.

- [ ] **Step 3: Run CI and commit**

Run: `nix develop --command task ci`
Expected: exit code 0.

```bash
git add README.md AGENTS.md
git commit -m "docs: explain connecting an AI assistant over MCP"
```
