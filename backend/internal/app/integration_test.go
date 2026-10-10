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
	migration, e = os.ReadFile("../../migrations/003_roles_scope_archive.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	migration, e = os.ReadFile("../../migrations/004_finance_lifecycle.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	migration, e = os.ReadFile("../../migrations/005_organization_codes.sql")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(ctx, string(migration)); e != nil {
		t.Fatal(e)
	}
	pw, _ := bcrypt.GenerateFromPassword([]byte("test-password-123"), bcrypt.MinCost)
	for _, q := range []string{`INSERT INTO organizations(name) VALUES('One'),('Two')`, `INSERT INTO branches(org_id,code,name) VALUES(1,'BDG','Bandung'),(1,'JKT','Jakarta'),(1,'HQ','Main'),(2,'HQ','Secret')`, `INSERT INTO locations(org_id,branch_id,name) VALUES(1,1,'Bandung'),(1,2,'Jakarta'),(1,3,'HQ'),(2,4,'Secret')`, `INSERT INTO categories(org_id,name,useful_life_months) VALUES(1,'IT',48),(2,'IT',48)`} {
		if _, e = db.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	for _, u := range []struct {
		org  int
		role string
	}{{1, "admin"}, {1, "manager"}, {1, "operator"}, {1, "auditor"}, {2, "admin"}, {1, "finance"}, {1, "it_support"}, {1, "it_developer"}} {
		_, e = db.Exec(ctx, `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES($1,$2,$3,$4,$2,$5)`, u.org, u.role, u.role+"@test.local", string(pw), u.role == "admin")
		if e != nil {
			t.Fatal(e)
		}
	}
	if _, e = db.Exec(ctx, `INSERT INTO user_branches(org_id,user_id,branch_id) SELECT u.org_id,u.id,b.id FROM users u CROSS JOIN branches b WHERE u.org_id=1 AND u.role<>'admin' AND b.org_id=1 AND b.code IN ('BDG','JKT')`); e != nil {
		t.Fatal(e)
	}
	var branchAdminID int64
	tx, e := db.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRow(ctx, `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Branch Admin','branch-admin@test.local',$1,'branch_admin',false) RETURNING id`, string(pw)).Scan(&branchAdminID); e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,$1,2)`, branchAdminID); e != nil {
		tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
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
		branchID := 1
		if org == 2 {
			branchID = 4
		} else if role == "branch-admin" {
			branchID = 2
		}
		w := call("POST", "/api/login", "", map[string]any{"email": role + "@test.local", "password": "test-password-123", "org_code": fmt.Sprintf("ORG-%06d", org), "branch_id": branchID})
		expect(t, w, 200)
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
			t.Fatal("cookie attributes missing")
		}
		return c.Name + "=" + c.Value
	}
	admin, manager, operator, auditor, other := cookie("admin", 1), cookie("manager", 1), cookie("operator", 1), cookie("auditor", 1), cookie("admin", 2)
	finance, branchAdmin, itSupport, itDeveloper := cookie("finance", 1), cookie("branch-admin", 1), cookie("it_support", 1), cookie("it_developer", 1)
	assetBody := func(tag string) map[string]any {
		return map[string]any{"tag": tag, "name": "Laptop", "serial_number": "", "category_id": 1, "location_id": 1, "purchase_date": "2026-01-01", "purchase_cost": 10000000, "salvage_value": 1000000, "useful_life_months": 48, "warranty_until": nil}
	}
	t.Run("login-options-return-organization-code", func(t *testing.T) {
		w := call("GET", "/api/login/options", "", nil)
		expect(t, w, 200)
		var options struct {
			Organizations []struct {
				Code     string `json:"code"`
				Branches []struct {
					Code string `json:"code"`
				} `json:"branches"`
			} `json:"organizations"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &options); e != nil {
			t.Fatal(e)
		}
		if len(options.Organizations) != 2 || options.Organizations[0].Code != "ORG-000001" || len(options.Organizations[0].Branches) != 3 {
			t.Fatalf("unexpected login options: %s", w.Body.String())
		}
	})
	t.Run("authorization-and-tenant", func(t *testing.T) {
		expect(t, call("GET", "/api/assets", "", nil), 401)
		expect(t, call("POST", "/api/assets", auditor, assetBody("DENIED")), 403)
		expect(t, call("GET", "/api/users", operator, nil), 403)
		expect(t, call("GET", "/api/audit", manager, nil), 403)
		expect(t, call("GET", "/api/activity", auditor, nil), 403)
		expect(t, call("GET", "/api/assets?branch_id=3", manager, nil), 403)
		expect(t, call("POST", "/api/branches", branchAdmin, map[string]any{"code": "OUT", "name": "Forbidden"}), 403)
		expect(t, call("POST", "/api/assets", itDeveloper, assetBody("ITDEV-DENIED")), 403)
		expect(t, call("POST", "/api/users", branchAdmin, map[string]any{"name": "Escalation", "email": "escalate@test.local", "password": "test-password-123", "role": "admin", "all_branches": true}), 403)
		expect(t, call("POST", "/api/assets", operator, assetBody("AST-1")), 201)
		expect(t, call("POST", "/api/assets", operator, assetBody("AST-1")), 409)
		b := assetBody("CROSS")
		b["location_id"] = 4
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
		expect(t, call("GET", "/api/audit", auditor, nil), 403)
		expect(t, call("GET", "/api/audit", branchAdmin, nil), 200)
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
		login := call("POST", "/api/login", "", map[string]any{"email": "branch-admin@test.local", "password": "test-password-123", "org_code": "ORG-000001", "branch_id": 2})
		expect(t, login, 200)
		var loginBody struct {
			ActiveBranch int64 `json:"active_branch_id"`
		}
		if e := json.Unmarshal(login.Body.Bytes(), &loginBody); e != nil || loginBody.ActiveBranch != 2 {
			t.Fatalf("branch admin login did not select its permitted branch: %+v (%v)", loginBody, e)
		}
		branchList := call("GET", "/api/branches", branchAdmin, nil)
		expect(t, branchList, 200)
		var allowedBranches []map[string]any
		if e := json.Unmarshal(branchList.Body.Bytes(), &allowedBranches); e != nil || len(allowedBranches) != 1 || allowedBranches[0]["id"] != float64(2) {
			t.Fatalf("branch admin received out-of-scope branches: %s", branchList.Body.String())
		}
		expect(t, call("GET", "/api/assets?branch_id=1", branchAdmin, nil), 403)
		expect(t, call("GET", "/api/audit?branch_id=1", branchAdmin, nil), 403)
		expect(t, call("GET", "/api/activity", branchAdmin, nil), 200)
		created := call("POST", "/api/users", branchAdmin, map[string]any{"name": "Branch Employee", "email": "branch-employee@test.local", "password": "test-password-123", "role": "employee", "all_branches": true, "branch_ids": []int64{1}})
		expect(t, created, 201)
		var createdUser map[string]int64
		if e := json.Unmarshal(created.Body.Bytes(), &createdUser); e != nil {
			t.Fatal(e)
		}
		var scopedBranches []int64
		if e := db.QueryRow(ctx, `SELECT ARRAY(SELECT branch_id FROM user_branches WHERE user_id=$1)`, createdUser["id"]).Scan(&scopedBranches); e != nil || len(scopedBranches) != 1 || scopedBranches[0] != 2 {
			t.Fatalf("branch admin assignment escaped its branch: %v (%v)", scopedBranches, e)
		}
		localCategory := call("POST", "/api/categories", branchAdmin, map[string]any{"name": "Jakarta Assets", "useful_life_months": 48, "maintenance_interval_days": 0, "maintenance_instructions": "", "branch_id": 1})
		expect(t, localCategory, 201)
		var localCategoryID int64
		if e := db.QueryRow(ctx, `SELECT id FROM categories WHERE org_id=1 AND name='Jakarta Assets' AND branch_id=2`).Scan(&localCategoryID); e != nil {
			t.Fatalf("branch admin category escaped scope or was not created locally: %v", e)
		}
		if _, e := db.Exec(ctx, `INSERT INTO categories(org_id,name,useful_life_months,branch_id) VALUES(1,'Bandung Assets',48,1)`); e != nil {
			t.Fatal(e)
		}
		branchCategories := call("GET", "/api/categories", branchAdmin, nil)
		expect(t, branchCategories, 200)
		if strings.Contains(branchCategories.Body.String(), "Bandung Assets") || !strings.Contains(branchCategories.Body.String(), "Jakarta Assets") {
			t.Fatalf("branch admin category list crossed branch scope: %s", branchCategories.Body.String())
		}
		expect(t, call("POST", fmt.Sprintf("/api/categories/%d/policy", localCategoryID), branchAdmin, map[string]any{"maintenance_interval_days": 30, "maintenance_instructions": "Inspect locally", "version": 1, "apply_to_existing": false}), 200)
		expect(t, call("DELETE", fmt.Sprintf("/api/categories/%d", localCategoryID), branchAdmin, nil), 200)
		expect(t, call("GET", "/api/categories?archived=true", branchAdmin, nil), 200)
		expect(t, call("POST", fmt.Sprintf("/api/categories/%d/restore", localCategoryID), branchAdmin, nil), 200)
		var branchAsset int64
		if e := db.QueryRow(ctx, `INSERT INTO assets(org_id,tag,name,category_id,location_id,purchase_date,purchase_cost,salvage_value,useful_life_months) VALUES(1,'BRANCH-1','Branch laptop',1,2,'2026-01-01',9900000,0,48) RETURNING id`).Scan(&branchAsset); e != nil {
			t.Fatal(e)
		}
		branchEdit := assetBody("BRANCH-1")
		branchEdit["name"] = "Branch-managed laptop"
		branchEdit["location_id"] = 2
		branchEdit["purchase_cost"] = int64(9900000)
		branchEdit["version"] = 1
		expect(t, call("POST", fmt.Sprintf("/api/assets/%d", branchAsset), branchAdmin, branchEdit), 200)
		var editedName string
		if e := db.QueryRow(ctx, `SELECT name FROM assets WHERE id=$1`, branchAsset).Scan(&editedName); e != nil || editedName != "Branch-managed laptop" {
			t.Fatalf("branch admin could not edit its asset: %q (%v)", editedName, e)
		}
		expect(t, call("POST", "/api/assets/1", branchAdmin, map[string]any{"tag": "CROSS-BRANCH", "name": "Forbidden", "category_id": 1, "location_id": 1, "purchase_date": "2026-01-01", "purchase_cost": 0, "salvage_value": 0, "useful_life_months": 48, "version": 1}), 404)
		branchAssets := call("GET", "/api/assets", branchAdmin, nil)
		expect(t, branchAssets, 200)
		if !strings.Contains(branchAssets.Body.String(), "BRANCH-1") || !strings.Contains(branchAssets.Body.String(), "9900000") {
			t.Fatalf("branch administrator cannot see its branch or financial data: %s", branchAssets.Body.String())
		}
		financeAssets := call("GET", "/api/assets", finance, nil)
		expect(t, financeAssets, 200)
		if !strings.Contains(financeAssets.Body.String(), "9900000") {
			t.Fatalf("finance role cannot see allowed financial data: %s", financeAssets.Body.String())
		}
		itAssets := call("GET", "/api/assets", itSupport, nil)
		expect(t, itAssets, 200)
		if strings.Contains(itAssets.Body.String(), "purchase_cost") || strings.Contains(itAssets.Body.String(), "9900000") {
			t.Fatalf("IT support received financial data: %s", itAssets.Body.String())
		}
		itDashboard := call("GET", "/api/dashboard", itSupport, nil)
		expect(t, itDashboard, 200)
		if strings.Contains(itDashboard.Body.String(), "purchase_value") {
			t.Fatalf("IT support received financial summary: %s", itDashboard.Body.String())
		}
		export := call("POST", "/api/exports/assets", itSupport, map[string]any{"page": 1, "size": 25, "branch_id": 0})
		expect(t, export, 200)
		if strings.Contains(export.Body.String(), "purchase_cost") || strings.Contains(export.Body.String(), "9900000") {
			t.Fatalf("non-finance export leaked costs: %s", export.Body.String())
		}
		developerAudit := call("GET", "/api/audit", itDeveloper, nil)
		expect(t, developerAudit, 403)
		_ = branchAsset

		var restrictedID int64
		e := db.QueryRow(ctx, `INSERT INTO users(org_id,name,email,password_hash,role,all_branches) VALUES(1,'Branch operator','scoped@test.local',$1,'operator',false) RETURNING id`, string(pw)).Scan(&restrictedID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(ctx, `INSERT INTO user_branches(org_id,user_id,branch_id) VALUES(1,$1,1)`, restrictedID); e != nil {
			t.Fatal(e)
		}
		scopedLogin := call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "test-password-123", "org_code": "ORG-000001", "branch_id": 1})
		expect(t, scopedLogin, 200)
		c := scopedLogin.Result().Cookies()[0]
		scoped := c.Name + "=" + c.Value
		rejectedBranch := call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "test-password-123", "org_code": "ORG-000001", "branch_id": 2})
		expect(t, rejectedBranch, 403)
		if !strings.Contains(rejectedBranch.Body.String(), "Email ini tidak terdaftar pada cabang yang dipilih") {
			t.Fatalf("wrong branch login did not explain the access denial: %s", rejectedBranch.Body.String())
		}
		expect(t, call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "test-password-123", "org_code": "ORG-000001"}), 422)
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
		editBody := assetBody("LOCAL-1")
		editBody["name"] = "Edited by operator"
		editBody["version"] = 1
		expect(t, call("POST", fmt.Sprintf("/api/assets/%d", localAsset), scoped, editBody), 200)
		var protectedCost int64
		if e := db.QueryRow(ctx, `SELECT purchase_cost FROM assets WHERE id=$1`, localAsset).Scan(&protectedCost); e != nil || protectedCost != 0 {
			t.Fatalf("non-finance role changed protected cost: %d (%v)", protectedCost, e)
		}
		expect(t, call("POST", "/api/requests", scoped, map[string]any{"asset_id": localAsset, "kind": "transfer", "target_location_id": 2, "reason": "Cross branch", "version": 1}), 403)
		body = assetBody("LOCAL-DENIED")
		body["location_id"] = 2
		expect(t, call("POST", "/api/assets", scoped, body), 403)
		expect(t, call("POST", "/api/login", "", map[string]any{"email": "scoped@test.local", "password": "wrong", "org_code": "ORG-000001", "branch_id": 1}), 401)
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
		expect(t, w, 403)
		w = call("POST", "/api/exports/assets", finance, map[string]any{"page": 1, "size": 25, "search": "", "status": "", "branch_id": 0})
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
	t.Run("soft-archive-and-restore", func(t *testing.T) {
		body := assetBody("ARCHIVE-1")
		expect(t, call("POST", "/api/assets", operator, body), 201)
		var assetID int64
		if e := db.QueryRow(ctx, `SELECT id FROM assets WHERE org_id=1 AND tag='ARCHIVE-1'`).Scan(&assetID); e != nil {
			t.Fatal(e)
		}
		expect(t, call("DELETE", fmt.Sprintf("/api/assets/%d", assetID), admin, nil), 200)
		var deleted bool
		if e := db.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM assets WHERE id=$1`, assetID).Scan(&deleted); e != nil || !deleted {
			t.Fatalf("asset was not soft archived: %v (%v)", deleted, e)
		}
		expect(t, call("POST", fmt.Sprintf("/api/assets/%d/restore", assetID), admin, nil), 200)
		if e := db.QueryRow(ctx, `SELECT deleted_at IS NULL FROM assets WHERE id=$1`, assetID).Scan(&deleted); e != nil || !deleted {
			t.Fatalf("asset restore failed: %v (%v)", deleted, e)
		}
	})
	t.Run("finance-lifecycle-and-compliance", func(t *testing.T) {
		period := time.Now().In(financeZone).Format("2006-01")
		periodKey := strings.ReplaceAll(period, "-", "")
		body := assetBody("FINANCE-TEST")
		expect(t, call("POST", "/api/assets", admin, body), 201)
		var assetID int64
		if e := db.QueryRow(ctx, `SELECT id FROM assets WHERE org_id=1 AND tag='FINANCE-TEST'`).Scan(&assetID); e != nil {
			t.Fatal(e)
		}

		compliance := call("GET", "/api/reports/compliance?branch_id=1", admin, nil)
		expect(t, compliance, 200)
		var complianceBody map[string]any
		if e := json.Unmarshal(compliance.Body.Bytes(), &complianceBody); e != nil || complianceBody["assets_active"] == nil {
			t.Fatalf("compliance report response invalid: %s (%v)", compliance.Body.String(), e)
		}
		expect(t, call("GET", "/api/reports/compliance?branch_id=1", auditor, nil), 200)
		expect(t, call("GET", "/api/finance/depreciation?period="+period+"&branch_id=1", manager, nil), 403)
		expect(t, call("GET", "/api/finance/depreciation?period="+period+"&branch_id=1", finance, nil), 200)

		proposal := map[string]any{"asset_id": assetID, "revalued_amount": 8_500_000, "remaining_life_months": 24, "reason": "Independent valuation"}
		expect(t, call("POST", "/api/finance/valuations", finance, proposal), 201)
		var valuationID int64
		if e := db.QueryRow(ctx, `SELECT id FROM asset_valuations WHERE org_id=1 AND asset_id=$1 AND status='pending'`, assetID).Scan(&valuationID); e != nil {
			t.Fatal(e)
		}
		decisionPath := fmt.Sprintf("/api/finance/valuations/%d/decision", valuationID)
		expect(t, call("POST", decisionPath, finance, map[string]any{"approve": true}), 403)
		expect(t, call("POST", decisionPath, admin, map[string]any{"approve": true}), 200)

		accounts := map[string]string{"asset_account": "1500", "accumulated_depreciation_account": "1590", "depreciation_expense_account": "6000", "cash_account": "1100", "disposal_gain_account": "7990", "disposal_loss_account": "6990", "revaluation_reserve_account": "3100", "impairment_expense_account": "6900"}
		expect(t, call("POST", "/api/finance/settings", finance, accounts), 200)
		expect(t, call("GET", "/api/finance/settings", finance, nil), 200)
		journals := call("GET", "/api/finance/journals?period="+period+"&branch_id=1", finance, nil)
		expect(t, journals, 200)
		if !strings.Contains(journals.Body.String(), "REVAL-FINANCE-TEST-"+periodKey) {
			t.Fatalf("approved valuation missing from journal preview: %s", journals.Body.String())
		}
		journalCSV := call("POST", "/api/exports/journal", finance, map[string]any{"period": period, "branch_id": 1})
		expect(t, journalCSV, 200)
		if !strings.HasPrefix(journalCSV.Header().Get("Content-Type"), "text/csv") || !strings.Contains(journalCSV.Body.String(), "REVAL-FINANCE-TEST-"+periodKey) {
			t.Fatalf("journal export invalid: %s", journalCSV.Body.String())
		}

		contract := map[string]any{"branch_id": 1, "asset_id": assetID, "name": "Presentation support", "vendor": "Demo Vendor", "contract_number": "SUP-001", "start_date": "2026-01-01", "end_date": "2027-01-01", "renewal_notice_days": 30, "annual_cost": 500_000}
		expect(t, call("POST", "/api/contracts", manager, contract), 201)
		managerContracts := call("GET", "/api/contracts?branch_id=1", manager, nil)
		expect(t, managerContracts, 200)
		if strings.Contains(managerContracts.Body.String(), "500000") {
			t.Fatalf("contract finance fields leaked to manager: %s", managerContracts.Body.String())
		}
		financeContracts := call("GET", "/api/contracts?branch_id=1", finance, nil)
		expect(t, financeContracts, 200)
	})
	t.Run("revoke-access-and-password", func(t *testing.T) {
		expect(t, call("POST", "/api/users/3/access", admin, map[string]any{"role": "operator", "active": false, "all_branches": false, "branch_ids": []int64{1}}), 200)
		expect(t, call("GET", "/api/me", operator, nil), 401)
		expect(t, call("POST", "/api/password", manager, map[string]any{"current_password": "test-password-123", "new_password": "changed-password-456"}), 200)
		expect(t, call("GET", "/api/me", manager, nil), 401)
	})
}
