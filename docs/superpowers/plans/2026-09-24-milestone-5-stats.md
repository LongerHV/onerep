# onerep Milestone 5: Stats — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show progress from logged sets:
- An e1RM-over-time chart and a rep-max (PR) table on each exercise page.
- PR badges in history and, as advisory badges, in the offline workout screen.
- A weekly hard-sets-per-muscle page with a table and a stacked bar chart.

**Architecture:**
- **`internal/store`** gains four read-only queries in a new `stats.go`: rep maxes, session-best e1RM per session, PR sets of a session, and hard sets in a date range. It needs no schema change, because spec §5 says stats are queries over `sets`.
- **New `internal/stats` service:** turns those rows into what the pages, and later the MCP tools, need:
  - an e1RM series with an RPE-based or rep-based flag from `calc.E1RM`
  - the 1–12 rep-max table
  - weekly muscle volume, bucketed into ISO weeks, with primary muscles counting 1 and secondary muscles 0.5
- **Web:**
  - Pages render the tables on the server.
  - Charts fetch JSON from `/api/stats/...`, and `stats.js` draws them with vendored uPlot. `chart-data.js` holds pure, Node-tested data shaping.
  - `/api/` GET requests without a login get 401 instead of a redirect.
- **Companion:** the bootstrap carries each exercise's rep-max table (excluding the session itself). `companion-core.js` gains `isPR`.

**Tech Stack:** unchanged, plus vendored **uPlot 1.6.32** (MIT, `internal/web/static/vendor/uplot/`). The spec (§3) names uPlot, and AGENTS.md asks for approval before vendoring a JS library, so the user confirms it at plan review. No new Go modules.

**Spec:** `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. This plan implements:
- §13 stats: the exercise page, PR detection and the muscles page
- §9 the PR table in the companion bootstrap and the advisory PR badge after Done
- §4 the `stats/` package

The MCP tools that read these stats (`get_exercise_stats`, `get_weekly_muscle_volume`) come in milestone 6. The service is shaped for them: it takes kg in and out, and dates as a range.

**Deliberate differences from and clarifications of spec §13:**
- **PRs are computed when shown, not stored on write.** "Server computes on write" becomes a query over the stored sets each time a PR is shown. A stored flag would go stale when an earlier set is edited or deleted in history, and this needs no schema change.
- **The first set at a rep count is not a PR.** There's no previous best to exceed (the spec's wording), and otherwise every set of a new exercise would be badged.
- **Ties on "previous"** are broken by `done_at`, then set id. Weight ties are never PRs, because the set must *exceed* the previous best.
- **e1RM points** are the best non-warmup set of each session, dated by that set's `done_at`. A point is "RPE-based" when `calc.E1RM` used the RTS table (RPE present and in range), otherwise "rep-based".
- **ISO weeks are UTC.** Users have no time zone yet (see `docs/backlog.md`, "History times are in server time"), so a set logged just after local midnight on a Monday may count in the previous week.
- **Hard sets need a `done_at`**, and sets of unknown or deleted exercises count toward no muscle.

## Global Constraints

Everything from milestones 1–4 applies. The commit trailer for agent-authored commits is `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>` plus the session's `Claude-Session:` line. In addition:

- **PR rule (spec §13):**
  - Only `working` and `amrap` sets that aren't deleted, with `weight_kg` recorded and `reps ≥ 1`, can be a PR or count toward one.
  - A PR has a weight strictly greater than the heaviest earlier such set with the same slug and the same reps.
- **Rep-max table:** rep counts 1–12. Each shows the heaviest weight, the date it was first reached, and a link to its session.
- **Hard set (spec §13):**
  - a set that isn't deleted, of kind `working`, `drop` or `amrap`, with `rpe ≥ 7` or no RPE recorded
  - its primary muscles count 1.0 each and its secondary muscles 0.5 each
  - muscles come from the user's view of the exercise (their customised copy if they have one)
- **Muscles page:** the last 12 ISO weeks including the current one, oldest first. Week labels look like `2026-W39`.
- **Units:** services and the store work in kg. The web layer converts API values and page text to the user's unit. API numbers are rounded to 0.1.
- **Timestamps:** stored strings compare correctly in SQL (one fixed format). JavaScript compares timestamps with `Date.parse`, never as strings, because Go's JSON omits zero milliseconds.
- **Chart API:** `GET /api/stats/exercises/{slug}/e1rm` and `GET /api/stats/muscles`. These are same-origin JSON with cookie auth: 401 without a session, 404 for an exercise the user can't see.
- **Charts are an enhancement:** every number shown in a chart is also in a server-rendered table, except the e1RM series, whose best is in the rep-max table. Pages work without JS.
- **Vendored files** get their license next to them: `internal/web/static/vendor/uplot/LICENSE`.

## Review Focus

Inputs the spec doesn't mention that are most likely to hurt a real user. Each one has a test in the task that owns the code:

1. **Deleted sets, warmups and other users' sets** must never show up in a PR, a rep max, an e1RM point or muscle volume. Test: `TestStatsIgnoreDeletedWarmupAndOtherUsers` (Task 1).
2. **The year boundary.** A set on 2027-01-02 belongs to ISO week 2026-W53, and 12 weeks ending there span two years without gaps or duplicates. Test: `TestWeeklyMuscleSetsISOWeeks` (Task 2).
3. **A login that expired** while the exercise page was open: the chart fetch must get 401 JSON, not a redirect to the login page's HTML. Test: `TestStatsAPIWithoutSessionIs401` (Task 3).
4. **An exercise with no sets, or a time-based exercise:** the page shows a plain message, and the chart code draws nothing instead of throwing on empty data. Tests: `TestExercisePageWithoutHistory` (Task 4) and "empty series draws no chart" (Task 5).
5. **Mixed timestamp formats in the companion.** A synced set's `done_at` from Go (`…:00Z`) against a local one (`…:00.500Z`) must order as times when deciding which set came first. Test: "PR compares done_at as times" (Task 6).

The whole path (log sets, see the chart and the rep-max table, see a PR badge in the workout screen) is covered by new checks in `task e2e` (Task 7).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/store/stats.go` (new) | `RepMaxes`, `E1RMSeries`, `SessionPRs`, `HardSets` queries |
| `internal/store/stats_test.go` (new) | query tests |
| `internal/stats/stats.go` (new) | `Service`: `ExerciseStats`, `SessionPRs`, `WeeklyMuscleSets`, `RecentMuscleSets` |
| `internal/stats/stats_test.go` (new) | service tests on a real database with the seeded catalog |
| `internal/auth/middleware.go` | `/api/` requests without a session get 401 |
| `internal/web/stats.go` (new) | `/stats/muscles` page and the two chart JSON endpoints |
| `internal/web/server.go`, `cmd/onerep/main.go`, `internal/web/server_test.go` | wire `Stats` |
| `internal/web/exercises.go`, `history.go` | load stats and PR flags for the pages |
| `internal/web/views/stats.templ` (new), `exercises.templ`, `history.templ`, `layout.templ`, `models.go`, `ui.go` | tables, chart holders, badges, nav link, head scripts |
| `internal/web/stats_test.go` (new) | page and API tests |
| `internal/web/static/vendor/uplot/{uPlot.esm.js,uPlot.min.css,LICENSE}` (new) | vendored uPlot 1.6.32 |
| `internal/web/static/js/chart-data.js` (new) | pure data shaping for charts |
| `internal/web/static/js/stats.js` (new) | draws `[data-chart]` elements with uPlot |
| `internal/web/jstest/charts.test.mjs` (new) | Node tests for `chart-data.js` |
| `internal/web/static/js/sw.js` | new head files in the offline shell |
| `internal/training/bootstrap.go`, `service.go` | `prs` per exercise in the bootstrap |
| `internal/web/static/js/companion-core.js`, `companion.js`, `jstest/companion.test.mjs` | `isPR`, PR badge and banner |
| `test/e2e/companion.mjs` | chart, rep-max table and PR badge checks |
| `AGENTS.md` | layout lines for `stats/` and the chart scripts |

---

### Task 1: Stats queries in the store

**Files:**
- Create: `internal/store/stats.go`
- Test: `internal/store/stats_test.go`

**Interfaces:**
- Consumes: `store.Set`, `formatTime`, `parseTime`, `nullFloat`, `db.read`; test helpers `newTestDB`, `newUser`, `newSession`, `set` (in `sessions_test.go`: a working 5-rep `barbell-back-squat` set at `kg`, done and updated at `updated`), `withE1RM`, `t0`.
- Produces:
  ```go
  type RepMax struct { Reps int; WeightKg float64; DoneAt time.Time; SessionID string }
  func (db *DB) RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]RepMax, error) // by reps ascending
  type E1RMPoint struct { SessionID string; DoneAt time.Time; E1RMKg, WeightKg float64; Reps int; RPE *float64 }
  func (db *DB) E1RMSeries(ctx context.Context, userID, slug string) ([]E1RMPoint, error) // one per session, oldest first
  func (db *DB) SessionPRs(ctx context.Context, userID, sessionID string) (map[string]bool, error) // PR set ids
  type HardSet struct { Slug string; DoneAt time.Time }
  func (db *DB) HardSets(ctx context.Context, userID string, from, to time.Time) ([]HardSet, error) // done_at in [from, to)
  ```

- [ ] **Step 1: Write the failing tests**

`internal/store/stats_test.go`:

```go
package store

import (
	"context"
	"testing"
	"time"
)

// logged is set() with explicit kind, reps and rpe.
func logged(sessionID, id, kind string, kg float64, reps int, rpe *float64, done time.Time) Set {
	s := set(sessionID, id, kg, done)
	s.Kind, s.Reps, s.RPE = kind, &reps, rpe
	return s
}

func rpe(v float64) *float64 { return &v }

func mustUpsert(t *testing.T, db *DB, userID string, s Set) {
	t.Helper()
	if _, err := db.UpsertSet(context.Background(), userID, s, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRepMaxesAndPRs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	first := newSession(t, db, u.ID, "first")
	mustUpsert(t, db, u.ID, logged(first.ID, "a", "working", 100, 5, nil, t0))
	mustUpsert(t, db, u.ID, logged(first.ID, "b", "amrap", 90, 8, nil, t0.Add(time.Minute)))
	second := newSession(t, db, u.ID, "second")
	mustUpsert(t, db, u.ID, logged(second.ID, "c", "working", 100, 5, nil, t0.Add(24*time.Hour)))            // tie: not a PR
	mustUpsert(t, db, u.ID, logged(second.ID, "d", "working", 105, 5, nil, t0.Add(24*time.Hour+time.Minute))) // PR
	mustUpsert(t, db, u.ID, logged(second.ID, "e", "working", 110, 5, nil, t0.Add(24*time.Hour+2*time.Minute))) // PR over d
	mustUpsert(t, db, u.ID, logged(second.ID, "f", "working", 60, 3, nil, t0.Add(24*time.Hour+3*time.Minute)))  // first 3-rep set: not a PR

	maxes, err := db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []RepMax{{3, 60, t0.Add(24*time.Hour + 3*time.Minute), second.ID}, {5, 110, t0.Add(24*time.Hour + 2*time.Minute), second.ID},
		{8, 90, t0.Add(time.Minute), first.ID}}
	if len(maxes) != len(want) {
		t.Fatalf("rep maxes = %+v", maxes)
	}
	for i := range want {
		if maxes[i].Reps != want[i].Reps || maxes[i].WeightKg != want[i].WeightKg || !maxes[i].DoneAt.Equal(want[i].DoneAt) || maxes[i].SessionID != want[i].SessionID {
			t.Errorf("rep max %d = %+v, want %+v", i, maxes[i], want[i])
		}
	}
	// Excluding a session leaves only the other sessions' bests.
	maxes, _ = db.RepMaxes(ctx, u.ID, "barbell-back-squat", second.ID)
	if len(maxes) != 2 || maxes[0].Reps != 5 || maxes[0].WeightKg != 100 {
		t.Fatalf("rep maxes excluding the second session = %+v", maxes)
	}
	// The first date a weight was reached wins a tie.
	mustUpsert(t, db, u.ID, logged(second.ID, "g", "working", 90, 8, nil, t0.Add(48*time.Hour)))
	maxes, _ = db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if maxes[2].SessionID != first.ID {
		t.Fatalf("tied 8-rep max should keep the first session: %+v", maxes[2])
	}

	prs, err := db.SessionPRs(ctx, u.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || !prs["d"] || !prs["e"] {
		t.Fatalf("PRs = %v, want d and e", prs)
	}
	if prs, _ := db.SessionPRs(ctx, u.ID, first.ID); len(prs) != 0 {
		t.Fatalf("first session has no earlier bests, PRs = %v", prs)
	}
}

func TestE1RMSeries(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	one := newSession(t, db, u.ID, "one")
	mustUpsert(t, db, u.ID, withE1RM(logged(one.ID, "a", "working", 100, 5, rpe(8), t0), 120))
	mustUpsert(t, db, u.ID, withE1RM(logged(one.ID, "b", "working", 105, 3, nil, t0.Add(time.Minute)), 115.5))
	two := newSession(t, db, u.ID, "two")
	mustUpsert(t, db, u.ID, withE1RM(logged(two.ID, "c", "working", 110, 3, nil, t0.Add(48*time.Hour)), 121))

	series, err := db.E1RMSeries(ctx, u.ID, "barbell-back-squat")
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %+v", series)
	}
	if p := series[0]; p.SessionID != one.ID || p.E1RMKg != 120 || p.WeightKg != 100 || p.Reps != 5 || p.RPE == nil || *p.RPE != 8 || !p.DoneAt.Equal(t0) {
		t.Errorf("first point = %+v", p)
	}
	if p := series[1]; p.SessionID != two.ID || p.E1RMKg != 121 || p.RPE != nil {
		t.Errorf("second point = %+v", p)
	}
}

func TestHardSets(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	s := newSession(t, db, u.ID, "s")
	mustUpsert(t, db, u.ID, logged(s.ID, "no-rpe", "working", 100, 5, nil, t0))
	mustUpsert(t, db, u.ID, logged(s.ID, "rpe7", "drop", 80, 8, rpe(7), t0.Add(time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "amrap", "amrap", 90, 9, rpe(9.5), t0.Add(2*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "easy", "working", 60, 5, rpe(6.5), t0.Add(3*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "warm", "warmup", 60, 5, nil, t0.Add(4*time.Minute)))
	mustUpsert(t, db, u.ID, logged(s.ID, "late", "working", 100, 5, nil, t0.Add(time.Hour))) // == to: excluded

	hard, err := db.HardSets(ctx, u.ID, t0, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(hard) != 3 || hard[0].Slug != "barbell-back-squat" || !hard[0].DoneAt.Equal(t0) || !hard[2].DoneAt.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("hard sets = %+v", hard)
	}
}

func TestStatsIgnoreDeletedWarmupAndOtherUsers(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	u := newUser(t, db, "u")
	other := newUser(t, db, "other")
	mine := newSession(t, db, u.ID, "mine")
	mustUpsert(t, db, u.ID, withE1RM(logged(mine.ID, "base", "working", 100, 5, nil, t0), 116))
	theirs := newSession(t, db, other.ID, "theirs")
	mustUpsert(t, db, other.ID, withE1RM(logged(theirs.ID, "x", "working", 200, 5, nil, t0), 233))
	later := newSession(t, db, u.ID, "later")
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "warm", "warmup", 150, 5, nil, t0.Add(time.Hour)), 175))
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "gone", "working", 150, 5, nil, t0.Add(2*time.Hour)), 175))
	if _, err := db.DeleteSet(ctx, u.ID, later.ID, "gone", t0.Add(3*time.Hour), ""); err != nil {
		t.Fatal(err)
	}
	mustUpsert(t, db, u.ID, withE1RM(logged(later.ID, "real", "working", 102.5, 5, nil, t0.Add(4*time.Hour)), 119))

	maxes, _ := db.RepMaxes(ctx, u.ID, "barbell-back-squat", "")
	if len(maxes) != 1 || maxes[0].WeightKg != 102.5 {
		t.Errorf("rep maxes = %+v, want only 102.5", maxes)
	}
	prs, _ := db.SessionPRs(ctx, u.ID, later.ID)
	if len(prs) != 1 || !prs["real"] {
		t.Errorf("PRs = %v, want only real (the warmup, deleted and other user's sets don't count)", prs)
	}
	series, _ := db.E1RMSeries(ctx, u.ID, "barbell-back-squat")
	if len(series) != 2 || series[1].E1RMKg != 119 {
		t.Errorf("series = %+v", series)
	}
	hard, _ := db.HardSets(ctx, u.ID, t0.Add(-time.Hour), t0.Add(24*time.Hour))
	if len(hard) != 2 {
		t.Errorf("hard sets = %+v, want base and real", hard)
	}
	if prs, _ := db.SessionPRs(ctx, other.ID, later.ID); len(prs) != 0 {
		t.Errorf("another user's session leaked PRs: %v", prs)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `nix develop --command go test ./internal/store/ -run 'RepMaxes|E1RMSeries|HardSets|StatsIgnore'`
Expected: FAIL to compile: `db.RepMaxes undefined` (and the other three).

- [ ] **Step 3: Write the queries**

`internal/store/stats.go`:

```go
package store

import (
	"context"
	"database/sql"
	"time"
)

// Stats are plain queries over sets (spec §5: no aggregate tables in v1).
// Stored timestamps share one fixed format, so they compare correctly as text.

// prKinds are the set kinds that can be, or count toward, a PR (spec §13).
const prKinds = `kind IN ('working', 'amrap') AND weight_kg IS NOT NULL AND reps >= 1 AND done_at IS NOT NULL AND deleted_at IS NULL`

// RepMax is the heaviest working or AMRAP set of an exercise at a rep count.
type RepMax struct {
	Reps      int
	WeightKg  float64
	DoneAt    time.Time // when that weight was first reached
	SessionID string
}

// RepMaxes returns the best weight per rep count for slug, lowest reps first,
// ignoring sets of excludeSessionID ("" excludes nothing).
func (db *DB) RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]RepMax, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT reps, weight_kg, done_at, session_id FROM (
		SELECT reps, weight_kg, done_at, session_id,
			row_number() OVER (PARTITION BY reps ORDER BY weight_kg DESC, done_at, id) AS rank
		FROM sets WHERE user_id = ? AND slug = ? AND session_id != ? AND `+prKinds+`
	) WHERE rank = 1 ORDER BY reps`, userID, slug, excludeSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RepMax
	for rows.Next() {
		var m RepMax
		var done string
		if err := rows.Scan(&m.Reps, &m.WeightKg, &done, &m.SessionID); err != nil {
			return nil, err
		}
		if m.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// E1RMPoint is the best e1RM of one session.
type E1RMPoint struct {
	SessionID string
	DoneAt    time.Time
	E1RMKg    float64
	WeightKg  float64
	Reps      int
	RPE       *float64
}

// E1RMSeries returns the session-best e1RM of slug for each session, oldest
// first. Warmups don't count.
func (db *DB) E1RMSeries(ctx context.Context, userID, slug string) ([]E1RMPoint, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT session_id, done_at, e1rm_kg, weight_kg, reps, rpe FROM (
		SELECT session_id, done_at, e1rm_kg, weight_kg, reps, rpe,
			row_number() OVER (PARTITION BY session_id ORDER BY e1rm_kg DESC, done_at, id) AS rank
		FROM sets WHERE user_id = ? AND slug = ? AND kind != 'warmup' AND e1rm_kg IS NOT NULL
			AND weight_kg IS NOT NULL AND reps IS NOT NULL AND done_at IS NOT NULL AND deleted_at IS NULL
	) WHERE rank = 1 ORDER BY done_at, session_id`, userID, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []E1RMPoint
	for rows.Next() {
		var p E1RMPoint
		var done string
		var rpe sql.NullFloat64
		if err := rows.Scan(&p.SessionID, &done, &p.E1RMKg, &p.WeightKg, &p.Reps, &rpe); err != nil {
			return nil, err
		}
		if p.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		p.RPE = nullFloat(rpe)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SessionPRs returns the ids of the session's sets that beat the heaviest
// earlier set of the same exercise at the same reps (spec §13). A set with no
// earlier set to beat is not a PR.
func (db *DB) SessionPRs(ctx context.Context, userID, sessionID string) (map[string]bool, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT s.id FROM sets s
		WHERE s.user_id = ? AND s.session_id = ? AND s.`+prKinds+` AND s.weight_kg > (
			SELECT max(p.weight_kg) FROM sets p
			WHERE p.user_id = s.user_id AND p.slug = s.slug AND p.reps = s.reps
				AND p.kind IN ('working', 'amrap') AND p.weight_kg IS NOT NULL AND p.done_at IS NOT NULL AND p.deleted_at IS NULL
				AND (p.done_at < s.done_at OR (p.done_at = s.done_at AND p.id < s.id)))`, userID, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// HardSet is a set that counts toward weekly muscle volume.
type HardSet struct {
	Slug   string
	DoneAt time.Time
}

// HardSets returns the hard sets done in [from, to), oldest first: working,
// drop and AMRAP sets at RPE 7 or more, or with no RPE (spec §13).
func (db *DB) HardSets(ctx context.Context, userID string, from, to time.Time) ([]HardSet, error) {
	rows, err := db.read.QueryContext(ctx, `SELECT slug, done_at FROM sets
		WHERE user_id = ? AND kind IN ('working', 'drop', 'amrap') AND (rpe IS NULL OR rpe >= 7)
			AND deleted_at IS NULL AND done_at >= ? AND done_at < ?
		ORDER BY done_at, id`, userID, formatTime(from), formatTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HardSet
	for rows.Next() {
		var h HardSet
		var done string
		if err := rows.Scan(&h.Slug, &done); err != nil {
			return nil, err
		}
		if h.DoneAt, err = parseTime(done); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `nix develop --command go test ./internal/store/`
Expected: PASS (all store tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/stats.go internal/store/stats_test.go
git commit -m "feat(store): add rep max, e1RM series, PR and hard set queries"
```

---

### Task 2: The stats service

**Files:**
- Create: `internal/stats/stats.go`
- Test: `internal/stats/stats_test.go`

**Interfaces:**
- Consumes (Task 1): `RepMaxes`, `E1RMSeries`, `SessionPRs`, `HardSets`, `store.RepMax`, `store.E1RMPoint`, `store.HardSet`. From `calc`: `E1RM(weightKg float64, reps int, rpe float64) (float64, calc.Method, bool)` and `calc.MethodRPE`. From `exercise`: `Muscles []string`, `(*exercise.Service).Get(ctx, userID, slug) (store.Exercise, error)`.
- Produces:
  ```go
  type Service struct { Store Store; Exercises Exercises; Now func() time.Time }
  type Point struct { SessionID string; DoneAt time.Time; E1RMKg float64; RPEBased bool }
  type ExerciseStats struct { Exercise store.Exercise; Series []Point; RepMaxes []store.RepMax /* reps 1–12 */ }
  func (s *Service) ExerciseStats(ctx context.Context, user store.User, slug string) (ExerciseStats, error) // store.ErrNotFound for unknown slugs
  func (s *Service) SessionPRs(ctx context.Context, user store.User, sessionID string) (map[string]bool, error)
  type Week struct { Label string; Start time.Time }
  type MuscleVolume struct { Muscle string; Sets []float64; Total float64 }
  type MuscleWeeks struct { Weeks []Week; Muscles []MuscleVolume }
  func (s *Service) WeeklyMuscleSets(ctx context.Context, user store.User, from, to time.Time) (MuscleWeeks, error)
  func (s *Service) RecentMuscleSets(ctx context.Context, user store.User, weeks int) (MuscleWeeks, error)
  const MaxRepMax = 12
  ```

- [ ] **Step 1: Write the failing tests**

`internal/stats/stats_test.go`:

```go
package stats

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/store/storetest"
)

type env struct {
	svc  *Service
	db   *store.DB
	user store.User
	sess store.Session
}

func newEnv(t *testing.T, now time.Time) env {
	t.Helper()
	db := storetest.New(t)
	ctx := context.Background()
	if err := exercise.Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	u, err := db.UpsertOIDCUser(ctx, "iss", "u", "", "u")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := db.CreateSession(ctx, store.Session{UserID: u.ID, Name: "s", Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{Store: db, Exercises: &exercise.Service{Store: db}, Now: func() time.Time { return now }}
	return env{svc: svc, db: db, user: u, sess: sess}
}

var n int

// log stores a set of slug for the env's session.
func (e env) log(t *testing.T, slug, kind string, kg float64, reps int, rpe *float64, done time.Time, e1rm *float64) {
	t.Helper()
	n++
	id := "00000000-0000-7000-8000-" + time.Unix(int64(n), 0).UTC().Format("150405") + "000000"
	s := store.Set{ID: id, SessionID: e.sess.ID, Slug: slug, Kind: kind, WeightKg: &kg, Reps: &reps, RPE: rpe,
		E1RMKg: e1rm, DoneAt: &done, UpdatedAt: done}
	if _, err := e.db.UpsertSet(context.Background(), e.user.ID, s, ""); err != nil {
		t.Fatal(err)
	}
}

func f(v float64) *float64 { return &v }

var monday = time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC) // 2026-W39

func TestExerciseStats(t *testing.T) {
	e := newEnv(t, monday)
	e.log(t, "barbell-back-squat", "working", 100, 5, f(8), monday, f(123.5))    // RTS: RPE-based
	e.log(t, "barbell-back-squat", "working", 100, 13, nil, monday, nil)          // 13 reps: not in the table
	e.log(t, "barbell-back-squat", "working", 90, 1, f(5), monday, f(90))          // RPE 5 is off the RTS table
	st, err := e.svc.ExerciseStats(context.Background(), e.user, "barbell-back-squat")
	if err != nil {
		t.Fatal(err)
	}
	if st.Exercise.Slug != "barbell-back-squat" || len(st.Series) != 1 || !st.Series[0].RPEBased || st.Series[0].E1RMKg != 123.5 {
		t.Fatalf("stats = %+v", st)
	}
	if len(st.RepMaxes) != 2 || st.RepMaxes[0].Reps != 1 || st.RepMaxes[1].Reps != 5 {
		t.Fatalf("rep maxes = %+v, want reps 1 and 5 only", st.RepMaxes)
	}
	if _, err := e.svc.ExerciseStats(context.Background(), e.user, "no-such-exercise"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown slug: %v", err)
	}
}

func TestRPEBasedFollowsCalc(t *testing.T) {
	// Only the method calc.E1RM used decides the style; an RPE off the table is rep-based.
	if isRPEBased(store.E1RMPoint{WeightKg: 90, Reps: 1, RPE: f(5)}) {
		t.Error("RPE 5 single counted as RPE-based")
	}
	if !isRPEBased(store.E1RMPoint{WeightKg: 100, Reps: 5, RPE: f(8)}) || isRPEBased(store.E1RMPoint{WeightKg: 100, Reps: 5}) {
		t.Error("RPE 8 must be RPE-based and no RPE rep-based")
	}
}

func TestWeeklyMuscleSets(t *testing.T) {
	e := newEnv(t, monday)
	// Bench: chest and triceps primary, front delts secondary.
	e.log(t, "barbell-bench-press", "working", 80, 5, nil, monday.Add(time.Hour), nil)
	e.log(t, "barbell-bench-press", "working", 80, 5, f(8), monday.Add(-time.Minute), nil) // Sunday: week 38
	e.log(t, "barbell-bench-press", "working", 60, 5, f(6), monday.Add(2*time.Hour), nil)  // too easy
	e.log(t, "no-longer-exists", "working", 60, 5, nil, monday.Add(3*time.Hour), nil)      // unknown: no muscles
	got, err := e.svc.WeeklyMuscleSets(context.Background(), e.user, monday.AddDate(0, 0, -7), monday.Add(6*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Weeks) != 2 || got.Weeks[0].Label != "2026-W38" || got.Weeks[1].Label != "2026-W39" || !got.Weeks[1].Start.Equal(monday) {
		t.Fatalf("weeks = %+v", got.Weeks)
	}
	want := map[string][]float64{"chest": {1, 1}, "triceps": {1, 1}, "front-delts": {0.5, 0.5}}
	if len(got.Muscles) != len(want) {
		t.Fatalf("muscles = %+v", got.Muscles)
	}
	for _, m := range got.Muscles {
		w := want[m.Muscle]
		if len(m.Sets) != 2 || m.Sets[0] != w[0] || m.Sets[1] != w[1] || m.Total != w[0]+w[1] {
			t.Errorf("%s = %+v, want %v", m.Muscle, m, w)
		}
	}
	// Most volume first; ties in vocabulary order (chest before triceps).
	if got.Muscles[0].Muscle != "chest" || got.Muscles[1].Muscle != "triceps" || got.Muscles[2].Muscle != "front-delts" {
		t.Errorf("order = %s, %s, %s", got.Muscles[0].Muscle, got.Muscles[1].Muscle, got.Muscles[2].Muscle)
	}
}

func TestWeeklyMuscleSetsUseTheUsersMuscles(t *testing.T) {
	e := newEnv(t, monday)
	ctx := context.Background()
	ex := &exercise.Service{Store: e.db}
	if _, err := ex.Update(ctx, e.user.ID, exercise.Input{Slug: "dumbbell-curl", Name: "Dumbbell Curl", Measurement: "weight_reps",
		EquipmentKind: "dumbbell", PrimaryMuscles: []string{"forearms"}}); err != nil {
		t.Fatal(err)
	}
	e.log(t, "dumbbell-curl", "working", 12, 10, nil, monday, nil)
	got, err := e.svc.WeeklyMuscleSets(ctx, e.user, monday, monday)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Muscles) != 1 || got.Muscles[0].Muscle != "forearms" || got.Muscles[0].Sets[0] != 1 {
		t.Fatalf("muscles = %+v, want the customised forearms only", got.Muscles)
	}
}

func TestWeeklyMuscleSetsISOWeeks(t *testing.T) {
	sat := time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC) // ISO 2026-W53
	e := newEnv(t, sat)
	e.log(t, "barbell-back-squat", "working", 100, 5, nil, sat, nil)
	got, err := e.svc.RecentMuscleSets(context.Background(), e.user, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Weeks) != 12 || got.Weeks[11].Label != "2026-W53" || got.Weeks[0].Label != "2026-W42" {
		t.Fatalf("weeks = %v … %v (%d)", got.Weeks[0], got.Weeks[len(got.Weeks)-1], len(got.Weeks))
	}
	for i := 1; i < len(got.Weeks); i++ {
		if !got.Weeks[i].Start.Equal(got.Weeks[i-1].Start.AddDate(0, 0, 7)) {
			t.Fatalf("week %d starts %v after %v", i, got.Weeks[i].Start, got.Weeks[i-1].Start)
		}
	}
	// Glutes and quads tie at 1 set; vocabulary order puts glutes first.
	if len(got.Muscles) != 4 || got.Muscles[1].Muscle != "quads" || got.Muscles[1].Sets[11] != 1 || got.Muscles[1].Sets[0] != 0 {
		t.Fatalf("muscles = %+v", got.Muscles)
	}
	// Next Monday is 2027-W01.
	if w := weekOf(time.Date(2027, 1, 4, 0, 0, 0, 0, time.UTC)); w.Label != "2027-W01" {
		t.Fatalf("2027-01-04 is %s", w.Label)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `nix develop --command go test ./internal/stats/`
Expected: FAIL to compile: `undefined: Service` (the package has no code yet).

- [ ] **Step 3: Write the service**

`internal/stats/stats.go`:

```go
// Package stats turns logged sets into progress views (spec §13): e1RM over
// time, rep-max PRs and weekly hard sets per muscle. Everything is in kg; the
// web layer converts to the user's unit.
package stats

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/exercise"
	"github.com/LongerHV/onerep/internal/store"
)

// MaxRepMax is the highest rep count in the rep-max table.
const MaxRepMax = 12

// Store is the persistence the service needs. *store.DB implements it.
type Store interface {
	RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]store.RepMax, error)
	E1RMSeries(ctx context.Context, userID, slug string) ([]store.E1RMPoint, error)
	SessionPRs(ctx context.Context, userID, sessionID string) (map[string]bool, error)
	HardSets(ctx context.Context, userID string, from, to time.Time) ([]store.HardSet, error)
}

// Exercises resolves the user's view of an exercise. *exercise.Service implements it.
type Exercises interface {
	Get(ctx context.Context, userID, slug string) (store.Exercise, error)
}

type Service struct {
	Store     Store
	Exercises Exercises
	Now       func() time.Time // defaults to time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Point is one session's best e1RM.
type Point struct {
	SessionID string
	DoneAt    time.Time
	E1RMKg    float64
	RPEBased  bool // from the RTS table; otherwise estimated from reps
}

// ExerciseStats is what the exercise page (and the AI) shows about progress.
type ExerciseStats struct {
	Exercise store.Exercise
	Series   []Point
	RepMaxes []store.RepMax // rep counts 1–MaxRepMax
}

// ExerciseStats returns the e1RM series and rep maxes of one of the user's exercises.
func (s *Service) ExerciseStats(ctx context.Context, user store.User, slug string) (ExerciseStats, error) {
	ex, err := s.Exercises.Get(ctx, user.ID, slug)
	if err != nil {
		return ExerciseStats{}, err
	}
	st := ExerciseStats{Exercise: ex}
	points, err := s.Store.E1RMSeries(ctx, user.ID, slug)
	if err != nil {
		return st, err
	}
	for _, p := range points {
		st.Series = append(st.Series, Point{SessionID: p.SessionID, DoneAt: p.DoneAt, E1RMKg: p.E1RMKg, RPEBased: isRPEBased(p)})
	}
	maxes, err := s.Store.RepMaxes(ctx, user.ID, slug, "")
	if err != nil {
		return st, err
	}
	for _, m := range maxes {
		if m.Reps <= MaxRepMax {
			st.RepMaxes = append(st.RepMaxes, m)
		}
	}
	return st, nil
}

// isRPEBased reports whether calc derived the point's e1RM from the RTS table.
func isRPEBased(p store.E1RMPoint) bool {
	if p.RPE == nil {
		return false
	}
	_, method, ok := calc.E1RM(p.WeightKg, p.Reps, *p.RPE)
	return ok && method == calc.MethodRPE
}

// SessionPRs returns the ids of a session's PR sets (spec §13).
func (s *Service) SessionPRs(ctx context.Context, user store.User, sessionID string) (map[string]bool, error) {
	return s.Store.SessionPRs(ctx, user.ID, sessionID)
}

// Week is an ISO week, starting Monday 00:00 UTC.
type Week struct {
	Label string // "2026-W39"
	Start time.Time
}

func weekOf(t time.Time) Week {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	start := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	year, week := start.ISOWeek()
	return Week{Label: fmt.Sprintf("%d-W%02d", year, week), Start: start}
}

// MuscleVolume is one muscle's hard sets per week, aligned with MuscleWeeks.Weeks.
type MuscleVolume struct {
	Muscle string
	Sets   []float64
	Total  float64
}

// MuscleWeeks is weekly hard sets per muscle. Muscles without sets are left
// out; the rest are sorted by total, most first.
type MuscleWeeks struct {
	Weeks   []Week
	Muscles []MuscleVolume
}

// WeeklyMuscleSets counts hard sets per muscle for the ISO weeks from the
// week of from to the week of to, inclusive. Primary muscles count 1, each
// secondary muscle 0.5 (spec §13).
func (s *Service) WeeklyMuscleSets(ctx context.Context, user store.User, from, to time.Time) (MuscleWeeks, error) {
	var out MuscleWeeks
	last := weekOf(to).Start
	for w := weekOf(from).Start; !w.After(last); w = w.AddDate(0, 0, 7) {
		out.Weeks = append(out.Weeks, weekOf(w))
	}
	if len(out.Weeks) == 0 {
		return out, nil
	}
	sets, err := s.Store.HardSets(ctx, user.ID, out.Weeks[0].Start, last.AddDate(0, 0, 7))
	if err != nil {
		return out, err
	}
	exercises := map[string]*store.Exercise{} // nil: unknown to this user
	volume := map[string][]float64{}
	add := func(muscle string, week int, v float64) {
		if volume[muscle] == nil {
			volume[muscle] = make([]float64, len(out.Weeks))
		}
		volume[muscle][week] += v
	}
	for _, set := range sets {
		ex, seen := exercises[set.Slug]
		if !seen {
			got, err := s.Exercises.Get(ctx, user.ID, set.Slug)
			switch {
			case errors.Is(err, store.ErrNotFound):
			case err != nil:
				return out, err
			default:
				ex = &got
			}
			exercises[set.Slug] = ex
		}
		if ex == nil {
			continue
		}
		week := int(weekOf(set.DoneAt).Start.Sub(out.Weeks[0].Start).Hours() / (24 * 7))
		for _, m := range ex.PrimaryMuscles {
			add(m, week, 1)
		}
		for _, m := range ex.SecondaryMuscles {
			add(m, week, 0.5)
		}
	}
	for muscle, weeks := range volume {
		mv := MuscleVolume{Muscle: muscle, Sets: weeks}
		for _, v := range weeks {
			mv.Total += v
		}
		out.Muscles = append(out.Muscles, mv)
	}
	order := func(m string) int {
		if i := slices.Index(exercise.Muscles, m); i >= 0 {
			return i
		}
		return len(exercise.Muscles)
	}
	slices.SortFunc(out.Muscles, func(a, b MuscleVolume) int {
		if a.Total != b.Total {
			if a.Total > b.Total {
				return -1
			}
			return 1
		}
		return order(a.Muscle) - order(b.Muscle)
	})
	return out, nil
}

// RecentMuscleSets is WeeklyMuscleSets for the last weeks ISO weeks, this one included.
func (s *Service) RecentMuscleSets(ctx context.Context, user store.User, weeks int) (MuscleWeeks, error) {
	now := s.now()
	return s.WeeklyMuscleSets(ctx, user, now.AddDate(0, 0, -7*(weeks-1)), now)
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `nix develop --command go test ./internal/stats/`
Expected: PASS. If `TestWeeklyMuscleSetsUseTheUsersMuscles` fails because `exercise.Service.Update` refuses the input, check `exercise.Input`'s required fields in `internal/exercise/service.go` and fill them in the test. The behaviour under test is only that the user's customised muscles are used.

- [ ] **Step 5: Commit**

```bash
git add internal/stats/
git commit -m "feat(stats): add e1RM series, rep maxes, PRs and weekly muscle sets"
```

---

### Task 3: Chart JSON API and 401 for API requests

**Files:**
- Modify: `internal/auth/middleware.go` (RequireUser), `internal/auth/middleware_test.go`
- Create: `internal/web/stats.go`, `internal/web/stats_test.go`
- Modify: `internal/web/server.go` (the `Stats` field and routes), `internal/web/server_test.go` (`newAppDB`), `cmd/onerep/main.go`

**Interfaces:**
- Consumes (Task 2): `stats.Service`, `ExerciseStats`, `RecentMuscleSets`. Also `calc.FromKg(kg float64, unit string) float64`, `writeJSON`, `user(r)`, `views.Label`.
- Produces:
  - `Server.Stats *stats.Service`; `func (s *Server) statsRoutes(r chi.Router)`, called from `Routes()`.
  - `GET /api/stats/exercises/{slug}/e1rm` → `{"unit":"kg","points":[{"t":<unix s>,"e1rm":<user unit, 0.1>,"rpe_based":bool}]}`
  - `GET /api/stats/muscles` → `{"weeks":["2026-W28",…12],"muscles":[{"muscle":"quads","label":"quads","sets":[…12]}]}`

- [ ] **Step 1: Write the failing tests**

Append to `internal/auth/middleware_test.go`, following the style of its existing `RequireUser` tests. Use the file's existing imports, and add `net/http/httptest` if it's missing:

```go
// API requests can't follow a redirect to the login page: they get 401.
func TestRequireUserAPIGetsUnauthorized(t *testing.T) {
	h := RequireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/stats/muscles", nil))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("Location") != "" {
		t.Fatalf("GET /api/… without a session = %d (Location %q), want 401", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/history", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /history without a session = %d, want a redirect", rec.Code)
	}
}
```

`internal/web/stats_test.go`:

```go
package web

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/LongerHV/onerep/internal/auth"
	"github.com/LongerHV/onerep/internal/store"
)

// logSet stores a finished working set for the dev user directly.
func logSet(t *testing.T, db *store.DB, userID, sessionID, id, slug string, kg float64, reps int, rpe *float64, e1rm *float64, done time.Time) {
	t.Helper()
	s := store.Set{ID: id, SessionID: sessionID, Slug: slug, Kind: "working", WeightKg: &kg, Reps: &reps, RPE: rpe,
		E1RMKg: e1rm, DoneAt: &done, UpdatedAt: done}
	if _, err := db.UpsertSet(context.Background(), userID, s, ""); err != nil {
		t.Fatal(err)
	}
}

// devUser signs the dev user in (by visiting /) and returns them.
func devUser(t *testing.T, srvURL string, c *http.Client, db *store.DB) store.User {
	t.Helper()
	read(t, mustGet(t, c, srvURL+"/"))
	u, err := db.UpsertOIDCUser(context.Background(), auth.DevIssuer, "alice", "alice@localhost", "alice") // returns the existing user
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func newSessionFor(t *testing.T, db *store.DB, userID string) store.Session {
	t.Helper()
	s, err := db.CreateSession(context.Background(), store.Session{UserID: userID, Name: "Workout", Snapshot: []byte(`{"groups":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ptr(v float64) *float64 { return &v }

func TestStatsAPI(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	now := time.Now().UTC()
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000001", "barbell-bench-press", 100, 5, ptr(8), ptr(123.46), now)

	var e1rm struct {
		Unit   string `json:"unit"`
		Points []struct {
			T        int64   `json:"t"`
			E1RM     float64 `json:"e1rm"`
			RPEBased bool    `json:"rpe_based"`
		} `json:"points"`
	}
	resp := mustGet(t, c, srv.URL+"/api/stats/exercises/barbell-bench-press/e1rm")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("e1rm API = %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if err := json.Unmarshal([]byte(read(t, resp)), &e1rm); err != nil {
		t.Fatal(err)
	}
	if e1rm.Unit != "kg" || len(e1rm.Points) != 1 || e1rm.Points[0].E1RM != 123.5 || !e1rm.Points[0].RPEBased || e1rm.Points[0].T != now.Unix() {
		t.Fatalf("e1rm = %+v", e1rm)
	}

	var muscles struct {
		Weeks   []string `json:"weeks"`
		Muscles []struct {
			Muscle string    `json:"muscle"`
			Label  string    `json:"label"`
			Sets   []float64 `json:"sets"`
		} `json:"muscles"`
	}
	if err := json.Unmarshal([]byte(read(t, mustGet(t, c, srv.URL+"/api/stats/muscles"))), &muscles); err != nil {
		t.Fatal(err)
	}
	if len(muscles.Weeks) != 12 || len(muscles.Muscles) != 3 || muscles.Muscles[2].Label != "front delts" || muscles.Muscles[2].Sets[11] != 0.5 {
		t.Fatalf("muscles = %+v", muscles)
	}

	if resp := mustGet(t, c, srv.URL+"/api/stats/exercises/no-such-thing/e1rm"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown exercise = %d, want 404", resp.StatusCode)
	}
}

func TestStatsAPIInPounds(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	if err := db.UpdateUserSettings(context.Background(), u.ID, "lb", u.E1RMWindowDays); err != nil {
		t.Fatal(err)
	}
	sess := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000002", "barbell-bench-press", 100, 5, nil, ptr(116.67), time.Now())
	body := read(t, mustGet(t, c, srv.URL+"/api/stats/exercises/barbell-bench-press/e1rm"))
	var got struct {
		Unit   string `json:"unit"`
		Points []struct {
			E1RM float64 `json:"e1rm"`
		} `json:"points"`
	}
	_ = json.Unmarshal([]byte(body), &got)
	if got.Unit != "lb" || len(got.Points) != 1 || got.Points[0].E1RM != 257.2 {
		t.Fatalf("lb e1rm = %s", body)
	}
}

// A chart fetch after the login expired must see 401, not the login page.
func TestStatsAPIWithoutSessionIs401(t *testing.T) {
	srv, c := newApp(t, "")
	for _, path := range []string{"/api/stats/muscles", "/api/stats/exercises/barbell-bench-press/e1rm"} {
		resp := mustGet(t, c, srv.URL+path)
		read(t, resp)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without a session = %d, want 401", path, resp.StatusCode)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `nix develop --command go test ./internal/auth/ ./internal/web/ -run 'APIGetsUnauthorized|StatsAPI'`
Expected: auth test FAIL, `GET /api/… without a session = 303`. The web package fails to compile until `Server.Stats` exists, or gets 404 for the routes.

- [ ] **Step 3: Implement**

In `internal/auth/middleware.go` `RequireUser`, add a first case to the switch:

```go
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/"):
			// Scripts can't follow a redirect to the IdP; tell them to sign in again.
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case r.Header.Get("HX-Request") == "true":
```

(Add the `strings` import if it's missing. Update the doc comment: "API requests and everything that isn't a page load get 401.")

`internal/web/stats.go`:

```go
package web

import (
	"errors"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/LongerHV/onerep/internal/calc"
	"github.com/LongerHV/onerep/internal/store"
	"github.com/LongerHV/onerep/internal/web/views"
)

// MuscleWeeks is how many ISO weeks the muscles page shows (spec §13).
const MuscleWeeks = 12

func (s *Server) statsRoutes(r chi.Router) {
	r.Get("/api/stats/exercises/{slug}/e1rm", s.apiE1RM)
	r.Get("/api/stats/muscles", s.apiMuscles)
}

// round1 rounds a chart value to 0.1.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

type e1rmPoint struct {
	T        int64   `json:"t"` // unix seconds, what uPlot plots
	E1RM     float64 `json:"e1rm"`
	RPEBased bool    `json:"rpe_based"`
}

func (s *Server) apiE1RM(w http.ResponseWriter, r *http.Request) {
	u := user(r)
	st, err := s.Stats.ExerciseStats(r.Context(), u, chi.URLParam(r, "slug"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "exercise not found"})
		return
	}
	if err != nil {
		s.apiFail(w, r, err)
		return
	}
	points := make([]e1rmPoint, 0, len(st.Series))
	for _, p := range st.Series {
		points = append(points, e1rmPoint{T: p.DoneAt.Unix(), E1RM: round1(calc.FromKg(p.E1RMKg, u.Unit)), RPEBased: p.RPEBased})
	}
	writeJSON(w, http.StatusOK, map[string]any{"unit": u.Unit, "points": points})
}

type muscleSeries struct {
	Muscle string    `json:"muscle"`
	Label  string    `json:"label"`
	Sets   []float64 `json:"sets"`
}

func (s *Server) apiMuscles(w http.ResponseWriter, r *http.Request) {
	mw, err := s.Stats.RecentMuscleSets(r.Context(), user(r), MuscleWeeks)
	if err != nil {
		s.apiFail(w, r, err)
		return
	}
	weeks := make([]string, 0, len(mw.Weeks))
	for _, wk := range mw.Weeks {
		weeks = append(weeks, wk.Label)
	}
	muscles := make([]muscleSeries, 0, len(mw.Muscles))
	for _, m := range mw.Muscles {
		muscles = append(muscles, muscleSeries{Muscle: m.Muscle, Label: views.Label(m.Muscle), Sets: m.Sets})
	}
	writeJSON(w, http.StatusOK, map[string]any{"weeks": weeks, "muscles": muscles})
}

// apiFail logs an unexpected error and answers with JSON, not an HTML page.
func (s *Server) apiFail(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "api request failed", "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "server error"})
}
```

(Add `"log/slog"` to the imports. If `apiSync` already has an equivalent helper, use it instead of adding `apiFail`.)

In `internal/web/server.go`, add the `Stats *stats.Service` field (import `internal/stats`) and call `s.statsRoutes(r)` after `s.historyRoutes(r)`.

In `cmd/onerep/main.go`, after `srv.Training = …`:

```go
	srv.Stats = &stats.Service{Store: db, Exercises: srv.Exercises}
```

In `internal/web/server_test.go` `newAppDB`, after `s.Training = …`:

```go
	s.Stats = &stats.Service{Store: db, Exercises: s.Exercises}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `nix develop --command go test ./internal/auth/ ./internal/web/ ./cmd/...`
Expected: PASS, including `TestSyncWithoutSessionIs401` and `TestAnonymousIsRedirectedToLogin` (page loads still redirect).

- [ ] **Step 5: Commit**

```bash
git add internal/auth internal/web/stats.go internal/web/stats_test.go internal/web/server.go internal/web/server_test.go cmd/onerep/main.go
git commit -m "feat(web): serve e1RM and muscle volume chart data; 401 for API calls without a session"
```

---

### Task 4: Stats on the pages

**Files:**
- Create: `internal/web/views/stats.templ`
- Modify: `internal/web/stats.go` (the `/stats/muscles` route), `internal/web/exercises.go` (`exerciseDetailData`), `internal/web/history.go` (`historyData`), `internal/web/views/models.go`, `exercises.templ`, `history.templ`, `layout.templ` (the "Stats" nav link), `ui.go` (`prBadge`, `repMaxRows`)
- Test: `internal/web/stats_test.go`

**Interfaces:**
- Consumes (Tasks 2–3): `stats.ExerciseStats`, `stats.MuscleWeeks`, `Server.Stats`, `MuscleWeeks` const.
- Produces:
  - `views.ExerciseDetail.Stats stats.ExerciseStats`
  - `views.HistoryDetail.PRs map[string]bool`
  - `views.MusclesPage(p Page, mw stats.MuscleWeeks)`
  - `views.RepMaxRow{Reps int; Max *store.RepMax}` and `views.RepMaxRows(maxes []store.RepMax) []RepMaxRow` (12 rows)
  - Chart holders `<div data-chart="e1rm" data-src="…">` and `<div data-chart="muscles" data-src="/api/stats/muscles">`, which `stats.js` (Task 5) looks for.

- [ ] **Step 1: Write the failing tests**

Append to `internal/web/stats_test.go`:

```go
func TestExercisePageShowsProgress(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	day := time.Date(2026, 9, 21, 18, 0, 0, 0, time.UTC)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000011", "barbell-bench-press", 102.5, 3, ptr(9), ptr(112), day)
	html := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-bench-press"))
	for _, want := range []string{
		`data-chart="e1rm"`, `data-src="/api/stats/exercises/barbell-bench-press/e1rm"`,
		"Rep maxes", "102.5 kg", "2026-09-21", `href="/history/` + sess.ID + `"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("exercise page lacks %q", want)
		}
	}
	if strings.Count(html, `data-rep-max`) != 12 {
		t.Errorf("rep-max table should have 12 rows, has %d", strings.Count(html, `data-rep-max`))
	}
}

func TestExercisePageWithoutHistory(t *testing.T) {
	srv, c := newApp(t, "alice")
	html := read(t, mustGet(t, c, srv.URL+"/exercises/barbell-bench-press"))
	if !strings.Contains(html, "No working sets logged yet.") || strings.Contains(html, `data-chart=`) {
		t.Error("an exercise without sets should say so and draw no chart")
	}
	// Timed exercises have neither an e1RM nor weight PRs.
	html = read(t, mustGet(t, c, srv.URL+"/exercises/plank"))
	if strings.Contains(html, "Rep maxes") || strings.Contains(html, `data-chart=`) {
		t.Error("a timed exercise shows no rep maxes or e1RM chart")
	}
}

func TestMusclesPage(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	sess := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, sess.ID, "00000000-0000-7000-8000-000000000021", "barbell-back-squat", 100, 5, nil, nil, time.Now())
	html := read(t, mustGet(t, c, srv.URL+"/stats/muscles"))
	for _, want := range []string{`data-chart="muscles"`, "quads", "adductors", "0.5", "Hard sets", `href="/stats/muscles"`} {
		if !strings.Contains(html, want) {
			t.Errorf("muscles page lacks %q", want)
		}
	}
}

func TestHistoryShowsPRBadges(t *testing.T) {
	srv, c, db := newAppDB(t, "alice")
	u := devUser(t, srv.URL, c, db)
	old := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, old.ID, "00000000-0000-7000-8000-000000000031", "barbell-bench-press", 100, 5, nil, nil, time.Now().Add(-48*time.Hour))
	cur := newSessionFor(t, db, u.ID)
	logSet(t, db, u.ID, cur.ID, "00000000-0000-7000-8000-000000000032", "barbell-bench-press", 105, 5, nil, nil, time.Now())
	html := read(t, mustGet(t, c, srv.URL+"/history/"+cur.ID))
	if strings.Count(html, `data-pr-badge`) != 1 {
		t.Errorf("history should badge the one PR set, found %d", strings.Count(html, `data-pr-badge`))
	}
	if html := read(t, mustGet(t, c, srv.URL+"/history/"+old.ID)); strings.Contains(html, `data-pr-badge`) {
		t.Error("the first set at a rep count is not a PR")
	}
}
```

(Add `"strings"` to the file's imports.)

- [ ] **Step 2: Run them to verify they fail**

Run: `nix develop --command go test ./internal/web/ -run 'ExercisePage|MusclesPage|HistoryShowsPR'`
Expected: FAIL: the pages lack `data-chart`, "Rep maxes", `data-pr`, and `/stats/muscles` is a 404.

- [ ] **Step 3: Implement**

`internal/web/views/models.go`: add `Stats stats.ExerciseStats` to `ExerciseDetail` and `PRs map[string]bool` to `HistoryDetail` (import `internal/stats`).

`internal/web/views/ui.go`:

```go
	prBadge = "rounded bg-amber-200 px-1.5 py-0.5 text-xs font-medium text-amber-900 dark:bg-amber-900 dark:text-amber-100"
```

(inside the existing `const` block), and:

```go
// RepMaxRow is one row of the rep-max table; Max is nil when nothing was logged at Reps.
type RepMaxRow struct {
	Reps int
	Max  *store.RepMax
}

// RepMaxRows lays maxes out as rows for 1 to stats.MaxRepMax reps.
func RepMaxRows(maxes []store.RepMax) []RepMaxRow {
	rows := make([]RepMaxRow, stats.MaxRepMax)
	for i := range rows {
		rows[i].Reps = i + 1
	}
	for i := range maxes {
		if r := maxes[i].Reps; r >= 1 && r <= stats.MaxRepMax {
			rows[r-1].Max = &maxes[i]
		}
	}
	return rows
}

// hasWeights reports whether an exercise records a weight (and so has PRs).
func hasWeights(measurement string) bool { return measurement == "weight_reps" || measurement == "bw_reps" }
```

`internal/web/views/exercises.templ`: in `ExerciseDetailPage`, directly after the `</dl>` of the exercise facts and before `<h2 class={ h2 }>Your settings</h2>`, insert:

```templ
	if hasWeights(d.Exercise.Measurement) {
		<h2 class={ h2 }>Progress</h2>
		if len(d.Stats.RepMaxes) == 0 {
			<p class={ hint }>No working sets logged yet.</p>
		} else {
			if d.Exercise.Measurement == "weight_reps" && len(d.Stats.Series) > 0 {
				<div
					data-chart="e1rm"
					data-src={ "/api/stats/exercises/" + d.Exercise.Slug + "/e1rm" }
					class="mt-2 h-64 w-full"
					aria-label="Estimated one-rep max over time"
				></div>
				<p class={ hint }>Best estimated 1RM of each workout. Filled points come from RPE, hollow ones from reps alone.</p>
			}
			<h3 class="mt-4 font-medium">Rep maxes</h3>
			<table class="mt-2 w-full text-sm">
				<thead class="text-left text-zinc-500">
					<tr><th class="py-1">Reps</th><th>Best</th><th>Date</th></tr>
				</thead>
				<tbody>
					for _, row := range RepMaxRows(d.Stats.RepMaxes) {
						<tr data-rep-max class="border-t border-zinc-200 dark:border-zinc-800">
							<td class="py-1">{ strconv.Itoa(row.Reps) }</td>
							if row.Max != nil {
								<td>{ Weight(row.Max.WeightKg, p.Identity.User.Unit) }</td>
								<td><a href={ templ.URL("/history/" + row.Max.SessionID) } class="underline">{ row.Max.DoneAt.Format("2006-01-02") }</a></td>
							} else {
								<td class="text-zinc-400">—</td>
								<td></td>
							}
						</tr>
					}
				</tbody>
			</table>
		}
	}
```

(Add `"strconv"` to the templ file's imports if it isn't there.)

`internal/web/exercises.go` `exerciseDetailData`, before `return d, nil`:

```go
	if d.Stats, err = s.Stats.ExerciseStats(ctx, u, slug); err != nil {
		return d, err
	}
```

`internal/web/history.go` `historyData`, after the catalog is loaded:

```go
	if d.PRs, err = s.Stats.SessionPRs(ctx, u, id); err != nil {
		return d, err
	}
```

`internal/web/views/history.templ`, next to the existing kind badge inside the set form:

```templ
						if d.PRs[set.ID] {
							<span data-pr-badge class={ prBadge }>PR</span>
						}
```

`internal/web/views/stats.templ`:

```templ
package views

import (
	"strconv"

	"github.com/LongerHV/onerep/internal/stats"
)

// MusclesPage shows weekly hard sets per muscle (spec §13).
templ MusclesPage(p Page, mw stats.MuscleWeeks) {
	<h1 class={ h1 }>Weekly sets per muscle</h1>
	<p class={ hint }>
		Hard sets: working, drop and AMRAP sets at RPE 7 or more, or with no RPE. A primary muscle counts 1, a secondary muscle 0.5. Weeks are ISO weeks (Monday to Sunday, UTC).
	</p>
	if len(mw.Muscles) == 0 {
		<p class="mt-4">No hard sets in the last { strconv.Itoa(len(mw.Weeks)) } weeks.</p>
	} else {
		<div data-chart="muscles" data-src="/api/stats/muscles" class="mt-4 h-72 w-full" aria-label="Weekly sets per muscle"></div>
		<div class="mt-4 overflow-x-auto">
			<table class="w-full text-sm">
				<thead class="text-left text-zinc-500">
					<tr>
						<th class="py-1 pr-2">Muscle</th>
						for _, w := range mw.Weeks {
							<th class="px-1 text-right font-normal" title={ w.Label }>{ w.Label[5:] }</th>
						}
					</tr>
				</thead>
				<tbody>
					for _, m := range mw.Muscles {
						<tr class="border-t border-zinc-200 dark:border-zinc-800">
							<td class="py-1 pr-2 whitespace-nowrap">{ Label(m.Muscle) }</td>
							for _, v := range m.Sets {
								<td class="px-1 text-right tabular-nums">
									if v > 0 {
										{ Number(v) }
									} else {
										<span class="text-zinc-400">·</span>
									}
								</td>
							}
						</tr>
					}
				</tbody>
			</table>
		</div>
	}
}
```

`internal/web/stats.go`: register the page in `statsRoutes` and add its handler:

```go
	r.Get("/stats/muscles", s.statsMuscles)
```

```go
func (s *Server) statsMuscles(w http.ResponseWriter, r *http.Request) {
	mw, err := s.Stats.RecentMuscleSets(r.Context(), user(r), MuscleWeeks)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	render(w, r, http.StatusOK, views.MusclesPage(page(r, "Muscles"), mw))
}
```

`internal/web/views/layout.templ`: add `<a href="/stats/muscles" class="text-sm hover:underline">Stats</a>` after the History link.

Then run `nix develop --command task generate`.

- [ ] **Step 4: Run them to verify they pass**

Run: `nix develop --command go test ./internal/web/`
Expected: PASS (every web test, including the earlier history and exercise tests).

- [ ] **Step 5: Commit**

```bash
git add internal/web
git commit -m "feat(web): show rep maxes, PR badges and weekly sets per muscle"
```

---

### Task 5: Charts with uPlot

**Files:**
- Create: `internal/web/static/vendor/uplot/uPlot.esm.js`, `uPlot.min.css`, `LICENSE` (from the npm tarball)
- Create: `internal/web/static/js/chart-data.js`, `internal/web/static/js/stats.js`, `internal/web/jstest/charts.test.mjs`
- Modify: `internal/web/views/layout.templ` (head), `internal/web/static/js/sw.js` (`SHELL_FILES`)

**Interfaces:**
- Consumes (Tasks 3–4): the two JSON shapes, `[data-chart][data-src]` holders, `htmx.onLoad`.
- Produces (pure, in `chart-data.js`):
  ```js
  export function e1rmData(points) // null when empty, else [ts[], all[], rpe[] (null gaps), reps[] (null gaps)]
  export function stackMuscles(muscles) // { data: [xs, ...cumulative series, topmost first], order: muscles in draw order }
  export function muscleColor(i, n) // "hsl(…)" string
  ```

- [ ] **Step 1: Vendor uPlot 1.6.32**

```bash
cd "$(git rev-parse --show-toplevel)"
tmp=$(mktemp -d)
curl -fsSL https://registry.npmjs.org/uplot/-/uplot-1.6.32.tgz | tar -xz -C "$tmp"
mkdir -p internal/web/static/vendor/uplot
cp "$tmp/package/dist/uPlot.esm.js" "$tmp/package/dist/uPlot.min.css" "$tmp/package/LICENSE" internal/web/static/vendor/uplot/
head -3 internal/web/static/vendor/uplot/LICENSE
rm -rf "$tmp"
```

Expected: the LICENSE starts with `The MIT License (MIT)`. The first comment of `uPlot.esm.js` names v1.6.32.

- [ ] **Step 2: Write the failing JS tests**

`internal/web/jstest/charts.test.mjs`:

```js
import { test } from "node:test";
import assert from "node:assert/strict";
import { e1rmData, muscleColor, stackMuscles } from "../static/js/chart-data.js";

test("empty series draws no chart", () => {
  assert.equal(e1rmData([]), null);
  assert.equal(e1rmData(undefined), null);
});

test("e1RM points split into RPE-based and rep-based series", () => {
  const data = e1rmData([
    { t: 100, e1rm: 120, rpe_based: true },
    { t: 200, e1rm: 118.5, rpe_based: false },
  ]);
  assert.deepEqual(data, [[100, 200], [120, 118.5], [120, null], [null, 118.5]]);
});

test("muscles stack cumulatively, topmost series first", () => {
  const { data, order } = stackMuscles([
    { muscle: "quads", label: "quads", sets: [2, 4] },
    { muscle: "glutes", label: "glutes", sets: [1, 0.5] },
  ]);
  assert.deepEqual(data[0], [0, 1]); // week indexes
  // Drawn first = whole stack (quads + glutes), then quads alone on top of it.
  assert.deepEqual(order.map((m) => m.muscle), ["glutes", "quads"]);
  assert.deepEqual(data[1], [3, 4.5]);
  assert.deepEqual(data[2], [2, 4]);
});

test("no muscles, no chart", () => {
  assert.equal(stackMuscles([]), null);
});

test("muscle colors differ", () => {
  const colors = new Set(Array.from({ length: 20 }, (_, i) => muscleColor(i, 20)));
  assert.equal(colors.size, 20);
});
```

Run: `nix develop --command task test:js`
Expected: FAIL, `Cannot find module …/chart-data.js`.

- [ ] **Step 3: Write `chart-data.js`**

```js
// Shapes the stats API's JSON for uPlot. Pure, so Node can test it
// (internal/web/jstest/charts.test.mjs); stats.js draws the result.

// e1rmData turns [{t, e1rm, rpe_based}] into uPlot columns: time, every
// point (the line), RPE-based points and rep-based points. null when empty.
export function e1rmData(points) {
  if (!points || points.length === 0) return null;
  return [
    points.map((p) => p.t),
    points.map((p) => p.e1rm),
    points.map((p) => (p.rpe_based ? p.e1rm : null)),
    points.map((p) => (p.rpe_based ? null : p.e1rm)),
  ];
}

// stackMuscles builds a stacked bar chart from [{muscle, label, sets[]}]
// (most volume first): each series is the cumulative sum up to that muscle,
// and the tallest is drawn first so shorter ones paint over it. order lists
// the muscles in draw order, matching data[1..]. null when there is nothing.
export function stackMuscles(muscles) {
  if (!muscles || muscles.length === 0) return null;
  const weeks = muscles[0].sets.length;
  const running = new Array(weeks).fill(0);
  const cumulative = muscles.map((m) => m.sets.map((v, i) => (running[i] += v)));
  return {
    data: [Array.from({ length: weeks }, (_, i) => i), ...cumulative.reverse()],
    order: [...muscles].reverse(),
  };
}

// muscleColor spreads n colors around the hue wheel.
export function muscleColor(i, n) {
  return `hsl(${Math.round((i * 360) / n)} 65% 55%)`;
}
```

Run: `nix develop --command task test:js`
Expected: PASS (the companion, calc, sw and chart tests).

- [ ] **Step 4: Write `stats.js` and load it**

`internal/web/static/js/stats.js`:

```js
// Draws [data-chart] elements with uPlot from their data-src JSON (spec §13).
// Loaded once from the layout like the other page scripts: htmx swaps pages
// into <main>, so charts are set up from htmx.onLoad, and uPlot (vendored) is
// only fetched on a page that has one. Charts whose element left the page are
// destroyed on the next swap. The tables next to each chart carry the numbers,
// so a failed chart only logs.

import { e1rmData, muscleColor, stackMuscles } from "./chart-data.js";

let lib; // the uPlot module, loaded on first use
const live = new Map(); // element → { plot, observer }

const dark = () => window.matchMedia("(prefers-color-scheme: dark)").matches;
const ink = () => (dark() ? "#a1a1aa" : "#52525b"); // zinc-400 / zinc-600
const grid = () => (dark() ? "#27272a" : "#e4e4e7"); // zinc-800 / zinc-200
const paper = () => (dark() ? "#09090b" : "#fafafa"); // zinc-950 / zinc-50

function axes(extraX = {}) {
  const common = { stroke: ink(), grid: { stroke: grid(), width: 1 }, ticks: { stroke: grid(), width: 1 } };
  return [{ ...common, ...extraX }, { ...common }];
}

function e1rmOptions(json, el) {
  const data = e1rmData(json.points);
  if (!data) return null;
  const accent = dark() ? "#e4e4e7" : "#18181b";
  const noLine = () => null;
  return {
    data,
    opts: {
      width: el.clientWidth, height: el.clientHeight || 256,
      axes: axes(),
      scales: { y: { auto: true } },
      series: [
        {},
        { label: `e1RM (${json.unit})`, stroke: accent, width: 2, points: { show: false } },
        { label: "RPE-based", stroke: accent, paths: noLine, points: { show: true, size: 8, fill: accent } },
        { label: "rep-based", stroke: accent, paths: noLine, points: { show: true, size: 8, fill: paper(), width: 2 } },
      ],
    },
  };
}

function musclesOptions(json, el) {
  const stack = stackMuscles(json.muscles);
  if (!stack) return null;
  const n = json.muscles.length;
  const raw = new Map(json.muscles.map((m) => [m.muscle, m.sets]));
  const bars = lib.paths.bars({ size: [0.7, 48] });
  return {
    data: stack.data,
    opts: {
      width: el.clientWidth, height: el.clientHeight || 288,
      axes: axes({ values: (_, ticks) => ticks.map((i) => (json.weeks[i] || "").slice(5)), space: 30 }),
      scales: { x: { time: false, range: [-0.5, json.weeks.length - 0.5] }, y: { range: (_, __, max) => [0, Math.max(1, max)] } },
      series: [
        { label: "Week", value: (_, i) => (i == null ? "" : json.weeks[i]) },
        ...stack.order.map((m) => {
          const color = muscleColor(json.muscles.indexOf(m), n);
          return { label: m.label, fill: color, stroke: color, paths: bars, points: { show: false },
            value: (_, __, ___, i) => (i == null ? "" : String(raw.get(m.muscle)[i])) };
        }),
      ],
    },
  };
}

const builders = { e1rm: e1rmOptions, muscles: musclesOptions };

async function draw(el) {
  const build = builders[el.dataset.chart];
  if (!build || el.dataset.drawn) return;
  el.dataset.drawn = "starting";
  try {
    const res = await fetch(el.dataset.src, { headers: { Accept: "application/json" } });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const json = await res.json();
    lib ??= (await import("/static/vendor/uplot/uPlot.esm.js")).default;
    const chart = build(json, el);
    if (!chart || !el.isConnected) return;
    const plot = new lib(chart.opts, chart.data, el);
    const observer = new ResizeObserver(() => plot.setSize({ width: el.clientWidth, height: chart.opts.height }));
    observer.observe(el);
    live.set(el, { plot, observer });
    el.dataset.drawn = "done";
  } catch (err) {
    console.error("chart unavailable", el.dataset.src, err);
    el.dataset.drawn = "failed";
  }
}

function sweep() {
  for (const [el, { plot, observer }] of live) {
    if (el.isConnected) continue;
    observer.disconnect();
    plot.destroy();
    live.delete(el);
  }
}

htmx.onLoad((root) => {
  sweep();
  const els = root.matches?.("[data-chart]") ? [root] : [...(root.querySelectorAll?.("[data-chart]") || [])];
  els.forEach(draw);
});
```

`internal/web/views/layout.templ` `<head>`: after the jsoneditor stylesheet, add `<link rel="stylesheet" href="/static/vendor/uplot/uPlot.min.css"/>`. After the `companion.js` script tag, add `<script type="module" src="/static/js/stats.js"></script>`.

`internal/web/static/js/sw.js` `SHELL_FILES`: add `"/static/vendor/uplot/uPlot.min.css"`, `"/static/js/chart-data.js"` and `"/static/js/stats.js"`. The head loads these on every page, including offline ones. `uPlot.esm.js` is loaded only with a chart, and charts need the network anyway.

Run: `nix develop --command task generate`

- [ ] **Step 5: Check it in a browser**

Run `nix develop --command task dev` in the background. Log a few sets through the history editor (add sets to a finished workout with different weights). Then open `/exercises/barbell-bench-press` and `/stats/muscles` in headless Chromium, using the e2e harness or `chromium --headless --screenshot`. Look at the screenshots: a line with filled or hollow points, and coloured stacked bars with week labels such as `W39`. Test the fetch path by navigating between pages with htmx (click Exercises, then back) and confirming no console errors. Task 7 automates this check; this step catches drawing mistakes the e2e can't see.

Expected: both charts draw and resize with the window. No errors in the console.

- [ ] **Step 6: Run everything and commit**

Run: `nix develop --command task ci`
Expected: exit code 0.

```bash
git add internal/web/static internal/web/jstest/charts.test.mjs internal/web/views
git commit -m "feat(web): draw e1RM and weekly muscle charts with vendored uPlot 1.6.32"
```

---

### Task 6: Advisory PR badges in the companion

**Files:**
- Modify: `internal/training/service.go` (the `Store` interface), `internal/training/bootstrap.go` (`BootExercise.PRs`), `internal/training/service_test.go`
- Modify: `internal/web/static/js/companion-core.js` (`isPR`), `internal/web/static/js/companion.js` (badge and banner), `internal/web/jstest/companion.test.mjs`

**Interfaces:**
- Consumes (Task 1): `RepMaxes(ctx, userID, slug, excludeSessionID)`.
- Produces:
  - `BootExercise.PRs map[int]float64` as JSON `"prs": {"5": 100}`: reps → best kg, from other sessions.
  - `export function isPR(boot, state, set) // bool`
  - The companion shows `<span data-pr-badge>PR</span>` in the overview row of a PR set, and a `[data-pr-banner]` line after logging one.

- [ ] **Step 1: Write the failing Go test**

Extend `TestBootstrap` in `internal/training/service_test.go` (or add a sibling test using `newEnv`/`follow`). Before the session under test starts, log a finished earlier session with a 100 kg × 5 squat and a 90 kg × 5 warmup. Assert:

```go
	if got := b.Exercises["barbell-back-squat"].PRs; got[5] != 100 || len(got) != 1 {
		t.Fatalf("bootstrap PRs = %v, want {5: 100} (warmups don't count)", got)
	}
```

Also log a 120 kg × 5 set *in the session under test*, then re-bootstrap and assert that `PRs[5]` is still 100: the session's own sets are judged by the companion, not baked into the table.

Run: `nix develop --command go test ./internal/training/ -run Bootstrap`
Expected: FAIL to compile: `BootExercise has no field PRs`.

- [ ] **Step 2: Implement the bootstrap part**

`internal/training/service.go` `Store` interface: add

```go
	RepMaxes(ctx context.Context, userID, slug, excludeSessionID string) ([]store.RepMax, error)
```

`internal/training/bootstrap.go` `BootExercise`: add

```go
	PRs           map[int]float64 `json:"prs"` // best kg per rep count in other sessions, for the advisory PR badge
```

In `bootExercise`, after `be.E1RMKg` is set:

```go
	maxes, err := s.Store.RepMaxes(ctx, user.ID, slug, sessionID)
	if err != nil {
		return BootExercise{}, nil, err
	}
	be.PRs = map[int]float64{}
	for _, m := range maxes {
		be.PRs[m.Reps] = m.WeightKg
	}
```

If `failingStore` in `service_test.go` embeds `*store.DB`, it picks up the method, so nothing more is needed. If it doesn't compile, embed the store as the other test doubles do.

Run: `nix develop --command go test ./internal/training/`
Expected: PASS.

- [ ] **Step 3: Write the failing JS tests**

Append to `internal/web/jstest/companion.test.mjs`, using its existing boot fixtures. Build a minimal boot inline if none fits:

```js
test("isPR beats the heaviest earlier set at the same reps", () => {
  const boot = { exercises: { squat: { name: "Squat", measurement: "weight_reps", prs: { 5: 100 } } }, catalog: [] };
  const mk = (id, kg, reps, done, kind = "working") => ({ id, slug: "squat", kind, weight_kg: kg, reps, done_at: done });
  const a = mk("a", 100, 5, "2026-09-24T10:00:00.000Z");
  const b = mk("b", 102.5, 5, "2026-09-24T10:05:00.000Z");
  const c = mk("c", 102.5, 5, "2026-09-24T10:10:00.000Z");
  const state = { sets: { a, b, c } };
  assert.equal(core.isPR(boot, state, a), false, "a tie is not a PR");
  assert.equal(core.isPR(boot, state, b), true);
  assert.equal(core.isPR(boot, state, c), false, "b already reached 102.5");
  assert.equal(core.isPR(boot, state, mk("w", 200, 5, "2026-09-24T10:20:00.000Z", "warmup")), false, "warmups are never PRs");
  assert.equal(core.isPR(boot, state, mk("n", 50, 3, "2026-09-24T10:20:00.000Z")), false, "no earlier 3-rep set to beat");
  state.sets.b = { ...b, deleted: true };
  assert.equal(core.isPR(boot, state, c), true, "deleted sets don't count");
});

test("PR compares done_at as times", () => {
  const boot = { exercises: { squat: { name: "Squat", measurement: "weight_reps", prs: {} } }, catalog: [] };
  // Go's JSON drops zero milliseconds: "…:00Z" is earlier than "…:00.500Z" but sorts after it as text.
  const synced = { id: "s", slug: "squat", kind: "working", weight_kg: 100, reps: 5, done_at: "2026-09-24T10:00:00Z" };
  const local = { id: "l", slug: "squat", kind: "working", weight_kg: 105, reps: 5, done_at: "2026-09-24T10:00:00.500Z" };
  const state = { sets: { s: synced, l: local } };
  assert.equal(core.isPR(boot, state, local), true);
  assert.equal(core.isPR(boot, state, synced), false);
});
```

Run: `nix develop --command task test:js`
Expected: FAIL, `core.isPR is not a function`.

- [ ] **Step 4: Implement `isPR` and the UI**

`internal/web/static/js/companion-core.js`:

```js
const prKinds = ["working", "amrap"];
const counts = (s) => s && !s.deleted && prKinds.includes(s.kind) && typeof s.weight_kg === "number" && s.reps >= 1;
const time = (s) => Date.parse(s.done_at);

// isPR reports whether set beats the heaviest earlier working or AMRAP set of
// its exercise at the same reps (spec §13): earlier sessions come from the
// bootstrap's table, this session's from state. Advisory; history is judged
// by the server. The first set at a rep count has nothing to beat.
export function isPR(boot, state, set) {
  if (!counts(set)) return false;
  let best = exerciseInfo(boot, set.slug).prs?.[set.reps] ?? null;
  for (const o of Object.values(state.sets)) {
    if (o.id === set.id || o.slug !== set.slug || o.reps !== set.reps || !counts(o)) continue;
    const earlier = time(o) < time(set) || (time(o) === time(set) && o.id < set.id);
    if (earlier) best = best === null ? o.weight_kg : Math.max(best, o.weight_kg);
  }
  return best !== null && set.weight_kg > best;
}
```

`internal/web/static/js/companion.js`:
- In `overview()`, for a done row, show the PR badge after the set text:
  ```js
  st === "done" ? h("span", { class: "flex items-center gap-2" },
      core.isPR(b, s, set) && h("span", { "data-pr-badge": true, class: prBadge }, "PR"),
      h("button", { class: "text-sm underline", onclick: () => { this.editing = set.id; this.render(); } }, setText(set, this.unit)))
  ```
  Add `const prBadge = "rounded bg-amber-200 px-1.5 py-0.5 text-xs font-medium text-amber-900 dark:bg-amber-900 dark:text-amber-100";` next to `btn2`.
- In `submitSet`, keep the result so the banner can name the set:
  ```js
    const res = core.logSet(this.boot, this.state, step, values);
    this.prBanner = core.isPR(this.boot, res.state, res.op.payload) ? `New PR: ${res.op.payload.slug === t.slug ? t.name : core.exerciseInfo(this.boot, res.op.payload.slug).name} ${setText(res.op.payload, this.unit)}` : null;
    this.apply(res);
  ```
- In `render()`, put `this.prBanner && h("p", { "data-pr-banner": true, class: "mt-3 rounded bg-amber-100 p-2 text-sm font-medium text-amber-900 dark:bg-amber-950 dark:text-amber-100", role: "status" }, this.prBanner)` directly before the step or done view.

Run: `nix develop --command task test:js && nix develop --command task e2e`
Expected: PASS. The e2e still shows 14/14 (its PR check comes in Task 7).

- [ ] **Step 5: Commit**

```bash
git add internal/training internal/web/static/js/companion-core.js internal/web/static/js/companion.js internal/web/jstest/companion.test.mjs
git commit -m "feat(web): show advisory PR badges in the workout screen"
```

---

### Task 7: End-to-end checks and docs

**Files:**
- Modify: `test/e2e/companion.mjs`, `AGENTS.md`

**Interfaces:**
- Consumes: everything above, plus the e2e helpers `go`, `evaluate`, `waitFor`, `check`, `doneButton` and `done`.

- [ ] **Step 1: Add the checks**

In `test/e2e/companion.mjs`, after "finishing advances the plan" (the bench session is finished and synced):

```js
  // The exercise page draws the e1RM chart and lists the rep max.
  await go("/exercises/barbell-bench-press");
  await waitFor(() => evaluate(`!!document.querySelector('[data-chart=e1rm] canvas')`), "the e1RM chart").catch(() => {});
  check("the exercise page draws the e1RM chart", await evaluate(`!!document.querySelector('[data-chart=e1rm] canvas')`));
  check("the rep-max table lists the logged weight", await evaluate(`document.body.textContent.includes('Rep maxes') && [...document.querySelectorAll('[data-rep-max]')].some(r => r.textContent.includes('kg'))`));
  await go("/stats/muscles");
  await waitFor(() => evaluate(`!!document.querySelector('[data-chart=muscles] canvas')`), "the muscles chart").catch(() => {});
  check("the muscles page draws its chart", await evaluate(`!!document.querySelector('[data-chart=muscles] canvas')`));
```

At the end of the empty-workout section (curl 12 × 10, then 12 × 8), log one more set at 14 × 8. It beats the earlier 8-rep set of the same session:

```js
  await evaluate(`[...document.querySelectorAll('#companion button')].find(b => b.textContent.trim() === 'Add set').click()`);
  await waitFor(() => evaluate(`!!${doneButton}`), "curl set 3");
  await evaluate(`document.querySelector('input[name=weight]').value = '14'; document.querySelector('input[name=reps]').value = '8'; ${doneButton}.click()`);
  await waitFor(async () => (await done()) === 3, "curl set 3 logged");
  check("a heavier set at the same reps gets a PR badge",
    await evaluate(`document.querySelectorAll('#companion [data-pr-badge]').length === 1 && !!document.querySelector('#companion [data-pr-banner]')`));
  await waitFor(async () => (await adhocSets()) === 3, "curl set 3 on the server").catch(() => {});
  check("history agrees it is a PR", await evaluate(`fetch("/history/${adhocID}").then(r => r.text()).then(t => (t.match(/data-pr-badge/g) || []).length === 1)`));
```

(Adjust the existing "several sets" check to run before this block, so it still counts 2.)

- [ ] **Step 2: Run the e2e**

Run: `nix develop --command task e2e`
Expected: `19/19 checks passed`.

- [ ] **Step 3: Update AGENTS.md**

In the Layout block:
- add `internal/stats/          e1RM series, rep maxes and PRs, weekly hard sets per muscle (queries in store/stats.go)` after `internal/training/`
- extend the `internal/web/` note with `static/js/stats.js + chart-data.js: uPlot charts from /api/stats/*`
- delete the line "Later milestones add `stats/` and `mcp/`…" and replace it with "Milestone 6 adds `mcp/` under `internal/`. See the spec, §4."

In Rules, add:

- **Stats are queries, not stored values.** PRs, rep maxes, the e1RM series and muscle volume are computed from `sets` when shown, so history edits are reflected immediately. The companion's PR badge is advisory, from the bootstrap's `prs` table; history and the exercise page are authoritative.

- [ ] **Step 4: Run CI and commit**

Run: `nix develop --command task ci`
Expected: exit code 0.

```bash
git add test/e2e/companion.mjs AGENTS.md
git commit -m "test(e2e): check charts, rep maxes and PR badges; document stats"
```
