# Backlog

Known issues, deferred on purpose: each was found in a milestone's final review and judged minor. Pick items up when touching the area, or batch them in a clean-up branch. Remove an entry when you fix it, with the fix referenced in the commit message.

## Operations and deployment

- **No favicon, app icons or web app manifest.** `/favicon.ico` is a 404, and installing onerep on a phone shows a generic icon and name. Fix: an icon set and a `manifest.webmanifest` (name, icons, `display: standalone`, theme colours) linked from the layout, both added to the service worker shell. (`views/layout.templ`, `static/`, `static/js/sw.js`)
- **Static assets are unversioned and cached for an hour.** Users can see stale CSS and JS for up to an hour after an upgrade. Fix: content-hashed URLs, served as immutable. (`internal/web/static.go`)

## Training and companion mode

- **Logout doesn't clear offline data.** The outbox, IndexedDB state and cached live pages survive logout. On a shared browser, the next user's session flushes the previous user's queue, which is rejected, and the cached pages still show the old workout. (`companion.js`, `sw.js`)
- **Nothing is pruned:** the PAGES cache, IndexedDB `sessions` and `applied_ops` grow without bound (slowly).
- **The shell version ignores templates.** A changed `/offline` page isn't re-cached until a static file changes. (`internal/web/sessions.go`, staticVersion)

## Plan form editor

- **Real exercise search.** The exercise select is a native select (type-ahead only), not the spec's "searchable select". The user accepted this for now (2026-09-26); add a search field later. (`definitions.slug` enum, `plan-form-theme.js`)