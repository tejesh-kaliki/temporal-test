// Package migrate applies goose SQL migrations at startup. It is a runtime
// concern kept separate from internal/database, which holds only sqlc-generated
// query code.
package migrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Up applies all goose migrations in migrationsDir. Each migration runs in its
// own transaction (goose default), so a failure mid-file rolls back cleanly.
//
// schema, when non-empty, scopes the connection's search_path — used by tests
// to isolate each package in its own schema. Pass "" for the default (public).
func Up(ctx context.Context, dsn, schema, migrationsDir string) error {
	connCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse dsn: %w", err)
	}
	if schema != "" {
		connCfg.RuntimeParams["search_path"] = schema
	}

	sqlDB := stdlib.OpenDB(*connCfg)
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
