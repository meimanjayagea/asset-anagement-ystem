package app

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
)

func exerciseGeneralSetup(t *testing.T, call testCall, admin, branchAdmin, other string) {
	t.Helper()
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	create := func(path string, body any) int64 {
		t.Helper()
		w := call("POST", path, admin, body)
		check(w, 201)
		var result struct{ ID int64 }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.ID
	}
	list := func(path string) []map[string]any {
		t.Helper()
		w := call("GET", path, admin, nil)
		check(w, 200)
		var result []map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	find := func(path string, n int64) map[string]any {
		t.Helper()
		for _, r := range list(path) {
			if int64(r["id"].(float64)) == n {
				return r
			}
		}
		t.Fatalf("id %d not found", n)
		return nil
	}
	parent := create("/api/general-codes", map[string]any{"code": "COMPANY", "name": "Company code", "company_prefix": "MJ"})
	parentPath := fmt.Sprintf("/api/general-codes/%d", parent)
	masterBody := map[string]any{"code": "COMPANY", "name": "Company updated", "company_prefix": "MJ", "description": "Branch namespace", "version": 1}
	check(call("PUT", parentPath, admin, masterBody), 200)
	check(call("PUT", parentPath, admin, masterBody), 409)
	check(call("GET", "/api/general-codes", branchAdmin, nil), 403)
	check(call("PUT", parentPath, other, masterBody), 404)
	detailBody := map[string]any{"general_code_id": parent, "code": "BRANCH", "name": "Branches", "prefix": "CBG", "entity_type": "branch", "sequence_width": 6}
	detail := create("/api/general-code-details", detailBody)
	detailPath := fmt.Sprintf("/api/general-code-details/%d", detail)
	detailBody["version"] = 1
	detailBody["name"] = "Branch identifiers"
	check(call("PUT", detailPath, admin, detailBody), 200)
	check(call("PUT", detailPath, admin, detailBody), 409)
	check(call("PUT", detailPath, other, detailBody), 404)
	check(call("DELETE", parentPath, admin, nil), 409)
	check(call("POST", "/api/branches", admin, map[string]any{"name": "Wrong type", "code": "MANUAL", "general_code_detail_id": detail}), 422)
	check(call("POST", "/api/branches", other, map[string]any{"name": "Other org", "general_code_detail_id": detail}), 404)
	// Atomic allocation under two concurrent branch registrations.
	var wg sync.WaitGroup
	responses := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses <- call("POST", "/api/branches", admin, map[string]any{"name": fmt.Sprintf("Generated %d", i), "general_code_detail_id": detail})
		}(i)
	}
	wg.Wait()
	close(responses)
	var branchID int64
	for response := range responses {
		check(response, 201)
		var v struct{ ID int64 }
		_ = json.Unmarshal(response.Body.Bytes(), &v)
		branchID = v.ID
	}
	allocations := list("/api/general-codes/issued")
	if len(allocations) != 2 || allocations[0]["code"] == allocations[1]["code"] {
		t.Fatal("allocation not unique")
	}
	masterBody["version"] = 2
	masterBody["company_prefix"] = "NEW"
	check(call("PUT", parentPath, admin, masterBody), 409)
	current := find("/api/general-code-details", detail)
	detailBody["version"] = current["version"]
	detailBody["prefix"] = "NEW"
	check(call("PUT", detailPath, admin, detailBody), 409)
	check(call("DELETE", detailPath, admin, nil), 200)
	check(call("POST", "/api/branches", admin, map[string]any{"name": "Archived rule", "general_code_detail_id": detail}), 409)
	check(call("DELETE", parentPath, admin, nil), 200)
	check(call("GET", "/api/general-codes?archived=true", admin, nil), 200)
	check(call("GET", "/api/general-code-details?archived=true", admin, nil), 200)
	check(call("POST", detailPath+"/restore", admin, map[string]any{}), 409)
	check(call("POST", parentPath+"/restore", admin, map[string]any{}), 200)
	check(call("POST", detailPath+"/restore", admin, map[string]any{}), 200)
	branch := find("/api/branches", branchID)
	branchPath := fmt.Sprintf("/api/branches/%d", branchID)
	body := map[string]any{"name": "Edited generated branch", "address": "New address", "version": branch["version"]}
	check(call("PUT", branchPath, admin, body), 200)
	check(call("PUT", branchPath, admin, body), 409)
	check(call("PUT", branchPath, other, body), 404)
	if find("/api/branches", branchID)["business_code"] != branch["business_code"] {
		t.Fatal("code changed on name edit")
	}
	hqDetail := create("/api/general-code-details", map[string]any{"general_code_id": parent, "code": "HEAD_OFFICE", "name": "Head office", "prefix": "PST", "entity_type": "hq", "sequence_width": 6})
	check(call("POST", "/api/branches", admin, map[string]any{"name": "Wrong namespace", "general_code_detail_id": hqDetail}), 422)
	check(call("DELETE", branchPath, admin, nil), 200)
	nextBranch := create("/api/branches", map[string]any{"name": "Next after archive", "general_code_detail_id": detail})
	if find("/api/branches", nextBranch)["code"] != "MJ-CBG-000003" {
		t.Fatal("archived number reused")
	}
	check(call("POST", branchPath+"/restore", admin, map[string]any{}), 200)
	var hq map[string]any
	for _, row := range list("/api/branches") {
		if row["code"] == "HQ" {
			hq = row
		}
	}
	check(call("PUT", fmt.Sprintf("/api/branches/%d", int64(hq["id"].(float64))), admin, map[string]any{"name": hq["name"], "version": hq["version"], "general_code_detail_id": hqDetail}), 200)
	location := create("/api/locations", map[string]any{"name": "Editable location", "branch_id": branchID})
	locationPath := fmt.Sprintf("/api/locations/%d", location)
	check(call("PUT", locationPath, admin, map[string]any{"name": "Location renamed", "building": "A", "floor": "2", "room": "201", "version": 1}), 200)
	check(call("PUT", locationPath, other, map[string]any{"name": "Location hacked", "version": 2}), 404)
	category := create("/api/categories", map[string]any{"name": "Editable category", "branch_id": branchID, "useful_life_months": 36})
	check(call("PUT", fmt.Sprintf("/api/categories/%d", category), admin, map[string]any{"name": "Category renamed", "useful_life_months": 48, "version": 1}), 200)
	user := create("/api/users", map[string]any{"name": "Editable user", "email": "editable@example.test", "password": "test-password-123", "role": "employee", "branch_ids": []int64{branchID}})
	userPath := fmt.Sprintf("/api/users/%d", user)
	profile := find("/api/users", user)
	check(call("PUT", userPath, admin, map[string]any{"name": "User renamed", "email": "renamed@example.test", "version": profile["version"]}), 200)
	check(call("PUT", userPath, other, map[string]any{"name": "User hacked", "email": "hacker@example.test", "version": 2}), 404)
	if find("/api/users", user)["employee_id"] != profile["employee_id"] {
		t.Fatal("employee login id changed")
	}
	check(call("POST", "/api/demo-data", branchAdmin, map[string]any{"confirmation": "SEED_DEMO_KEEP_EXISTING"}), 403)
	check(call("POST", "/api/demo-data", admin, map[string]any{}), 422)
	seeded := call("POST", "/api/demo-data", admin, map[string]any{"confirmation": "SEED_DEMO_KEEP_EXISTING"})
	check(seeded, 200)
	var manifest map[string]any
	_ = json.Unmarshal(seeded.Body.Bytes(), &manifest)
	if manifest["assets"] != float64(26) {
		t.Fatal("missing sample cases")
	}
	replay := call("POST", "/api/demo-data", admin, map[string]any{"confirmation": "SEED_DEMO_KEEP_EXISTING"})
	check(replay, 200)
	var again map[string]any
	_ = json.Unmarshal(replay.Body.Bytes(), &again)
	if again["already_seeded"] != true || again["branch_id"] != manifest["branch_id"] {
		t.Fatal("seed not idempotent")
	}
	check(call("GET", "/api/loans?branch_id=0", admin, nil), 200)
	check(call("GET", "/api/maintenance?branch_id=0", admin, nil), 200)
	check(call("GET", "/api/stocktakes?branch_id=0", admin, nil), 200)
}
