# Backlog

Known issues, deferred on purpose: each was found in a milestone's final review and judged minor. Pick items up when touching the area, or batch them in a clean-up branch. Remove an entry when you fix it, with the fix referenced in the commit message.

## Operations and deployment

- **No favicon, app icons or web app manifest.** `/favicon.ico` is a 404, and installing onerep on a phone shows a generic icon and name. Fix: an icon set and a `manifest.webmanifest` (name, icons, `display: standalone`, theme colours) linked from the layout, both added to the service worker shell. (`views/layout.templ`, `static/`, `static/js/sw.js`)
- **Static assets are unversioned and cached for an hour.** Users can see stale CSS and JS for up to an hour after an upgrade. Fix: content-hashed URLs, served as immutable. (`internal/web/static.go`)

## Plans

- **A completed plan restarts when a shorter version is activated.** The comparison page warns first; whether "complete" should be preserved is undecided. (`internal/plan/service.go`, keepsCursor)
- **The "1 MB" document limit is really about 400–650 KB of JSON** once form-encoded, so the message misleads. (`internal/web/server.go`, limitBody)
- **Discarding the newest draft reuses its version number,** which makes notes or AI conversations that mention "v3" ambiguous. Fix: a `next_version` counter on `plans`. (`internal/store/plans.go`)
- **Extra queries.** `planDetail` calls `Plans.Next`, which resolves a whole day, just to know `Following`. `Service.Plans` loads every version document of every plan. (`internal/web/plans.go`, `internal/plan/service.go`)
- **Small loose ends:** `views.HomePage.Notice` is never set; `Service.Follow` doesn't refuse archived plans; following a plan that has only drafts shows a full 409 page instead of an inline message.

## Training and companion mode

- **The shell version ignores templates.** A changed `/offline` page isn't re-cached until a static file changes. (`internal/web/sessions.go`, staticVersion)

## Plan form editor

- **Real exercise search.** The exercise select is a native select (type-ahead only), not the spec's "searchable select". The user accepted this for now (2026-09-26); add a search field later. (`definitions.slug` enum, `plan-form-theme.js`)