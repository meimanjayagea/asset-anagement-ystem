package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Uses a disposable schema. Never point TEST_DATABASE_URL at a production database.
func TestAPIIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	base, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer base.Close()
	schema := fmt.Sprintf("test_assetflow_%d", time.Now().UnixNano())
	if _, e = base.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	migration, e := os.ReadFile("../../migrations/001_init.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	migration, e = os.ReadFile("../../migrations/002_branches_activity.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	pw, _ := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	for _, q := range []string{`INSERT INTO organizations(name) VALUES('One'),('Two')`, `INSERT INTO branches(org_id,code,name) VALUES(1,'HQ','Main'),(1,'BDG','Bandung'),(2,'HQ','Secret')`, `INSERT INTO locations(org_id,branch_id,name) VALUES(1,1,'HQ'),(1,2,'Branch'),(2,3,'Secret')`, `INSERT INTO categories(org_id,name,useful_life_months) VALUES(1,'IT',48),(2,'IT',48)`} {
		if _, e = db.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []struct {
		org  int
		role string
	}{{1, "admin"}, {1, "manager"}, {1, "operator"}, {1, "auditor"}, {2, "admin"}} {
		_, e = db.Exec(ctx, `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES($1,$2,$3,$4,$2,true)`, u.org, u.role, u.role+"@test.local", string(pw))
		if e != nil {
			t.Fatal(e)
		}
	}
	h := (&Server{DB: db, Origin: "http://localhost", Secure: false}).Routes()
	call := func(method, path, cookie string, body any) *httptest.ResponseRecorder {
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Requested-With", "AssetFlow")
		r.Header.Set("Origin", "http://localhost")
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(t *testing.T, w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	cookie := func(role string, org int) string {
		w := call("POST", "/api/login", "", map[string]any{"email": role + "@test.local", "password": "test-password-123", "org_id": org})
		expect(t, w, 200)
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
			t.Fatal("cookie attributes missing")
		}
		return c.Name + "=" + c.Value
	}
	admin, manager, operator, auditor, other := cookie("admin", 1), cookie("manager", 1), cookie("operator", 1), cookie("auditor", 1), cookie("admin", 2)
	assetBody := func(tag string) map[string]any {
		return map[string]any{"tag": tag, "name": "Laptop", "serial_number": "", "category_id": 1, "location_id": 1, "purchase_date": "2026-01-01", "purchase_cost": 10000000, "salvage_value": 1000000, "useful_life_months": 48, "warranty_until": nil}
	}
	t.Run("authorization-and-tenant", func(t *testing.T) {
		expect(t, call("GET", "/api/assets", "", nil), 401)
		expect(t, call("POST", "/api/assets", auditor, assetBody("DENIED")), 403)
		expect(t, call("GET", "/api/users", operator, nil), 403)
		expect(t, call("POST", "/api/assets", operator, assetBody("AST-1")), 201)
		expect(t, call("POST", "/api/assets", operator, assetBody("AST-1")), 409)
		b := assetBody("CROSS")
		b["location_id"] = 3
		expect(t, call("POST", "/api/assets", operator, b), 403)
		w := call("GET", "/api/assets", other, nil)
		expect(t, w, 200)
		if strings.Contains(w.Body.String(), "AST-1") {
			t.Fatal("tenant leaked")
		}
		expect(t, call("POST", "/api/assets/1/action", other, map[string]any{"action": "assign", "custodian": "Other", "version": 1}), 404)
	})
	t.Run("approval-maker-checker-and-conflicts", func(t *testing.T) {
		req := map[string]any{"asset_id": 1, "kind": "transfer", "target_location_id": 2, "reason": "Move to branch", "version": 1}
		expect(t, call("POST", "/api/requests", operator, req), 201)
		expect(t, call("POST", "/api/requests", operator, req), 409)
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "assign", "custodian": "Arya", "version": 1}), 409)
		expect(t, call("POST", "/api/requests/1/decision", operator, map[string]any{"approve": true, "note": ""}), 403)
		expect(t, call("POST", "/api/requests/1/decision", manager, map[string]any{"approve": true, "note": "Approved"}), 200)
		expect(t, call("POST", "/api/requests/1/decision", manager, map[string]any{"approve": true, "note": ""}), 409)
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "assign", "custodian": "Arya", "version": 1}), 409)
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "assign", "custodian": "Arya", "version": 2}), 200)
		expect(t, call("POST", "/api/requests", operator, map[string]any{"asset_id": 1, "kind": "dispose", "target_location_id": nil, "reason": "Retire", "version": 3}), 409)
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "return", "version": 3}), 200)
		expect(t, call("POST", "/api/requests", admin, map[string]any{"asset_id": 1, "kind": "dispose", "target_location_id": nil, "reason": "Retire", "version": 4}), 201)
		// Identity sequences may have gaps after unique violation. Resolve newest pending request.
		var requestID int64
		db.QueryRow(ctx, `SELECT id FROM requests WHERE asset_id=1 AND status='pending'`).Scan(&requestID)
		expect(t, call("POST", fmt.Sprintf("/api/requests/%d/decision", requestID), admin, map[string]any{"approve": true, "note": ""}), 403)
		expect(t, call("POST", fmt.Sprintf("/api/requests/%d/decision", requestID), manager, map[string]any{"approve": false, "note": "Still usable"}), 200)
	})
	t.Run("maintenance", func(t *testing.T) {
		expect(t, call("POST", "/api/maintenance", operator, map[string]any{"asset_id": 1, "title": "Annual service", "due_date": "2026-10-01", "version": 4}), 201)
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "assign", "custodian": "Arya", "version": 4}), 409)
		expect(t, call("POST", "/api/maintenance/1/action", operator, map[string]any{"action": "complete", "cost": 1, "notes": "", "version": 4}), 409)
		expect(t, call("POST", "/api/maintenance/1/action", operator, map[string]any{"action": "start", "cost": 0, "notes": "", "version": 4}), 200)
		expect(t, call("POST", "/api/maintenance/1/action", operator, map[string]any{"action": "complete", "cost": 250000, "notes": "Service done", "version": 5}), 200)
	})
	t.Run("concurrent-assignment", func(t *testing.T) {
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- call("POST", "/api/assets/1/action", operator, map[string]any{"action": "assign", "custodian": "Arya", "version": 6}).Code
			}()
		}
		wg.Wait()
		close(codes)
		ok, conflict := 0, 0
		for c := range codes {
			if c == 200 {
				ok++
			}
			if c == 409 {
				conflict++
			}
		}
		if ok != 1 || conflict != 1 {
			t.Fatalf("ok=%d conflicts=%d", ok, conflict)
		}
		expect(t, call("POST", "/api/assets/1/action", operator, map[string]any{"action": "return", "version": 7}), 200)
	})
	t.Run("stocktake-snapshot", func(t *testing.T) {
		expect(t, call("POST", "/api/stocktakes", operator, map[string]any{"title": "Branch audit", "location_id": 2}), 201)
		expect(t, call("POST", "/api/stocktakes/1/close", manager, map[string]any{"acknowledge_discrepancies": false}), 409)
		expect(t, call("POST", "/api/stocktakes/1/observe", operator, map[string]any{"tag": "AST-1", "notes": "Present"}), 200)
		expect(t, call("POST", "/api/stocktakes/1/observe", operator, map[string]any{"tag": "AST-1", "notes": ""}), 409)
		expect(t, call("POST", "/api/stocktakes/1/observe", operator, map[string]any{"tag": "UNKNOWN", "notes": ""}), 422)
		expect(t, call("POST", "/api/stocktakes/1/close", manager, map[string]any{"acknowledge_discrepancies": false}), 200)
		expect(t, call("POST", "/api/stocktakes/1/observe", operator, map[string]any{"tag": "AST-1", "notes": ""}), 409)
	})
	t.Run("audit-immutability", func(t *testing.T) {
		expect(t, call("GET", "/api/audit", auditor, nil), 200)
		expect(t, call("GET", "/api/audit", operator, nil), 403)
		if _, e := db.Exec(ctx, `DELETE FROM audit_logs`); e == nil {
			t.Fatal("audit deletion allowed")
		}
	})
	t.Run("csrf", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/api/logout", strings.NewReader("{}"))
		r.Header.Set("Cookie", admin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		expect(t, w, 403)
	})

	t.Run("branch-scopes-and-activity", func(t *testing.T) {
		var restrictedID int64
		e := db.QueryRow(ctx, `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Branch operator','scoped@test.local',$1,'operator',false) RETURNING id`, string(pw)).Scan(&restrictedID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,$1,1)`, restrictedID); e != nil {
			t.Fatal(e)
		}
		login := call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "test-password-123", "org_id": 1})
		expect(t, login, 200)
		c := login.Result().Cookies()[0]
		scoped := c.Name + "=" + c.Value
		expect(t, call("GET", "/api/assets?branch_id=2", scoped, nil), 403)
		w := call("GET", "/api/assets", scoped, nil)
		expect(t, w, 200)
		if strings.Contains(w.Body.String(), "AST-1") {
			t.Fatal("branch leak after asset transfer")
		}
		expect(t, call("POST", "/api/assets/1/action", scoped, map[string]any{"action": "assign", "custodian": "Branch user", "version": 8}), 404)
		expect(t, call("POST", "/api/stocktakes/1/observe", scoped, map[string]any{"tag": "AST-1", "notes": ""}), 404)
		body := assetBody("LOCAL-1")
		expect(t, call("POST", "/api/assets", scoped, body), 201)
		var localAsset int64
		db.QueryRow(ctx, `SELECT id FROM assets WHERE tag='LOCAL-1' AND org_id=1`).Scan(&localAsset)
		expect(t, call("POST", "/api/requests", scoped, map[string]any{"asset_id": localAsset, "kind": "transfer", "target_location_id": 2, "reason": "Cross branch", "version": 1}), 403)
		body = assetBody("LOCAL-DENIED")
		body["location_id"] = 2
		expect(t, call("POST", "/api/assets", scoped, body), 403)
		expect(t, call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "wrong", "org_id": 1}), 401)
		expect(t, call("GET", "/api/activity", admin, nil), 200)
		var logins, failed, denied, reads int
		e = db.QueryRow(ctx, `SELECT count(*) FILTER(WHERE event='login_success'),count(*) FILTER(WHERE event='login_failed'),count(*) FILTER(WHERE event='denied'),count(*) FILTER(WHERE event='read') FROM user_activity_logs WHERE org_id=1`).Scan(&logins, &failed, &denied, &reads)
		if e != nil {
			t.Fatal(e)
		}
		if logins < 1 || failed < 1 || denied < 1 || reads < 1 {
			t.Fatalf("activity missing %d/%d/%d/%d", logins, failed, denied, reads)
		}
		if _, e = db.Exec(ctx, `DELETE FROM user_activity_logs`); e == nil {
			t.Fatal("activity mutable")
		}
		expect(t, call("POST", fmt.Sprintf("/api/users/%d/access", restrictedID), admin, map[string]any{"role": "operator", "active": true, "all_branches": false, "branch_ids": []int64{2}}), 200)
		expect(t, call("GET", "/api/me", scoped, nil), 401)
	})
	t.Run("category-maintenance-policy", func(t *testing.T) {
		expect(t, call("POST", "/api/categories/1/policy", admin, map[string]any{"maintenance_interval_days": 90, "maintenance_instructions": "Inspect battery", "version": 1, "apply_to_existing": true}), 200)
		expect(t, call("POST", "/api/categories/1/policy", admin, map[string]any{"maintenance_interval_days": 30, "maintenance_instructions": "", "version": 1, "apply_to_existing": false}), 409)
		expect(t, call("POST", "/api/categories/1/policy", manager, map[string]any{"maintenance_interval_days": 30, "maintenance_instructions": "", "version": 2, "apply_to_existing": false}), 403)
		body := assetBody("POLICY-1")
		expect(t, call("POST", "/api/assets", operator, body), 201)
		var next string
		e := db.QueryRow(ctx, `SELECT next_maintenance_date::text FROM assets WHERE org_id=1 AND tag='POLICY-1'`).Scan(&next)
		if e != nil || next != "2026-04-01" {
			t.Fatalf("policy date %s error %v", next, e)
		}
	})

	t.Run("audited-export", func(t *testing.T) {
		w := call("POST", "/api/exports/assets", auditor, map[string]any{"page": 1, "size": 25, "search": "", "status": "", "branch_id": 0})
		expect(t, w, 200)
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
			t.Fatal("not CSV")
		}
		var count int
		db.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='export_csv' AND org_id=1`).Scan(&count)
		if count < 1 {
			t.Fatal("export not audited")
		}
	})
	t.Run("revoke-access-and-password", func(t *testing.T) {
		expect(t, call("POST", "/api/users/3/access", admin, map[string]any{"role": "operator", "active": false, "all_branches": true, "branch_ids": []int64{}}), 200)
		expect(t, call("GET", "/api/me", operator, nil), 401)
		expect(t, call("POST", "/api/password", manager, map[string]any{"current_password": "test-password-123", "new_password": "changed-password-456"}), 200)
		expect(t, call("GET", "/api/me", manager, nil), 401)
	})
}
