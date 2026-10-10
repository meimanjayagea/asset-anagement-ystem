package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationConnection(t *testing.T) {
	for _, tc := range []struct {
		name, command, owner, runtime, optIn, want string
		wantError                                  bool
	}{
		{"owner", "serve", "owner", "runtime", "", "owner", false},
		{"restricted-default", "serve", "", "runtime", "", "", true},
		{"explicit-recovery", "serve", "", "runtime", "true", "runtime", false},
		{"invalid-opt-in", "serve", "", "runtime", "1", "", true},
		{"missing-recovery-connection", "serve", "", "", "true", "", true},
		{"manual", "migrate", "", "runtime", "", "runtime", false},
		{"owner-precedence", "serve", "owner", "runtime", "true", "owner", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"MIGRATION_DATABASE_URL": tc.owner, "DATABASE_URL": tc.runtime, "MIGRATION_USE_RUNTIME_DATABASE": tc.optIn}
			got, err := migrationConnection(tc.command, func(key string) string { return env[key] })
			if got != tc.want || (err != nil) != tc.wantError {
				t.Fatalf("connection selection = %q, %v", got, err)
			}
		})
	}
}

func TestMigrationUpgradeAndConcurrentStartup(t *testing.T) {
	for _, version := range []int{2, 4} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			testMigrationUpgrade(t, version)
		})
	}
}

func testMigrationUpgrade(t *testing.T, version int) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := fmt.Sprintf("test_migration_%d", time.Now().UnixNano())
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer base.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"001_init.sql", "002_branches_activity.sql", "003_roles_scope_archive.sql", "004_finance_lifecycle.sql"}[:version] {
		sql, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO organizations(name) VALUES('Existing'); INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Meiman','meiman@example.test','preserved','admin',true)`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- migrateDatabase(ctx, u.String()) }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var employee, code, password string
	if err := db.QueryRow(ctx, `SELECT employee_id,code,password_hash FROM users JOIN organizations ON organizations.id=users.org_id WHERE email='meiman@example.test'`).Scan(&employee, &code, &password); err != nil {
		t.Fatal(err)
	}
	if employee != "EMP-1" || code != "ORG-000001" || password != "preserved" {
		t.Fatalf("account not preserved: %s %s", employee, code)
	}
	var versions int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 6 {
		t.Fatalf("expected six migrations, got %d", versions)
	}
}
