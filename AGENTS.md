# AGENTS.md

Guidance for AI coding agents (and humans) working on onerep. `CLAUDE.md` is a symlink to this file.

## What this is

onerep is a self-hosted gym progress tracker: a single Go binary serving a PWA (templ + htmx + Tailwind), SQLite storage, OIDC login, and an MCP server so an AI can analyse history and draft training plans.

- Design spec: `docs/superpowers/specs/2026-09-23-onerep-v1-design.md`. Read it before changing behaviour.
- Implementation plans: `docs/superpowers/plans/`. One plan per milestone.

## Environment

All tools come from the Nix flake. Run everything inside `nix develop` (or `direnv allow`). Don't install tools globally or add a Node toolchain to the build. Node is only there for the JS test runner.

The dev shell sets `CGO_ENABLED=0` and `ONEREP_ENV=dev`.

## Commands

| Task | Command |
|---|---|
| Live-reload dev server, signed in as `$USER`, no IdP needed | `task dev` |
| Local Dex identity provider (alice@example.com / password) | `task dex`, then `task dev:oidc` |
| Regenerate templ components and Tailwind CSS | `task generate` |
| Tests | `task test` (or `go test ./internal/<pkg>/ -run TestName`) |
| Lint | `task lint` |
| Everything CI runs | `task ci` |
| New migration after editing `internal/store/schema.sql` | `task migrate:diff NAME=<snake_case>` |
| Container image (local, no push) | `task image` |

## Layout

```
cmd/onerep/            main, config wiring, subcommands (serve, migrate, backup)
internal/config/       env-var configuration
internal/store/        ALL SQL: schema.sql, generated migrations/, repositories
internal/store/storetest/  migrated temp DB for tests
internal/auth/         OIDC, cookie sessions, CSRF, dev login bypass
internal/web/          chi router, handlers, views/ (templ), static/ (embedded)
```

Later milestones add `exercise/`, `plan/`, `training/`, `calc/`, `stats/`, and `mcp/` under `internal/`. See the spec, §4.

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
- **Every non-GET request with a session must carry the CSRF token**: the `X-CSRF-Token` header, set globally for htmx via `hx-headers`, or the `csrf_token` form field.
- **The dev login bypass** (`ONEREP_DEV_USER`) only runs with `ONEREP_ENV=dev`, and only on authenticated app routes.
- **Keep dependencies few.** Ask before adding a Go module or a vendored JS library.
- **Licensing:** onerep is AGPL-3.0-only. Dependencies must use MIT, BSD-2/3-Clause, ISC, 0BSD or Apache-2.0. `task licenses:check` enforces this for Go modules. A vendored file gets its license next to it (`<name>.LICENSE`). Keep the footer link to the source code.
