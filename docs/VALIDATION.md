# Validation report — 7 October 2026

This report distinguishes source verification from server acceptance. No production deployment has been performed.

| Check | Actual result | Scope |
|---|---|---|
| Go 1.26.8 build (`-buildvcs=false`) | PASS | Executable produced locally |
| `go test -race ./...` | PASS | Domain transitions, money/date logic, strict JSON, origin validation, CSV formula escaping, embedded v1/v2 migration parity |
| `go vet ./...` | PASS | Static checks |
| Native PostgreSQL integration suite | NOT VERIFIED locally | Test source and GitHub Actions PostgreSQL 17 job supplied. Native PostgreSQL/Docker unavailable in current environment. `go test` skips integration without TEST_DATABASE_URL. |
| SQL migration application | PASS | PGlite PostgreSQL WASM engine; v1 → v2 upgrade with legacy fixture preservation |
| SQL conformance checks | PASS, 30 checks | HQ backfill, branch access/constraints, exact API query conformance, category maintenance dates, append-only activity, scoped CSV |
| Vue TypeScript + Vite production build | PASS | Static bundle generated |
| Browser contract tests | PASS, 20 checks | Desktop/mobile, branch filter, scope preservation, category policy, audited CSV, stale-response navigation, role visibility, no JS runtime errors |
| Browser API fixtures | Mocked | Screenshots show demo records; not proof of live Go API integration |
| npm audit | 0 vulnerabilities | Lockfile at verification time, including dev dependencies |
| govulncheck | 0 affected symbols/imported packages | One module-level advisory GO-2026-5932 for unused x/crypto/openpgp; app imports bcrypt, not openpgp |
| Compose YAML/topology | PASS static | Five services, no API/DB host ports, read-only API, matching migration copies |
| Backup script shell syntax | PASS | `sh -n`; restore not executed |
| Docker image build / Compose up | NOT EXECUTED | Must run on Docker-enabled staging/server |
| Full accessibility audit | NOT EXECUTED | Dialog includes focus trap/Escape and restored focus; full audit still required |
| Load/HA/PITR/restore tests | NOT EXECUTED | Operations acceptance gates |

A temporary PGlite TCP adapter was also attempted for HTTP integration, but its PostgreSQL wire/error/transaction behavior was insufficient. That attempt is **not** counted as a native integration pass. No adapter workaround was added to application code. Native integration tests must run in the supplied CI/staging PostgreSQL environment before release.

## Repeat browser tests

```bash
cd frontend
npm ci
npx playwright install chromium
npm run build
# Serve dist on 127.0.0.1:5173 in another terminal, e.g.:
python3 -m http.server 5173 --bind 127.0.0.1 --directory dist
# Then:
npm run test:browser
```

Optional `TEST_BASE_URL` overrides URL; `CHROMIUM_PATH` selects an installed browser executable. Browser tests intercept `/api` with explicit fixtures, do not send writes to real backend, and regenerate `docs/*-demo.png`.

## Go integration tests

Use a dedicated disposable database. Tests create a unique schema and drop it afterward. Environment permissions must allow creating schemas. The suite covers session login, tenant/role/branch denials, cross-branch transfer restrictions, user scope reassignment, unique/reference checks, maker–checker, state/version conflicts, two concurrent assignment requests, maintenance policy, stocktake, domain/activity audit immutability, audited CSV, CSRF, disable/revoke and password changes.

```bash
cd backend
TEST_DATABASE_URL='postgres://postgres:test@localhost:5432/assetflow?sslmode=disable' \
  go test -race -run TestAPIIntegration -v ./internal/app
```

Read docs/UAT.md and docs/PRODUCTION.md for unfulfilled acceptance requirements. Core implementation readiness must not be confused with enterprise production certification.

## Portable SQL upgrade verification

```bash
cd validation
npm ci
npm test
```

The 30 checks run the supplied migration and exact SQL strings extracted from Go source in PGlite. This complements the native PostgreSQL HTTP suite; it does not verify the PostgreSQL wire protocol, concurrent transactions, or container operation.
