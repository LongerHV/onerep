# Backlog

Known issues, deferred on purpose: each was found in a milestone's final review and judged minor. Pick items up when touching the area, or batch them in a clean-up branch. Remove an entry when you fix it, with the fix referenced in the commit message.

## Operations and deployment

- **Backup onto an existing file.** An empty existing destination is silently overwritten. A non-database file fails with only `file is not a database (26)`, which reads as if the live database were corrupt. Fix: `os.Stat(dest)` first and wrap errors as `backup to <dest>: …`. (`internal/store/store.go`, Backup)
- **`migrate` and `backup` need the full server config.** `config.Load` requires `ONEREP_BASE_URL` and the OIDC settings for every subcommand, so a host-side cron backup needs the client secret. Fix: only validate those for `serve`. (`cmd/onerep/main.go`)
- **A failed migration leaves the database "dirty" with no way to recover in the image.** golang-migrate marks it dirty, and every later start fails. The distroless image has no `sqlite3` or `migrate force`. Fix: on `ErrDirty`, force the previous version (migrations run in a transaction, so the schema was rolled back), or add `onerep migrate --force N`. (`internal/store/store.go`)
- **Base URL with a path is accepted but unsupported.** `https://host/onerep` passes validation, but routes, assets and the cookie path are absolute. Fix: reject a non-empty path, or document that onerep must be served at the root. (`internal/config/config.go`)
- **Static assets are unversioned and cached for an hour.** Users can see stale CSS and JS for up to an hour after an upgrade. Fix: content-hashed URLs, served as immutable. (`internal/web/static.go`)
- **Pre-release tags publish `latest`,** and the release workflow doesn't wait for CI. (`.github/workflows/release.yml`)
- **A second SIGTERM or Ctrl-C during shutdown is ignored.** Fix: call `stop()` as soon as the first signal arrives. (`cmd/onerep/main.go`)
- **Compose user default.** `${UID:-1000}:${GID:-1000}`: bash doesn't export `UID` or define `GID`, so compose always runs as 1000:1000. Fix: document `UID=$(id -u) GID=$(id -g) docker compose up`, or use `.env`. (`deploy/compose.yaml`)
- **Task name differs from the spec.** Spec §11 says `task dev:dex`; the Taskfile and docs say `task dex`.

## Auth

- **Two login tabs at once make each other fail** with "state mismatch", because they share one `onerep_oidc` flow cookie. Fix: name the cookie per state, or restart the login on a mismatch. (`internal/auth/oidc.go`)
- **Login and logout errors are bare text pages** with no way back. An IdP `access_denied` or an expired session on logout shows plain text. Fix: render the error page with a "Sign in again" link. (`internal/auth/oidc.go`, `middleware.go`)
- **Unused field `OIDC.issuer`.** (`internal/auth/oidc.go`)

## Calculations

- **Go and JS disagree above about 9.2e16 kg or for +Inf,** because of int64 overflow in `cents`. It can't be reached from the UI. Fix: clamp non-finite or huge targets in both `Round` implementations, and add a shared test vector. (`internal/calc/units.go`, `calc.js`)
- **Go `E1RM(NaN, …)` returns ok; JS returns null.** Fix: `if !(weightKg > 0)`. (`internal/calc/e1rm.go`)

## Exercises and equipment

- **`nan` is accepted as a number.** As a training max it silently clears the TM and logs a "none → none" change. As a calculator percentage it renders "NaN%". Fix: reject non-finite values when parsing the TM and pct, and in `SetTrainingMax`. (`internal/web/exercises.go`, `internal/exercise/service.go`)
- **The exercise settings form isn't saved all at once.** The equipment link is saved even when the training max is rejected. The 1,500 kg limit says "enter a positive weight", which is misleading. (`internal/web/exercises.go`, `internal/exercise/service.go`)
- **A custom slug that later collides with a new seeded slug** shows as "customized", picks up the seed's alternatives, and its Delete button becomes "Reset to default". (`internal/store/exercises.go`, `views/exercises.templ`)
- **Decimal comma handling is inconsistent.** The training max accepts `142,5`, but the bar weight rejects `7,5`. In plate, weight and stack lists, `2,5` silently becomes the two values 2 and 5. (`internal/web/equipment.go`, `internal/exercise/weights.go`)
- **Duplicate muscles are stored** from a crafted POST, and would double-count in the milestone 5 stats. An unknown secondary muscle is reported under `primary_muscles`. Fix: dedupe in `normalize`. (`internal/exercise/service.go`)
- **Hidden seeded slugs are "taken" but invisible.** Create reports "already exists", and Update revives the slug as a user copy. (`internal/exercise/service.go`)
- **Deleting a custom exercise keeps its settings.** The TM, equipment link, TM history and user alternatives remain, and come back if the slug is re-created. This may be intended (history is keyed by slug), but it's undocumented.
- **Equipment links aren't checked against the exercise's kind** (a barbell exercise can link to a dumbbell profile), and changing a profile's kind keeps its links. (`internal/store/user_exercise.go`)

## Plans

- **A completed plan restarts when a shorter version is activated.** The comparison page warns first; whether "complete" should be preserved is undecided. (`internal/plan/service.go`, keepsCursor)
- **Whole numbers written as floats** (`"weeks": 1.0`, `"reps": 5.0`, `3e0`) pass the schema, then fail with Go decode messages. Fix: accept integral floats when decoding, or reject them in the schema with a friendly message. (`internal/plan/doc.go`, `validate.go`)
- **`duration_s: 0` is allowed** and shows as "3 × 0". Fix: a separate duration definition with `minimum: 1`. (`internal/plan/plan.schema.json`)
- **Some messages still use library wording** (reps and slug patterns, `multipleOf`, `maxProperties`). Fix: extend `friendlier` in `internal/plan/validate.go`.
- **The "1 MB" document limit is really about 400–650 KB of JSON** once form-encoded, so the message misleads. (`internal/web/server.go`, limitBody)
- **Discarding the newest draft reuses its version number,** which makes notes or AI conversations that mention "v3" ambiguous. Fix: a `next_version` counter on `plans`. (`internal/store/plans.go`)
- **Extra queries.** `planDetail` calls `Plans.Next`, which resolves a whole day, just to know `Following`. `Service.Plans` loads every version document of every plan. (`internal/web/plans.go`, `internal/plan/service.go`)
- **Small loose ends:** `views.HomePage.Notice` is never set; `Service.Follow` doesn't refuse archived plans; following a plan that has only drafts shows a full 409 page instead of an inline message.

## Training and companion mode

- **Notes from a slow phone clock are dropped.** `SetSessionNotes` compares the client's `updated_at` with `sessions.updated_at`, which is stamped by the server at start. A phone that is behind by more than the time since the start gets its notes Ignored but reported as applied. Fix: compare only against earlier notes edits. (`internal/store/sessions.go`)
- **Server-side deletes don't reach an open companion.** A set deleted in history or on another device stays "done" locally. Fix: in `mergeServerSets`, drop synced local sets that are absent from the server and not in the outbox. (`companion-core.js`)
- **Local state and the outbox are written in separate IndexedDB transactions.** A tab killed between them keeps a set locally that never syncs, and `tx()` has no `onabort`, so a quota abort hangs. Fix: one readwrite transaction over both stores, and reject on abort. (`companion.js`)
- **Wake lock and audio aren't released on finish.** The screen stays on after "Finish" until you navigate away, and each companion's `AudioContext` is never closed. (`companion.js`)
- **Logout doesn't clear offline data.** The outbox, IndexedDB state and cached live pages survive logout. On a shared browser, the next user's session flushes the previous user's queue, which is rejected, and the cached pages still show the old workout. (`companion.js`, `sw.js`)
- **Nothing is pruned:** the PAGES cache, IndexedDB `sessions` and `applied_ops` grow without bound (slowly).
- **History times are in server time.** `.Local()` in `views/history.templ` is UTC in the container.
- **The shell version ignores templates.** A changed `/offline` page isn't re-cached until a static file changes. (`internal/web/sessions.go`, staticVersion)
- **History headings alternate for supersets** (A, B, A, B), because it groups consecutive sets of the same exercise. (`internal/web/history.go`)

## Stats

- **The 401 for `/api/*` is plain text, not JSON.** Scripts only check `res.ok`, so nothing breaks, but spec §16 describes JSON errors. (`internal/auth/middleware.go`, RequireUser)
- **The companion's PR table can include later sets.** `RepMaxes(…, excludeSessionID)` takes other sessions' bests regardless of time, so resuming an older workout after a newer one was logged can hide an advisory badge. Fix: bound it by the session's `started_at`. (`internal/training/bootstrap.go`)
- **The PR banner goes stale.** "New PR: …" stays after that set is edited below the record, deleted, or the workout is finished, until the next set is logged. Fix: recompute it in `render()` from the last logged set. (`companion.js`)
- **"No working sets logged yet" is keyed on the 1–12 rep table,** so someone who only logged sets above 12 reps sees it. A `bw_reps` set saved from the history form with an empty weight stores NULL, where the companion stores 0, so it drops out of PRs. (`views/exercises.templ`, `internal/web/history.go`)
- **`HardSets` has no `(user_id, done_at)` index,** so the muscles page scans all of a user's sets, twice per visit (page and API). Fine now, and grows with years of history. (`internal/store/stats.go`, `schema.sql`)
- **Redundant work.** The exercise page calls `Exercises.Get` twice, and the e1RM API runs `RepMaxes`, which it never uses. (`internal/web/exercises.go`, `internal/web/stats.go`)
- **Chart colours don't follow a theme change** until the page is reloaded. (`static/js/stats.js`)
- **Muscles chart ticks could fall between weeks** on a wider layout; set `incrs: [1, 2, 4]` on the x axis. (`static/js/stats.js`)
