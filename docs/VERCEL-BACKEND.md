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

The database schema is not created by starting the API. Run `/server migrate`
once for a new database and once for each release that adds migrations, using a
separate migration role. Run `/server bootstrap` only once against an empty
database, with `BOOTSTRAP_EMAIL`, `BOOTSTRAP_PASSWORD`, and `ORG_NAME` supplied
for that one-off operation. Do not expose migration-owner or bootstrap
credentials to the API runtime.

The frontend currently calls same-origin `/api` and `/health` paths. Add
frontend rewrites to the backend only after its database, schema, and health
endpoint are ready. Keep the database connection pool's total across scaled
instances within the provider's connection limit.

