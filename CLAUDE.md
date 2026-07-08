# CLAUDE.md

Guidance for AI agents working in Temporal Test.

## Stack

- `backend/` — Go HTTP server (gin + pgx). One package per domain under `internal/`.
- `api/` — OpenAPI specs; the contract is the source of truth. Update the spec first.

## Order of operations for API changes

1. Update `api/services/*.yaml`.
2. Implement the backend handler in `internal/<domain>`.
3. Register routes in `internal/server/server.go`.

## Conventions

- Verify backend changes with `go test ./...`, not just `go build`.
- Migrations are goose-style files in `backend/sql/schema`, applied on startup.
- Observability is always-on with a no-op default — never gate it behind a flag.

See `TEMPLATE_NOTES.md` for how domains are wired.
