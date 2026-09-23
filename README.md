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
