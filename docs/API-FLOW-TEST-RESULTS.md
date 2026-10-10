# API and process verification - 2026-10-10

## Result

The modified local application passes the API/process tests below. The public
production deployment remains unhealthy: 22 of 24 unauthenticated GET probes
return HTTP 503 with an activity audit persistence error. Both health endpoints
return HTTP 200. These probes do not verify authenticated production workflows.
See `production-api-smoke.json` for status codes and request IDs.

## Local verification

| Area | Evidence | Result |
|---|---|---|
| Registered routes | All 60 HTTP routes, including two health endpoints, have successful response coverage against PostgreSQL 17.11 | PASS |
| Authentication boundary | Every protected route also rejects an unauthenticated request with 401 | PASS |
| Database runtime permissions | API tests run through a restricted NOLOGIN role using the actual `least-privilege.sql` grants; fixture setup uses the owner | PASS |
| Migration | Native PostgreSQL concurrent startup upgrade from v4, account/password preservation, and migration parity | PASS |
| Login | Organization code + employee ID + password, wrong password, wrong organization, branch selection, throttling | PASS |
| Session lifecycle | HttpOnly/SameSite cookie, logout, password change, access reassignment and revocation | PASS |
| Master data | Branch/location/category/user create, list, archive, restore; branch archive disables orphaned users | PASS |
| Assets | Registration, duplicate tag, update, assignment/return, archive/restore, history, protected finance fields | PASS |
| Approvals | Transfer approval, maker-checker restriction, rejection, repeated decision, pending-state conflicts, disposal completion | PASS |
| Concurrency | Two simultaneous assignment attempts yield exactly one success and one conflict | PASS |
| Maintenance | Schedule, start, complete, cancel, invalid transition, maintenance policy and next-due date | PASS |
| Stocktake | Snapshot/list/items, observe, unknown/duplicate tag rejection, missing-item acknowledgement, close and post-close rejection | PASS |
| Finance | Depreciation, valuation proposal/approval, accounting settings, journal preview/export, contracts and compliance report | PASS |
| Tenant/branch/role scope | Cross-organization/cross-branch denial, role escalation denial, confidential finance fields hidden | PASS |
| Audit and request protection | Domain/activity audit immutability, audited CSV, missing CSRF header, foreign Origin, injected audit INSERT denial | PASS |
| Readiness | Missing migration, incompatible audit column and missing runtime audit privileges return 503 | PASS |
| Browser contracts | 29 checks with mocked API fixtures, no JavaScript runtime errors | PASS |
| Login errors on desktop/mobile | Audit failure, non-JSON gateway failure, normal unauthenticated startup at 1440px and 390px | PASS |
| Live browser + real database | Invalid password, sign-in, session refresh, form registration, asset persistence after refresh, logout and 401 afterward; no API mocks | PASS |
| CLI bootstrap/startup | Fresh migration, first bootstrap success, second bootstrap refuses; AUTO_MIGRATE startup reaches HTTP listener | PASS |
| Go checks | Unit/integration tests and go vet, no skipped tests in the database-enabled run | PASS |
| Statement coverage | Backend overall 72.1%; app package 75.9%. Route coverage does not mean every internal branch is exercised | MEASURED |

The previously incomplete runtime grants were corrected: branches now allow
UPDATE, asset_movements allow SELECT/INSERT, and asset_valuations,
service_contracts and accounting_profiles allow SELECT/INSERT/UPDATE. Audit
tables still do not allow runtime UPDATE/DELETE. Regression coverage uses these
grants to catch permission gaps.

## Reproduction

Use an isolated database with a test owner that can create schemas and roles.
Never set TEST_DATABASE_URL to the production database.

```sh
cd backend
TEST_DATABASE_URL='postgres://test-owner:password@localhost:5432/test-db' go test -v ./...
go vet ./...
cd ../validation
npm ci
npm test
cd ../frontend
npm ci
npm run build
npx playwright install chromium
npm run test:login
npm run test:flows
```

For `npm run test:live`, start a bootstrapped local/staging backend on port
18088 with APP_ORIGIN=http://127.0.0.1:18089. Set TEST_LOGIN_PASSWORD for the
test account; optional TEST_ORG_CODE/TEST_EMPLOYEE_ID default to ORG-000001/EMP-1.
This test registers a uniquely tagged asset, so use a test database. It starts
and stops its own frontend proxy on port 18089. CHROMIUM_PATH can select an
installed browser. CI now includes a dedicated live-flow job.

## Limits

Production SQL errors and authenticated production operations could not be
inspected because project access was denied. The modified code and grants have
not been deployed or applied to production. Apply the migration/runtime grants,
deploy this revision, then repeat the production probes and authenticated UAT.

These tests do not establish performance capacity, backup restore readiness,
TLS deployment configuration, every validation edge case, or every mobile
workflow. Go race instrumentation was not run in this Windows setup; CI's
existing backend job runs the suite with race instrumentation.
