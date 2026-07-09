# Temporal Test

Generated from the kaliki-template stack (Go + Flutter + OpenAPI codegen, Docker-first).

## Quick start (Docker)

```sh
cp .env.example .env
docker compose up --build
curl localhost:8080/health
```

Postgres and the backend all come up together. Schema migrations run automatically on backend startup.

## Layout

- `backend/` — Go HTTP server (gin + pgx). Domains live in `internal/<domain>`.
- `api/` — OpenAPI specs (source of truth for the API contract).

## Observability

Tracing is wired in by default with a **no-op** exporter. Set
`observability.endpoint` in `backend/config/env.yaml` (or
`OTEL_EXPORTER_OTLP_ENDPOINT`) to export to a real collector.

`backend/config/env.yaml` is the live, git-ignored config; it is seeded from the
committed `backend/config/env.example.yaml` on generation. Edit the example to
change the defaults a fresh checkout starts from.

## CORS

The API enables CORS for browser clients. In development (`APP_ENV` unset or not
`production`) it reflects any `Origin`, so the frontend works with no setup. In
production set `CORS_ALLOWED_ORIGINS` to a comma-separated allowlist.

## Fitness checks

`cd backend && make fitness` runs the repo-wide static checks (also enforced
in CI). Tool versions are pinned per-check via `go run`, so nothing needs
installing first — see `TEMPLATE_NOTES.md` for details.

See `TEMPLATE_NOTES.md` for how to add a domain and remove the example.
