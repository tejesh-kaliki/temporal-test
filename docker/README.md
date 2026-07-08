# Docker setup

Base + flavor composition: `docker-compose.yml` provides the app-level
dependencies (ledger Postgres, Qdrant, and the ingestion worker); combine it
with exactly one Temporal flavor file to get a working stack.

```sh
# Full/prod-like: real Postgres-backed Temporal cluster (server, admin-tools, UI)
docker compose -f docker/docker-compose.yml -f docker/docker-compose.temporal.yml up -d --build

# Lightweight dev: single-container `temporal server start-dev`, embedded SQLite
docker compose -f docker/docker-compose.yml -f docker/docker-compose.temporal-dev.yml up -d --build
```

Both flavors expose the Temporal frontend as a service literally named
`temporal` on `TEMPORAL_PORT` (default `7233`), so `docker-compose.yml`'s
worker always points at `temporal:7233` regardless of which flavor is up. Only
run one flavor at a time alongside the base file.

Web UI: http://localhost:8233 in both flavors (a dedicated `temporal-ui`
container in the prod-like flavor, bundled into the dev server itself in the
lightweight flavor).

Worker's ingest folder (drop files here to trigger the pipeline):
`backend/data/inbox`. Full pipeline also needs a host Ollama serving the
configured models:

```sh
ollama pull nomic-embed-text
```

## Files

- `docker-compose.yml` — base: `app-postgres`, `qdrant`, `worker`.
- `docker-compose.temporal.yml` — flavor: Postgres-backed Temporal cluster.
- `docker-compose.temporal-dev.yml` — flavor: single-container dev-mode Temporal.
- `dynamicconfig/development-sql.yaml` — dynamic config mounted into the
  prod-like flavor's `temporal` service.

## Tearing down

```sh
docker compose -f docker/docker-compose.yml -f docker/docker-compose.temporal.yml down -v
```

(swap in whichever flavor file you brought up; `-v` also drops the named
volumes, including Postgres/Qdrant/Temporal data).
