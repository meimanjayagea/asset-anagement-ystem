package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func migrationConnection(command string, getenv func(string) string) (string, error) {
	if dsn := getenv("MIGRATION_DATABASE_URL"); dsn != "" {
		return dsn, nil
	}
	// Explicit recovery opt-in lets managed secrets stay inside the deployment.
	if command == "migrate" || getenv("MIGRATION_USE_RUNTIME_DATABASE") == "true" {
		if dsn := getenv("DATABASE_URL"); dsn != "" {
			return dsn, nil
		}
		return "", fmt.Errorf("DATABASE_URL required for runtime-connection migration")
	}
	return "", fmt.Errorf("MIGRATION_DATABASE_URL required for AUTO_MIGRATE; temporary owner-connection recovery requires MIGRATION_USE_RUNTIME_DATABASE=true")
}

func migrateDatabase(ctx context.Context, dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid migration connection configuration")
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "240000"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("migration connection initialization failed")
	}
	defer db.Close()
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migration connection failed: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}()
	// Transaction locks remain on the same connection even behind a transaction pooler.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7842301)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version integer PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	for _, migration := range []struct {
		version int
		file    string
	}{{1, "001_init.sql"}, {2, "002_branches_activity.sql"}, {3, "003_roles_scope_archive.sql"}, {4, "004_finance_lifecycle.sql"}, {5, "005_organization_codes.sql"}, {6, "006_employee_login.sql"}, {7, "007_field_lifecycle.sql"}, {8, "008_record_metadata.sql"}} {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, migration.version).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		sql, err := migrations.ReadFile("migrations/" + migration.file)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("migration %d failed: %w", migration.version, err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("migrations complete", "version", 8)
	return nil
}
