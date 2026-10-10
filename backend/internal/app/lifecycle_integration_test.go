package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"testing"
	"time"
)

type testCall func(string, string, string, any) *httptest.ResponseRecorder

func uploadProof(t *testing.T, call testCall, cookie string, asset int64, purpose string) int64 {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{30, 180, 110, 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	w := call("POST", fmt.Sprintf("/api/assets/%d/photos", asset), cookie, map[string]any{"purpose": purpose, "caption": "Condition evidence", "data": base64.StdEncoding.EncodeToString(buffer.Bytes())})
	if w.Code != 201 {
		t.Fatalf("upload %d: %s", w.Code, w.Body.String())
	}
	var result struct{ ID int64 }
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.ID
}

func exerciseLifecycle(t *testing.T, call testCall, admin, operator, auditor, otherOrg string) {
	t.Helper()
	check := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("got %d want %d: %s", w.Code, status, w.Body.String())
		}
	}
	readID := func(w *httptest.ResponseRecorder) int64 {
		t.Helper()
		check(w, 201)
		var result struct{ ID int64 }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.ID
	}
	asset := readID(call("POST", "/api/assets", admin, map[string]any{"tag": "FIELD-LIFECYCLE", "name": "Field laptop", "serial_number": "FIELD-001", "category_id": 1, "location_id": 1, "purchase_date": "2026-01-01", "purchase_cost": 12000000, "salvage_value": 1000000, "useful_life_months": 48}))
	path := fmt.Sprintf("/api/assets/%d", asset)
	check(call("POST", path+"/catalog", admin, map[string]any{"brand": "Lenovo", "model": "ThinkPad", "specifications": "16 GB RAM", "rfid_tag": "TAG-RFID-001", "version": 1}), 200)
	check(call("GET", path+"/catalog", operator, nil), 200)
	check(call("GET", "/api/assets/lookup?code=TAG-RFID-001", operator, nil), 200)
	check(call("GET", "/api/assets/lookup?code=TAG-RFID-001", otherOrg, nil), 404)
	cover := uploadProof(t, call, admin, asset, "catalog")
	check(call("GET", path+"/photos", operator, nil), 200)
	imageResponse := call("GET", fmt.Sprintf("/api/photos/%d", cover), operator, nil)
	check(imageResponse, 200)
	if imageResponse.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("untrusted image output")
	}
	check(call("GET", fmt.Sprintf("/api/photos/%d", cover), otherOrg, nil), 404)
	check(call("POST", path+"/photos", auditor, map[string]any{"purpose": "catalog", "data": "bad"}), 403)
	check(call("POST", path+"/photos", admin, map[string]any{"purpose": "catalog", "data": "bad"}), 422)
	check(call("POST", "/api/locations/1/details", admin, map[string]any{"building": "Tower A", "floor": "2", "room": "201"}), 200)
	loan := readID(call("POST", "/api/loans", operator, map[string]any{"asset_id": asset, "version": 2, "due_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339), "reason": "Field inspection"}))
	loanPath := fmt.Sprintf("/api/loans/%d/action", loan)
	check(call("POST", "/api/loans", operator, map[string]any{"asset_id": asset, "version": 2, "due_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339), "reason": "Duplicate loan"}), 409)
	check(call("POST", loanPath, admin, map[string]any{"action": "checkout", "version": 2, "notes": "Good condition"}), 422)
	before := uploadProof(t, call, admin, asset, "checkout")
	check(call("POST", loanPath, admin, map[string]any{"action": "checkout", "version": 2, "notes": "Good condition", "photo_ids": []int64{before}}), 200)
	check(call("POST", path+"/action", admin, map[string]any{"action": "return", "version": 3}), 409)
	check(call("DELETE", path, admin, nil), 409)
	check(call("GET", "/api/loans", operator, nil), 200)
	notifications := call("GET", "/api/notifications", operator, nil)
	check(notifications, 200)
	var notices []map[string]any
	if err := json.Unmarshal(notifications.Body.Bytes(), &notices); err != nil {
		t.Fatal(err)
	}
	var key string
	for _, notice := range notices {
		if notice["kind"] == "loan" {
			key = notice["key"].(string)
		}
	}
	if key == "" {
		t.Fatal("due reminder missing")
	}
	check(call("POST", "/api/notifications/read", operator, map[string]any{"key": key}), 200)
	after := uploadProof(t, call, admin, asset, "return")
	check(call("POST", loanPath, admin, map[string]any{"action": "return", "version": 3, "notes": "Screen damaged", "condition": "damaged", "photo_ids": []int64{after}}), 200)
	check(call("POST", loanPath, admin, map[string]any{"action": "return", "version": 4, "notes": "Duplicate return", "condition": "good", "photo_ids": []int64{after}}), 409)
	damage := uploadProof(t, call, operator, asset, "damage")
	checklist := []map[string]any{{"label": "Replace screen", "done": false}}
	job := readID(call("POST", "/api/repairs", operator, map[string]any{"asset_id": asset, "version": 4, "title": "Repair damaged screen", "due_date": time.Now().Format("2006-01-02"), "checklist": checklist, "photo_ids": []int64{damage}}))
	jobPath := fmt.Sprintf("/api/maintenance/%d/action", job)
	check(call("POST", jobPath, admin, map[string]any{"action": "start", "version": 4}), 200)
	repaired := uploadProof(t, call, admin, asset, "repair")
	check(call("POST", jobPath, admin, map[string]any{"action": "complete", "version": 5, "cost": 200000, "notes": "Screen replaced", "photo_ids": []int64{repaired}, "checklist": checklist}), 422)
	checklist[0]["done"] = true
	check(call("POST", jobPath, admin, map[string]any{"action": "complete", "version": 5, "cost": 200000, "notes": "Screen replaced", "photo_ids": []int64{repaired}, "checklist": checklist, "parts": []map[string]any{{"name": "Screen", "quantity": 1, "cost": 150000}}}), 200)
	check(call("GET", "/api/reports/lifecycle", admin, nil), 200)
	check(call("GET", "/api/maintenance/planned", admin, nil), 200)
	check(call("GET", "/api/maintenance/assignees", admin, nil), 200)
	report := call("GET", "/api/reports/lifecycle", auditor, nil)
	check(report, 200)
	if bytes.Contains(report.Body.Bytes(), []byte("\"purchase_cost\":12000000")) || bytes.Contains(report.Body.Bytes(), []byte("\"maintenance_cost\":200000")) {
		t.Fatal("financial information leaked")
	}
}
