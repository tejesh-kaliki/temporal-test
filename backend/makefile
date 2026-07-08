# Pinned codegen tool versions (run via `go run`, no global install needed).
SQLC_VERSION := v1.27.0
OAPI_VERSION := v2.4.1

.PHONY: build test test-up test-down generate sqlc oapi

TEST_COMPOSE := docker compose -f docker/docker-compose-test.yaml

build:
	go build -o bin/server ./cmd/server

# Self-starting integration tests: brings the test postgres up, runs tests,
# tears it down (and reports the test exit status). Each test package uses its
# own schema, so packages run in parallel — no -p 1 needed.
test: test-up
	go test ./... ; status=$$? ; $(TEST_COMPOSE) down -v >/dev/null 2>&1 ; exit $$status

test-up:
	$(TEST_COMPOSE) up -d --wait

test-down:
	$(TEST_COMPOSE) down -v

# Regenerate all code from sql/queries and api/services specs.
generate: sqlc oapi

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

# Runs every //go:generate directive (the per-domain gen/api/<domain>/generate.go
# files). Add a domain by dropping in its codegen.yaml + generate.go.
oapi:
	go generate ./...
