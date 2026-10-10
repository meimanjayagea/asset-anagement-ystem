# API contract v2

Base path `/api`. v2 introduces branch scopes and category maintenance policy. JSON request/response. POST wajib header `Content-Type: application/json`, `X-Requested-With: AssetFlow`. Jika Origin ada harus cocok exact `APP_ORIGIN`. Browser menggunakan session cookie; token tidak dikirim ke localStorage. Org tidak boleh dipilih ulang sesudah login; berasal dari DB user session.

Desain alur dan batas akuntansi 10 fitur dijelaskan di [FEATURE-FLOW-AND-DESIGN.md](FEATURE-FLOW-AND-DESIGN.md).

Errors: `{"error":"message"}`. HTTP 400 malformed JSON, 401 unauthenticated/session expired, 403 role/CSRF/self approval, 409 version/state/duplicate conflict, 415 content-type, 422 invalid reference/validation, 429 login throttled, 500 internal, 503 activity persistence unavailable. Pada timeout atau 503 activity audit, mutasi domain mungkin sudah commit; client harus refresh untuk memeriksa state sebelum retry. Belum ada generic idempotency-key middleware.

| Method | Endpoint | Role / response |
|---|---|---|
| GET | /health/live | Public, process live |
| GET | /health/ready | Public, DB ping |
| GET | /login/options | Public, organization codes and active branches |
| POST | /login | Public, authenticated user + cookie |
| POST | /logout | All authenticated, ok |
| GET | /me | All, id/org_id/name/email/role |
| POST | /password | All, verify old password, revoke sessions |
| GET | /dashboard | All, counts + purchase_value |
| GET | /assets?page=1&size=25&search=&status= | All, items/total/page/size |
| POST | /assets | Admin/manager/operator, created id |
| POST | /assets/{id}/action | Admin/manager/operator, assign/return |
| GET | /locations | All, array, max 1000 |
| POST | /locations | Admin/manager |
| GET | /categories | All, array, max 1000 |
| POST | /categories | Admin/manager |
| GET | /requests?page=1&size=25 | All, array |
| POST | /requests | Admin/manager/operator |
| POST | /requests/{id}/decision | Admin/manager, checker different |
| GET | /maintenance?page=1&size=25 | All, array |
| POST | /maintenance | Admin/manager/operator |
| POST | /maintenance/{id}/action | Admin/manager/operator |
| GET | /stocktakes?page=1&size=25 | All, array with counts |
| POST | /stocktakes | Admin/manager/operator |
| GET | /stocktakes/{id}/items?page=1&size=25 | All, expected/current version |
| POST | /stocktakes/{id}/observe | Admin/manager/operator |
| POST | /stocktakes/{id}/close | Admin/manager |
| POST | /exports/assets | All authenticated, server CSV current page + audit |
| GET | /assets/{id}/history | Role dengan `assets.history`, immutable movement timeline, detail disamarkan di luar cabang |
| GET | /contracts | Role dengan `contracts.read`, renewal status by branch |
| POST | /contracts | Role dengan `contracts.manage`, creates asset- or branch-level service contract |
| DELETE | /contracts/{id} | Role dengan `contracts.manage`, soft delete |
| GET | /finance/depreciation?period=YYYY-MM | Finance reader, period schedule and carrying values |
| GET/POST | /finance/valuations | Propose or list maker-checker revaluations |
| POST | /finance/valuations/{id}/decision | Valuation checker, approval/rejection with reason |
| GET/POST | /finance/settings | Read or update generic chart-of-account codes |
| GET | /finance/journals?period=YYYY-MM | Finance reader, generic double-entry preview |
| POST | /exports/journal | Finance role with report export, audited generic CSV |
| GET | /reports/compliance | Manager/auditor and report roles, non-financial compliance snapshot |
| GET | /branches | All, only accessible branches |
| POST | /branches | Admin company master |
| GET | /activity?page=1&size=25&event= | Admin/manager/auditor, scoped activity |
| POST | /categories/{id}/policy | Admin, versioned policy update |
| GET | /audit?page=1&size=25 | Admin/manager/auditor |
| GET | /users | Admin, max 1000 |
| POST | /users | Admin, created id |
| POST | /users/{id}/access | Admin, revoke sessions |

Pagination size clamps to 1–100 (default 25); page starts at 1. General arrays do not return total count; next page determined by returned length. Missing stocktake ID GET items currently returns empty array, write returns 404.

## Payload examples

Login:
```json
{"email":"admin@example.com","password":"your-unique-password","org_code":"ORG-000001","branch_id":1}
```

Kode organisasi diperoleh saat bootstrap dan dicantumkan pada log bootstrap. Cabang harus dipilih; login hanya berhasil jika akun memiliki akses ke cabang tersebut. Email yang tidak terdaftar pada cabang terpilih menerima HTTP 403.

Register:
```json
{"tag":"AST-000001","name":"Lenovo ThinkPad T14","serial_number":"SN-UNIQUE-001","category_id":1,"location_id":1,"purchase_date":"2026-10-01","purchase_cost":18000000,"salvage_value":1000000,"useful_life_months":48,"depreciation_method":"straight_line","depreciation_start_date":"2026-10-01","supplier_name":"Example supplier","acquisition_reference":"PO-2026-001","warranty_until":"2029-10-01"}
```

Assign / return:
```json
{"action":"assign","custodian":"EMP-1001 / Arya","version":1}
```
```json
{"action":"return","version":2}
```

Transfer / disposal (asset must be available):
```json
{"asset_id":1,"kind":"transfer","target_location_id":2,"reason":"Dipindahkan ke cabang Bandung","version":3}
```
```json
{"asset_id":1,"kind":"dispose","target_location_id":null,"reason":"Tidak ekonomis diperbaiki","disposal_proceeds":2500000,"version":4}
```

Decision:
```json
{"approve":true,"note":"Dokumen serah terima diverifikasi"}
```

Maintenance:
```json
{"asset_id":1,"title":"Preventive maintenance","due_date":"2026-11-01","version":5}
```
```json
{"action":"start","cost":0,"notes":"","version":5}
```
```json
{"action":"complete","cost":350000,"notes":"Komponen diganti dan diuji","version":6}
```

Stocktake:
```json
{"title":"Stocktake Q4 HQ","location_id":1}
```
```json
{"tag":"AST-000001","notes":"Ditemukan di ruang kerja"}
```
```json
{"acknowledge_discrepancies":true}
```

Master / user:
```json
{"name":"Bandung Warehouse","branch_id":2}
```
```json
{"name":"Vehicles","useful_life_months":96,"maintenance_interval_days":180,"maintenance_instructions":"Inspect engine and brakes"}
```
```json
{"name":"Checker","email":"checker@example.com","password":"strong-password-123","role":"manager","all_branches":false,"branch_ids":[1,2]}
```
```json
{"role":"auditor","active":false,"all_branches":false,"branch_ids":[2]}
```
```json
{"current_password":"old-password-123","new_password":"new-password-456"}
```

Create: HTTP 201 `{"id":123}`. Action: HTTP 200 `{"ok":true}`. IDs/version/nilai rupiah JSON number; nilai dibatasi agar representable dalam JavaScript safe integer. Tanggal bisnis string `YYYY-MM-DD`; timestamps RFC3339 dari PostgreSQL JSON encoder.

## curl smoke

```bash
curl -c /tmp/assetflow.cookies http://localhost:8088/api/login \
 -H 'Content-Type: application/json' -H 'X-Requested-With: AssetFlow' \
 -d '{"org_code":"ORG-000001","branch_id":1,"email":"admin@example.com","password":"your-password"}'
curl -b /tmp/assetflow.cookies http://localhost:8088/api/dashboard
rm -f /tmp/assetflow.cookies
```

Jangan memasukkan password produksi ke shell history; gunakan file chmod 600 sementara atau secret tooling.

## v2 payloads and scope

Create branch:
```json
{"code":"BDG-01","name":"Cabang Bandung","address":"Bandung"}
```

Update maintenance policy:
```json
{"maintenance_interval_days":90,"maintenance_instructions":"Inspect battery and cooling","version":1,"apply_to_existing":true}
```

`apply_to_existing=false`: existing due dates unchanged; new assets and future completed jobs use current policy. `true`: recompute eligible non-disposed assets using last completed maintenance date (fallback purchase_date), increment asset versions; skip pending request and scheduled/in_progress job assets. Response includes `assets_updated`.

GET dashboard/assets/locations/requests/maintenance/stocktakes/audit accepts `branch_id=0` (all authorized branches) or accessible positive ID. Foreign/inaccessible branch filter returns 403, malformed/negative returns 400. Activity also accepts branch filter: resource branch snapshots first; events without resource use actor membership snapshot. Company-wide login events remain company-level. Categories/users are company masters and do not narrow by branch filter.

User creation/access change requires `all_branches` and `branch_ids`. Admin is forced company-wide. Other users with `all_branches=false` must have at least one valid company branch. Access changes revoke all sessions. Asset registration chooses location; backend derives branch from its location (no caller-controlled branch override).

Activity event values: login_success, login_failed, login_throttled, logout, read, action, denied. Each has timestamp/status/request ID/peer IP/user agent/duration. Failed login `attempted_email` describes submitted identity, not an authenticated actor. Unknown company attempts are retained with null company and available only to infrastructure/DB audit operators, not a company's activity UI.

Audited CSV export:
```json
{"page":1,"size":25,"search":"","status":"","branch_id":2}
```
Response is `text/csv`, not JSON. Backend applies branch permissions, generates exact selected page, neutralizes spreadsheet formula prefixes and records export row count.
