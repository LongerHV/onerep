# onerep Milestone 4: Training — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workouts:
- Start the planned day, or an empty workout, from the home page.
- Log sets in a companion screen that keeps working offline (IndexedDB, an outbox and a service worker) and syncs every change exactly once.
- Correct history afterwards in an editor.
- Logged sets feed back into planning: RPE weights use the recent e1RM, and finishing a day moves the plan on.

**Architecture:** `internal/store` adds `sessions`, `sets` (client-generated UUIDv7 ids, soft deletes) and `applied_ops`. Every sync write runs in a transaction that records its `op_id`, so a replay changes nothing. `internal/training` owns sessions, applying the companion's operations (validation, e1RM derivation, clamping future timestamps, advancing the plan cursor on finish), the companion bootstrap and history editing. `internal/plan` gains an e1RM source (`History`) and `AdvanceFrom`. In the browser, `companion-core.js` is pure logic tested in Node; `companion.js` renders it, keeps state and an outbox in IndexedDB, syncs to `/api/sync`, runs the rest timer and holds a wake lock. `sw.js` caches the app shell and live workout pages. `test/e2e/companion.mjs` drives headless Chromium through an offline workout.

**Tech Stack:** unchanged. No new Go modules or vendored JS (vanilla JS, as the user chose). Chromium for `task e2e` comes from the flake's nixpkgs.

**Spec:** `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. This plan implements:
- §5 sessions, sets and applied_ops
- §7 in-session adjustment
- §8 finishing advances the cursor
- §9 companion mode, sync, service worker and history editing
- §16 JSON errors for the sync API
- §17 the E2E test

PR badges and the PR table (spec §9/§13) come with milestone 5's stats.

**Deliberate differences from spec §9:**
- **No `swap_exercise` operation.** Swaps are local to the companion, and every logged set carries the slug actually performed.
- **Two new columns on `sets`:** `exercise_pos` (a superset's exercise within its group) and a denormalised `user_id` (for isolation and stats queries).
- **Exercises added during a workout** round with the user's default equipment for their kind, but have no TM or e1RM offline.

## Global Constraints

Everything from milestones 1–3 applies (the commit trailer for agent-authored commits is `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`). In addition:

- Set and operation ids are UUIDs, validated as `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`. The companion generates UUIDv7.
- **Sync rules (spec §9):**
  - operations apply in order, one transaction each, recorded in `applied_ops`
  - a replayed `op_id` gets `duplicate`
  - the newer `updated_at` wins
  - a deleted set stays deleted, even against a late upsert, including a delete that arrives before the set's creation
  - client timestamps are clamped to server time + 5 minutes
  - rejected operations are reported with a reason and never stop the batch
  - at most 200 operations per request
- **Set value limits:** weight 0–2000 kg, reps 0–1000, RPE 1–10, duration 0–86,400 s, distance 0–1,000,000 m, all finite.
- **e1RM:** stored on every write, and only for `weight_reps` exercises (bodyweight exercises have none until body weight is tracked).
- **At most one unfinished session per user.** Starting another continues the open one.
- **Finishing** marks the session finished once; only that first finish moves the cursor, and only if the user follows that plan and the cursor is still on that day.
- **Companion pages** are full-layout pages. `companion.js` loads once from the layout `<head>` and sets itself up via `htmx.onLoad`, like the plan editor.
- The service worker is served at `/sw.js` (scope `/`), versioned with a hash of the static files. `/offline` is a public page.

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **Out-of-order delivery around deletes.** An edit made before a delete but synced after it, or a delete that reaches the server before the set it deletes, must not bring the set back. Test: `TestDeletedSetsStayDeleted` (Task 1).
2. **A phone with a wrong clock** sending timestamps far in the future must not lock a set against later edits. Test: `TestFutureTimestampsAreClamped` (Task 3).
3. **Finishing from two devices, or retrying a finish,** must move the plan on once, not twice. Test: `TestFinishAdvancesThePlanOnce` (Task 3).
4. **Go and JS timestamp formats.** Go omits zero milliseconds, so `…:00Z` sorts after `…:00.500Z` as a string, and comparing strings would let an older server copy win. Test: "timestamps compare as times, not strings" (Task 4).
5. **A login session that expired while offline.** Sync must get a 401, so the companion keeps its queue and asks the user to sign in again, rather than following a redirect to the login page. Test: `TestSyncWithoutSessionIs401` (Task 6).

The whole offline path (log online, lose the server, log and edit, reload from the offline cache, reconnect, check every set arrived exactly once, finish) is covered by `task e2e` (Task 7).

## File Map

```
internal/store/schema.sql                 + sessions, sets, applied_ops
internal/store/migrations/*_sessions.{up,down}.sql, atlas.sum    generated
internal/store/sessions.go(+_test)        sessions, sets, op dedupe, sync rules, BestE1RM, LastSets
internal/plan/service.go(+_test)          + History (e1RM source), AdvanceFrom
internal/training/service.go              start, ApplyOps (validation, e1RM, clamp, finish -> cursor)
internal/training/bootstrap.go            companion bootstrap
internal/training/history.go              history list/detail/editing
internal/training/service_test.go
internal/web/static/js/companion-core.js  pure companion logic
internal/web/jstest/companion.test.mjs    Node tests for it
internal/web/static/js/companion.js       rendering, IndexedDB, outbox, sync, timer, wake lock
internal/web/static/js/sw.js              service worker
internal/web/sessions.go, history.go      handlers (/sessions, /api/sync, /api/csrf, /sw.js, /offline, /history)
internal/web/server.go, plans.go          wiring, home page data, failJSON
internal/web/views/sessions.templ, history.templ, pages.templ, layout.templ, models.go, ui.go
internal/web/server_test.go, sessions_test.go
cmd/onerep/main.go
test/e2e/companion.mjs, Taskfile.yml, .github/workflows/ci.yml, AGENTS.md
```

---

### Task 1: Sessions and sets storage

**Files:**
- Replace: `internal/store/schema.sql`
- Create (generated): `internal/store/migrations/<timestamp>_sessions.{up,down}.sql`, updated `atlas.sum`
- Create: `internal/store/sessions.go`
- Test: `internal/store/sessions_test.go`

**Interfaces:**
- Consumes: store helpers (`tx`, `newID`, `mustAffect`, `formatTime`, `parseTime`, `nullString`, `newUser`)
- Produces:
  - `store.Session{ID, UserID, PlanID, PlanVersionID string; Week, Day int; Name string; Snapshot []byte; StartedAt time.Time; FinishedAt *time.Time; Notes string; UpdatedAt time.Time}`
  - `store.Set{ID, SessionID, UserID, Slug string; GroupPos, ExercisePos, SetPos int; Kind string; Prescribed []byte; WeightKg *float64; Reps *int; RPE *float64; DurationS *int; DistanceM *float64; E1RMKg *float64; DoneAt *time.Time; UpdatedAt time.Time; DeletedAt *time.Time}`
  - `store.Outcome` with the values `Applied`, `Duplicate` and `Ignored`; `store.SessionSummary{Session; Sets int}`
  - Sessions: `CreateSession`, `SessionByID`, `OpenSession`, `ListSessions(ctx, userID, limit, offset)`, `DeleteSession`
  - Sets: `SessionSets`, `LastSets(ctx, userID, slug, excludeSessionID)`, `BestE1RM(ctx, userID, slug, since) (*float64, error)`
  - Sync writes, each taking an `opID` (`""` = no dedupe) and returning `(Outcome, error)`: `UpsertSet(ctx, userID, Set, opID)`, `DeleteSet(ctx, userID, sessionID, setID, at, opID)`, `SetSessionNotes(ctx, userID, sessionID, notes, at, opID)`, `FinishSession(ctx, userID, sessionID, at, opID)`

- [ ] **Step 1: Schema and migration**

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

-- A workout. Planned sessions snapshot the resolved day they started from
-- (spec §9); ad-hoc sessions have no plan and an empty snapshot.
CREATE TABLE sessions (
  id              TEXT    NOT NULL PRIMARY KEY,
  user_id         TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  plan_id         TEXT    REFERENCES plans (id) ON DELETE SET NULL,
  plan_version_id TEXT    REFERENCES plan_versions (id) ON DELETE SET NULL,
  week            INTEGER NOT NULL DEFAULT 0, -- 0 for ad-hoc sessions
  day             INTEGER NOT NULL DEFAULT 0,
  name            TEXT    NOT NULL,
  snapshot        TEXT    NOT NULL, -- plan.ExpandedDay as JSON
  started_at      TEXT    NOT NULL,
  finished_at     TEXT,
  notes           TEXT    NOT NULL DEFAULT '',
  updated_at      TEXT    NOT NULL
);

CREATE INDEX sessions_user_started ON sessions (user_id, started_at);

-- Logged sets. IDs come from the client so offline replays are idempotent.
-- Deletes are soft so a late replay cannot bring a set back.
CREATE TABLE sets (
  id           TEXT    NOT NULL PRIMARY KEY,
  session_id   TEXT    NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
  user_id      TEXT    NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  slug         TEXT    NOT NULL, -- the exercise actually performed
  group_pos    INTEGER NOT NULL,
  exercise_pos INTEGER NOT NULL,
  set_pos      INTEGER NOT NULL,
  kind         TEXT    NOT NULL CHECK (kind IN ('warmup', 'working', 'drop', 'amrap')),
  prescribed   TEXT, -- plan.PrescribedSet as JSON; NULL for unplanned sets
  weight_kg    REAL,
  reps         INTEGER,
  rpe          REAL,
  duration_s   INTEGER,
  distance_m   REAL,
  e1rm_kg      REAL, -- derived, recomputed on every write
  done_at      TEXT,
  updated_at   TEXT    NOT NULL,
  deleted_at   TEXT
);

CREATE INDEX sets_session ON sets (session_id);
CREATE INDEX sets_user_slug_done ON sets (user_id, slug, done_at);

-- Sync operations already applied, so replays are answered, not re-applied.
CREATE TABLE applied_ops (
  op_id      TEXT NOT NULL PRIMARY KEY,
  user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  result     TEXT NOT NULL,
  applied_at TEXT NOT NULL
);
```

Run: `task migrate:diff NAME=sessions && task migrate:check`
Expected: a new `<timestamp>_sessions.up.sql` creating the three tables; `migrate:check` exits 0.

- [ ] **Step 2: Write the failing tests**

`internal/store/sessions_test.go`:
```go
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)

func newSession(t *testing.T, db *DB, userID, name string) Session {
	t.Helper()
	s, err := db.CreateSession(context.Background(), Session{UserID: userID, Name: name, Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func set(sessionID, id string, kg float64, updated time.Time) Set {
	reps := 5
	done := updated
	return Set{ID: id, SessionID: sessionID, Slug: "barbell-back-squat", Kind: "working", WeightKg: &kg, Reps: &reps,
		DoneAt: &done, UpdatedAt: updated}
}

func TestUpsertSetSyncRules(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	alice, bob := newUser(t, db, "alice"), newUser(t, db, "bob")
	s := newSession(t, db, alice.ID, "A")

	if out, err := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 100, t0), "op-1"); err != nil || out != Applied {
		t.Fatalf("insert: %v %v", out, err)
	}
	// The same op again changes nothing, even with different content.
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 999, t0.Add(time.Hour)), "op-1"); out != Duplicate {
		t.Fatalf("replay: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 102.5, t0.Add(time.Minute)), "op-2"); out != Applied {
		t.Fatalf("newer edit: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, alice.ID, set(s.ID, "set-1", 90, t0.Add(30*time.Second)), "op-3"); out != Ignored {
		t.Fatalf("older edit: %v", out)
	}
	sets, _ := db.SessionSets(ctx, alice.ID, s.ID)
	if len(sets) != 1 || *sets[0].WeightKg != 102.5 {
		t.Fatalf("sets = %+v", sets)
	}

	if _, err := db.UpsertSet(ctx, bob.ID, set(s.ID, "set-2", 1, t0), "op-4"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob writes into alice's session: %v", err)
	}
	other := newSession(t, db, alice.ID, "B")
	if _, err := db.UpsertSet(ctx, alice.ID, set(other.ID, "set-1", 1, t0.Add(time.Hour)), "op-5"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("moving a set to another session: %v", err)
	}
}

func TestDeletedSetsStayDeleted(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "A")

	_, _ = db.UpsertSet(ctx, u.ID, set(s.ID, "set-1", 100, t0), "op-1")
	if out, err := db.DeleteSet(ctx, u.ID, s.ID, "set-1", t0.Add(time.Minute), "op-2"); err != nil || out != Applied {
		t.Fatalf("delete: %v %v", out, err)
	}
	// An edit made before the delete but synced after it must not revive the set.
	if out, _ := db.UpsertSet(ctx, u.ID, set(s.ID, "set-1", 105, t0.Add(2*time.Minute)), "op-3"); out != Ignored {
		t.Fatalf("late upsert: %v", out)
	}
	// Deleted before the server ever saw it (both ops queued offline, delivered out of order).
	if out, _ := db.DeleteSet(ctx, u.ID, s.ID, "set-2", t0.Add(time.Minute), "op-4"); out != Ignored {
		t.Fatalf("delete unseen: %v", out)
	}
	if out, _ := db.UpsertSet(ctx, u.ID, set(s.ID, "set-2", 100, t0), "op-5"); out != Ignored {
		t.Fatalf("upsert after tombstone: %v", out)
	}
	if sets, _ := db.SessionSets(ctx, u.ID, s.ID); len(sets) != 0 {
		t.Fatalf("deleted sets visible: %+v", sets)
	}
}

func TestSessionNotesAndFinish(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "A")
	later := time.Now().Add(time.Hour)

	if out, _ := db.SetSessionNotes(ctx, u.ID, s.ID, "felt heavy", later, "op-1"); out != Applied {
		t.Fatalf("notes: %v", out)
	}
	if out, _ := db.SetSessionNotes(ctx, u.ID, s.ID, "older text", later.Add(-time.Minute), "op-2"); out != Ignored {
		t.Fatalf("older notes: %v", out)
	}
	if open, err := db.OpenSession(ctx, u.ID); err != nil || open.ID != s.ID {
		t.Fatalf("open session = %+v, %v", open, err)
	}
	if out, _ := db.FinishSession(ctx, u.ID, s.ID, later, "op-3"); out != Applied {
		t.Fatalf("finish: %v", out)
	}
	if out, _ := db.FinishSession(ctx, u.ID, s.ID, later, "op-4"); out != Ignored {
		t.Fatalf("finish twice: %v", out)
	}
	got, _ := db.SessionByID(ctx, u.ID, s.ID)
	if got.Notes != "felt heavy" || got.FinishedAt == nil {
		t.Fatalf("session = %+v", got)
	}
	if _, err := db.OpenSession(ctx, u.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("finished session still open")
	}
}

func TestHistoryQueries(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	old := newSession(t, db, u.ID, "old")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(old.ID, "a", 100, t0.Add(-60*24*time.Hour)), 130), "")
	prev := newSession(t, db, u.ID, "prev")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(prev.ID, "b", 100, t0), 120), "")
	_, _ = db.UpsertSet(ctx, u.ID, withE1RM(set(prev.ID, "c", 105, t0), 125), "")
	_, _ = db.DeleteSet(ctx, u.ID, prev.ID, "c", t0.Add(time.Minute), "")
	cur := newSession(t, db, u.ID, "current")

	best, err := db.BestE1RM(ctx, u.ID, "barbell-back-squat", t0.Add(-30*24*time.Hour))
	if err != nil || best == nil || *best != 120 {
		t.Fatalf("best e1RM in window = %v, %v (older and deleted sets must not count)", best, err)
	}
	if none, _ := db.BestE1RM(ctx, u.ID, "pull-up", t0.Add(-time.Hour)); none != nil {
		t.Fatalf("no sets: %v", *none)
	}
	last, err := db.LastSets(ctx, u.ID, "barbell-back-squat", cur.ID)
	if err != nil || len(last) != 1 || last[0].ID != "b" {
		t.Fatalf("last sets = %+v, %v", last, err)
	}

	list, err := db.ListSessions(ctx, u.ID, 10, 0)
	if err != nil || len(list) != 3 || list[0].Name != "current" || list[1].Sets != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if err := db.DeleteSession(ctx, u.ID, prev.ID); err != nil {
		t.Fatal(err)
	}
	if sets, _ := db.SessionSets(ctx, u.ID, prev.ID); len(sets) != 0 {
		t.Fatal("sets of a deleted session remain")
	}
}

func withE1RM(s Set, kg float64) Set {
	s.E1RMKg = &kg
	return s
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/store/`
Expected: FAIL, `undefined: Session`, `db.UpsertSet undefined`.

- [ ] **Step 4: Implement**

`internal/store/sessions.go`: newest-first queries break ties by id (UUIDv7), because sessions started in the same millisecond have equal `started_at`:
```go
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Session is a workout (spec §9).
type Session struct {
	ID            string
	UserID        string
	PlanID        string // "" for ad-hoc sessions
	PlanVersionID string
	Week, Day     int // 0 for ad-hoc sessions
	Name          string
	Snapshot      []byte // plan.ExpandedDay as JSON
	StartedAt     time.Time
	FinishedAt    *time.Time
	Notes         string
	UpdatedAt     time.Time
}

// Set is a logged set. Pointer fields are nil when not recorded.
type Set struct {
	ID          string
	SessionID   string
	UserID      string
	Slug        string
	GroupPos    int
	ExercisePos int
	SetPos      int
	Kind        string
	Prescribed  []byte // plan.PrescribedSet as JSON, nil for unplanned sets
	WeightKg    *float64
	Reps        *int
	RPE         *float64
	DurationS   *int
	DistanceM   *float64
	E1RMKg      *float64
	DoneAt      *time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Outcome of applying a sync operation.
type Outcome string

const (
	Applied   Outcome = "applied"
	Duplicate Outcome = "duplicate" // op_id seen before; nothing changed
	Ignored   Outcome = "ignored"   // older than what is stored, or the set was deleted
)

const sessionColumns = `id, user_id, coalesce(plan_id, ''), coalesce(plan_version_id, ''), week, day, name, snapshot,
	started_at, finished_at, notes, updated_at`

func scanSession(row interface{ Scan(...any) error }) (Session, error) {
	var s Session
	var snapshot, started, updated string
	var finished sql.NullString
	err := row.Scan(&s.ID, &s.UserID, &s.PlanID, &s.PlanVersionID, &s.Week, &s.Day, &s.Name, &snapshot,
		&started, &finished, &s.Notes, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	s.Snapshot = []byte(snapshot)
	if s.StartedAt, err = parseTime(started); err != nil {
		return Session{}, err
	}
	if s.FinishedAt, err = parseNullTime(finished); err != nil {
		return Session{}, err
	}
	s.UpdatedAt, err = parseTime(updated)
	return s, err
}

func parseNullTime(s sql.NullString) (*time.Time, error) {
	if !s.Valid {
		return nil, nil
	}
	t, err := parseTime(s.String)
	return &t, err
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

// CreateSession stores a new session; ID, StartedAt and UpdatedAt are set here.
func (db *DB) CreateSession(ctx context.Context, s Session) (Session, error) {
	var err error
	if s.ID, err = newID(); err != nil {
		return Session{}, err
	}
	s.StartedAt = time.Now().UTC()
	s.UpdatedAt = s.StartedAt
	_, err = db.write.ExecContext(ctx, `INSERT INTO sessions (id, user_id, plan_id, plan_version_id, week, day, name,
		snapshot, started_at, notes, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		s.ID, s.UserID, nullString(s.PlanID), nullString(s.PlanVersionID), s.Week, s.Day, s.Name, string(s.Snapshot),
		formatTime(s.StartedAt), formatTime(s.UpdatedAt))
	return s, err
}

func (db *DB) SessionByID(ctx context.Context, userID, id string) (Session, error) {
	return scanSession(db.read.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = ? AND id = ?`, userID, id))
}

// OpenSession returns the user's most recent unfinished session, or ErrNotFound.
func (db *DB) OpenSession(ctx context.Context, userID string) (Session, error) {
	return scanSession(db.read.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions
		WHERE user_id = ? AND finished_at IS NULL ORDER BY started_at DESC, id DESC LIMIT 1`, userID))
}

// SessionSummary is a session as listed in the history.
type SessionSummary struct {
	Session
	Sets int
}

// ListSessions returns sessions newest first.
func (db *DB) ListSessions(ctx context.Context, userID string, limit, offset int) ([]SessionSummary, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT `+sessionColumns+`,
		(SELECT count(*) FROM sets WHERE sets.session_id = sessions.id AND deleted_at IS NULL)
		FROM sessions WHERE user_id = ? ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var sum SessionSummary
		var snapshot, started, updated string
		var finished sql.NullString
		if err := rows.Scan(&sum.ID, &sum.UserID, &sum.PlanID, &sum.PlanVersionID, &sum.Week, &sum.Day, &sum.Name,
			&snapshot, &started, &finished, &sum.Notes, &updated, &sum.Sets); err != nil {
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
		out = append(out, sum)
	}
	return out, rows.Err()
}

func (db *DB) DeleteSession(ctx context.Context, userID, id string) error {
	res, err := db.write.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return err
	}
	return mustAffect(res)
}

const setColumns = `id, session_id, user_id, slug, group_pos, exercise_pos, set_pos, kind, prescribed,
	weight_kg, reps, rpe, duration_s, distance_m, e1rm_kg, done_at, updated_at, deleted_at`

func scanSet(row interface{ Scan(...any) error }) (Set, error) {
	var s Set
	var prescribed, done, deleted sql.NullString
	var updated string
	var weight, rpe, distance, e1rm sql.NullFloat64
	var reps, duration sql.NullInt64
	err := row.Scan(&s.ID, &s.SessionID, &s.UserID, &s.Slug, &s.GroupPos, &s.ExercisePos, &s.SetPos, &s.Kind,
		&prescribed, &weight, &reps, &rpe, &duration, &distance, &e1rm, &done, &updated, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, err
	}
	if prescribed.Valid {
		s.Prescribed = []byte(prescribed.String)
	}
	s.WeightKg, s.RPE, s.DistanceM, s.E1RMKg = nullFloat(weight), nullFloat(rpe), nullFloat(distance), nullFloat(e1rm)
	s.Reps, s.DurationS = nullInt(reps), nullInt(duration)
	if s.DoneAt, err = parseNullTime(done); err != nil {
		return Set{}, err
	}
	if s.DeletedAt, err = parseNullTime(deleted); err != nil {
		return Set{}, err
	}
	s.UpdatedAt, err = parseTime(updated)
	return s, err
}

func nullFloat(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func nullInt(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int64)
	return &n
}

func scanSets(rows *sql.Rows, err error) ([]Set, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Set
	for rows.Next() {
		s, err := scanSet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SessionSets returns a session's sets that are not deleted, in plan order.
func (db *DB) SessionSets(ctx context.Context, userID, sessionID string) ([]Set, error) {
	return scanSets(db.read.QueryContext(ctx, `SELECT `+setColumns+` FROM sets
		WHERE user_id = ? AND session_id = ? AND deleted_at IS NULL
		ORDER BY group_pos, set_pos, exercise_pos, done_at`, userID, sessionID))
}

// LastSets returns the sets of slug from the most recent other session that has any.
func (db *DB) LastSets(ctx context.Context, userID, slug, excludeSessionID string) ([]Set, error) {
	return scanSets(db.read.QueryContext(ctx, `SELECT `+setColumns+` FROM sets WHERE session_id = (
			SELECT s.session_id FROM sets s JOIN sessions ss ON ss.id = s.session_id
			WHERE s.user_id = ? AND s.slug = ? AND s.deleted_at IS NULL AND s.session_id != ?
			ORDER BY ss.started_at DESC, ss.id DESC LIMIT 1)
		AND slug = ? AND deleted_at IS NULL ORDER BY set_pos, done_at`, userID, slug, excludeSessionID, slug))
}

// BestE1RM is the highest e1RM of slug logged since since, or nil.
func (db *DB) BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error) {
	var best sql.NullFloat64
	err := db.read.QueryRowContext(ctx, `SELECT max(e1rm_kg) FROM sets WHERE user_id = ? AND slug = ?
		AND deleted_at IS NULL AND done_at >= ?`, userID, slug, formatTime(since)).Scan(&best)
	return nullFloat(best), err
}

// recordOp runs apply once per opID ("" = no dedupe) inside a transaction.
func (db *DB) recordOp(ctx context.Context, userID, opID string, apply func(*sql.Tx) (Outcome, error)) (Outcome, error) {
	var out Outcome
	err := db.tx(ctx, func(tx *sql.Tx) error {
		if opID != "" {
			var prev string
			err := tx.QueryRowContext(ctx, `SELECT result FROM applied_ops WHERE op_id = ? AND user_id = ?`, opID, userID).Scan(&prev)
			if err == nil {
				out = Duplicate
				return nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		var err error
		if out, err = apply(tx); err != nil {
			return err
		}
		if opID == "" {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO applied_ops (op_id, user_id, result, applied_at) VALUES (?, ?, ?, ?)`,
			opID, userID, string(out), formatTime(time.Now()))
		return err
	})
	return out, err
}

func ownSession(ctx context.Context, tx *sql.Tx, userID, sessionID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id = ? AND user_id = ?`, sessionID, userID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// UpsertSet creates or updates a set. The newer UpdatedAt wins; a deleted set
// stays deleted. A set of another user or session is ErrNotFound.
func (db *DB) UpsertSet(ctx context.Context, userID string, s Set, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, s.SessionID); err != nil {
			return "", err
		}
		existing, err := scanSet(tx.QueryRowContext(ctx, `SELECT `+setColumns+` FROM sets WHERE id = ?`, s.ID))
		switch {
		case errors.Is(err, ErrNotFound):
			_, err = tx.ExecContext(ctx, `INSERT INTO sets (`+setColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`,
				s.ID, s.SessionID, userID, s.Slug, s.GroupPos, s.ExercisePos, s.SetPos, s.Kind, nullBytes(s.Prescribed),
				s.WeightKg, s.Reps, s.RPE, s.DurationS, s.DistanceM, s.E1RMKg, nullTime(s.DoneAt), formatTime(s.UpdatedAt))
			return Applied, err
		case err != nil:
			return "", err
		case existing.UserID != userID || existing.SessionID != s.SessionID:
			return "", ErrNotFound
		case existing.DeletedAt != nil || !s.UpdatedAt.After(existing.UpdatedAt):
			return Ignored, nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE sets SET slug = ?, group_pos = ?, exercise_pos = ?, set_pos = ?, kind = ?,
			prescribed = ?, weight_kg = ?, reps = ?, rpe = ?, duration_s = ?, distance_m = ?, e1rm_kg = ?, done_at = ?,
			updated_at = ? WHERE id = ?`,
			s.Slug, s.GroupPos, s.ExercisePos, s.SetPos, s.Kind, nullBytes(s.Prescribed), s.WeightKg, s.Reps, s.RPE,
			s.DurationS, s.DistanceM, s.E1RMKg, nullTime(s.DoneAt), formatTime(s.UpdatedAt), s.ID)
		return Applied, err
	})
}

func nullBytes(b []byte) sql.NullString {
	return sql.NullString{String: string(b), Valid: b != nil}
}

// DeleteSet soft-deletes a set of the user's session.
func (db *DB) DeleteSet(ctx context.Context, userID, sessionID, setID string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sets SET deleted_at = ?, updated_at = ? WHERE id = ? AND session_id = ?
			AND user_id = ? AND deleted_at IS NULL`, formatTime(at), formatTime(at), setID, sessionID, userID)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Deleting a set the server never saw: remember it as deleted so
			// its late upsert cannot create it.
			if _, err := tx.ExecContext(ctx, `INSERT INTO sets (id, session_id, user_id, slug, group_pos, exercise_pos,
				set_pos, kind, updated_at, deleted_at) VALUES (?, ?, ?, '', 0, 0, 0, 'working', ?, ?)
				ON CONFLICT (id) DO NOTHING`, setID, sessionID, userID, formatTime(at), formatTime(at)); err != nil {
				return "", err
			}
			return Ignored, nil
		}
		return Applied, nil
	})
}

// SetSessionNotes replaces the notes when at is newer than the last change.
func (db *DB) SetSessionNotes(ctx context.Context, userID, sessionID, notes string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sessions SET notes = ?, updated_at = ? WHERE id = ? AND updated_at < ?`,
			notes, formatTime(at), sessionID, formatTime(at))
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Ignored, nil
		}
		return Applied, nil
	})
}

// FinishSession marks the session finished at at. Finishing an already
// finished session is Ignored, so callers act on Applied only once.
func (db *DB) FinishSession(ctx context.Context, userID, sessionID string, at time.Time, opID string) (Outcome, error) {
	return db.recordOp(ctx, userID, opID, func(tx *sql.Tx) (Outcome, error) {
		if err := ownSession(ctx, tx, userID, sessionID); err != nil {
			return "", err
		}
		res, err := tx.ExecContext(ctx, `UPDATE sessions SET finished_at = ? WHERE id = ? AND finished_at IS NULL`,
			formatTime(at), sessionID)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Ignored, nil
		}
		return Applied, nil
	})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/store/ && golangci-lint run ./internal/...`
Expected: `ok`, `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/store
git commit -m "feat(store): add sessions and sets with idempotent sync writes"
```

---

### Task 2: Planning uses logged history

**Files:**
- Replace: `internal/plan/service.go`, `internal/plan/service_test.go`

**Interfaces:**
- Consumes: `(*store.DB).BestE1RM`, `store.Session`, `UpsertSet` (Task 1, used by the tests)
- Produces:
  - `plan.History` interface (`BestE1RM`), the new field `plan.Service.History`. When set, RPE loads resolve from the best e1RM within the user's `E1RMWindowDays`.
  - `(*plan.Service).AdvanceFrom(ctx, user, planID string, week, day int) error`: moves the cursor past `(week, day)` only if the user follows `planID` and the cursor is on that day.

- [ ] **Step 1: Write the failing tests**

Replace `internal/plan/service_test.go` with (it adds `TestRPELoadsUseLoggedE1RM` and `TestAdvanceFrom`, and wires `History: db`):
```go
package plan

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	return env{svc: &Service{Store: db, Exercises: ex, History: db}, ex: ex, alice: alice, bob: bob}
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

// Weeks often repeat a day name (A/B/A); each occurrence is compared on its own.
func TestCompareRepeatedDayNames(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	doc := func(thirdSets int) []byte {
		return []byte(`{"name": "ABA", "weeks": 1, "days": [
			{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5}]}]}]},
			{"name": "B", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": 5}]}]}]},
			{"name": "A", "groups": [{"exercises": [{"slug": "barbell-back-squat", "sets": [{"count": ` + string(rune('0'+thirdSets)) + `, "reps": 5}]}]}]}]}`)
	}
	p, _, err := e.svc.Create(ctx, e.alice, doc(3), SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	draft, _, err := e.svc.Save(ctx, e.alice, p.ID, doc(5), SaveDraft, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	cmp, err := e.svc.Compare(ctx, e.alice, draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.Days) != 1 || cmp.Days[0].Name != "A (2nd)" || cmp.Days[0].Week != 1 {
		t.Fatalf("changed days = %+v", cmp.Days)
	}
}

// The plan's name is the active version's name: drafts (from the editor or,
// later, the AI) must not rename a plan the user is training.
func TestPlanNameFollowsTheActiveVersion(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	named := func(name string) []byte {
		return []byte(strings.Replace(string(weeksDoc(2)), `"name": "Test"`, `"name": "`+name+`"`, 1))
	}
	p, v1, err := e.svc.Create(ctx, e.alice, named("Block A"), SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	name := func() string {
		got, _, err := e.svc.Plan(ctx, e.alice, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Name
	}
	draft, _, err := e.svc.Save(ctx, e.alice, p.ID, named("Block B idea"), SaveDraft, "mcp", "")
	if err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block A" {
		t.Fatalf("a draft renamed the plan to %q", n)
	}
	if _, err := e.svc.Activate(ctx, e.alice, draft.ID); err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block B idea" {
		t.Fatalf("after activating the draft: %q", n)
	}
	if _, err := e.svc.Activate(ctx, e.alice, v1.ID); err != nil {
		t.Fatal(err)
	}
	if n := name(); n != "Block A" {
		t.Fatalf("after rolling back: %q", n)
	}
}

// Logged sets give RPE targets a weight: the best e1RM in the user's window.
func TestRPELoadsUseLoggedE1RM(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	raw := []byte(`{"name": "R", "weeks": 1, "days": [{"name": "A", "groups": [{"exercises": [
		{"slug": "barbell-back-squat", "sets": [{"count": 3, "reps": 5, "load": {"rpe": 8}}]}]}]}]}`)
	p, _, err := e.svc.Create(ctx, e.alice, raw, SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = e.svc.Follow(ctx, e.alice, p.ID)
	n, _ := e.svc.Next(ctx, e.alice)
	if n.Today.Groups[0].Exercises[0].Sets[0].Kg != nil {
		t.Fatal("without history the RPE weight must be left open")
	}

	db := e.svc.Store.(*store.DB)
	s, _ := db.CreateSession(ctx, store.Session{UserID: e.alice.ID, Name: "log", Snapshot: []byte(`{}`)})
	kg, reps, e1rm, now := 120.0, 5, 150.0, time.Now()
	if _, err := db.UpsertSet(ctx, e.alice.ID, store.Set{ID: "s1", SessionID: s.ID, Slug: "barbell-back-squat", Kind: "working",
		WeightKg: &kg, Reps: &reps, E1RMKg: &e1rm, DoneAt: &now, UpdatedAt: now}, ""); err != nil {
		t.Fatal(err)
	}
	n, _ = e.svc.Next(ctx, e.alice)
	// 5 @ RPE 8 = 81.1% of 150 = 121.65 -> 120 on the starter barbell.
	if got := n.Today.Groups[0].Exercises[0].Sets[0].Kg; got == nil || *got != 120 {
		t.Fatalf("RPE weight = %v", got)
	}
}

func TestAdvanceFrom(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	p, _, _ := e.svc.Create(ctx, e.alice, weeksDoc(2), SaveActivate, "web", "")
	_ = e.svc.Follow(ctx, e.alice, p.ID)

	// Finishing a day the cursor is not on (an older session) changes nothing.
	if err := e.svc.AdvanceFrom(ctx, e.alice, p.ID, 2, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Week != 1 || n.Day != 0 {
		t.Fatalf("cursor moved: (%d, %d)", n.Week, n.Day)
	}
	if err := e.svc.AdvanceFrom(ctx, e.alice, p.ID, 1, 0); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Week != 1 || n.Day != 1 {
		t.Fatalf("cursor after finishing day 1: (%d, %d)", n.Week, n.Day)
	}
	if err := e.svc.AdvanceFrom(ctx, e.alice, "other-plan", 1, 1); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.svc.Next(ctx, e.alice); n.Day != 1 {
		t.Fatal("another plan's session moved the cursor")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/plan/`
Expected: FAIL, `unknown field History in struct literal of type Service`, `e.svc.AdvanceFrom undefined`.

- [ ] **Step 3: Implement**

Replace `internal/plan/service.go` with:
```go
package plan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

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
	ActivateVersion(ctx context.Context, userID, versionID, name string) error
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

// History supplies estimated 1RMs from logged sets. *store.DB implements it.
type History interface {
	BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error)
}

// Service implements plan rules on top of the store.
type Service struct {
	Store     Store
	Exercises Exercises
	History   History // optional: without it RPE loads stay unresolved
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
	e1rm     *float64
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
		if c.svc.History != nil {
			since := time.Now().AddDate(0, 0, -c.user.E1RMWindowDays)
			if best, err := c.svc.History.BestE1RM(c.ctx, c.user.ID, slug, since); err == nil {
				i.e1rm = best
			}
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
	i := c.info(slug)
	st := i.settings
	ctx := calc.LoadContext{TMKg: st.TrainingMaxKg, E1RMKg: i.e1rm, Unit: c.user.Unit}
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
	if err := s.Store.ActivateVersion(ctx, user.ID, versionID, doc.Name); err != nil {
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
		keys := dayKeys(baseDoc, w)
		for _, k := range dayKeys(targetDoc, w) {
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		for _, k := range keys {
			a := dayLinesByKey(c, baseDoc, w, k)
			b := dayLinesByKey(c, targetDoc, w, k)
			if d := DiffLines(a, b); Changed(d) {
				cmp.Days = append(cmp.Days, DayChange{Week: w, Name: k.label(), Lines: d})
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

// dayKey identifies a day within a week across versions: its name and which
// occurrence of that name it is (weeks often repeat names, like A/B/A).
type dayKey struct {
	name string
	nth  int // 1 for the first day with this name in the week
}

func (k dayKey) label() string {
	if k.nth == 1 {
		return k.name
	}
	suffix := "th"
	switch k.nth {
	case 2:
		suffix = "nd"
	case 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%s (%d%s)", k.name, k.nth, suffix)
}

func dayKeys(doc Doc, week int) []dayKey {
	if week > doc.Weeks {
		return nil
	}
	var out []dayKey
	seen := map[string]int{}
	for _, i := range DaysForWeek(doc, week) {
		name := doc.Days[i].Name
		seen[name]++
		out = append(out, dayKey{name, seen[name]})
	}
	return out
}

func dayLinesByKey(c *catalog, doc Doc, week int, k dayKey) []string {
	for d, key := range dayKeys(doc, week) {
		if key == k {
			day, _ := c.day(doc, week, d)
			return DayLines(day, c.user.Unit)
		}
	}
	return nil
}

// AdvanceFrom moves the cursor past (week, day) of planID after that day was
// trained, if the user follows planID and the cursor is still on that day.
func (s *Service) AdvanceFrom(ctx context.Context, user store.User, planID string, week, day int) error {
	a, err := s.Store.ActivePlan(ctx, user.ID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && (a.PlanID != planID || a.Week != week || a.Day != day)) {
		return nil
	}
	if err != nil {
		return err
	}
	v, err := s.Store.ActivePlanVersion(ctx, user.ID, planID)
	if err != nil {
		return err
	}
	doc, err := Decode(v.Doc)
	if err != nil {
		return err
	}
	w, d := NextPosition(doc, week, day)
	return s.Store.SetActivePlan(ctx, user.ID, store.ActivePlan{PlanID: planID, Week: w, Day: d})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/plan/ && golangci-lint run ./internal/plan/`
Expected: `ok`, `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/plan
git commit -m "feat(plan): resolve RPE loads from logged e1RM and advance after training"
```

---

### Task 3: Training service

**Files:**
- Create: `internal/training/service.go`, `internal/training/bootstrap.go`, `internal/training/history.go`
- Test: `internal/training/service_test.go`

**Interfaces:**
- Consumes: store (Task 1), `plan.Service.Next` and `AdvanceFrom` (Task 2), `exercise.Service` (`Get`, `Catalog`, `Settings`, `Alternatives`, `ListEquipment`)
- Produces:
  - Setup and errors: `training.Service{Store, Plans, Exercises; Now func() time.Time}`, `training.ErrNothingToStart`, `training.OpenSessionError{ID}`, `training.InvalidError{Reason}`
  - Starting sessions: `StartPlanned(ctx, user) (store.Session, error)`, `StartAdHoc`, `Open(ctx, user)`
  - Sync:
    - `training.SetInput`: JSON with `id`, `session_id`, `slug`, `group_pos`, `exercise_pos`, `set_pos`, `kind`, `prescribed`, `weight_kg`, `reps`, `rpe`, `duration_s`, `distance_m`, `done_at`, `updated_at`
    - `training.Op{OpID, Op string; Payload json.RawMessage; ClientTS time.Time}`, `training.OpResult{OpID, Status, Reason}`, `training.MaxOps = 200`
    - operation names `training.OpUpsertSet|OpDeleteSet|OpEditNotes|OpFinishSession`
    - `ApplyOps(ctx, user, []Op) ([]OpResult, error)`
  - Bootstrap: `Bootstrap(ctx, user, sessionID) (Bootstrap, error)`, with `training.Bootstrap{Session BootSession; Snapshot plan.ExpandedDay; Sets []SetInput; Exercises map[string]BootExercise; Catalog []CatalogEntry; Defaults map[string]*calc.Equipment ("equipment_defaults"); Unit string}`
  - History: `History(ctx, user, page) ([]store.SessionSummary, bool, error)`, `Session(ctx, user, id) (store.Session, []store.Set, error)`, `SaveSet(ctx, user, SetInput)`, `DeleteSet(ctx, user, sessionID, setID)`, `SetNotes`, `Finish`, `Delete`

- [ ] **Step 1: Write the failing tests**

`internal/training/service_test.go`:
```go
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc   *Service
	plans *plan.Service
	ex    *exercise.Service
	db    *store.DB
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
	plans := &plan.Service{Store: db, Exercises: ex, History: db}
	alice, _ := db.UpsertOIDCUser(ctx, "iss", "alice", "", "alice")
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	for _, u := range []store.User{alice, bob} {
		if err := ex.EnsureStarterEquipment(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	return env{svc: &Service{Store: db, Plans: plans, Exercises: ex}, plans: plans, ex: ex, db: db, alice: alice, bob: bob}
}

const twoDays = `{"name": "Test", "weeks": 2, "days": [
	{"name": "Squat day", "groups": [{"rest_s": 180, "exercises": [{"slug": "barbell-back-squat", "sets": [
		{"count": 3, "reps": 5, "load": {"pct_tm": 0.75}}]}]}]},
	{"name": "Pull day", "groups": [{"exercises": [{"slug": "pull-up", "sets": [{"count": 3, "reps": "6-10"}]}]}]}]}`

// follow creates, activates and follows the two-day plan with a 140 kg squat TM.
func (e env) follow(t *testing.T) store.Plan {
	t.Helper()
	ctx := context.Background()
	p, _, err := e.plans.Create(ctx, e.alice, []byte(twoDays), plan.SaveActivate, "web", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.plans.Follow(ctx, e.alice, p.ID); err != nil {
		t.Fatal(err)
	}
	tm := 140.0
	if err := e.ex.SetTrainingMax(ctx, e.alice.ID, "barbell-back-squat", &tm, "web"); err != nil {
		t.Fatal(err)
	}
	return p
}

var opSeq int

// opID returns a fresh UUID-shaped operation id.
func opID() string {
	opSeq++
	return fmt.Sprintf("00000000-0000-7000-8000-%012d", opSeq)
}

func setOp(id, sessionID, slug string, kg float64, reps int, rpe float64, updated time.Time) Op {
	in := SetInput{ID: id, SessionID: sessionID, Slug: slug, Kind: "working", WeightKg: &kg, Reps: &reps, DoneAt: &updated, UpdatedAt: updated}
	if rpe > 0 {
		in.RPE = &rpe
	}
	payload, _ := json.Marshal(in)
	return Op{OpID: opID(), Op: OpUpsertSet, Payload: payload}
}

func payload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestStartSessions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if _, err := e.svc.StartPlanned(ctx, e.alice); !errors.Is(err, ErrNothingToStart) {
		t.Fatalf("no plan: %v", err)
	}
	e.follow(t)
	sess, err := e.svc.StartPlanned(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}
	var day plan.ExpandedDay
	if err := json.Unmarshal(sess.Snapshot, &day); err != nil {
		t.Fatal(err)
	}
	if sess.Name != "Squat day" || sess.Week != 1 || sess.Day != 0 || day.Groups[0].Exercises[0].Sets[0].Kg == nil ||
		*day.Groups[0].Exercises[0].Sets[0].Kg != 105 {
		t.Fatalf("session = %+v, snapshot = %+v", sess, day)
	}
	var open OpenSessionError
	if _, err := e.svc.StartPlanned(ctx, e.alice); !errors.As(err, &open) || open.ID != sess.ID {
		t.Fatalf("second start: %v", err)
	}
	if _, err := e.svc.StartAdHoc(ctx, e.alice); !errors.As(err, &open) {
		t.Fatalf("ad-hoc while open: %v", err)
	}
	adhoc, err := e.svc.StartAdHoc(ctx, e.bob)
	if err != nil || adhoc.PlanID != "" || adhoc.Name != "Workout" {
		t.Fatalf("bob's ad-hoc session = %+v, %v", adhoc, err)
	}
}

func TestApplyOps(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	sess, _ := e.svc.StartPlanned(ctx, e.alice)
	bobs, _ := e.svc.StartAdHoc(ctx, e.bob)
	now := time.Now().UTC()
	setID := "01900000-0000-7000-8000-000000000001"

	first := setOp(setID, sess.ID, "barbell-back-squat", 100, 5, 8, now)
	ops := []Op{
		first,
		{OpID: first.OpID, Op: first.Op, Payload: first.Payload}, // replayed
		setOp("not-a-uuid", sess.ID, "barbell-back-squat", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000002", sess.ID, "no-such-exercise", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000003", sess.ID, "barbell-back-squat", 5000, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000004", bobs.ID, "barbell-back-squat", 100, 5, 0, now),
		setOp("01900000-0000-7000-8000-000000000005", sess.ID, "pull-up", 10, 8, 8, now),
		{OpID: opID(), Op: OpEditNotes, Payload: payload(map[string]any{"session_id": sess.ID, "notes": "good day", "updated_at": now.Add(time.Second)})},
		{OpID: opID(), Op: "explode", Payload: payload(map[string]any{})},
	}
	results, err := e.svc.ApplyOps(ctx, e.alice, ops)
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	for _, r := range results {
		statuses = append(statuses, r.Status)
	}
	want := []string{"applied", "duplicate", "rejected", "rejected", "rejected", "rejected", "applied", "applied", "rejected"}
	if !slices.Equal(statuses, want) {
		t.Fatalf("statuses = %v\nresults = %+v", statuses, results)
	}

	sets, _ := e.db.SessionSets(ctx, e.alice.ID, sess.ID)
	if len(sets) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	// 100 x 5 @ RPE 8 = 100 / 0.811; pull-ups are bodyweight, so no e1RM.
	for _, s := range sets {
		switch s.Slug {
		case "barbell-back-squat":
			if s.E1RMKg == nil || *s.E1RMKg < 123.3 || *s.E1RMKg > 123.31 {
				t.Fatalf("squat e1RM = %v", s.E1RMKg)
			}
		case "pull-up":
			if s.E1RMKg != nil {
				t.Fatalf("pull-up e1RM = %v", *s.E1RMKg)
			}
		}
	}
	if got, _ := e.db.SessionByID(ctx, e.alice.ID, sess.ID); got.Notes != "good day" {
		t.Fatalf("notes = %q", got.Notes)
	}

	tooMany := make([]Op, MaxOps+1)
	var invalid InvalidError
	if _, err := e.svc.ApplyOps(ctx, e.alice, tooMany); !errors.As(err, &invalid) {
		t.Fatalf("oversized batch: %v", err)
	}
}

func TestFinishAdvancesThePlanOnce(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	sess, _ := e.svc.StartPlanned(ctx, e.alice)
	finish := func() {
		res, err := e.svc.ApplyOps(ctx, e.alice, []Op{{OpID: opID(), Op: OpFinishSession,
			Payload: payload(map[string]any{"session_id": sess.ID, "finished_at": time.Now().UTC()})}})
		if err != nil || res[0].Status != "applied" {
			t.Fatalf("finish: %+v %v", res, err)
		}
	}
	finish()
	finish() // a second device, or a retry with a new op id
	n, _ := e.plans.Next(ctx, e.alice)
	if n.Week != 1 || n.Day != 1 {
		t.Fatalf("cursor = (%d, %d), want (1, 1)", n.Week, n.Day)
	}
	if _, err := e.svc.Open(ctx, e.alice); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("finished session still open")
	}
}

func TestFutureTimestampsAreClamped(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	future := time.Now().Add(1000 * time.Hour)
	if _, err := e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-000000000009", sess.ID, "barbell-back-squat", 100, 5, 0, future)}); err != nil {
		t.Fatal(err)
	}
	sets, _ := e.db.SessionSets(ctx, e.alice.ID, sess.ID)
	if len(sets) != 1 || sets[0].UpdatedAt.After(time.Now().Add(6*time.Minute)) {
		t.Fatalf("updated_at not clamped: %+v", sets)
	}
}

func TestBootstrap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.follow(t)
	prev, _ := e.svc.StartPlanned(ctx, e.alice)
	then := time.Now().UTC().Add(-time.Hour)
	_, _ = e.svc.ApplyOps(ctx, e.alice, []Op{setOp("01900000-0000-7000-8000-00000000000a", prev.ID, "barbell-back-squat", 110, 5, 9, then)})
	_ = e.svc.Finish(ctx, e.alice, prev.ID)
	_ = e.plans.Choose(ctx, e.alice, 1, 0)
	sess, err := e.svc.StartPlanned(ctx, e.alice)
	if err != nil {
		t.Fatal(err)
	}

	b, err := e.svc.Bootstrap(ctx, e.alice, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	squat, ok := b.Exercises["barbell-back-squat"]
	if !ok || squat.Name != "Barbell Back Squat" || squat.TMKg == nil || *squat.TMKg != 140 || squat.E1RMKg == nil ||
		squat.Equipment == nil || len(squat.Last) != 1 || *squat.Last[0].WeightKg != 110 {
		t.Fatalf("squat = %+v", squat)
	}
	if !slices.Contains(squat.Alternatives, "hack-squat") {
		t.Fatalf("catalog alternatives missing: %v", squat.Alternatives)
	}
	if _, ok := b.Exercises["hack-squat"]; !ok {
		t.Fatal("an alternative's context is needed to swap offline")
	}
	if len(b.Catalog) < 100 || b.Defaults["barbell"] == nil || b.Defaults["dumbbell"] == nil || b.Unit != "kg" {
		t.Fatalf("catalog %d, defaults %v, unit %q", len(b.Catalog), b.Defaults, b.Unit)
	}
	if b.Session.ID != sess.ID || b.Snapshot.Name != "Squat day" || len(b.Sets) != 0 {
		t.Fatalf("session part = %+v", b.Session)
	}
	if _, err := e.svc.Bootstrap(ctx, e.bob, sess.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob bootstraps alice's session: %v", err)
	}
}

func TestHistoryEditing(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sess, _ := e.svc.StartAdHoc(ctx, e.alice)
	id := "01900000-0000-7000-8000-00000000000b"
	kg, reps := 60.0, 8
	in := SetInput{ID: id, SessionID: sess.ID, Slug: "barbell-bench-press", Kind: "working", WeightKg: &kg, Reps: &reps}
	if err := e.svc.SaveSet(ctx, e.alice, in); err != nil {
		t.Fatal(err)
	}
	kg = 62.5
	if err := e.svc.SaveSet(ctx, e.alice, in); err != nil {
		t.Fatal(err)
	}
	_, sets, _ := e.svc.Session(ctx, e.alice, sess.ID)
	if len(sets) != 1 || *sets[0].WeightKg != 62.5 {
		t.Fatalf("sets = %+v", sets)
	}
	var invalid InvalidError
	reps = -1
	if err := e.svc.SaveSet(ctx, e.alice, in); !errors.As(err, &invalid) {
		t.Fatalf("negative reps: %v", err)
	}
	if err := e.svc.SetNotes(ctx, e.alice, sess.ID, "note"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeleteSet(ctx, e.alice, sess.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, sets, _ := e.svc.Session(ctx, e.alice, sess.ID); len(sets) != 0 {
		t.Fatal("set not deleted")
	}
	if err := e.svc.Finish(ctx, e.alice, sess.ID); err != nil {
		t.Fatal(err)
	}
	list, more, err := e.svc.History(ctx, e.alice, 0)
	if err != nil || more || len(list) != 1 || list[0].FinishedAt == nil {
		t.Fatalf("history = %+v %v %v", list, more, err)
	}
	if err := e.svc.Delete(ctx, e.bob, sess.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bob deletes alice's session: %v", err)
	}
	if err := e.svc.Delete(ctx, e.alice, sess.ID); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/training/`
Expected: FAIL, `undefined: Service`, `undefined: Op`.

- [ ] **Step 3: Implement**

`internal/training/service.go`:
```go
// Package training runs workouts: starting sessions, applying the companion's
// offline operations (spec §9), and editing logged history.
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	CreateSession(ctx context.Context, s store.Session) (store.Session, error)
	SessionByID(ctx context.Context, userID, id string) (store.Session, error)
	OpenSession(ctx context.Context, userID string) (store.Session, error)
	ListSessions(ctx context.Context, userID string, limit, offset int) ([]store.SessionSummary, error)
	DeleteSession(ctx context.Context, userID, id string) error
	SessionSets(ctx context.Context, userID, sessionID string) ([]store.Set, error)
	LastSets(ctx context.Context, userID, slug, excludeSessionID string) ([]store.Set, error)
	BestE1RM(ctx context.Context, userID, slug string, since time.Time) (*float64, error)
	UpsertSet(ctx context.Context, userID string, s store.Set, opID string) (store.Outcome, error)
	DeleteSet(ctx context.Context, userID, sessionID, setID string, at time.Time, opID string) (store.Outcome, error)
	SetSessionNotes(ctx context.Context, userID, sessionID, notes string, at time.Time, opID string) (store.Outcome, error)
	FinishSession(ctx context.Context, userID, sessionID string, at time.Time, opID string) (store.Outcome, error)
}

// Plans is what training needs from plans. *plan.Service implements it.
type Plans interface {
	Next(ctx context.Context, user store.User) (*plan.Next, error)
	AdvanceFrom(ctx context.Context, user store.User, planID string, week, day int) error
}

// Exercises is what training needs from the catalog. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
	Catalog(ctx context.Context, userID, query string) ([]store.Exercise, error)
	Settings(ctx context.Context, userID string, ex store.Exercise) (exercise.Settings, error)
	Alternatives(ctx context.Context, userID, slug string) ([]exercise.AlternativeView, error)
	ListEquipment(ctx context.Context, userID string) ([]store.Equipment, error)
}

type Service struct {
	Store     Store
	Plans     Plans
	Exercises Exercises
	Now       func() time.Time // defaults to time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Errors.
var (
	// ErrNothingToStart: the user follows no plan, or has completed it.
	ErrNothingToStart = errors.New("no planned day to start")
)

// OpenSessionError is returned when starting while a session is still open.
type OpenSessionError struct{ ID string }

func (e OpenSessionError) Error() string { return "a workout is already in progress" }

func (s *Service) ensureNoOpen(ctx context.Context, user store.User) error {
	open, err := s.Store.OpenSession(ctx, user.ID)
	if err == nil {
		return OpenSessionError{ID: open.ID}
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}

// StartPlanned starts the next day of the followed plan, snapshotting its
// resolved prescription.
func (s *Service) StartPlanned(ctx context.Context, user store.User) (store.Session, error) {
	if err := s.ensureNoOpen(ctx, user); err != nil {
		return store.Session{}, err
	}
	n, err := s.Plans.Next(ctx, user)
	if err != nil {
		return store.Session{}, err
	}
	if n == nil || n.Complete {
		return store.Session{}, ErrNothingToStart
	}
	snapshot, err := json.Marshal(n.Today)
	if err != nil {
		return store.Session{}, err
	}
	return s.Store.CreateSession(ctx, store.Session{
		UserID: user.ID, PlanID: n.Plan.ID, PlanVersionID: n.Version.ID,
		Week: n.Week, Day: n.Day, Name: n.Today.Name, Snapshot: snapshot,
	})
}

// StartAdHoc starts an empty workout; exercises are added as it goes.
func (s *Service) StartAdHoc(ctx context.Context, user store.User) (store.Session, error) {
	if err := s.ensureNoOpen(ctx, user); err != nil {
		return store.Session{}, err
	}
	snapshot, _ := json.Marshal(plan.ExpandedDay{Name: "Workout", Groups: []plan.ExpandedGroup{}})
	return s.Store.CreateSession(ctx, store.Session{UserID: user.ID, Name: "Workout", Snapshot: snapshot})
}

// Open returns the user's unfinished session, or ErrNotFound.
func (s *Service) Open(ctx context.Context, user store.User) (store.Session, error) {
	return s.Store.OpenSession(ctx, user.ID)
}

// SetInput is a set as sent by the companion or the history editor. Weights are kg.
type SetInput struct {
	ID          string          `json:"id"`
	SessionID   string          `json:"session_id"`
	Slug        string          `json:"slug"`
	GroupPos    int             `json:"group_pos"`
	ExercisePos int             `json:"exercise_pos"`
	SetPos      int             `json:"set_pos"`
	Kind        string          `json:"kind"`
	Prescribed  json.RawMessage `json:"prescribed,omitempty"`
	WeightKg    *float64        `json:"weight_kg"`
	Reps        *int            `json:"reps"`
	RPE         *float64        `json:"rpe"`
	DurationS   *int            `json:"duration_s"`
	DistanceM   *float64        `json:"distance_m"`
	DoneAt      *time.Time      `json:"done_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// InvalidError is returned for values a set or operation cannot have.
type InvalidError struct{ Reason string }

func (e InvalidError) Error() string { return e.Reason }

var idRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func finite(v *float64) bool { return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0)) }

func (in SetInput) validate() error {
	switch {
	case !idRE.MatchString(in.ID):
		return InvalidError{"set id must be a UUID"}
	case in.Kind != "warmup" && in.Kind != "working" && in.Kind != "drop" && in.Kind != "amrap":
		return InvalidError{"unknown set kind " + in.Kind}
	case !finite(in.WeightKg) || !finite(in.RPE) || !finite(in.DistanceM):
		return InvalidError{"numbers must be finite"}
	case in.WeightKg != nil && (*in.WeightKg < 0 || *in.WeightKg > 2000):
		return InvalidError{"weight out of range"}
	case in.Reps != nil && (*in.Reps < 0 || *in.Reps > 1000):
		return InvalidError{"reps out of range"}
	case in.RPE != nil && (*in.RPE < 1 || *in.RPE > 10):
		return InvalidError{"RPE must be between 1 and 10"}
	case in.DurationS != nil && (*in.DurationS < 0 || *in.DurationS > 86400):
		return InvalidError{"duration out of range"}
	case in.DistanceM != nil && (*in.DistanceM < 0 || *in.DistanceM > 1e6):
		return InvalidError{"distance out of range"}
	case in.GroupPos < 0 || in.ExercisePos < 0 || in.SetPos < 0 || in.GroupPos > 1000 || in.SetPos > 1000 || in.ExercisePos > 100:
		return InvalidError{"position out of range"}
	case len(in.Prescribed) > 4096:
		return InvalidError{"prescription too large"}
	}
	return nil
}

// toStore validates in and derives the e1RM from the exercise's measurement.
func (s *Service) toStore(ctx context.Context, user store.User, in SetInput) (store.Set, error) {
	if err := in.validate(); err != nil {
		return store.Set{}, err
	}
	ex, err := s.Exercises.Get(ctx, user.ID, in.Slug)
	if errors.Is(err, store.ErrNotFound) {
		return store.Set{}, InvalidError{"unknown exercise " + in.Slug}
	}
	if err != nil {
		return store.Set{}, err
	}
	set := store.Set{
		ID: in.ID, SessionID: in.SessionID, Slug: in.Slug, GroupPos: in.GroupPos, ExercisePos: in.ExercisePos,
		SetPos: in.SetPos, Kind: in.Kind, WeightKg: in.WeightKg, Reps: in.Reps, RPE: in.RPE,
		DurationS: in.DurationS, DistanceM: in.DistanceM, DoneAt: in.DoneAt, UpdatedAt: s.clamp(in.UpdatedAt),
	}
	if len(in.Prescribed) > 0 && string(in.Prescribed) != "null" {
		set.Prescribed = []byte(in.Prescribed)
	}
	if ex.Measurement == "weight_reps" && in.WeightKg != nil && in.Reps != nil {
		rpe := 0.0
		if in.RPE != nil {
			rpe = *in.RPE
		}
		if e1rm, _, ok := calc.E1RM(*in.WeightKg, *in.Reps, rpe); ok {
			set.E1RMKg = &e1rm
		}
	}
	return set, nil
}

// clamp keeps client clocks from pushing timestamps into the future (spec §9).
func (s *Service) clamp(t time.Time) time.Time {
	if limit := s.now().Add(5 * time.Minute); t.IsZero() || t.After(limit) {
		if t.IsZero() {
			return s.now()
		}
		return limit
	}
	return t
}

// Op is one queued companion operation (spec §9).
type Op struct {
	OpID     string          `json:"op_id"`
	Op       string          `json:"op"`
	Payload  json.RawMessage `json:"payload"`
	ClientTS time.Time       `json:"client_ts"`
}

// OpResult reports what happened to an operation.
type OpResult struct {
	OpID   string `json:"op_id"`
	Status string `json:"status"` // applied, duplicate or rejected
	Reason string `json:"reason,omitempty"`
}

// MaxOps bounds one sync request.
const MaxOps = 200

// Operation names.
const (
	OpUpsertSet     = "upsert_set"
	OpDeleteSet     = "delete_set"
	OpEditNotes     = "edit_notes"
	OpFinishSession = "finish_session"
)

// ApplyOps applies operations in order. A rejected operation does not stop
// the ones after it; the client keeps rejected ones for the user to see.
func (s *Service) ApplyOps(ctx context.Context, user store.User, ops []Op) ([]OpResult, error) {
	if len(ops) > MaxOps {
		return nil, InvalidError{fmt.Sprintf("at most %d operations per request", MaxOps)}
	}
	results := make([]OpResult, 0, len(ops))
	for _, op := range ops {
		res := OpResult{OpID: op.OpID}
		outcome, err := s.applyOp(ctx, user, op)
		var invalid InvalidError
		switch {
		case errors.As(err, &invalid):
			res.Status, res.Reason = "rejected", invalid.Reason
		case errors.Is(err, store.ErrNotFound):
			res.Status, res.Reason = "rejected", "unknown session or set"
		case err != nil:
			return results, err
		case outcome == store.Duplicate:
			res.Status = "duplicate"
		default:
			res.Status = "applied" // applied or ignored: either way the client is done with it
		}
		results = append(results, res)
	}
	return results, nil
}

func (s *Service) applyOp(ctx context.Context, user store.User, op Op) (store.Outcome, error) {
	if !idRE.MatchString(op.OpID) {
		return "", InvalidError{"op_id must be a UUID"}
	}
	switch op.Op {
	case OpUpsertSet:
		var in SetInput
		if err := json.Unmarshal(op.Payload, &in); err != nil {
			return "", InvalidError{"bad set: " + err.Error()}
		}
		set, err := s.toStore(ctx, user, in)
		if err != nil {
			return "", err
		}
		return s.Store.UpsertSet(ctx, user.ID, set, op.OpID)
	case OpDeleteSet:
		var p struct {
			ID        string    `json:"id"`
			SessionID string    `json:"session_id"`
			DeletedAt time.Time `json:"deleted_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil || !idRE.MatchString(p.ID) {
			return "", InvalidError{"bad delete"}
		}
		return s.Store.DeleteSet(ctx, user.ID, p.SessionID, p.ID, s.clamp(p.DeletedAt), op.OpID)
	case OpEditNotes:
		var p struct {
			SessionID string    `json:"session_id"`
			Notes     string    `json:"notes"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil || len(p.Notes) > 10000 {
			return "", InvalidError{"bad notes"}
		}
		return s.Store.SetSessionNotes(ctx, user.ID, p.SessionID, p.Notes, s.clamp(p.UpdatedAt), op.OpID)
	case OpFinishSession:
		var p struct {
			SessionID  string    `json:"session_id"`
			FinishedAt time.Time `json:"finished_at"`
		}
		if err := json.Unmarshal(op.Payload, &p); err != nil {
			return "", InvalidError{"bad finish"}
		}
		return s.finish(ctx, user, p.SessionID, s.clamp(p.FinishedAt), op.OpID)
	}
	return "", InvalidError{"unknown operation " + op.Op}
}

// finish marks the session finished and, the first time, moves the plan
// cursor past the day it trained (spec §8).
func (s *Service) finish(ctx context.Context, user store.User, sessionID string, at time.Time, opID string) (store.Outcome, error) {
	out, err := s.Store.FinishSession(ctx, user.ID, sessionID, at, opID)
	if err != nil || out != store.Applied {
		return out, err
	}
	sess, err := s.Store.SessionByID(ctx, user.ID, sessionID)
	if err != nil || sess.PlanID == "" {
		return out, err
	}
	return out, s.Plans.AdvanceFrom(ctx, user, sess.PlanID, sess.Week, sess.Day)
}
```

`internal/training/bootstrap.go`: the companion gets full context (TM, e1RM, equipment, last time's sets) for the planned exercises, their alternatives and anything logged, plus the catalog and default equipment per kind for exercises added during the workout:
```go
package training

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/store"
)

// Bootstrap is everything the companion needs to run a session offline
// (spec §9). It is embedded in the live page as JSON.
type Bootstrap struct {
	Session   BootSession                `json:"session"`
	Snapshot  plan.ExpandedDay           `json:"snapshot"`
	Sets      []SetInput                 `json:"sets"`
	Exercises map[string]BootExercise    `json:"exercises"`
	Catalog   []CatalogEntry             `json:"catalog"`
	Defaults  map[string]*calc.Equipment `json:"equipment_defaults"` // by kind, for exercises added during the session
	Unit      string                     `json:"unit"`
}

type BootSession struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Week      int       `json:"week"`
	Day       int       `json:"day"`
	StartedAt time.Time `json:"started_at"`
	Finished  bool      `json:"finished"`
	Notes     string    `json:"notes"`
}

// BootExercise is what the companion knows about an exercise of the session.
type BootExercise struct {
	Name          string          `json:"name"`
	Measurement   string          `json:"measurement"`
	EquipmentKind string          `json:"equipment_kind"`
	TMKg          *float64        `json:"tm_kg,omitempty"`
	E1RMKg        *float64        `json:"e1rm_kg,omitempty"`
	Equipment     *calc.Equipment `json:"equipment,omitempty"`
	Alternatives  []string        `json:"alternatives"`
	Last          []SetInput      `json:"last"` // sets from the previous session with this exercise
}

// CatalogEntry is a catalog exercise the user can add or swap to.
type CatalogEntry struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Measurement   string `json:"measurement"`
	EquipmentKind string `json:"equipment_kind"`
}

// Bootstrap assembles the companion data for one of the user's sessions.
func (s *Service) Bootstrap(ctx context.Context, user store.User, sessionID string) (Bootstrap, error) {
	sess, err := s.Store.SessionByID(ctx, user.ID, sessionID)
	if err != nil {
		return Bootstrap{}, err
	}
	b := Bootstrap{
		Session: BootSession{ID: sess.ID, Name: sess.Name, Week: sess.Week, Day: sess.Day,
			StartedAt: sess.StartedAt, Finished: sess.FinishedAt != nil, Notes: sess.Notes},
		Exercises: map[string]BootExercise{},
		Defaults:  map[string]*calc.Equipment{},
		Unit:      user.Unit,
	}
	if err := json.Unmarshal(sess.Snapshot, &b.Snapshot); err != nil {
		return Bootstrap{}, err
	}
	sets, err := s.Store.SessionSets(ctx, user.ID, sess.ID)
	if err != nil {
		return Bootstrap{}, err
	}
	b.Sets = make([]SetInput, 0, len(sets))
	for _, set := range sets {
		b.Sets = append(b.Sets, toInput(set))
	}

	// Exercises of the session, their alternatives, and anything logged.
	var slugs []string
	planned := map[string]bool{}
	for _, g := range b.Snapshot.Groups {
		for _, slot := range g.Exercises {
			planned[slot.Slug] = true
			slugs = append(slugs, slot.Slug)
			slugs = append(slugs, slot.Alternatives...)
		}
	}
	for _, set := range sets {
		slugs = append(slugs, set.Slug)
	}
	for i := 0; i < len(slugs); i++ { // grows as catalog alternatives are found
		slug := slugs[i]
		if _, done := b.Exercises[slug]; done {
			continue
		}
		be, alts, err := s.bootExercise(ctx, user, sess.ID, slug)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return Bootstrap{}, err
		}
		b.Exercises[slug] = be
		if planned[slug] { // catalog alternatives of planned exercises, not of alternatives
			slugs = append(slugs, alts...)
		}
	}

	catalog, err := s.Exercises.Catalog(ctx, user.ID, "")
	if err != nil {
		return Bootstrap{}, err
	}
	for _, e := range catalog {
		b.Catalog = append(b.Catalog, CatalogEntry{Slug: e.Slug, Name: e.Name, Measurement: e.Measurement, EquipmentKind: e.EquipmentKind})
	}
	equipment, err := s.Exercises.ListEquipment(ctx, user.ID)
	if err != nil {
		return Bootstrap{}, err
	}
	for i, e := range equipment {
		if e.IsDefault {
			b.Defaults[e.Spec.Kind] = &equipment[i].Spec
		}
	}
	return b, nil
}

func (s *Service) bootExercise(ctx context.Context, user store.User, sessionID, slug string) (BootExercise, []string, error) {
	ex, err := s.Exercises.Get(ctx, user.ID, slug)
	if err != nil {
		return BootExercise{}, nil, err
	}
	st, err := s.Exercises.Settings(ctx, user.ID, ex)
	if err != nil {
		return BootExercise{}, nil, err
	}
	be := BootExercise{Name: ex.Name, Measurement: ex.Measurement, EquipmentKind: ex.EquipmentKind,
		TMKg: st.TrainingMaxKg, Alternatives: []string{}, Last: []SetInput{}}
	if st.Equipment != nil {
		be.Equipment = &st.Equipment.Spec
	}
	if be.E1RMKg, err = s.Store.BestE1RM(ctx, user.ID, slug, s.now().AddDate(0, 0, -user.E1RMWindowDays)); err != nil {
		return BootExercise{}, nil, err
	}
	alts, err := s.Exercises.Alternatives(ctx, user.ID, slug)
	if err != nil {
		return BootExercise{}, nil, err
	}
	for _, a := range alts {
		be.Alternatives = append(be.Alternatives, a.Exercise.Slug)
	}
	last, err := s.Store.LastSets(ctx, user.ID, slug, sessionID)
	if err != nil {
		return BootExercise{}, nil, err
	}
	for _, set := range last {
		be.Last = append(be.Last, toInput(set))
	}
	return be, be.Alternatives, nil
}

func toInput(s store.Set) SetInput {
	return SetInput{ID: s.ID, SessionID: s.SessionID, Slug: s.Slug, GroupPos: s.GroupPos, ExercisePos: s.ExercisePos,
		SetPos: s.SetPos, Kind: s.Kind, Prescribed: json.RawMessage(s.Prescribed), WeightKg: s.WeightKg, Reps: s.Reps,
		RPE: s.RPE, DurationS: s.DurationS, DistanceM: s.DistanceM, DoneAt: s.DoneAt, UpdatedAt: s.UpdatedAt}
}
```

`internal/training/history.go`:
```go
package training

import (
	"context"

	"github.com/LongerHV/onerep/internal/store"
)

// HistoryPageSize is the number of sessions per history page.
const HistoryPageSize = 20

// History lists sessions newest first; more reports whether another page exists.
func (s *Service) History(ctx context.Context, user store.User, page int) (sessions []store.SessionSummary, more bool, err error) {
	if page < 0 {
		page = 0
	}
	list, err := s.Store.ListSessions(ctx, user.ID, HistoryPageSize+1, page*HistoryPageSize)
	if err != nil {
		return nil, false, err
	}
	if len(list) > HistoryPageSize {
		return list[:HistoryPageSize], true, nil
	}
	return list, false, nil
}

// Session returns one of the user's sessions with its sets.
func (s *Service) Session(ctx context.Context, user store.User, id string) (store.Session, []store.Set, error) {
	sess, err := s.Store.SessionByID(ctx, user.ID, id)
	if err != nil {
		return store.Session{}, nil, err
	}
	sets, err := s.Store.SessionSets(ctx, user.ID, id)
	return sess, sets, err
}

// SaveSet creates or corrects a set from the history editor. The edit is
// stamped with the server's clock, so it wins over older companion edits.
func (s *Service) SaveSet(ctx context.Context, user store.User, in SetInput) error {
	in.UpdatedAt = s.now()
	set, err := s.toStore(ctx, user, in)
	if err != nil {
		return err
	}
	_, err = s.Store.UpsertSet(ctx, user.ID, set, "")
	return err
}

func (s *Service) DeleteSet(ctx context.Context, user store.User, sessionID, setID string) error {
	_, err := s.Store.DeleteSet(ctx, user.ID, sessionID, setID, s.now(), "")
	return err
}

func (s *Service) SetNotes(ctx context.Context, user store.User, sessionID, notes string) error {
	if len(notes) > 10000 {
		return InvalidError{"notes are too long"}
	}
	_, err := s.Store.SetSessionNotes(ctx, user.ID, sessionID, notes, s.now(), "")
	return err
}

// Finish finishes a session from the history editor (advancing the plan
// like the companion does).
func (s *Service) Finish(ctx context.Context, user store.User, sessionID string) error {
	_, err := s.finish(ctx, user, sessionID, s.now(), "")
	return err
}

func (s *Service) Delete(ctx context.Context, user store.User, sessionID string) error {
	return s.Store.DeleteSession(ctx, user.ID, sessionID)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/training/ && golangci-lint run ./internal/...`
Expected: `ok`, `0 issues.`

- [ ] **Step 5: Commit**

```bash
git add internal/training
git commit -m "feat(training): start sessions, apply companion sync operations and edit history"
```

---

### Task 4: Companion logic (pure JS)

**Files:**
- Create: `internal/web/static/js/companion-core.js`
- Test: `internal/web/jstest/companion.test.mjs`

**Interfaces:**
- Consumes: `calc.js` (`e1rm`, `resolveLoad`, `dropLoad`)
- Produces (ES module):
  - `uuidv7(now?, random?)`, `parseReps(v)`
  - State: `newState(boot)`, `mergeServerSets(state, sets)`
  - Structure: `groups(boot, state)`, `exerciseInfo(boot, slug)`, `steps(boot, state)` returning `[{g, e, s, key}]`, `logged(state, step)`, `status(state, step)` ("done", "skipped" or "todo"), `currentStep(boot, state)`
  - Targets: `sessionE1RM(state, slug)`, `target(boot, state, step)`, `restAfter(boot, state, step)`
  - Changes that return `{state, op}`: `logSet(boot, state, step, values, now?)`, `deleteSet(state, id, now?)`, `editNotes`, `finish`
  - Changes that return a new state: `skip`, `unskip`, `addSet(state, g, e)`, `addExercise(boot, state, slug)`, `swap(state, g, e, slug, planned)`
  - Outbox: `settle(outbox, results)` returning `{remaining, failed}`
  - Operations use the same JSON as `training.Op` and `training.SetInput`.

- [ ] **Step 1: Write the failing tests**

`internal/web/jstest/companion.test.mjs` (the expected weights were worked out by hand from the RTS table and the plate set):
```javascript
// Tests for the companion's pure logic. Run with: node --test "internal/web/jstest/*.test.mjs"
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addExercise, addSet, currentStep, deleteSet, finish, groups, logSet, logged, mergeServerSets,
  newState, parseReps, restAfter, sessionE1RM, settle, skip, status, steps, swap, target, uuidv7,
} from "../static/js/companion-core.js";

const bar = { kind: "barbell", unit: "kg", config: { bar: 20, plates: [25, 20, 15, 10, 5, 2.5, 1.25] } };

// A day with a squat block (RPE sets then a drop set) and a pull-up/raise superset.
function boot() {
  return {
    session: { id: "sess", name: "Day", notes: "", finished: false },
    unit: "kg",
    snapshot: {
      name: "Day",
      groups: [
        {
          rest_s: 180,
          exercises: [{
            slug: "barbell-back-squat",
            alternatives: ["hack-squat"],
            sets: [
              { kind: "warmup", reps: 5, load_kind: "pct_tm", load_value: 0.5 },
              { kind: "working", reps: 5, load_kind: "rpe", load_value: 8, target_rpe: 8 },
              { kind: "working", reps: 5, load_kind: "rpe", load_value: 8, target_rpe: 8 },
              { kind: "drop", reps: 10, load_kind: "drop_pct", load_value: 0.2 },
            ],
          }],
        },
        {
          rest_s: 90,
          exercises: [
            { slug: "pull-up", sets: [{ kind: "working", reps: "6-10" }, { kind: "working", reps: "6-10" }] },
            { slug: "cable-lateral-raise", sets: [{ kind: "working", reps: 15, load_kind: "weight", load_value: 7.5 }] },
          ],
        },
      ],
    },
    exercises: {
      "barbell-back-squat": { name: "Squat", measurement: "weight_reps", equipment_kind: "barbell", tm_kg: 140, e1rm_kg: 150, equipment: bar, alternatives: ["hack-squat", "leg-press"], last: [] },
      "hack-squat": { name: "Hack Squat", measurement: "weight_reps", equipment_kind: "machine", tm_kg: 200, alternatives: [], last: [] },
      "pull-up": { name: "Pull-Up", measurement: "bw_reps", equipment_kind: "bodyweight", alternatives: [], last: [] },
      "cable-lateral-raise": { name: "Raise", measurement: "weight_reps", equipment_kind: "cable", alternatives: [], last: [] },
    },
    catalog: [{ slug: "plank", name: "Plank", measurement: "time", equipment_kind: "bodyweight" }],
    equipment_defaults: { barbell: bar },
  };
}

const T0 = Date.UTC(2026, 8, 24, 10, 0, 0);

test("uuidv7 is a time-ordered UUID", () => {
  const a = uuidv7(T0);
  const b = uuidv7(T0 + 1);
  assert.match(a, /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/);
  assert.ok(a < b, "later ids sort after earlier ones");
  assert.equal(a.slice(0, 13).replace("-", ""), T0.toString(16).padStart(12, "0"));
});

test("parseReps", () => {
  assert.deepEqual(parseReps(5), { min: 5, max: 5, amrap: false });
  assert.deepEqual(parseReps("6-10"), { min: 6, max: 10, amrap: false });
  assert.deepEqual(parseReps("AMRAP"), { min: 0, max: 0, amrap: true });
  assert.equal(parseReps(undefined), null);
});

test("steps run groups in order and supersets round by round", () => {
  const b = boot();
  assert.deepEqual(steps(b, newState(b)).map((s) => s.key), [
    "0:0:0", "0:0:1", "0:0:2", "0:0:3",
    "1:0:0", "1:1:0", "1:0:1",
  ]);
});

test("rest follows each round, not each superset exercise", () => {
  const b = boot();
  const s = newState(b);
  const at = (k) => steps(b, s).find((x) => x.key === k);
  assert.equal(restAfter(b, s, at("0:0:0")), 180);
  assert.equal(restAfter(b, s, at("0:0:3")), 0, "last set of a group");
  assert.equal(restAfter(b, s, at("1:0:0")), 0, "A1 goes straight to B1");
  assert.equal(restAfter(b, s, at("1:1:0")), 90, "rest after B1 completes the round");
});

test("targets resolve and adapt to the sets actually done", () => {
  const b = boot();
  let s = newState(b);
  const [warm, rpe1, rpe2, drop] = steps(b, s);
  assert.equal(target(b, s, warm).kg, 70); // 50% of 140
  assert.equal(target(b, s, rpe1).kg, 120); // 0.811 * 150 = 121.65 -> 120
  assert.deepEqual(target(b, s, rpe1).per_side, [25, 25]);
  assert.equal(target(b, s, drop).kg, 95, "drop from the planned 120 before anything is logged");

  // The first RPE set felt easier than planned: 120 x 5 @ 7 means an e1RM of
  // 120 / 0.786 = 152.67, so the next 5 @ 8 is 0.811 * 152.67 = 123.8 -> 122.5.
  ({ state: s } = logSet(b, s, warm, { weight_kg: 70, reps: 5 }, T0));
  ({ state: s } = logSet(b, s, rpe1, { weight_kg: 120, reps: 5, rpe: 7 }, T0 + 1000));
  assert.ok(Math.abs(sessionE1RM(s, "barbell-back-squat") - 152.67) < 0.01);
  assert.equal(target(b, s, rpe2).kg, 122.5);
  assert.deepEqual(target(b, s, rpe2).per_side, [25, 25, 1.25]);
  // The drop set follows the weight actually lifted on the set before it.
  ({ state: s } = logSet(b, s, rpe2, { weight_kg: 115, reps: 5, rpe: 8 }, T0 + 2000));
  // 80% of 115 = 92; these plates only make multiples of 2.5 kg above the bar -> 90.
  assert.equal(target(b, s, drop).kg, 90);
});

test("logging produces idempotent operations and moves on", () => {
  const b = boot();
  let s = newState(b);
  const first = currentStep(b, s);
  let op;
  ({ state: s, op } = logSet(b, s, first, { weight_kg: 70, reps: 5 }, T0));
  assert.equal(op.op, "upsert_set");
  assert.equal(op.payload.id, logged(s, first).id);
  assert.equal(op.payload.session_id, "sess");
  assert.equal(op.payload.kind, "warmup");
  assert.equal(s.restEnd, T0 + 180_000);
  assert.equal(currentStep(b, s).key, "0:0:1");

  // Correcting the same step keeps the set id (the server applies the newer edit).
  const again = logSet(b, s, first, { weight_kg: 72.5, reps: 5 }, T0 + 5000);
  assert.equal(again.op.payload.id, op.payload.id);
  assert.equal(again.op.payload.done_at, op.payload.done_at);
  assert.notEqual(again.op.op_id, op.op_id);
});

test("deleting a set reopens its step; skipping moves past it", () => {
  const b = boot();
  let s = newState(b);
  const first = currentStep(b, s);
  let { state, op } = logSet(b, s, first, { weight_kg: 70, reps: 5 }, T0);
  ({ state, op } = deleteSet(state, op.payload.id, T0 + 1000));
  assert.equal(op.op, "delete_set");
  assert.equal(status(state, first), "todo");
  state = skip(state, first);
  assert.equal(status(state, first), "skipped");
  assert.equal(currentStep(b, state).key, "0:0:1");
});

test("swapping, extra sets and added exercises", () => {
  const b = boot();
  let s = swap(newState(b), 0, 0, "hack-squat", "barbell-back-squat");
  const warm = steps(b, s)[0];
  assert.equal(target(b, s, warm).slug, "hack-squat");
  assert.equal(target(b, s, warm).kg, 100, "50% of the hack squat's own TM, no equipment -> 0.5 kg steps");
  assert.ok(groups(b, s)[0].exercises[0].alternatives.includes("barbell-back-squat"), "can swap back");

  s = addSet(s, 1, 1);
  assert.equal(steps(b, s).filter((x) => x.g === 1 && x.e === 1).length, 2);
  s = addExercise(b, s, "plank");
  const last = steps(b, s).at(-1);
  assert.equal(last.g, 2);
  assert.equal(target(b, s, last).measurement, "time");
});

test("server sets merge in; the newer edit wins", () => {
  const b = boot();
  const [first] = steps(b, newState(b));
  const { state: local, op } = logSet(b, newState(b), first, { weight_kg: 70, reps: 5 }, T0 + 60_000);
  const older = { ...op.payload, weight_kg: 60, updated_at: new Date(T0).toISOString() };
  assert.equal(logged(mergeServerSets(local, [older]), first).weight_kg, 70);
  const newer = { ...op.payload, weight_kg: 75, updated_at: new Date(T0 + 120_000).toISOString() };
  assert.equal(logged(mergeServerSets(local, [newer]), first).weight_kg, 75);
  // A set logged on another device beyond the plan shows up as an extra step.
  const extra = { ...op.payload, id: uuidv7(T0), set_pos: 9 };
  assert.equal(steps(b, mergeServerSets(newState(b), [extra])).filter((x) => x.g === 0).length, 10);
});

test("finishing and settling the outbox", () => {
  const b = boot();
  const { state, op } = finish(newState(b), T0);
  assert.ok(state.finished);
  assert.equal(op.op, "finish_session");
  const a = { op_id: "a" }, d = { op_id: "d" }, r = { op_id: "r" }, p = { op_id: "p" };
  const { remaining, failed } = settle([a, d, r, p], [
    { op_id: "a", status: "applied" }, { op_id: "d", status: "duplicate" }, { op_id: "r", status: "rejected", reason: "bad" },
  ]);
  assert.deepEqual(remaining, [p]);
  assert.deepEqual(failed, [{ op_id: "r", reason: "bad" }]);
});

test("timestamps compare as times, not strings (Go omits zero milliseconds)", () => {
  const b = boot();
  const [first] = steps(b, newState(b));
  const { state: local, op } = logSet(b, newState(b), first, { weight_kg: 70, reps: 5 }, T0 + 500);
  // The server's copy of an older edit, formatted by Go: "…T10:00:00Z" sorts after "…T10:00:00.500Z" as a string.
  const serverOlder = { ...op.payload, weight_kg: 60, updated_at: "2026-09-24T10:00:00Z" };
  assert.equal(logged(mergeServerSets(local, [serverOlder]), first).weight_kg, 70);
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `node --test "internal/web/jstest/*.test.mjs"`
Expected: FAIL, `ERR_MODULE_NOT_FOUND` for `companion-core.js`.

- [ ] **Step 3: Implement**

`internal/web/static/js/companion-core.js`:
```javascript
// Companion mode logic, free of DOM and storage so Node can test it
// (internal/web/jstest/companion.test.mjs). companion.js renders it and
// persists the state and outbox in IndexedDB.
//
// A session's steps come from its snapshot (plan.ExpandedDay): group g,
// exercise e, set s. Supersets alternate A1, B1, A2, B2 within a group.
// Every logged set has a client-generated UUIDv7, so replaying an operation
// after a lost response is harmless (spec §9).

import { dropLoad, e1rm, resolveLoad } from "./calc.js";

// uuidv7 returns a time-ordered UUID (RFC 9562).
export function uuidv7(now = Date.now(), random = crypto.getRandomValues(new Uint8Array(16))) {
  const b = new Uint8Array(random);
  let ts = BigInt(now);
  for (let i = 5; i >= 0; i--) {
    b[i] = Number(ts & 0xffn);
    ts >>= 8n;
  }
  b[6] = (b[6] & 0x0f) | 0x70;
  b[8] = (b[8] & 0x3f) | 0x80;
  const hex = [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

const iso = (now) => new Date(now).toISOString();
const key = (g, e, s) => `${g}:${e}:${s}`;

// parseReps reads a prescription's reps: 5, "6-10" or "AMRAP" (absent for timed sets).
export function parseReps(v) {
  if (v === undefined || v === null) return null;
  if (typeof v === "number") return { min: v, max: v, amrap: false };
  if (v === "AMRAP") return { min: 0, max: 0, amrap: true };
  const [lo, hi] = String(v).split("-").map(Number);
  return { min: lo, max: hi, amrap: false };
}

// newState is the empty local state of a session.
export function newState(boot) {
  return {
    sessionId: boot.session.id,
    sets: {}, // id -> set (as sent to the server), deleted ones kept with deleted: true
    positions: {}, // "g:e:s" -> set id
    skipped: {}, // "g:e:s" -> true
    extra: {}, // "g:e" -> sets added beyond the prescription
    swaps: {}, // "g:e" -> slug done instead of the planned one
    added: [], // exercises added during the session: [{slug}]
    restEnd: null, // ms timestamp the rest timer ends at
    notes: boot.session.notes || "",
    notesAt: null,
    finished: boot.session.finished,
  };
}

// mergeServerSets folds the server's copy of the session's sets into local
// state; the newer edit of each set wins.
export function mergeServerSets(state, serverSets) {
  const s = structuredClone(state);
  for (const set of serverSets) {
    const local = s.sets[set.id];
    if (local && Date.parse(local.updated_at) >= Date.parse(set.updated_at)) continue;
    s.sets[set.id] = set;
    s.positions[key(set.group_pos, set.exercise_pos, set.set_pos)] = set.id;
  }
  return s;
}

// groups returns the session's groups with swaps, extra and added exercises applied.
export function groups(boot, state) {
  const out = boot.snapshot.groups.map((g, gi) => ({
    rest_s: g.rest_s,
    exercises: g.exercises.map((slot, ei) => exerciseView(boot, state, gi, ei, slot.slug, slot.sets, slot.alternatives || [])),
  }));
  state.added.forEach((a, i) => {
    const gi = boot.snapshot.groups.length + i;
    out.push({ rest_s: 90, exercises: [exerciseView(boot, state, gi, 0, a.slug, [], [])] });
  });
  return out;
}

function exerciseView(boot, state, g, e, planned, prescribed, planAlts) {
  const slug = state.swaps[`${g}:${e}`] || planned;
  let count = prescribed.length + (state.extra[`${g}:${e}`] || 0);
  for (const k of Object.keys(state.positions)) {
    const [pg, pe, ps] = k.split(":").map(Number);
    if (pg === g && pe === e && ps >= count) count = ps + 1; // sets logged elsewhere (another device)
  }
  const info = exerciseInfo(boot, slug);
  const alternatives = [...new Set([...(planAlts || []), ...((boot.exercises[planned] || {}).alternatives || [])])].filter((a) => a !== slug);
  if (slug !== planned) alternatives.unshift(planned);
  return { g, e, planned, slug, name: info.name, measurement: info.measurement, prescribed, count, alternatives };
}

// exerciseInfo is what is known about slug: session exercises carry TM, e1RM
// and equipment; anything else comes from the catalog.
export function exerciseInfo(boot, slug) {
  const ex = boot.exercises[slug];
  if (ex) return ex;
  const c = (boot.catalog || []).find((x) => x.slug === slug) || { name: slug, measurement: "weight_reps" };
  return { name: c.name, measurement: c.measurement, equipment_kind: c.equipment_kind, alternatives: [], last: [] };
}

// steps lists every set in the order it is done: each group round by round.
export function steps(boot, state) {
  const out = [];
  groups(boot, state).forEach((grp, g) => {
    const rounds = Math.max(0, ...grp.exercises.map((x) => x.count));
    for (let s = 0; s < rounds; s++) {
      grp.exercises.forEach((x, e) => {
        if (s < x.count) out.push({ g, e, s, key: key(g, e, s) });
      });
    }
  });
  return out;
}

// logged returns the live (not deleted) set logged at a step, if any.
export function logged(state, step) {
  const id = state.positions[step.key];
  const set = id && state.sets[id];
  return set && !set.deleted ? set : null;
}

export function status(state, step) {
  if (logged(state, step)) return "done";
  return state.skipped[step.key] ? "skipped" : "todo";
}

// currentStep is the first step neither logged nor skipped, or null.
export function currentStep(boot, state) {
  return steps(boot, state).find((st) => status(state, st) === "todo") || null;
}

// sessionE1RM is the e1RM of the last working set of slug done this session
// (spec §7 in-session adjustment), or null.
export function sessionE1RM(state, slug) {
  const sets = Object.values(state.sets)
    .filter((x) => !x.deleted && x.slug === slug && (x.kind === "working" || x.kind === "amrap") && x.weight_kg > 0 && x.reps > 0)
    .sort((a, b) => Date.parse(a.done_at) - Date.parse(b.done_at));
  const last = sets.at(-1);
  if (!last) return null;
  const est = e1rm(last.weight_kg, last.reps, last.rpe || 0);
  return est ? est.kg : null;
}

// prescription returns the planned set for a step; extra sets repeat the last one.
function prescription(ex, s) {
  return ex.prescribed[s] || ex.prescribed.at(-1) || null;
}

// target is what the lifter should do at a step, with the weight resolved
// from the current state (session e1RM, previous actual weight for drops).
export function target(boot, state, step) {
  const ex = groups(boot, state)[step.g].exercises[step.e];
  const p = prescription(ex, step.s);
  const info = exerciseInfo(boot, ex.slug);
  const out = {
    slug: ex.slug,
    name: ex.name,
    measurement: ex.measurement,
    kind: p ? p.kind : "working",
    reps: p ? parseReps(p.reps) : null,
    duration_s: p && p.duration_s ? p.duration_s : null,
    rpe: p && p.target_rpe ? p.target_rpe : null,
    load_kind: p ? p.load_kind || "" : "",
    load_value: p ? p.load_value : null,
    prescribed: p,
    kg: null,
    per_side: [],
  };
  const ctx = {
    unit: boot.unit,
    tm_kg: info.tm_kg ?? null,
    e1rm_kg: sessionE1RM(state, ex.slug) ?? info.e1rm_kg ?? null,
    equipment: info.equipment || (boot.equipment_defaults || {})[info.equipment_kind] || null,
  };
  let r = null;
  switch (out.load_kind) {
    case "weight":
      r = { kg: p.load_value, per_side: [] };
      break;
    case "pct_tm":
      r = resolveLoad({ pct_tm: p.load_value }, 1, ctx);
      break;
    case "rpe":
      if (out.reps && !out.reps.amrap) r = resolveLoad({ rpe: p.load_value }, out.reps.min, ctx);
      break;
    case "drop_pct": {
      const prev = step.s > 0 ? { ...step, s: step.s - 1, key: key(step.g, step.e, step.s - 1) } : null;
      const prevKg = prev ? (logged(state, prev) || {}).weight_kg ?? target(boot, state, prev).kg : null;
      if (prevKg) r = dropLoad(prevKg, p.load_value, ctx);
      break;
    }
  }
  if (r) {
    out.kg = r.kg;
    out.per_side = r.per_side || [];
  }
  return out;
}

// restAfter is the rest in seconds after a step: a group's rest follows each
// complete round (after the last exercise of the round).
export function restAfter(boot, state, step) {
  const all = steps(boot, state);
  const i = all.findIndex((x) => x.key === step.key);
  const next = all[i + 1];
  if (next && next.g === step.g && next.s === step.s) return 0; // superset: go straight to the next exercise
  if (!next || next.g !== step.g) {
    const later = all.slice(i + 1).some((x) => x.g === step.g);
    if (!later) return 0; // last set of the group
  }
  return groups(boot, state)[step.g].rest_s || 0;
}

function op(name, payload, now) {
  return { op_id: uuidv7(now), op: name, payload, client_ts: iso(now) };
}

// logSet records values at a step (or corrects what is logged there).
// values: {weight_kg, reps, rpe, duration_s, distance_m}, null for "not recorded".
export function logSet(boot, state, step, values, now = Date.now()) {
  const s = structuredClone(state);
  const t = target(boot, s, step);
  const existing = logged(s, step);
  const set = {
    id: existing ? existing.id : uuidv7(now),
    session_id: s.sessionId,
    slug: existing ? existing.slug : t.slug,
    group_pos: step.g,
    exercise_pos: step.e,
    set_pos: step.s,
    kind: t.kind,
    prescribed: t.prescribed,
    weight_kg: values.weight_kg ?? null,
    reps: values.reps ?? null,
    rpe: values.rpe ?? null,
    duration_s: values.duration_s ?? null,
    distance_m: values.distance_m ?? null,
    done_at: existing ? existing.done_at : iso(now),
    updated_at: iso(now),
  };
  s.sets[set.id] = set;
  s.positions[step.key] = set.id;
  delete s.skipped[step.key];
  if (!existing) {
    const rest = restAfter(boot, s, step);
    s.restEnd = rest > 0 ? now + rest * 1000 : null;
  }
  return { state: s, op: op("upsert_set", set, now) };
}

// deleteSet removes a logged set; its step becomes "todo" again.
export function deleteSet(state, id, now = Date.now()) {
  const s = structuredClone(state);
  const set = s.sets[id];
  if (!set) return { state: s, op: null };
  set.deleted = true;
  set.updated_at = iso(now);
  for (const [k, v] of Object.entries(s.positions)) if (v === id) delete s.positions[k];
  return { state: s, op: op("delete_set", { id, session_id: s.sessionId, deleted_at: iso(now) }, now) };
}

export function skip(state, step) {
  const s = structuredClone(state);
  s.skipped[step.key] = true;
  return s;
}

export function unskip(state, step) {
  const s = structuredClone(state);
  delete s.skipped[step.key];
  return s;
}

// addSet appends one more set to exercise (g, e).
export function addSet(state, g, e) {
  const s = structuredClone(state);
  s.extra[`${g}:${e}`] = (s.extra[`${g}:${e}`] || 0) + 1;
  return s;
}

// addExercise appends a new group with slug and one set.
export function addExercise(boot, state, slug) {
  const s = structuredClone(state);
  s.added.push({ slug });
  const g = boot.snapshot.groups.length + s.added.length - 1;
  s.extra[`${g}:0`] = 1;
  return s;
}

// swap does the remaining sets of (g, e) as slug instead.
export function swap(state, g, e, slug, planned) {
  const s = structuredClone(state);
  if (slug === planned) delete s.swaps[`${g}:${e}`];
  else s.swaps[`${g}:${e}`] = slug;
  return s;
}

export function editNotes(state, notes, now = Date.now()) {
  const s = structuredClone(state);
  s.notes = notes;
  s.notesAt = iso(now);
  return { state: s, op: op("edit_notes", { session_id: s.sessionId, notes, updated_at: iso(now) }, now) };
}

export function finish(state, now = Date.now()) {
  const s = structuredClone(state);
  s.finished = true;
  s.restEnd = null;
  return { state: s, op: op("finish_session", { session_id: s.sessionId, finished_at: iso(now) }, now) };
}

// settle removes answered operations from the outbox: applied and duplicate
// ones are done, rejected ones move to failed with their reason.
export function settle(outbox, results) {
  const byId = new Map(results.map((r) => [r.op_id, r]));
  const remaining = [];
  const failed = [];
  for (const o of outbox) {
    const r = byId.get(o.op_id);
    if (!r) remaining.push(o);
    else if (r.status === "rejected") failed.push({ ...o, reason: r.reason || "rejected" });
  }
  return { remaining, failed };
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `node --test "internal/web/jstest/*.test.mjs"`
Expected: `ℹ pass 18`, `ℹ fail 0` (7 calc + 11 companion).

- [ ] **Step 5: Commit**

```bash
git add internal/web/static/js/companion-core.js internal/web/jstest/companion.test.mjs
git commit -m "feat(web): add companion mode logic with Node tests"
```

---

### Task 5: Companion screen and service worker

**Files:**
- Create: `internal/web/static/js/companion.js`, `internal/web/static/js/sw.js`

**Interfaces:**
- Consumes: `companion-core.js` (Task 4), `calc.js`. From the server (Task 6): the `#companion` element, `<script id="companion-boot">`, `POST /api/sync` (`{ops}` → `{results}`), `GET /api/csrf` (`{csrf}`), `/sw.js`, `/offline`.
- Produces:
  - **IndexedDB database `onerep`** (version 1) with three stores: `sessions` (key `sessionId`), `outbox` and `failed` (both keyed by `op_id`).
  - **Overview rows** carry `data-status` = `done`, `skipped` or `todo`; the sync badge is `[data-sync]`. The e2e test relies on both.
  - **Service worker caches:** `shell-<version>` (cache-first for `/static/*`) and `pages` (network-first for `/sessions/{id}/live` navigations, falling back to the cached copy, then `/offline`).

This task has no automated test of its own; Task 7's `task e2e` exercises it in a real browser.

- [ ] **Step 1: Write the companion**

`internal/web/static/js/companion.js`:
```javascript
// Companion mode (spec §9): the live workout screen. It works offline: state
// and queued operations live in IndexedDB and sync when the network allows.
// Loaded once from the layout; sets itself up when a #companion element
// appears (htmx.onLoad) and tears down when it is swapped away.
import * as core from "./companion-core.js";
import { fromKg, toKg } from "./calc.js";

if ("serviceWorker" in navigator) {
  navigator.serviceWorker.register("/sw.js").catch((err) => console.warn("service worker not registered", err));
}

// --- IndexedDB -------------------------------------------------------------

const DB_NAME = "onerep";
let dbPromise;

function db() {
  dbPromise ??= new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => {
      req.result.createObjectStore("sessions", { keyPath: "sessionId" });
      req.result.createObjectStore("outbox", { keyPath: "op_id" });
      req.result.createObjectStore("failed", { keyPath: "op_id" });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
  return dbPromise;
}

async function tx(store, mode, fn) {
  const d = await db();
  return new Promise((resolve, reject) => {
    const t = d.transaction(store, mode);
    const result = fn(t.objectStore(store));
    t.oncomplete = () => resolve(result && "result" in result ? result.result : undefined);
    t.onerror = () => reject(t.error);
  });
}

const idb = {
  get: (store, k) => tx(store, "readonly", (s) => s.get(k)),
  all: (store) => tx(store, "readonly", (s) => s.getAll()),
  put: (store, v) => tx(store, "readwrite", (s) => s.put(v)),
  del: (store, k) => tx(store, "readwrite", (s) => s.delete(k)),
};

// --- sync --------------------------------------------------------------------

let flushing = false;
let syncListeners = new Set();

function csrfToken() {
  try {
    return JSON.parse(document.body.getAttribute("hx-headers") || "{}")["X-CSRF-Token"] || "";
  } catch {
    return "";
  }
}

async function refreshToken() {
  const res = await fetch("/api/csrf", { credentials: "same-origin" });
  if (!res.ok) return false;
  const { csrf } = await res.json();
  document.body.setAttribute("hx-headers", JSON.stringify({ "X-CSRF-Token": csrf }));
  return true;
}

// syncStatus is shown in the header: pending count, failures, sign-in needed.
const syncStatus = { pending: 0, failed: 0, relogin: false, offline: !navigator.onLine };

function notify() {
  for (const fn of syncListeners) fn();
}

async function refreshCounts() {
  syncStatus.pending = (await idb.all("outbox")).length;
  syncStatus.failed = (await idb.all("failed")).length;
  syncStatus.offline = !navigator.onLine;
  notify();
}

// flush sends queued operations in order. It is safe to call any time.
async function flush() {
  if (flushing || !navigator.onLine) return refreshCounts();
  flushing = true;
  try {
    for (let retried = false; ; ) {
      const ops = (await idb.all("outbox")).sort((a, b) => (a.op_id < b.op_id ? -1 : 1)).slice(0, 100);
      if (ops.length === 0) break;
      const res = await fetch("/api/sync", {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken() },
        body: JSON.stringify({ ops }),
      });
      if (res.status === 401) {
        syncStatus.relogin = true;
        break;
      }
      if (res.status === 403 && !retried && (await refreshToken())) {
        retried = true;
        continue;
      }
      if (!res.ok) break;
      syncStatus.relogin = false;
      const { results } = await res.json();
      const { failed } = core.settle(ops, results);
      const answered = new Set(results.map((r) => r.op_id));
      for (const o of ops) if (answered.has(o.op_id)) await idb.del("outbox", o.op_id);
      for (const f of failed) await idb.put("failed", f);
    }
  } catch (err) {
    console.warn("sync failed, will retry", err);
  } finally {
    flushing = false;
    await refreshCounts();
  }
}

window.addEventListener("online", flush);
window.addEventListener("offline", refreshCounts);
setInterval(() => {
  if (syncStatus.pending > 0) flush();
}, 30_000);

// --- rendering helpers ---------------------------------------------------------

function h(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === null || v === undefined || v === false) continue;
    if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k === "class") el.className = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat()) if (c !== null && c !== undefined && c !== false) el.append(c);
  return el;
}

const fmt = (v) => String(Math.round(v * 100) / 100);
const weightText = (kg, unit) => (kg === null || kg === undefined ? "" : `${fmt(fromKg(kg, unit))} ${unit}`);
const repsText = (r) => (!r ? "" : r.amrap ? "AMRAP" : r.min === r.max ? String(r.min) : `${r.min}-${r.max}`);
const btn = "rounded bg-zinc-900 px-3 py-2 text-sm font-medium text-white dark:bg-zinc-100 dark:text-zinc-900";
const btn2 = "rounded border border-zinc-300 px-3 py-2 text-sm dark:border-zinc-700";
const input = "w-full rounded border border-zinc-300 bg-white px-2 py-2 text-lg dark:border-zinc-700 dark:bg-zinc-900";

function setText(set, unit) {
  const parts = [];
  if (set.weight_kg) parts.push(weightText(set.weight_kg, unit));
  if (set.reps !== null && set.reps !== undefined) parts.push(`× ${set.reps}`);
  if (set.duration_s) parts.push(`${set.duration_s}s`);
  if (set.distance_m) parts.push(`${set.distance_m} m`);
  if (set.rpe) parts.push(`@ ${set.rpe}`);
  return parts.join(" ") || "done";
}

// --- the companion ----------------------------------------------------------------

class Companion {
  constructor(root, boot) {
    this.root = root;
    this.boot = boot;
    this.unit = boot.unit;
    this.editing = null; // set id being edited from the overview
    this.timer = null;
    this.wakeLock = null;
    this.audio = null;
    this.onSync = () => this.renderSync();
  }

  async start() {
    const saved = await idb.get("sessions", this.boot.session.id).catch(() => null);
    this.state = core.mergeServerSets(saved || core.newState(this.boot), this.boot.sets);
    if (this.boot.session.finished) this.state.finished = true;
    await this.save();
    syncListeners.add(this.onSync);
    this.timer = setInterval(() => this.tick(), 250);
    this.onVisible = () => document.visibilityState === "visible" && this.lockScreen();
    document.addEventListener("visibilitychange", this.onVisible);
    this.lockScreen();
    // Keep a copy of this page for offline reloads, however the user got here.
    window.caches?.open("pages").then((c) => c.add(location.pathname)).catch(() => {});
    this.render();
    flush();
  }

  stop() {
    clearInterval(this.timer);
    syncListeners.delete(this.onSync);
    document.removeEventListener("visibilitychange", this.onVisible);
    this.wakeLock?.release().catch(() => {});
  }

  async lockScreen() {
    if (this.state?.finished || !("wakeLock" in navigator) || this.wakeLock) return;
    try {
      this.wakeLock = await navigator.wakeLock.request("screen");
      this.wakeLock.addEventListener("release", () => (this.wakeLock = null));
    } catch {
      // Not allowed right now (e.g. page hidden); retried when visible again.
    }
  }

  async save() {
    await idb.put("sessions", this.state);
  }

  // apply stores a new state and queues its operation, then syncs.
  async apply({ state, op }) {
    this.state = state;
    await this.save();
    if (op) await idb.put("outbox", op);
    this.render();
    await refreshCounts();
    flush();
  }

  async update(state) {
    this.state = state;
    await this.save();
    this.render();
  }

  // --- rest timer ---

  tick() {
    const el = this.root.querySelector("[data-rest]");
    if (!document.body.contains(this.root)) return this.stop(); // swapped away
    if (!el) return;
    const left = this.state.restEnd ? Math.ceil((this.state.restEnd - Date.now()) / 1000) : 0;
    if (left > 0) {
      el.textContent = `Rest ${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
      el.hidden = false;
    } else if (this.state.restEnd) {
      this.state.restEnd = null;
      this.save();
      el.hidden = true;
      this.alarm();
    } else {
      el.hidden = true;
    }
  }

  alarm() {
    navigator.vibrate?.([300, 150, 300]);
    try {
      this.audio ??= new AudioContext();
      const o = this.audio.createOscillator();
      const g = this.audio.createGain();
      o.frequency.value = 880;
      g.gain.setValueAtTime(0.2, this.audio.currentTime);
      g.gain.exponentialRampToValueAtTime(0.001, this.audio.currentTime + 0.6);
      o.connect(g).connect(this.audio.destination);
      o.start();
      o.stop(this.audio.currentTime + 0.6);
    } catch {
      // No audio available.
    }
  }

  // --- views ---

  render() {
    const b = this.boot;
    const s = this.state;
    const step = s.finished ? null : core.currentStep(b, s);
    this.root.replaceChildren(
      h("div", { class: "flex items-center justify-between" },
        h("h1", { class: "text-2xl font-semibold" }, b.session.name),
        h("span", { "data-sync": true, class: "text-sm" })),
      h("p", { "data-rest": true, class: "mt-2 text-3xl font-semibold tabular-nums", hidden: true }),
      s.finished ? this.finishedView() : step ? this.stepView(step) : this.doneView(),
      this.overview(),
      this.notesView(),
    );
    this.renderSync();
    this.tick();
  }

  renderSync() {
    const el = this.root.querySelector("[data-sync]");
    if (!el) return;
    const st = syncStatus;
    el.replaceChildren(
      st.relogin ? h("a", { href: "/auth/login", "hx-boost": "false", class: "text-red-600 underline" }, "Sign in again to sync")
        : st.failed > 0 ? h("span", { class: "text-red-600" }, `${st.failed} change(s) rejected`)
        : st.pending > 0 ? h("span", { class: "text-amber-600" }, `${st.pending} unsynced${st.offline ? " (offline)" : ""}`)
        : h("span", { class: "text-zinc-500" }, st.offline ? "offline" : "saved"));
  }

  stepView(step) {
    const b = this.boot;
    const t = core.target(b, this.state, step);
    const ex = core.groups(b, this.state)[step.g].exercises[step.e];
    const grp = core.groups(b, this.state)[step.g];
    const label = grp.exercises.length > 1 ? `${String.fromCharCode(65 + step.e)}${step.s + 1} · ` : "";
    const info = core.exerciseInfo(b, t.slug);
    const last = (info.last || []).map((x) => setText(x, this.unit)).join(", ");
    const fields = this.inputs(t);
    const form = h("form", {
      class: "mt-3 space-y-3",
      onsubmit: (e) => {
        e.preventDefault();
        this.apply(core.logSet(b, this.state, step, this.read(form, t)));
      },
    }, fields,
    h("div", { class: "flex flex-wrap gap-2" },
      h("button", { type: "submit", class: btn }, "Done"),
      h("button", { type: "button", class: btn2, onclick: () => this.update(core.skip(this.state, step)) }, "Skip"),
      h("button", { type: "button", class: btn2, onclick: () => this.update(core.addSet(this.state, step.g, step.e)) }, "Add set"),
      ex.alternatives.length > 0 && h("select", {
        class: btn2, "aria-label": "Swap exercise",
        onchange: (e) => e.target.value && this.update(core.swap(this.state, step.g, step.e, e.target.value, ex.planned)),
      }, h("option", { value: "" }, "Swap…"), ex.alternatives.map((a) => h("option", { value: a }, core.exerciseInfo(b, a).name)))));
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", { class: "text-sm text-zinc-500" }, `${label}${t.kind !== "working" ? t.kind : "set " + (step.s + 1)}`),
      h("h2", { class: "text-xl font-semibold" }, t.name),
      h("p", { class: "mt-1 text-lg" }, this.targetText(t)),
      t.per_side.length > 0 && h("p", { class: "text-sm text-zinc-500" }, `per side: ${t.per_side.map(fmt).join(" · ")}`),
      last && h("p", { class: "text-sm text-zinc-500" }, `Last time: ${last}`),
      form);
  }

  targetText(t) {
    const parts = [];
    if (t.duration_s) parts.push(`${t.duration_s}s`);
    else if (t.reps) parts.push(`${repsText(t.reps)} reps`);
    if (t.kg !== null) parts.push(`@ ${weightText(t.kg, this.unit)}`);
    else if (t.load_kind) parts.push("@ pick weight");
    if (t.rpe) parts.push(`RPE ${t.rpe}`);
    return parts.join(" ") || "as you like";
  }

  // inputs returns the fields a set of this measurement records, pre-filled
  // with the target.
  inputs(t, set = null) {
    const m = t.measurement;
    const field = (name, label, value, attrs = {}) =>
      h("label", { class: "block text-sm" }, label,
        h("input", { name, class: input, inputmode: "decimal", value: value ?? "", ...attrs }));
    const weight = set ? set.weight_kg : t.kg;
    const out = [];
    if (m === "weight_reps" || m === "bw_reps") {
      out.push(field("weight", m === "bw_reps" ? `Added weight (${this.unit})` : `Weight (${this.unit})`,
        weight !== null && weight !== undefined ? fmt(fromKg(weight, this.unit)) : ""));
    }
    if (m === "weight_reps" || m === "bw_reps" || m === "reps") {
      out.push(field("reps", "Reps", set ? set.reps : t.reps && !t.reps.amrap ? t.reps.min : "", { inputmode: "numeric" }));
      out.push(field("rpe", "RPE (optional)", set ? set.rpe : t.rpe, { placeholder: "6-10" }));
    }
    if (m === "time" || m === "distance_time") {
      out.push(field("duration", "Seconds", set ? set.duration_s : t.duration_s, { inputmode: "numeric" }));
    }
    if (m === "distance_time") out.push(field("distance", "Distance (m)", set ? set.distance_m : ""));
    return h("div", { class: "grid grid-cols-3 gap-2" }, out);
  }

  read(form, t) {
    const num = (name) => {
      const el = form.elements.namedItem(name);
      if (!el || el.value.trim() === "") return null;
      const v = Number(el.value.replace(",", "."));
      return Number.isFinite(v) ? v : null;
    };
    const w = num("weight");
    return {
      weight_kg: w === null ? (t.measurement === "bw_reps" ? 0 : null) : toKg(w, this.unit),
      reps: num("reps") === null ? null : Math.round(num("reps")),
      rpe: num("rpe"),
      duration_s: num("duration") === null ? null : Math.round(num("duration")),
      distance_m: num("distance"),
    };
  }

  doneView() {
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", {}, "All planned sets are done or skipped."),
      h("div", { class: "mt-3 flex flex-wrap gap-2" },
        h("button", { class: btn, onclick: () => this.finish() }, "Finish workout"),
        this.addExerciseControl()));
  }

  finishedView() {
    return h("section", { class: "mt-4 rounded border border-zinc-200 p-4 dark:border-zinc-800" },
      h("p", { class: "font-medium" }, "Workout finished."),
      h("p", { class: "mt-2 flex gap-4" },
        h("a", { href: `/history/${this.boot.session.id}`, class: "underline" }, "See it in history"),
        h("a", { href: "/", class: "underline" }, "Home")));
  }

  addExerciseControl() {
    return h("select", {
      class: btn2, "aria-label": "Add exercise",
      onchange: (e) => e.target.value && this.update(core.addExercise(this.boot, this.state, e.target.value)),
    }, h("option", { value: "" }, "Add exercise…"), (this.boot.catalog || []).map((c) => h("option", { value: c.slug }, c.name)));
  }

  async finish() {
    await this.apply(core.finish(this.state));
  }

  overview() {
    const b = this.boot;
    const s = this.state;
    const items = core.steps(b, s).map((step) => {
      const set = core.logged(s, step);
      const t = core.target(b, s, step);
      const st = core.status(s, step);
      if (set && this.editing === set.id) return this.editRow(step, set, t);
      return h("li", { class: "flex items-center justify-between gap-2 py-1", "data-status": st },
        h("span", {}, `${t.name} · ${step.s + 1}`),
        st === "done" ? h("button", { class: "text-sm underline", onclick: () => { this.editing = set.id; this.render(); } }, setText(set, this.unit))
          : st === "skipped" ? h("button", { class: "text-sm text-zinc-500 underline", onclick: () => this.update(core.unskip(s, step)) }, "skipped")
          : h("span", { class: "text-sm text-zinc-500" }, this.targetText(t)));
    });
    return h("details", { class: "mt-6", open: true },
      h("summary", { class: "cursor-pointer font-medium" }, "All sets"),
      h("ul", { class: "mt-2 divide-y divide-zinc-200 dark:divide-zinc-800" }, items),
      !s.finished && h("div", { class: "mt-3 flex flex-wrap gap-2" },
        this.addExerciseControl(),
        core.currentStep(b, s) && h("button", { class: btn2, onclick: () => this.finish() }, "Finish early")));
  }

  editRow(step, set, t) {
    const form = h("form", {
      class: "space-y-2 py-2",
      onsubmit: (e) => {
        e.preventDefault();
        this.editing = null;
        this.apply(core.logSet(this.boot, this.state, step, this.read(form, t)));
      },
    }, this.inputs(t, set),
    h("div", { class: "flex gap-2" },
      h("button", { type: "submit", class: btn }, "Save"),
      h("button", { type: "button", class: btn2, onclick: () => { this.editing = null; this.apply(core.deleteSet(this.state, set.id)); } }, "Delete"),
      h("button", { type: "button", class: btn2, onclick: () => { this.editing = null; this.render(); } }, "Cancel")));
    return h("li", {}, h("p", { class: "text-sm font-medium" }, `${t.name} · ${step.s + 1}`), form);
  }

  notesView() {
    const area = h("textarea", { class: input + " text-base", rows: "2", "aria-label": "Notes" });
    area.value = this.state.notes;
    return h("label", { class: "mt-6 block text-sm" }, "Notes",
      area,
      h("button", { class: btn2 + " mt-2", onclick: () => this.apply(core.editNotes(this.state, area.value)) }, "Save notes"));
  }
}

let current = null;

htmx.onLoad((root) => {
  const el = root.id === "companion" ? root : root.querySelector?.("#companion");
  if (!el || el.dataset.started) return;
  el.dataset.started = "1";
  const boot = JSON.parse(document.getElementById("companion-boot").textContent);
  current?.stop();
  current = new Companion(el, boot);
  current.start().catch((err) => {
    console.error(err);
    el.textContent = "The workout screen failed to start: " + err.message;
  });
});
```

- [ ] **Step 2: Write the service worker**

`internal/web/static/js/sw.js`:
```javascript
// Service worker (spec §9): keeps the app shell and open workouts available
// offline. Served at /sw.js (scope "/") with __VERSION__ replaced by a hash
// of the static files, so a new release replaces the cached shell.
const VERSION = "__VERSION__";
const SHELL = `shell-${VERSION}`;
const PAGES = "pages"; // live workout pages, kept across releases
const SHELL_FILES = [
  "/offline",
  "/static/app.css",
  "/static/vendor/htmx.min.js",
  "/static/vendor/jsoneditor/jse-theme-dark.css",
  "/static/js/calc.js",
  "/static/js/companion-core.js",
  "/static/js/companion.js",
  "/static/js/plan-editor.js",
];

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(SHELL).then((c) => c.addAll(SHELL_FILES)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith("shell-") && k !== SHELL).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

const isLive = (url) => /^\/sessions\/[^/]+\/live$/.test(url.pathname);

self.addEventListener("fetch", (event) => {
  const req = event.request;
  const url = new URL(req.url);
  if (req.method !== "GET" || url.origin !== location.origin) return;

  if (url.pathname.startsWith("/static/")) {
    // Cache first: static files are versioned with the shell.
    event.respondWith(caches.match(req).then((hit) => hit || fetch(req)));
    return;
  }
  if (req.mode === "navigate" && isLive(url)) {
    // Network first, so the page carries the latest sets; the copy serves offline reloads.
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            caches.open(PAGES).then((c) => c.put(url.pathname, copy));
          }
          return res;
        })
        .catch(() => caches.match(url.pathname, { cacheName: PAGES }).then((hit) => hit || caches.match("/offline"))),
    );
    return;
  }
  if (req.mode === "navigate") {
    event.respondWith(fetch(req).catch(() => caches.match("/offline")));
  }
  // Everything else (htmx requests, /api/*) goes to the network as usual.
});
```

- [ ] **Step 3: Check syntax and commit**

Run: `node --check internal/web/static/js/companion.js && node --check internal/web/static/js/sw.js`
Expected: no output.

```bash
git add internal/web/static/js/companion.js internal/web/static/js/sw.js
git commit -m "feat(web): add offline companion screen and service worker"
```

---

### Task 6: Pages, sync API, and wiring

**Files:**
- Create: `internal/web/sessions.go`, `internal/web/history.go`, `internal/web/views/sessions.templ`, `internal/web/views/history.templ`
- Replace: `internal/web/server.go`, `internal/web/plans.go`, `internal/web/views/pages.templ`, `internal/web/views/layout.templ`, `internal/web/views/models.go`, `internal/web/views/ui.go`, `internal/web/server_test.go`, `cmd/onerep/main.go`
- Create (generated): `internal/web/views/*_templ.go`, `internal/web/static/app.css`
- Test: `internal/web/sessions_test.go`

**Interfaces:**
- Consumes: `training.Service` (Task 3), `companion.js` and `sw.js` (Task 5)
- Produces:
  - `web.Server.Training *training.Service`; `(*Server).failJSON`; `writeJSON`
  - Public routes: `GET /sw.js`, `GET /offline`
  - Authenticated routes:
    - workouts: `POST /sessions` (`kind=plan|adhoc`), `GET /sessions/{id}/live`
    - sync: `POST /api/sync`, `GET /api/csrf`
    - history: `GET /history[?page=]`, `GET /history/{id}`, `POST /history/{id}/sets`, `POST /history/{id}/sets/{sid}/delete`, `POST /history/{id}/notes`, `POST /history/{id}/finish`, `POST /history/{id}/delete`
  - The home page shows "Workout in progress", "Start workout" and "Start an empty workout"; the nav gains History; the layout `<head>` loads `companion.js`.

- [ ] **Step 1: Write the failing tests**

Replace `internal/web/server_test.go` with (it wires the training service and plan history):
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
	"github.com/LongerHV/onerep/internal/training"
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
	s.Plans = &plan.Service{Store: db, Exercises: s.Exercises, History: db}
	s.Training = &training.Service{Store: db, Plans: s.Plans, Exercises: s.Exercises}
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

`internal/web/sessions_test.go`:
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
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/plan"
	"github.com/LongerHV/onerep/internal/training"
)

// startPlanned follows the starter plan and starts its first day; it returns
// the session id.
func startPlanned(t *testing.T, srvURL string, c *http.Client, csrf string) string {
	t.Helper()
	resp, _ := post(t, c, srvURL+"/plans", csrf, url.Values{"doc": {starter}, "action": {"activate"}})
	post(t, c, srvURL+"/plans/"+planID(t, resp)+"/follow", csrf, nil)
	post(t, c, srvURL+"/exercises/barbell-bench-press/settings", csrf, url.Values{"training_max": {"100"}})
	resp, _ = post(t, c, srvURL+"/sessions", csrf, url.Values{"kind": {"plan"}})
	m := regexp.MustCompile(`^/sessions/([0-9a-f-]+)/live$`).FindStringSubmatch(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusSeeOther || m == nil {
		t.Fatalf("start: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	return m[1]
}

func bootOf(t *testing.T, html string) training.Bootstrap {
	t.Helper()
	m := regexp.MustCompile(`(?s)<script id="companion-boot" type="application/json">(.*?)</script>`).FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no bootstrap in page:\n%.500s", html)
	}
	var b training.Bootstrap
	if err := json.Unmarshal([]byte(m[1]), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStartAndContinueWorkout(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)

	b := bootOf(t, read(t, mustGet(t, c, srv.URL+"/sessions/"+id+"/live")))
	bench := b.Exercises["barbell-bench-press"]
	if b.Session.ID != id || b.Snapshot.Name != "Upper" || bench.TMKg == nil || *bench.TMKg != 100 || b.Unit != "kg" {
		t.Fatalf("bootstrap = %+v", b.Session)
	}
	home := read(t, mustGet(t, c, srv.URL+"/"))
	if !strings.Contains(home, "Workout in progress: Upper") || strings.Contains(home, "Start workout") {
		t.Fatal("home does not offer to continue the open workout")
	}
	// Starting again continues the open workout.
	resp, _ := post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"adhoc"}})
	if resp.Header.Get("Location") != "/sessions/"+id+"/live" {
		t.Fatalf("second start: %s", resp.Header.Get("Location"))
	}
}

func TestStartWithoutPlan(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	resp, body := post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"plan"}})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "start an empty workout") {
		t.Fatalf("planned start without a plan: %d", resp.StatusCode)
	}
	resp, _ = post(t, c, srv.URL+"/sessions", csrf, url.Values{"kind": {"adhoc"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("empty workout: %d", resp.StatusCode)
	}
	b := bootOf(t, read(t, mustGet(t, c, srv.URL+resp.Header.Get("Location"))))
	if b.Session.Name != "Workout" || len(b.Snapshot.Groups) != 0 || len(b.Catalog) < 100 {
		t.Fatalf("ad-hoc bootstrap: %+v, %d catalog entries", b.Session, len(b.Catalog))
	}
}

func syncOps(t *testing.T, c *http.Client, srvURL, csrf string, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srvURL+"/api/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp, read(t, resp)
}

func setOpJSON(opID, setID, sessionID string, kg float64, reps int) string {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	op := map[string]any{"op_id": opID, "op": "upsert_set", "client_ts": now, "payload": map[string]any{
		"id": setID, "session_id": sessionID, "slug": "barbell-bench-press", "group_pos": 0, "exercise_pos": 0,
		"set_pos": 2, "kind": "working", "weight_kg": kg, "reps": reps, "rpe": 8, "done_at": now, "updated_at": now}}
	b, _ := json.Marshal(op)
	return string(b)
}

func TestSyncAPI(t *testing.T) {
	srv, c := newApp(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)
	op := setOpJSON("01900000-0000-7000-8000-0000000000a1", "01900000-0000-7000-8000-0000000000b1", id, 80, 5)

	if resp, _ := syncOps(t, c, srv.URL, "", `{"ops": [`+op+`]}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without CSRF: %d", resp.StatusCode)
	}
	resp, body := syncOps(t, c, srv.URL, csrf, `{"ops": [`+op+`, `+op+`]}`)
	var out struct {
		Results []training.OpResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("sync: %d %s", resp.StatusCode, body)
	}
	if len(out.Results) != 2 || out.Results[0].Status != "applied" || out.Results[1].Status != "duplicate" {
		t.Fatalf("results = %+v", out.Results)
	}
	if resp, _ := syncOps(t, c, srv.URL, csrf, `{"ops": [`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad JSON: %d", resp.StatusCode)
	}
	many := strings.TrimSuffix(strings.Repeat(`{"op_id": "x", "op": "upsert_set"},`, training.MaxOps+1), ",")
	if resp, body := syncOps(t, c, srv.URL, csrf, `{"ops": [`+many+`]}`); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "at most") {
		t.Fatalf("too many ops: %d %s", resp.StatusCode, body)
	}

	resp, body = getWith(t, c, srv.URL+"/api/csrf")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, csrf) {
		t.Fatalf("csrf endpoint: %d %s", resp.StatusCode, body)
	}
	// The synced set is in the next bootstrap, so another device picks it up.
	if b := bootOf(t, read(t, mustGet(t, c, srv.URL+"/sessions/"+id+"/live"))); len(b.Sets) != 1 || *b.Sets[0].WeightKg != 80 {
		t.Fatalf("bootstrap sets = %+v", b.Sets)
	}
}

func TestHistoryEditor(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	csrf := session(t, srv, c)
	id := startPlanned(t, srv.URL, c, csrf)
	setID := "01900000-0000-7000-8000-0000000000b2"
	syncOps(t, c, srv.URL, csrf, `{"ops": [`+setOpJSON("01900000-0000-7000-8000-0000000000a2", setID, id, 80, 5)+`]}`)

	list := read(t, mustGet(t, c, srv.URL+"/history"))
	if !strings.Contains(list, "/history/"+id) || !strings.Contains(list, "in progress") || !strings.Contains(list, "1 sets") {
		t.Fatalf("history list:\n%.600s", list)
	}
	page := read(t, mustGet(t, c, srv.URL+"/history/"+id))
	if !strings.Contains(page, "Barbell Bench Press") || !strings.Contains(page, `value="80"`) {
		t.Fatalf("history detail:\n%.600s", page)
	}

	// Correct the weight (in the user's unit), add a set, delete it again.
	edit := url.Values{"set_id": {setID}, "slug": {"barbell-bench-press"}, "kind": {"working"}, "group_pos": {"0"},
		"exercise_pos": {"0"}, "set_pos": {"2"}, "weight": {"82,5"}, "reps": {"5"}, "rpe": {"8"}}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, edit); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit: %d", resp.StatusCode)
	}
	add := url.Values{"slug": {"dumbbell-curl"}, "group_pos": {"5"}, "weight": {"12"}, "reps": {"10"}}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, add); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add: %d", resp.StatusCode)
	}
	page = read(t, mustGet(t, c, srv.URL+"/history/"+id))
	if !strings.Contains(page, `value="82.5"`) || !strings.Contains(page, "Dumbbell Curl") {
		t.Fatalf("after edit and add:\n%.800s", page)
	}
	bad := url.Values{"slug": {"dumbbell-curl"}, "weight": {"heavy"}}
	if resp, body := post(t, c, srv.URL+"/history/"+id+"/sets", csrf, bad); resp.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "weight must be a number") {
		t.Fatalf("bad weight: %d", resp.StatusCode)
	}
	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/sets/"+setID+"/delete", csrf, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete set: %d", resp.StatusCode)
	}
	post(t, c, srv.URL+"/history/"+id+"/notes", csrf, url.Values{"notes": {"shoulder ok"}})
	if page := read(t, mustGet(t, c, srv.URL+"/history/"+id)); strings.Contains(page, `value="82.5"`) || !strings.Contains(page, "shoulder ok") {
		t.Fatal("delete or notes did not apply")
	}

	// Finishing from history advances the plan to its next day.
	post(t, c, srv.URL+"/history/"+id+"/finish", csrf, nil)
	if home := read(t, mustGet(t, c, srv.URL+"/")); !strings.Contains(home, "Next: Lower") || strings.Contains(home, "Workout in progress") {
		t.Fatal("finishing did not advance the plan")
	}

	// Another user's workout is not there.
	ctx := context.Background()
	bob, _ := db.UpsertOIDCUser(ctx, "iss", "bob", "", "bob")
	ex := &exercise.Service{Store: db}
	ts := &training.Service{Store: db, Plans: &plan.Service{Store: db, Exercises: ex, History: db}, Exercises: ex}
	bobs, err := ts.StartAdHoc(ctx, bob)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/history/" + bobs.ID, "/sessions/" + bobs.ID + "/live"} {
		if resp := mustGet(t, c, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
	if resp, _ := post(t, c, srv.URL+"/history/"+bobs.ID+"/delete", csrf, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleting bob's workout: %d", resp.StatusCode)
	}

	if resp, _ := post(t, c, srv.URL+"/history/"+id+"/delete", csrf, nil); resp.Header.Get("Location") != "/history" {
		t.Fatalf("delete workout: %s", resp.Header.Get("Location"))
	}
}

func TestServiceWorkerAndOfflinePage(t *testing.T) {
	srv, c := newApp(t, "")
	resp, body := getWith(t, c, srv.URL+"/sw.js")
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript") ||
		strings.Contains(body, "__VERSION__") || !regexp.MustCompile(`const VERSION = "[0-9a-f]{12}"`).MatchString(body) {
		t.Fatalf("sw.js: %d %s\n%.200s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	resp, body = getWith(t, c, srv.URL+"/offline")
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "You are offline") || !strings.Contains(body, "<html") {
		t.Fatalf("offline page: %d", resp.StatusCode)
	}
}

// The companion keeps its queue when the session cookie has expired and asks
// the user to sign in again; that needs a 401, not a redirect to the IdP.
func TestSyncWithoutSessionIs401(t *testing.T) {
	srv, c := newApp(t, "")
	resp, _ := syncOps(t, c, srv.URL, "", `{"ops": []}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/web/`
Expected: FAIL, `s.Training undefined (type *Server has no field or method Training)`.

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
	Error        string
}

// HistoryGroup is a run of sets of one exercise within a session.
type HistoryGroup struct {
	Slug     string
	GroupPos int
	Sets     []store.Set
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

func optWeight(kg *float64, unit string) string {
	if kg == nil {
		return ""
	}
	return Number(calc.FromKg(*kg, unit))
}

func optFloat(v *float64) string {
	if v == nil {
		return ""
	}
	return Number(*v)
}

func optInt(v *int) string {
	if v == nil {
		return ""
	}
	return strconv.Itoa(*v)
}
```

Replace `internal/web/views/layout.templ` with:
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
			<meta name="htmx-config" content={ htmxConfig }/>
			<link rel="stylesheet" href="/static/app.css"/>
			<link rel="stylesheet" href="/static/vendor/jsoneditor/jse-theme-dark.css"/>
			<script src="/static/vendor/htmx.min.js" defer></script>
			<script type="module" src="/static/js/plan-editor.js"></script>
			<script type="module" src="/static/js/companion.js"></script>
		</head>
		<body
			class="min-h-screen bg-zinc-50 text-zinc-900 dark:bg-zinc-950 dark:text-zinc-100"
			hx-boost="true"
			hx-target="#main"
			hx-swap="innerHTML show:window:top"
			if p.Identity != nil {
				hx-headers={ csrfHeaders(p.Identity.CSRFToken) }
			}
		>
			<header class="border-b border-zinc-200 dark:border-zinc-800">
				<nav class="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-2 px-4 py-3">
					<div class="flex items-center gap-4">
						<a href="/" class="text-lg font-semibold">onerep</a>
						if p.Identity != nil {
							<a href="/plans" class="text-sm hover:underline">Plans</a>
							<a href="/exercises" class="text-sm hover:underline">Exercises</a>
							<a href="/equipment" class="text-sm hover:underline">Equipment</a>
							<a href="/history" class="text-sm hover:underline">History</a>
							<a href="/settings" class="text-sm hover:underline">Settings</a>
						}
					</div>
					if p.Identity != nil {
						<form method="post" action="/auth/logout" hx-boost="false" class="flex items-center gap-3 text-sm">
							<span>{ p.Identity.User.Name }</span>
							@CSRF(p)
							<button type="submit" class="rounded px-2 py-1 hover:bg-zinc-200 dark:hover:bg-zinc-800">Log out</button>
						</form>
					}
				</nav>
			</header>
			<main id="main" class="mx-auto max-w-3xl px-4 py-6">
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

templ Home(p Page, h HomePage) {
	<h1 class={ h1 }>Hi, { p.Identity.User.Name }</h1>
	if h.Notice != "" {
		<p class="mt-4 rounded bg-amber-100 p-2 text-sm dark:bg-amber-950">{ h.Notice }</p>
	}
	if h.Open != nil {
		<div class={ card + " mt-4" }>
			<p class="font-medium">Workout in progress: { h.Open.Name }</p>
			<a href={ templ.URL("/sessions/" + h.Open.ID + "/live") } class={ btn + " mt-3" }>Continue workout</a>
		</div>
	}
	switch  {
		case h.Next == nil:
			<p class="mt-2 text-zinc-600 dark:text-zinc-400">
				You are not following a plan. <a href="/plans" class="underline">Choose or create one</a>.
			</p>
			<ul class="mt-6 list-inside list-disc space-y-1">
				<li><a href="/exercises" class="underline">Browse exercises</a> and set training maxes</li>
				<li><a href="/equipment" class="underline">Set up your equipment</a> so weights round to what you can load</li>
			</ul>
			@startEmpty(p, h)
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
					if h.Open == nil {
						<form method="post" action="/sessions">
							@CSRF(p)
							<input type="hidden" name="kind" value="plan"/>
							<button type="submit" class={ btn }>Start workout</button>
						</form>
					}
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

templ SignedOut(p Page) {
	<h1 class={ h1 }>Signed out</h1>
	<p class="mt-2"><a href="/auth/login" class="underline" hx-boost="false">Sign in again</a></p>
}

templ Error(p Page, status int, message, requestID string) {
	<h1 class={ h1 }>{ strconv.Itoa(status) }</h1>
	<p class="mt-2">{ message }</p>
	if requestID != "" {
		<p class="mt-4 text-sm text-zinc-500">Request ID: <code>{ requestID }</code></p>
	}
}

templ startEmpty(p Page, h HomePage) {
	if h.Open == nil {
		<form method="post" action="/sessions" class="mt-4">
			@CSRF(p)
			<input type="hidden" name="kind" value="adhoc"/>
			<button type="submit" class={ btnSecondary }>Start an empty workout</button>
		</form>
	}
}
```

`internal/web/views/sessions.templ`:
```templ
package views

import "github.com/LongerHV/onerep/internal/training"

// SessionLive is the companion page. The screen itself is rendered by
// /static/js/companion.js from the embedded bootstrap, so it works offline.
templ SessionLive(p Page, boot training.Bootstrap) {
	<div id="companion" aria-live="polite">
		<p class="text-zinc-500">Loading your workout…</p>
		<noscript>The workout screen needs JavaScript. You can still log sets in <a href={ templ.URL("/history/" + boot.Session.ID) } class="underline">history</a>.</noscript>
	</div>
	@templ.JSONScript("companion-boot", boot)
}

templ Offline(p Page) {
	<h1 class={ h1 }>You are offline</h1>
	<p class="mt-2">This page needs a connection. A workout you already opened keeps working offline and syncs when you are back online.</p>
}
```

`internal/web/views/history.templ`:
```templ
package views

import (
	"strconv"

	"github.com/LongerHV/onerep/internal/store"
)

templ HistoryList(p Page, sessions []store.SessionSummary, pageNo int, more bool) {
	<h1 class={ h1 }>History</h1>
	if len(sessions) == 0 {
		<p class="mt-4 text-zinc-500">No workouts yet.</p>
	}
	<ul class="mt-4 divide-y divide-zinc-200 dark:divide-zinc-800">
		for _, s := range sessions {
			<li>
				<a href={ templ.URL("/history/" + s.ID) } class="flex items-baseline justify-between gap-4 py-2 hover:bg-zinc-100 dark:hover:bg-zinc-900">
					<span>
						{ s.Name }
						if s.FinishedAt == nil {
							<span class={ badge }>in progress</span>
						}
					</span>
					<span class="text-sm text-zinc-500">{ s.StartedAt.Local().Format("2006-01-02 15:04") } · { strconv.Itoa(s.Sets) } sets</span>
				</a>
			</li>
		}
	</ul>
	<div class="mt-4 flex gap-4">
		if pageNo > 0 {
			<a href={ templ.URL("/history?page=" + strconv.Itoa(pageNo-1)) } class="underline">Newer</a>
		}
		if more {
			<a href={ templ.URL("/history?page=" + strconv.Itoa(pageNo+1)) } class="underline">Older</a>
		}
	</div>
}

templ HistoryDetailPage(p Page, d HistoryDetail) {
	<p class="text-sm"><a href="/history" class="underline">History</a></p>
	<div class="flex flex-wrap items-start justify-between gap-2">
		<div>
			<h1 class={ h1 }>{ d.Session.Name }</h1>
			<p class="text-sm text-zinc-500">
				{ d.Session.StartedAt.Local().Format("Mon 2006-01-02 15:04") }
				if d.Session.FinishedAt == nil {
					<span class={ badge }>in progress</span>
				}
			</p>
		</div>
		<div class="flex flex-wrap gap-2">
			if d.Session.FinishedAt == nil {
				<a href={ templ.URL("/sessions/" + d.Session.ID + "/live") } class={ btn }>Continue workout</a>
				<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/finish") }>
					@CSRF(p)
					<button type="submit" class={ btnSecondary }>Mark finished</button>
				</form>
			}
			<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/delete") } onsubmit="return confirm('Delete this workout and all its sets?')">
				@CSRF(p)
				<button type="submit" class={ btnDanger }>Delete workout</button>
			</form>
		</div>
	</div>
	if d.Error != "" {
		<p class={ errorText + " mt-4" }>{ d.Error }</p>
	}
	for _, g := range d.Groups {
		<h2 class={ h2 }>{ d.Exercises[g.Slug].Name }</h2>
		<ul class="mt-2 space-y-2">
			for _, set := range g.Sets {
				<li class="flex flex-wrap items-end gap-2">
					<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/sets") } class="flex flex-wrap items-end gap-2">
						@CSRF(p)
						<input type="hidden" name="set_id" value={ set.ID }/>
						<input type="hidden" name="slug" value={ set.Slug }/>
						<input type="hidden" name="kind" value={ set.Kind }/>
						<input type="hidden" name="group_pos" value={ strconv.Itoa(set.GroupPos) }/>
						<input type="hidden" name="exercise_pos" value={ strconv.Itoa(set.ExercisePos) }/>
						<input type="hidden" name="set_pos" value={ strconv.Itoa(set.SetPos) }/>
						if set.Kind != "working" {
							<span class={ badge }>{ set.Kind }</span>
						}
						@setFields(p, d.Exercises[g.Slug].Measurement, set)
						<button type="submit" class={ btnSecondary }>Save</button>
					</form>
					<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/sets/" + set.ID + "/delete") }>
						@CSRF(p)
						<button type="submit" class="text-sm text-zinc-500 hover:underline">Delete</button>
					</form>
				</li>
			}
		</ul>
	}
	<h2 class={ h2 }>Add a set</h2>
	<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/sets") } class="mt-2 flex flex-wrap items-end gap-2">
		@CSRF(p)
		<input type="hidden" name="group_pos" value={ strconv.Itoa(d.NextGroupPos) }/>
		<label class={ label }>
			Exercise
			<select name="slug" class={ input }>
				for _, e := range d.Catalog {
					<option value={ e.Slug }>{ e.Name }</option>
				}
			</select>
		</label>
		@setFields(p, "all", store.Set{})
		<button type="submit" class={ btn }>Add</button>
	</form>
	<h2 class={ h2 }>Notes</h2>
	<form method="post" action={ templ.URL("/history/" + d.Session.ID + "/notes") } class="mt-2">
		@CSRF(p)
		<textarea name="notes" rows="3" class={ input } aria-label="Notes">{ d.Session.Notes }</textarea>
		<button type="submit" class={ btnSecondary + " mt-2" }>Save notes</button>
	</form>
}

// setFields shows the inputs a measurement records ("all" shows every field).
templ setFields(p Page, measurement string, set store.Set) {
	if measurement == "all" || measurement == "weight_reps" || measurement == "bw_reps" {
		<label class="text-sm">
			Weight ({ p.Identity.User.Unit })
			<input name="weight" inputmode="decimal" value={ optWeight(set.WeightKg, p.Identity.User.Unit) } class={ input + " w-24" }/>
		</label>
	}
	if measurement == "all" || measurement == "weight_reps" || measurement == "bw_reps" || measurement == "reps" {
		<label class="text-sm">
			Reps
			<input name="reps" inputmode="numeric" value={ optInt(set.Reps) } class={ input + " w-20" }/>
		</label>
		<label class="text-sm">
			RPE
			<input name="rpe" inputmode="decimal" value={ optFloat(set.RPE) } class={ input + " w-20" }/>
		</label>
	}
	if measurement == "all" || measurement == "time" || measurement == "distance_time" {
		<label class="text-sm">
			Seconds
			<input name="duration" inputmode="numeric" value={ optInt(set.DurationS) } class={ input + " w-24" }/>
		</label>
	}
	if measurement == "all" || measurement == "distance_time" {
		<label class="text-sm">
			Metres
			<input name="distance" inputmode="decimal" value={ optFloat(set.DistanceM) } class={ input + " w-24" }/>
		</label>
	}
}
```

- [ ] **Step 4: Handlers and wiring**

`internal/web/sessions.go`:
```go
package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) sessionRoutes(r chi.Router) {
	r.Post("/sessions", s.sessionStart)
	r.Get("/sessions/{id}/live", s.sessionLive)
	r.Post("/api/sync", s.apiSync)
	r.Get("/api/csrf", apiCSRF)
}

func (s *Server) sessionStart(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	start := s.Training.StartPlanned
	if r.PostFormValue("kind") == "adhoc" {
		start = s.Training.StartAdHoc
	}
	sess, err := start(r.Context(), u)
	var open training.OpenSessionError
	switch {
	case errors.As(err, &open):
		http.Redirect(w, r, "/sessions/"+open.ID+"/live", http.StatusSeeOther)
	case errors.Is(err, training.ErrNothingToStart):
		s.renderError(w, r, http.StatusConflict, "There is no planned day to start: follow a plan, or start an empty workout.")
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/sessions/"+sess.ID+"/live", http.StatusSeeOther)
	}
}

func (s *Server) sessionLive(w http.ResponseWriter, r *http.Request) {
	boot, err := s.Training.Bootstrap(r.Context(), user(r), chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.SessionLive(page(r, boot.Session.Name), boot))
}

type syncRequest struct {
	Ops []training.Op `json:"ops"`
}

type syncResponse struct {
	Results []training.OpResult `json:"results"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiSync applies the companion's queued operations (spec §9).
func (s *Server) apiSync(w http.ResponseWriter, r *http.Request) {
	var req syncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	results, err := s.Training.ApplyOps(r.Context(), user(r), req.Ops)
	var invalid training.InvalidError
	switch {
	case errors.As(err, &invalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": invalid.Reason})
	case err != nil:
		s.failJSON(w, r, err)
	default:
		writeJSON(w, http.StatusOK, syncResponse{Results: results})
	}
}

// apiCSRF returns the session's CSRF token, for a page loaded from the
// offline cache after the user signed in again.
func apiCSRF(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"csrf": id.CSRFToken})
}

// serviceWorker serves /sw.js with its cache version set to a hash of the
// static files, so every release refreshes the offline copy.
func serviceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	src, _ := fs.ReadFile(staticFS, "static/js/sw.js")
	_, _ = w.Write([]byte(strings.ReplaceAll(string(src), "__VERSION__", staticVersion())))
}

var staticVersion = sync.OnceValue(func() string {
	h := sha256.New()
	_ = fs.WalkDir(staticFS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(staticFS, path)
		h.Write([]byte(path))
		h.Write(b)
		return err
	})
	return hex.EncodeToString(h.Sum(nil))[:12]
})

func (s *Server) offline(w http.ResponseWriter, r *http.Request) {
	render(w, r, http.StatusOK, views.Offline(page(r, "Offline")))
}

// newSetID is a UUIDv7 for sets created in the history editor.
func newSetID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err) // crypto/rand failed
	}
	return id.String()
}
```

`internal/web/history.go`:
```go
package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/training"
	"github.com/LongerHV/onerep/internal/web/views"
)

func (s *Server) historyRoutes(r chi.Router) {
	r.Get("/history", s.historyList)
	r.Get("/history/{id}", s.historyDetail)
	r.Post("/history/{id}/sets", s.historySaveSet)
	r.Post("/history/{id}/sets/{sid}/delete", s.historyDeleteSet)
	r.Post("/history/{id}/notes", s.historyNotes)
	r.Post("/history/{id}/finish", s.historyFinish)
	r.Post("/history/{id}/delete", s.historyDelete)
}

func (s *Server) historyList(w http.ResponseWriter, r *http.Request) {
	pageNo, _ := strconv.Atoi(r.URL.Query().Get("page"))
	sessions, more, err := s.Training.History(r.Context(), user(r), pageNo)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.HistoryList(page(r, "History"), sessions, max(pageNo, 0), more))
}

// historyData loads a session with the names and measurements of its exercises.
func (s *Server) historyData(r *http.Request, id string) (views.HistoryDetail, error) {
	ctx, u := r.Context(), user(r)
	sess, sets, err := s.Training.Session(ctx, u, id)
	if err != nil {
		return views.HistoryDetail{}, err
	}
	d := views.HistoryDetail{Session: sess, Exercises: map[string]store.Exercise{}}
	catalog, err := s.Exercises.Catalog(ctx, u.ID, "")
	if err != nil {
		return d, err
	}
	d.Catalog = catalog
	for _, set := range sets {
		if _, ok := d.Exercises[set.Slug]; !ok {
			ex, err := s.Exercises.Get(ctx, u.ID, set.Slug)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return d, err
			}
			if errors.Is(err, store.ErrNotFound) {
				ex = store.Exercise{Slug: set.Slug, Name: set.Slug, Measurement: "weight_reps"}
			}
			d.Exercises[set.Slug] = ex
		}
		if n := len(d.Groups); n == 0 || d.Groups[n-1].Slug != set.Slug || d.Groups[n-1].GroupPos != set.GroupPos {
			d.Groups = append(d.Groups, views.HistoryGroup{Slug: set.Slug, GroupPos: set.GroupPos})
		}
		d.Groups[len(d.Groups)-1].Sets = append(d.Groups[len(d.Groups)-1].Sets, set)
		d.NextGroupPos = max(d.NextGroupPos, set.GroupPos+1)
	}
	return d, nil
}

func (s *Server) historyDetail(w http.ResponseWriter, r *http.Request) {
	d, err := s.historyData(r, chi.URLParam(r, "id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.HistoryDetailPage(page(r, d.Session.Name), d))
}

// parseSetForm reads a set from the history editor. Weights are in the user's unit.
func parseSetForm(r *http.Request, unit string) (training.SetInput, error) {
	f := r.PostForm
	num := func(name string) (*float64, error) {
		v := strings.TrimSpace(strings.ReplaceAll(f.Get(name), ",", "."))
		if v == "" {
			return nil, nil
		}
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, training.InvalidError{Reason: name + " must be a number"}
		}
		return &n, nil
	}
	integer := func(name string) (*int, error) {
		n, err := num(name)
		if err != nil || n == nil {
			return nil, err
		}
		i := int(*n)
		return &i, nil
	}
	in := training.SetInput{ID: f.Get("set_id"), Slug: f.Get("slug"), Kind: f.Get("kind")}
	var err error
	if in.WeightKg, err = num("weight"); err != nil {
		return in, err
	}
	if in.WeightKg != nil {
		kg := calc.ToKg(*in.WeightKg, unit)
		in.WeightKg = &kg
	}
	if in.Reps, err = integer("reps"); err != nil {
		return in, err
	}
	if in.RPE, err = num("rpe"); err != nil {
		return in, err
	}
	if in.DurationS, err = integer("duration"); err != nil {
		return in, err
	}
	if in.DistanceM, err = num("distance"); err != nil {
		return in, err
	}
	for name, dst := range map[string]*int{"group_pos": &in.GroupPos, "exercise_pos": &in.ExercisePos, "set_pos": &in.SetPos} {
		if v, err := strconv.Atoi(f.Get(name)); err == nil {
			*dst = v
		}
	}
	if in.Kind == "" {
		in.Kind = "working"
	}
	return in, nil
}

func (s *Server) historySaveSet(w http.ResponseWriter, r *http.Request) {
	u, id := user(r), chi.URLParam(r, "id")
	in, err := parseSetForm(r, u.Unit)
	if err == nil {
		in.SessionID = id
		if in.ID == "" { // a new set, done now
			in.ID = newSetID()
			now := time.Now().UTC()
			in.DoneAt = &now
		} else if sets := s.existingSet(r, id, in.ID); sets != nil {
			in.DoneAt, in.Prescribed = sets.DoneAt, sets.Prescribed
		}
		err = s.Training.SaveSet(r.Context(), u, in)
	}
	s.afterHistoryEdit(w, r, id, err)
}

// existingSet returns the stored set being edited, so edits keep when it was
// done and what was prescribed.
func (s *Server) existingSet(r *http.Request, sessionID, setID string) *store.Set {
	_, sets, err := s.Training.Session(r.Context(), user(r), sessionID)
	if err != nil {
		return nil
	}
	for i := range sets {
		if sets[i].ID == setID {
			return &sets[i]
		}
	}
	return nil
}

func (s *Server) historyDeleteSet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.DeleteSet(r.Context(), user(r), id, chi.URLParam(r, "sid")))
}

func (s *Server) historyNotes(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.SetNotes(r.Context(), user(r), id, r.PostFormValue("notes")))
}

func (s *Server) historyFinish(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.afterHistoryEdit(w, r, id, s.Training.Finish(r.Context(), user(r), id))
}

func (s *Server) historyDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.Training.Delete(r.Context(), user(r), chi.URLParam(r, "id")); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/history", http.StatusSeeOther)
}

func (s *Server) afterHistoryEdit(w http.ResponseWriter, r *http.Request, id string, err error) {
	var invalid training.InvalidError
	switch {
	case errors.As(err, &invalid):
		d, derr := s.historyData(r, id)
		if derr != nil {
			s.fail(w, r, derr)
			return
		}
		d.Error = invalid.Reason
		render(w, r, http.StatusUnprocessableEntity, views.HistoryDetailPage(page(r, d.Session.Name), d))
	case err != nil:
		s.fail(w, r, err)
	default:
		http.Redirect(w, r, "/history/"+id, http.StatusSeeOther)
	}
}
```

Replace `internal/web/plans.go` with (the home page also loads the open workout):
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
	home := views.HomePage{Next: next}
	if open, err := s.Training.Open(r.Context(), user(r)); err == nil {
		home.Open = &open
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.Home(page(r, "Home"), home))
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
	render(w, r, http.StatusOK, views.PlanPreview(fragmentPage(r), ps, weeks))
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
	"github.com/LongerHV/onerep/internal/training"
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
	Training  *training.Service
}

// Routes returns the application's HTTP handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, logRequests, s.recoverer, s.Sessions.Middleware)
	r.NotFound(s.layout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.renderError(w, r, http.StatusNotFound, "Page not found.")
	})).ServeHTTP)

	r.Get("/healthz", s.healthz)
	r.Get("/schema/plan.json", planSchema)
	r.Get("/sw.js", serviceWorker)
	r.With(s.layout).Get("/offline", s.offline)
	r.Handle("/static/*", staticHandler())
	r.With(s.layout).Get("/auth/signed-out", func(w http.ResponseWriter, r *http.Request) {
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
		r.Use(s.layout)
		if s.DevUser != "" {
			r.Use(auth.DevLogin(s.Sessions, s.DevUser))
		}
		r.Use(auth.RequireUser, s.limitBody, auth.CSRF, s.starterEquipment)
		r.Get("/", s.home)
		r.Post("/auth/logout", s.logout)
		s.exerciseRoutes(r)
		s.planRoutes(r)
		s.sessionRoutes(r)
		s.historyRoutes(r)
		s.equipmentRoutes(r)
		r.Get("/settings", s.settings)
		r.Post("/settings", s.saveSettings)
	})
	return r
}

// page builds the data for a page titled title and marks the response as a
// page, so the layout middleware wraps it (see layout.go).
func page(r *http.Request, title string) views.Page {
	p := fragmentPage(r)
	p.Title = title
	markPage(r, p)
	return p
}

// fragmentPage is the page data for rendering a fragment, which the layout
// middleware leaves alone.
func fragmentPage(r *http.Request) views.Page {
	var p views.Page
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
				s.layout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					s.renderError(w, r, http.StatusInternalServerError, "Something went wrong.")
				})).ServeHTTP(w, r)
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

// failJSON is fail for JSON endpoints (spec §16).
func (s *Server) failJSON(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := middleware.GetReqID(r.Context())
	slog.ErrorContext(r.Context(), "request failed", "err", err, "request_id", id)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong", "request_id": id})
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
	"github.com/LongerHV/onerep/internal/training"
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
	srv.Plans = &plan.Service{Store: db, Exercises: srv.Exercises, History: db}
	srv.Training = &training.Service{Store: db, Plans: srv.Plans, Exercises: srv.Exercises}
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

Run: `task generate && go vet ./... && go test ./internal/web/ ./cmd/... && golangci-lint run ./...`
Expected: both `ok`, `0 issues.`

- [ ] **Step 6: Commit**

```bash
git add internal/web cmd/onerep
git commit -m "feat(web): add workouts, sync API and history editor"
```

---

### Task 7: Browser end-to-end check, CI, and docs

**Files:**
- Create: `test/e2e/companion.mjs`
- Replace: `Taskfile.yml`, `.github/workflows/ci.yml`, `AGENTS.md`

**Interfaces:**
- Consumes: everything above; Chromium from `nix build --inputs-from . nixpkgs#chromium`
- Produces: the `task e2e` task and a separate `e2e` CI job

- [ ] **Step 1: Write the scenario**

`test/e2e/companion.mjs`: every wait that concerns sync polls the server itself, because the badge can say "saved" in the moment before a new operation is queued:
```javascript
// End-to-end check of companion mode's offline logging (spec §9, §17).
// Starts onerep and headless Chromium, then drives a workout over the Chrome
// DevTools Protocol: log a set, lose the server, keep logging and editing,
// reload from the offline cache, get the server back, and check every set
// arrived exactly once. No npm dependencies: Node's built-in WebSocket.
//
// Usage: ONEREP_BIN=bin/onerep CHROMIUM=chromium node test/e2e/companion.mjs
// (task e2e builds the binary and provides Chromium from nixpkgs).
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const BIN = process.env.ONEREP_BIN || "bin/onerep";
const CHROMIUM = process.env.CHROMIUM || "chromium";
const PORT = 18100 + Math.floor(Math.random() * 800);
const BASE = `http://127.0.0.1:${PORT}`;
const dir = mkdtempSync(join(tmpdir(), "onerep-e2e-"));
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PLAN = JSON.stringify({
  name: "E2E", weeks: 1, days: [
    { name: "Bench day", groups: [{ rest_s: 60, exercises: [{ slug: "barbell-bench-press", sets: [{ count: 4, reps: 5, load: { pct_tm: 0.8 } }] }] }] },
    { name: "Second day", groups: [{ exercises: [{ slug: "pull-up", sets: [{ count: 1, reps: 5 }] }] }] },
  ],
});

let server = null;
function startServer() {
  server = spawn(BIN, ["serve"], {
    env: { ...process.env, ONEREP_ENV: "dev", ONEREP_DEV_USER: "e2e", ONEREP_DB: join(dir, "e2e.db"), ONEREP_LISTEN: `127.0.0.1:${PORT}` },
    stdio: ["ignore", "ignore", "pipe"],
  });
  server.stderr.on("data", (d) => process.env.E2E_VERBOSE && process.stderr.write(d));
  return waitFor(async () => (await fetch(`${BASE}/healthz`).catch(() => null))?.ok, "server to start");
}
async function stopServer() {
  const exited = new Promise((r) => server.once("exit", r));
  server.kill("SIGTERM");
  await exited;
}

async function waitFor(check, what, timeout = 15_000) {
  const end = Date.now() + timeout;
  for (;;) {
    if (await check()) return;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(100);
  }
}

// --- CDP ---------------------------------------------------------------------

async function browser() {
  const proc = spawn(CHROMIUM, ["--headless=new", "--no-sandbox", "--disable-gpu", "--remote-debugging-port=0",
    `--user-data-dir=${join(dir, "chrome")}`, "about:blank"], { stdio: ["ignore", "ignore", "pipe"] });
  const wsURL = await new Promise((resolve, reject) => {
    let buf = "";
    proc.stderr.on("data", (d) => {
      buf += d;
      const m = buf.match(/DevTools listening on (ws:\/\/\S+)/);
      if (m) resolve(m[1]);
    });
    proc.on("exit", () => reject(new Error("chromium exited:\n" + buf)));
  });
  const http = wsURL.replace("ws://", "http://").replace(/\/devtools\/.*/, "");
  const target = await (await fetch(`${http}/json/new?about:blank`, { method: "PUT" })).json();
  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((r) => ws.addEventListener("open", r, { once: true }));
  let id = 0;
  const pending = new Map();
  ws.addEventListener("message", (e) => {
    const msg = JSON.parse(e.data);
    if (msg.id && pending.has(msg.id)) pending.get(msg.id)(msg);
  });
  const send = (method, params = {}) => new Promise((resolve) => {
    const i = ++id;
    pending.set(i, resolve);
    ws.send(JSON.stringify({ id: i, method, params }));
  });
  const evaluate = async (expression) => {
    const res = await send("Runtime.evaluate", { expression, awaitPromise: true, returnByValue: true });
    if (res.result?.exceptionDetails) throw new Error(`in page: ${res.result.exceptionDetails.exception?.description}`);
    return res.result?.result?.value;
  };
  await send("Runtime.enable");
  await send("Page.enable");
  return { proc, ws, send, evaluate };
}

// --- the scenario ---------------------------------------------------------------

const results = [];
function check(name, ok, detail = "") {
  results.push({ name, ok });
  console.log(`${ok ? "ok  " : "FAIL"} ${name}${ok ? "" : "  " + detail}`);
}

let b;
try {
  await startServer();
  b = await browser();
  const { send, evaluate } = b;
  const go = async (path) => {
    await send("Page.navigate", { url: BASE + path });
    await waitFor(() => evaluate("document.readyState === 'complete'"), `load of ${path}`);
  };
  const csrf = "JSON.parse(document.body.getAttribute('hx-headers'))['X-CSRF-Token']";
  const form = (path, fields) => evaluate(`fetch(${JSON.stringify(path)}, {method: 'POST',
    body: new URLSearchParams({...${JSON.stringify(fields)}, csrf_token: ${csrf}})}).then(r => r.url)`);

  // A plan with four bench sets at 80% of a 100 kg training max.
  await go("/");
  const planURL = await form("/plans", { doc: PLAN, action: "activate" });
  await form(new URL(planURL).pathname + "/follow", {});
  await form("/exercises/barbell-bench-press/settings", { training_max: "100" });

  // Start from the home page (a boosted form: no full page load).
  await go("/");
  await evaluate(`[...document.querySelectorAll('button')].find(b => b.textContent.trim() === 'Start workout').click()`);
  const doneButton = `[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Done')`;
  await waitFor(() => evaluate(`!!${doneButton}`), "the workout screen");
  const sessionID = await evaluate("location.pathname.split('/')[2]");
  check("start workout opens the live page", /^[0-9a-f-]{36}$/.test(sessionID), sessionID);
  check("target is 80% of the TM", await evaluate(`document.querySelector('#companion').textContent.includes('@ 80 kg')`));

  const done = () => evaluate(`document.querySelectorAll('#companion [data-status=done]').length`);
  const synced = () => evaluate(`(document.querySelector('#companion [data-sync]')?.textContent || '').includes('saved')`);

  // Set 1 online.
  await evaluate(`${doneButton}.click()`);
  // Wait on the server itself: the badge can say "saved" before the new operation is queued.
  const serverSets = () => evaluate(`fetch("/history/${sessionID}").then(r => r.text()).then(t => (t.match(/name="set_id"/g) || []).length)`);
  await waitFor(async () => (await done()) === 1 && (await serverSets()) === 1, "set 1 to reach the server");
  check("set 1 logged and synced", true);
  await waitFor(() => evaluate("navigator.serviceWorker.controller !== null || (location.reload(), false)"), "the service worker", 20_000)
    .catch(() => {});

  // The server goes away. Keep training.
  await stopServer();
  await evaluate(`document.querySelector('input[name=weight]').value = '82.5'; ${doneButton}.click()`);
  await waitFor(async () => (await done()) === 2, "set 2 locally");
  await evaluate(`${doneButton}.click()`);
  await waitFor(async () => (await done()) === 3, "set 3 locally");
  // Correct set 1 to 7 reps.
  await evaluate(`document.querySelector('#companion [data-status=done] button').click()`);
  await evaluate(`const f = document.querySelector('#companion li form'); f.querySelector('input[name=reps]').value = '7';
    [...f.querySelectorAll('button')].find(b => b.textContent === 'Save').click()`);
  await waitFor(() => evaluate(`document.querySelector('#companion [data-status=done] button').textContent.includes('× 7')`), "the edit");
  check("logging works without the server", await evaluate(
    `(document.querySelector('#companion [data-sync]').textContent || '').includes('unsynced')`), "badge should show unsynced changes");

  // Reload with the server still down: the page comes from the offline cache,
  // the sets from IndexedDB.
  await send("Page.reload");
  await waitFor(() => evaluate(`!!${doneButton}`), "the cached workout screen", 20_000);
  check("offline reload keeps all 3 sets", (await done()) === 3, `found ${await done()}`);

  // Back online: everything syncs once.
  await startServer();
  await send("Page.reload");
  await waitFor(async () => (await synced()) && (await done()) === 3, "sync after reconnecting", 20_000);
  const history = await evaluate(`fetch('/history/${sessionID}').then(r => r.text())`);
  const rows = (history.match(/name="set_id"/g) || []).length;
  check("server has exactly 3 sets", rows === 3, `found ${rows}`);
  check("the offline edit reached the server", /name="reps"[^>]*value="7"/.test(history));
  check("the offline weight change reached the server", history.includes('value="82.5"'));

  // Replaying everything again changes nothing.
  await send("Page.reload");
  await waitFor(() => synced(), "idle sync");
  const again = await evaluate(`fetch('/history/${sessionID}').then(r => r.text())`);
  check("reloading does not duplicate sets", (again.match(/name="set_id"/g) || []).length === 3);

  // Finish: the plan moves on to its second day.
  await evaluate(`[...document.querySelectorAll('#companion button')].find(b => /Finish/.test(b.textContent)).click()`);
  await waitFor(() => evaluate(`document.querySelector('#companion').textContent.includes('Workout finished')`), "the finished screen");
  let home = "";
  await waitFor(async () => (home = await evaluate(`fetch('/').then(r => r.text())`)).includes("Next: Second day"), "the plan to move on")
    .catch(() => {});
  check("finishing advances the plan", home.includes("Next: Second day"), (home.match(/<main[\s\S]*?<\/main>/) || [""])[0].replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").slice(0, 400));
} catch (err) {
  check("scenario ran to the end", false, err.stack || String(err));
} finally {
  b?.ws.close();
  b?.proc.kill("SIGKILL");
  if (server && server.exitCode === null) await stopServer().catch(() => {});
  rmSync(dir, { recursive: true, force: true });
}

const failed = results.filter((r) => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} checks passed`);
process.exit(failed ? 1 : 0);
```

- [ ] **Step 2: Task, CI job, and docs**

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

  e2e:
    desc: Browser end-to-end check of offline workout logging (headless Chromium from the flake's nixpkgs)
    deps: [build]
    cmds:
      - CHROMIUM="$(nix build --no-link --print-out-paths --inputs-from . nixpkgs#chromium)/bin/chromium" ONEREP_BIN=bin/onerep node test/e2e/companion.mjs

  image:
    desc: Build the container image locally without pushing
    deps: [licenses]
    cmds:
      - KO_DOCKER_REPO=ko.local ko build --bare --push=false ./cmd/onerep
```

Replace `.github/workflows/ci.yml` with:
```yaml
name: CI

on:
  push:
    branches: [master]
  pull_request:

permissions:
  contents: read

jobs:
  check:
    name: Generate, lint, test, migrations
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: cachix/install-nix-action@v31
        with:
          github_access_token: ${{ secrets.GITHUB_TOKEN }}
      - uses: actions/cache@v6
        with:
          path: |
            ~/go/pkg/mod
            ~/.cache/go-build
            ~/.cache/golangci-lint
          key: go-${{ runner.os }}-${{ hashFiles('go.sum') }}
          restore-keys: go-${{ runner.os }}-
      - run: nix develop --command task ci

  e2e:
    name: Browser end-to-end (offline logging)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: cachix/install-nix-action@v31
        with:
          github_access_token: ${{ secrets.GITHUB_TOKEN }}
      - run: nix develop --command task e2e

  image:
    name: Build image (no push)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7
      - uses: cachix/install-nix-action@v31
        with:
          github_access_token: ${{ secrets.GITHUB_TOKEN }}
      - run: nix develop --command task licenses
      - run: nix develop --command ko build --bare --push=false ./cmd/onerep
        env:
          KO_DOCKER_REPO: example.invalid/onerep

  pr-title:
    name: Conventional PR title
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    permissions:
      pull-requests: read
    steps:
      - uses: amannn/action-semantic-pull-request@v6
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Replace `AGENTS.md` with:
````markdown
# AGENTS.md

Guidance for AI coding agents (and humans) working on onerep. `CLAUDE.md` is a symlink to this file.

## What this is

onerep is a self-hosted gym progress tracker: a single Go binary serving a PWA (templ + htmx + Tailwind), SQLite storage, OIDC login, and an MCP server so an AI can analyse history and draft training plans.

- Design spec: `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. Read it before changing behaviour.
- Implementation plans: `docs/superpowers/plans/`. One plan per milestone.
- Known minor issues, deferred on purpose: `docs/backlog.md`. Fix entries when you touch that code, and remove them when fixed.

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
| Browser end-to-end check (offline workout logging, headless Chromium) | `task e2e` |
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
internal/training/     sessions, companion sync operations (idempotent, last write wins), bootstrap, history editing
internal/web/          chi router, handlers, views/ (templ), static/ (embedded), jstest/ (node tests)
                         static/js/companion*.js + sw.js: offline workout screen and service worker
test/e2e/              browser end-to-end checks (Node + Chrome DevTools Protocol, no npm deps)
testdata/calc_cases.json  shared Go/JS calc test vectors
```

Later milestones add `stats/` and `mcp/` under `internal/`. See the spec, §4.

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
- **Pages vs fragments:** page templates render only their content, never `@Layout`. A handler that renders a page calls `page(r, title)`; the `layout` middleware (`internal/web/layout.go`) then wraps it in the full document for direct visits, reloads and history restores, and sends content + `<title>` to htmx navigation (`<body hx-boost>` swaps it into `#main`). Fragments for a specific `hx-target` use `fragmentPage(r)` and are never wrapped. Links and forms that must do a real page load (login, logout, non-HTML resources) get `hx-boost="false"`. Page scripts go in the layout `<head>` and set themselves up via `htmx.onLoad`, since swapped-in `<script type="module">` runs only once.
- **Calc parity:** `internal/calc` (Go) and `internal/web/static/js/calc.js` implement the same math. Change both together and add a case to `testdata/calc_cases.json`; `task test` and `task test:js` both run it.
- **Seed catalog:** edit `internal/exercise/seed/exercises.json`; slugs are permanent (plans and history refer to them). Removing an entry hides it, never deletes it. `TestSeedIsValid` checks the file.
- **Plan documents:** `internal/plan/plan.schema.json` is the contract for the editor, the server and the AI. Change the schema, the Go types in `internal/plan/doc.go` and the semantic checks together; `internal/plan/testdata/*.golden.json` pins expansion (`go test ./internal/plan/ -update` rewrites it, so review the diff).
- **Companion mode:** the workout screen must keep working offline. Its logic lives in `companion-core.js` (pure, covered by `jstest/companion.test.mjs`); `companion.js` only renders and persists. Every change is an operation with a UUIDv7 `op_id` applied idempotently by `/api/sync`; never make the server depend on operation order across sessions. Run `task e2e` after touching the companion, the service worker or the sync endpoint.
- **Every non-GET request with a session must carry the CSRF token**: the `X-CSRF-Token` header, set globally for htmx via `hx-headers`, or the `csrf_token` form field.
- **The dev login bypass** (`ONEREP_DEV_USER`) only runs with `ONEREP_ENV=dev`, and only on authenticated app routes.
- **Keep dependencies few.** Ask before adding a Go module or a vendored JS library.
- **Licensing:** onerep is AGPL-3.0-only. Dependencies must use MIT, BSD-2/3-Clause, ISC, 0BSD or Apache-2.0. `task licenses:check` enforces this for Go modules. A vendored file gets its license next to it (`<name>.LICENSE`, or `LICENSE` inside its own vendor directory). Keep the footer link to the source code.
````

Run: `nix run nixpkgs#actionlint -- .github/workflows/*.yml`
Expected: no output.

- [ ] **Step 3: Run the end-to-end check (three times, to catch flakiness)**

Run: `task e2e && task e2e && task e2e`
Expected: each run ends with `10/10 checks passed`. The first run downloads Chromium (several hundred MB).

- [ ] **Step 4: Full CI and commit**

Commit, then run `task ci`.
Expected: exit 0. All 10 Go packages are `ok` (adding `training`), Node prints `ℹ pass 18`, and lint reports `0 issues.`

```bash
git add test/e2e Taskfile.yml .github/workflows/ci.yml AGENTS.md
git commit -m "test(e2e): check offline workout logging in headless Chromium"
```

---

## Done when

- `task ci` and `task e2e` pass on a clean checkout.
- In `task dev`, the user can start the planned day from the home page and log sets with targets, plates and last time's results. The rest timer counts down, and supersets alternate.
- Swapping an exercise, adding sets and exercises, skipping, and editing or deleting logged sets all work.
- With the server stopped or the network off, logging and reloading keep working; after reconnecting everything syncs exactly once.
- Finishing moves the plan to its next day.
- History lists workouts and edits their sets, notes and status.
