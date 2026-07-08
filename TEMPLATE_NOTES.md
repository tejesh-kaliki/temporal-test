# Template notes

This project was scaffolded from the kaliki-template stack. This file documents
the wiring conventions so you can extend it cleanly.

## Adding a domain (the "enumeration points")

A domain touches a fixed set of places. To add `widgets`:

1. `api/services/widgets.yaml` — define the contract first.
2. `backend/internal/widgets/{service,store}.go` — handler + data access.
3. `backend/sql/schema/00NN_widgets.sql` — goose-style migration.
4. `backend/internal/server/server.go` — register routes:
   `widgets.New(db).Register(api)`.

The `items` domain is a complete worked example of all of the above.

## Removing the example domain

The example domain was not generated.

## Admin / private API surface (upgrade path)

This project ships a **single** generated API client. Admin-only endpoints are
expected to be protected by **server-side authz** — that is the real security
boundary.

If you later add an admin/back-office app (intentionally not scaffolded — add it
by hand when you actually need it) and want to keep admin endpoints out of the
customer app's client bundle and docs, adopt the split-spec pattern:

1. Tag operations in `api/services/*.yaml` with `x-audience: public | admin`.
2. Build **two** combined specs (public, admin) from the redocly config.
3. Generate **two** Dart clients; the customer app imports only the public one,
   the admin app imports both.

This is documentation-only here. Note it hides endpoints from the bundle, it is
**not** a substitute for authz.


## Tier-2 modules

These are wired as integration points (client init + config), not full business
logic — extend them for your product:

Each is constructed in `internal/server/server.go`; inject it into the domains
that need it.

## Testing

Integration-style handler tests register real routes on a real gin router and
hit real endpoint paths, verifying DB state (not just HTTP status). They run
against a throwaway postgres.

```sh
cd backend && make test     # starts test postgres, runs tests, tears it down
```

Each test package uses its own postgres **schema** (via `search_path`), so
packages run in parallel without colliding — no `-p 1` needed. Per test, tables
are truncated. Shared helpers live in `internal/testsupport`.

Conventions: one top-level `Test<OperationId>` per OpenAPI operation; subtests
in the order `success`, `authorization`, `validation`, `not_found`, `conflict`.
See `internal/items` and `internal/auth` for worked examples.

### Database isolation — when to change it

The default (shared postgres, schema-per-package, truncate-per-test) is simple
and parallel-safe. Graduate only if you hit real pain:

- **Leftover-state flakiness or zero-setup `go test`** → adopt
  [testcontainers-go](https://golang.testcontainers.org/) so the DB lifecycle is
  owned by the test process (random ports, auto-cleanup via Ryuk).
- **Within-package parallelism** (`t.Parallel()` across tests in one package) →
  transaction-per-test, but only if you inject the `database.Queries` executor
  (sqlc accepts any `DBTX`, including `pgx.Tx`) into your services. This couples
  service construction to the test strategy, so prefer it only where it pays off.

## Observability

OpenTelemetry tracing is wired in: `otelgin` traces HTTP requests and `otelpgx`
traces DB queries. It is a no-op until you set `observability.endpoint` (or
`OTEL_EXPORTER_OTLP_ENDPOINT`) to an OTLP/**gRPC** collector `host:port` — then
the global tracer provider exports via OTLP (insecure; front it with a collector
or add TLS for production). Spans flush on graceful shutdown.

## Migrations

Schema migrations are goose files in `backend/sql/schema/` (`-- +goose Up` /
`-- +goose Down`), applied on server startup and in tests. Each runs in its own
transaction. Add one as `000N_name.sql` with a strictly increasing numeric
prefix; sqlc reads the same files for type generation.

### Numbering

The template files carry fixed prefixes (`0001`…`0006`) but are conditionally
rendered, so turning a module off leaves **gaps** in the sequence (e.g. with
`auth=none` you get `0001`, then `0005`). goose orders by version number and
tolerates gaps, so this is purely cosmetic — **don't renumber to close them.**
Migrations are append-only: when you add one, always pick a prefix **higher than
the largest existing one**, and never rename or re-prefix a migration that has
already been applied anywhere (goose tracks applied migrations by version, so a
rename re-runs it or breaks startup).

### Migrations are generate-once — exclude them from `copier update`

Migrations are **not** kept in sync by `copier update`. They are an append-only
ledger tied to your real database state, and letting the template re-merge them
invites out-of-order application, rename churn, and schema drift between a
from-scratch build and an updated project. Treat `backend/sql/schema/` as
generated once, then owned entirely by this project.

When you pull template updates, exclude the schema dir so the merge never
touches it:

```sh
copier update --exclude 'backend/sql/schema/**'
```

If a new template **version** adds migrations of its own (check its changelog /
release notes), don't let `copier update` apply them. Instead, **manually review
the new files and re-create the ones you want** as fresh migrations with the
next higher prefix in your own sequence. That keeps every migration strictly
in-order for goose and avoids all of the out-of-order edge cases — at the cost
of a manual check whenever the template ships schema changes.


