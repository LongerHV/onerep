# Backlog

Known issues, deferred on purpose: each was found in a milestone's final review and judged minor. Pick items up when touching the area, or batch them in a clean-up branch. Remove an entry when you fix it, with the fix referenced in the commit message.

## Operations and deployment

- **No favicon, app icons or web app manifest.** `/favicon.ico` is a 404, and installing onerep on a phone shows a generic icon and name. Fix: an icon set and a `manifest.webmanifest` (name, icons, `display: standalone`, theme colours) linked from the layout, both added to the service worker shell. (`views/layout.templ`, `static/`, `static/js/sw.js`)

## Auth

- **Two login tabs at once make each other fail** with "state mismatch", because they share one `onerep_oidc` flow cookie. Fix: name the cookie per state, or restart the login on a mismatch. (`internal/auth/oidc.go`)
- **Login and logout errors are bare text pages** with no way back. An IdP `access_denied` or an expired session on logout shows plain text. Fix: render the error page with a "Sign in again" link. (`internal/auth/oidc.go`, `middleware.go`)
- **Unused field `OIDC.issuer`.** (`internal/auth/oidc.go`)

## Exercises and equipment

- **The exercise settings form isn't saved all at once.** The equipment link is saved even when the training max is rejected. The 1,500 kg limit says "enter a positive weight", which is misleading. (`internal/web/exercises.go`, `internal/exercise/service.go`)
- **A custom slug that later collides with a new seeded slug** shows as "customized", picks up the seed's alternatives, and its Delete button becomes "Reset to default". (`internal/store/exercises.go`, `views/exercises.templ`)
- **Hidden seeded slugs are "taken" but invisible.** Create reports "already exists", and Update revives the slug as a user copy. (`internal/exercise/service.go`)
- **Deleting a custom exercise keeps its settings.** The TM, equipment link, TM history and user alternatives remain, and come back if the slug is re-created. This may be intended (history is keyed by slug), but it's undocumented.
- **Equipment links aren't checked against the exercise's kind** (a barbell exercise can link to a dumbbell profile), and changing a profile's kind keeps its links. (`internal/store/user_exercise.go`)

## Plans

- **A completed plan restarts when a shorter version is activated.** The comparison page warns first; whether "complete" should be preserved is undecided. (`internal/plan/service.go`, keepsCursor)
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
- **History headings alternate for supersets** (A, B, A, B), because it groups consecutive sets of the same exercise. (`internal/web/history.go`)

## Stats

- **The companion's PR table can include later sets.** `RepMaxes(…, excludeSessionID)` takes other sessions' bests regardless of time, so resuming an older workout after a newer one was logged can hide an advisory badge. Fix: bound it by the session's `started_at`. (`internal/training/bootstrap.go`)
- **The PR banner goes stale.** "New PR: …" stays after that set is edited below the record, deleted, or the workout is finished, until the next set is logged. Fix: recompute it in `render()` from the last logged set. (`companion.js`)
- **"No working sets logged yet" is keyed on the 1–12 rep table,** so someone who only logged sets above 12 reps sees it. A `bw_reps` set saved from the history form with an empty weight stores NULL, where the companion stores 0, so it drops out of PRs. (`views/exercises.templ`, `internal/web/history.go`)

## MCP

- **A draft can change between review and activation.** If the AI replaces a draft while the user has its compare page open, Activate takes the new document. `Service.Activate` also reads the doc outside its transaction. Fix: post a hash of the rendered doc and refuse on mismatch. (`internal/plan/service.go`, compare page)
- **`save_plan_draft` loose ends:** `plan_id` is ignored when `version_id` is given (even if it names another plan); archived plans accept drafts; replacing the only draft of a draft-only plan doesn't rename the plan.

## Plan form editor

- **Per-week parse errors are fragile:** the "enter reps like…" message is hidden by the next form change while the bad text stays in the input (the document keeps the old value), and the check runs on `change`, not as you type. (`plan-form-theme.js`)
- **Real exercise search.** The exercise select is a native select (type-ahead only), not the spec's "searchable select". The user accepted this for now (2026-09-26); add a search field later. (`definitions.slug` enum, `plan-form-theme.js`)
- **Server warnings flicker after every form change.** json-editor's own validation on change hides the markers from `showValidationErrors` until the preview returns and `markProblems` puts them back (~0.5 s later), so the form shrinks and regrows; scroll anchoring keeps the page in place. Fix: re-apply the last problems on json-editor's change as well. (`static/js/plan-form.js`)
