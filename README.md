# onerep

Self-hosted gym progress tracker: training plans, a live workout companion,
e1RM/PR history, and an MCP server so an AI assistant can review your training
and draft the next block. Single Go binary + SQLite, served as a PWA.

Status: early development. See `docs/superpowers/specs/` for the design.

## Run

```sh
docker run -p 8080:8080 -v ./data:/data --user "$(id -u):$(id -g)" \
  -e ONEREP_DB=/data/onerep.db \
  -e ONEREP_BASE_URL=https://gym.example.com \
  -e ONEREP_OIDC_ISSUER=https://id.example.com \
  -e ONEREP_OIDC_CLIENT_ID=onerep \
  -e ONEREP_OIDC_CLIENT_SECRET=... \
  ghcr.io/longerhv/onerep:latest
```

Register `${ONEREP_BASE_URL}/auth/callback` as the redirect URI at your OIDC
provider. See `deploy/compose.yaml` for a compose example.

| Variable | Default | Meaning |
|---|---|---|
| `ONEREP_DB` | `./onerep.db` | SQLite database path |
| `ONEREP_LISTEN` | `:8080` | Listen address |
| `ONEREP_BASE_URL` | — (dev: `http://localhost:8080`) | Public URL; `https` enables Secure cookies |
| `ONEREP_OIDC_ISSUER` / `_CLIENT_ID` / `_CLIENT_SECRET` | — | OIDC provider |
| `ONEREP_AUTO_MIGRATE` | `true` | Apply migrations on startup |
| `ONEREP_ENV` | `prod` | `dev` enables debug logs and allows `ONEREP_DEV_USER` |
| `ONEREP_DEV_USER` | — | Dev only: sign everyone in as this user, no IdP |

Commands: `onerep serve` (default), `onerep migrate`, `onerep backup <path>`.
For continuous backups, run [Litestream](https://litestream.io) next to the database.

## Connecting an AI assistant (MCP)

onerep includes an [MCP](https://modelcontextprotocol.io) server, so an assistant such as Claude can analyse your training and draft plans for you.

1. In onerep, open **Settings → API tokens** and create a token. It is shown once, so copy it.
2. Point your assistant at `<ONEREP_BASE_URL>/mcp` (Streamable HTTP) with the header `Authorization: Bearer <token>`. In Claude Code:

   ```sh
   claude mcp add --transport http onerep https://onerep.example.com/mcp --header "Authorization: Bearer onerep_…"
   ```

The assistant acts as you. It can read your workouts, stats and weekly volume, search and create exercises, set training maxes, and save plan **drafts**. It can't change logged workouts or activate a plan: every draft comes with a link to its comparison page, where you review it and then activate or discard it. Two prompts get you started: `review_block` (analyse your last block and propose the next) and `build_plan` (plan from scratch). You can revoke a token on the same settings page at any time.

## License

onerep is licensed under the [GNU Affero General Public License v3.0](LICENSE).
Third-party license texts ship in the container image under
`/var/run/ko/third_party`.

## Develop

```sh
nix develop          # or: direnv allow
task dev             # live reload, signed in as $USER, no IdP needed
task dex             # local Dex (alice@example.com / password) ...
task dev:oidc        # ... and onerep using it
task ci              # what CI runs
```
