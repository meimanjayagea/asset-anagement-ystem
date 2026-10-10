# Backend on Vercel

The backend project must use `backend` as its Root Directory. `backend/Dockerfile.vercel`
builds the Go API as a Vercel container function; it is separate from the local
Compose deployment. The server listens on Vercel's `PORT` value and falls back
to port `8080` for local use.

Container functions scale down when idle and must keep durable state in an
external service. Do not run PostgreSQL or the Compose stack inside this
deployment. An always-on worker or other non-HTTP process needs a separate
Docker-capable host.

## Production configuration

- `DATABASE_URL`: managed PostgreSQL connection URL, preferably the provider's
  pooled URL with TLS verification enabled.
- `APP_ORIGIN`: the exact public frontend origin, currently
  `https://frontend-iota-five-52.vercel.app`.
- `COOKIE_SECURE`: `true`.

By default, starting the API does not create the database schema. Run `/server migrate`
once for a new database and once for each release that adds migrations, using a
separate migration role. Run `/server bootstrap` only once against an empty
database, with `BOOTSTRAP_EMAIL`, `BOOTSTRAP_PASSWORD`, and `ORG_NAME` supplied
for that one-off operation. Do not expose migration-owner or bootstrap
credentials to the API runtime.

Alternatively, enable `AUTO_MIGRATE=true` and configure `MIGRATION_DATABASE_URL`
with a schema-owner connection to the same database used by `DATABASE_URL`.
Startup then applies missing migrations in one transaction before opening the
HTTP listener. Multiple instances serialize using a transaction advisory lock.
Prefer a direct database connection for migrations. Keep `DATABASE_URL` on the
restricted runtime role, and remove the migration credential and disable
`AUTO_MIGRATE` after the upgrade when using the separate migration-job model.
Startup and `/health/ready` now validate login schema and session/audit privileges;
database connectivity alone no longer reports readiness.

### Recovery with a provider-managed sensitive connection

If the existing `DATABASE_URL` is an owner-capable managed connection that cannot
be copied out of Vercel, temporarily set both `AUTO_MIGRATE=true` and
`MIGRATION_USE_RUNTIME_DATABASE=true`, then redeploy the backend. This explicit
opt-in uses the existing secret inside the server; it does not reveal it or
grant additional database privileges. A restricted runtime role will fail the
upgrade, requiring a separate `MIGRATION_DATABASE_URL` instead. An explicitly
configured migration connection always takes precedence.

After `/health/ready` and `/api/login/options` succeed, disable both recovery
flags and redeploy. Do not leave automatic owner-connection migration enabled
as the normal runtime configuration. An error such as `schema incomplete (2/6)`
means startup stopped before serving HTTP because migrations are missing,
not that the container build failed. Upgrades preserve existing password hashes;
they do not reset accounts or bootstrap a populated database.

The frontend currently calls same-origin `/api` and `/health` paths. Add
frontend rewrites to the backend only after its database, schema, and health
endpoint are ready. Keep the database connection pool's total across scaled
instances within the provider's connection limit.
