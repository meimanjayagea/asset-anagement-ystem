# Operations and production acceptance

## Deployment boundary

Compose is a single-server starting topology. DB primary, API, web share internal Docker network; only web is published on loopback. For production HTTPS use host reverse proxy or managed ingress. DB/API do not need exposed host ports. Use volume snapshot/persistent disk outside ephemeral container filesystem.

Example host nginx TLS virtual host (certificates provisioned separately):

```nginx
limit_req_zone $binary_remote_addr zone=asset_login:10m rate=6r/m;
server {
    listen 443 ssl;
    server_name assets.example.com;
    ssl_certificate /etc/letsencrypt/live/assets.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/assets.example.com/privkey.pem;
    add_header Strict-Transport-Security "max-age=31536000" always;
    client_max_body_size 1m;
    location = /api/login {
        limit_req zone=asset_login burst=5 nodelay;
        proxy_pass http://127.0.0.1:8088;
        proxy_set_header Host $host;
        proxy_set_header Origin $http_origin;
    }
    location / {
        proxy_pass http://127.0.0.1:8088;
        proxy_set_header Host $host;
        proxy_set_header Origin $http_origin;
        proxy_read_timeout 30s;
    }
}
```

Also redirect HTTP to HTTPS. Cookie secure + exact APP_ORIGIN must agree with public origin. Strict HSTS subdomain policy should follow domain ownership; not enabled blindly.

## Database permissions

Local Compose initially uses one bootstrap DB role for ease of setup. This role is an owner/superuser and must be separated at go-live. Run migration/bootstrap with owner credentials, then provision an `assetflow_runtime` login using `docs/least-privilege.sql` and set only API DATABASE_URL to that role. Owner credentials must not be reachable by runtime container.

Use `compose.production.yml.example` as an override: copy to compose.production.yml, set `RUNTIME_DATABASE_URL`, `APP_ORIGIN`, `COOKIE_SECURE=true`. Run `docker compose -f compose.yml -f compose.production.yml up -d --build`. The migrate job keeps owner DATABASE_URL; API override uses runtime role. DB SSL mode disable in initial Compose applies only to same-host network. Remote/managed database should use verify-full + trusted CA and private routing.

## Rollout procedure

1. Back up DB and verify artifact checksum; confirm migrations and rollback plan.
2. Build/test pinned source commit; scan image and dependencies, generate SBOM.
3. Deploy staging with production-like schema/roles and sanitized data.
4. Run UAT and integration tests; confirm role denial paths and cross-org isolation.
5. For initial release: build → migration → bootstrap → create checker → smoke dashboard and approval flow.
6. For subsequent release: backward-compatible migration first; update API and web; monitor error rate.
7. Never blindly re-run bootstrap; it refuses non-empty organization DB.
8. Rollback application to previous image only if schema backward compatibility permits. Otherwise restore to isolated recovery DB and follow incident plan; restoring over live DB may lose transactions and needs business authorization.

Schema v1/v2 SQL is duplicated for Go embed. CI/test asserts both copies match. The ordered runner applies each missing version under an advisory lock and transaction; migration timeout is 5 minutes. Future migrations should add stored checksums and configurable upgrade windows. No destructive down migration is supplied.

## Backup and recovery

```bash
sh scripts/backup.sh
# Verify restore in a separate disposable database, not production:
docker compose exec -T db createdb -U assetflow assetflow_restore_test
docker compose exec -T db pg_restore -U assetflow -d assetflow_restore_test --no-owner < backups/assetflow-TIMESTAMP.dump
docker compose exec -T db psql -U assetflow -d assetflow_restore_test -c 'SELECT count(*) FROM assets;'
# Drop recovery test DB only after validation.
```

Encrypt backups outside host, restricted access, signed manifests/checksums, daily retention and monthly restore rehearsal. Custom dump supports functional recovery but not point-in-time recovery. Enterprise requires continuous WAL archival/PITR or managed PG backup service. Proposed initial small deployment RPO ≤24h / RTO ≤4h must be demonstrated; for critical operations choose ≤15min RPO and ≤1h RTO with replicated infrastructure and runbooks. These are targets, not certified results.

## Observability and capacity

Available: request ID response, JSON HTTP logs method/path/duration, readiness ping, dashboard operational counts. Not yet instrumented: Prometheus histograms, distributed tracing, status code metrics, business-event metrics. Log HTTP duration is not a substitute for SLO monitoring.

Add central log aggregation, availability checks, p50/p95/p99, 4xx/5xx, DB pool wait, DB connections/locks/deadlocks, query latency, disk/WAL growth, backup age, replica lag, certificate expiry and memory saturation. Sample baseline targets: API p95 <300ms for bounded indexed reads, 5xx <0.5%, monthly availability 99.5% small / 99.9% HA. Load-test using representative data, not an empty DB.

Capacity tests: seed 10k/100k/1m assets as appropriate; concurrently list/search/approve/maintenance; use mixed roles/organizations; test hot asset contention and database failover. Add pg_trgm index and keyset pagination based on measured plans. Budget PG connections at max 20 per API replica plus headroom. Readiness calls DB; external monitor should avoid interpreting all DB maintenance as process failure.

## Security acceptance

- HTTPS, exact origin, secure cookies, strong unique bootstrap and database passwords.
- Runtime least privilege, no migration secret in API, no exposed DB port.
- Central login rate limiting with meaningful shared enforcement when replicas >1.
- User offboarding, periodic admin/manager access review, password lifecycle policy.
- Current dependency scan and image scan, patch cadence; image tags pinned to digest at release.
- Append-only external audit storage for regulatory/WORM needs.
- Backup encryption, restore test, retention and custodian PII handling.
- SSO/MFA required before enterprise rollout if organizational policy mandates it.
- Business authority validates disposal evidence; present application stores reason/note, not signed document attachments.

## Known implementation limitations

General retry idempotency keys are absent. Unique constraints/state transitions protect duplicate workflow action; asset create may return 409 after a network retry because the first attempt succeeded. Client should re-query by tag. Stocktake read array may be empty for nonexistent ID. Dashboard queries are separate snapshots. Stocktake close does not freeze later asset changes. Master list/users capped at 1000. Search offset pagination not intended as proven million-row solution. DB sessions are deleted on subsequent login when expired; no scheduled cleanup worker yet. Login throttle is memory-local. No full organization administration UX.

v2 Activity availability: API responses are buffered until activity persistence succeeds. If activity storage fails, API returns 503 and logs the request ID. Domain writes may have committed already (their transactional audit is preserved); refresh before retrying. Do not blindly retry mutations. This is an availability trade-off chosen to avoid returning success without activity evidence. Operations monitoring should alert on activity persistence errors.

Every API read creates an activity row, including reading the activity screen. Plan retention/partitioning and secure external archival for high traffic. Triggers block ordinary UPDATE/DELETE; append-only is not WORM against DB owner. `peer_ip` is the direct socket peer (usually reverse proxy), not a trusted original client IP. X-Forwarded-For is intentionally not accepted as authoritative; correlate with trusted edge logs if original client attribution is needed.

Docker is a packaging/deployment target, not an operational readiness certificate. Run container acceptance on the actual server and record evidence.
