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
	"golang.org/x/crypto/bcrypt"
)

func TestAccountRecovery(t *testing.T) {
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
	schema := fmt.Sprintf("test_recovery_%d", time.Now().UnixNano())
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
	if err := migrateDatabase(ctx, u.String()); err != nil {
		t.Fatal(err)
	}
	db, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `INSERT INTO organizations(name) VALUES('Target'),('Other');
		INSERT INTO users(org_id,email,name,password_hash,role,all_branches) VALUES
		(1,'meiman@example.test','Target','old','admin',true),
		(2,'meiman@example.test','Other organization','untouched','admin',true);
		INSERT INTO sessions(token_hash,user_id,expires_at) VALUES('target-old',1,now()+interval '1 hour'),('other',2,now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ org, email, password, operation string }{
		{"ORG-000001", "meiman@example.test", "short", "recovery-operation-1"},
		{"ORG-000001", "meiman@example.test", "Test-Recovery-Password", "short"},
		{"ORG-000001", "missing@example.test", "Test-Recovery-Password", "recovery-operation-1"},
	} {
		if err := resetAccountPassword(ctx, db, input.org, input.email, input.password, input.operation); err == nil {
			t.Fatal("invalid recovery unexpectedly succeeded")
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- resetAccountPassword(ctx, db, "ORG-000001", "MEIMAN@example.test", "Test-Recovery-Password", "recovery-operation-1")
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var password, role, other string
	var allBranches bool
	if err := db.QueryRow(ctx, `SELECT password_hash,role,all_branches FROM users WHERE id=1`).Scan(&password, &role, &allBranches); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(password), []byte("Test-Recovery-Password")) != nil || role != "admin" || !allBranches {
		t.Fatal("password recovery changed account permissions or failed to hash password")
	}
	var targetSessions, otherSessions, audits int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM sessions WHERE user_id=1),(SELECT count(*) FROM sessions WHERE user_id=2),(SELECT count(*) FROM audit_logs WHERE action='password_recovery')`).Scan(&targetSessions, &otherSessions, &audits); err != nil {
		t.Fatal(err)
	}
	if targetSessions != 0 || otherSessions != 1 || audits != 1 {
		t.Fatalf("sessions/audit not scoped or recovery repeated: %d %d %d", targetSessions, otherSessions, audits)
	}
	if err := db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id=2`).Scan(&other); err != nil || other != "untouched" {
		t.Fatal("other organization modified")
	}
	if _, err := db.Exec(ctx, `UPDATE users SET password_hash='changed-after-reset' WHERE id=1; INSERT INTO sessions(token_hash,user_id,expires_at) VALUES('new-session',1,now()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	if err := resetAccountPassword(ctx, db, "ORG-000001", "meiman@example.test", "Different-New-Password", "recovery-operation-1"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT password_hash,(SELECT count(*) FROM sessions WHERE user_id=1) FROM users WHERE id=1`).Scan(&password, &targetSessions); err != nil || password != "changed-after-reset" || targetSessions != 1 {
		t.Fatal("repeated startup reset password or removed new session")
	}
	if _, err := db.Exec(ctx, `CREATE FUNCTION reject_recovery_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test audit unavailable'; END $$;
		CREATE TRIGGER reject_recovery_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_recovery_audit()`); err != nil {
		t.Fatal(err)
	}
	if err := resetAccountPassword(ctx, db, "ORG-000001", "meiman@example.test", "Test-Recovery-Password", "recovery-operation-2"); err == nil {
		t.Fatal("reset committed without audit")
	}
	if err := db.QueryRow(ctx, `SELECT password_hash,(SELECT count(*) FROM sessions WHERE user_id=1) FROM users WHERE id=1`).Scan(&password, &targetSessions); err != nil || password != "changed-after-reset" || targetSessions != 1 {
		t.Fatal("audit failure did not roll back password and session changes")
	}
	if _, err := db.Exec(ctx, `DROP TRIGGER reject_recovery_audit ON audit_logs`); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"active=false", "active=true,deleted_at=now()"} {
		if _, err := db.Exec(ctx, "UPDATE users SET "+state+" WHERE id=1"); err != nil {
			t.Fatal(err)
		}
		if err := resetAccountPassword(ctx, db, "ORG-000001", "meiman@example.test", "Test-Recovery-Password", "recovery-operation-2"); err == nil {
			t.Fatal("disabled account recovered")
		}
	}
}
