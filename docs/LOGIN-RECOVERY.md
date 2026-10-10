# Deployment login recovery

The current login contract is `POST /api/login` with `org_code`, `employee_id`,
and `password`. Email remains account metadata, not a login identifier. Migration
5 generates organization codes; migration 6 generates employee IDs for existing
users and changes the activity audit identity columns. Existing password hashes
and account roles are preserved.

On 2026-10-10, the public frontend returned HTTP 200 for `/health/ready` but HTTP
503 for `/api/login/options`, with an activity audit persistence error. Readiness
only confirms connectivity. Missing migrations or runtime INSERT/sequence
permissions can cause this response; inspect runtime SQL errors to distinguish
them. The exact production SQL failure was not accessible during this check.

1. Use the same managed PostgreSQL database configured for the deployed backend.
2. Run `docs/login-upgrade.sql` using its schema owner in a SQL editor, or run
   the current backend's `migrate` command. The SQL bundle applies missing
   versions 1-6 atomically, uses the migration runner's advisory lock, and can
   be repeated. Back up the database before applying the upgrade.
   For migration during deployment startup, set `AUTO_MIGRATE=true` and
   `MIGRATION_DATABASE_URL` to an owner connection for the same database.
   `DATABASE_URL` remains the runtime connection. See `VERCEL-BACKEND.md`.
3. Run `docs/login-account-check.sql`. Locate the row for `meiman@example.test`
   and use its actual `org_code` and `employee_id` with the existing password.
   Do not assume that this user's ID or organization ID is 1. Zero rows means
   this database has no such account; provision it through the existing admin
   workflow. This upgrade does not create an administrator or reset passwords.
4. If audit errors continue, check INSERT on `user_activity_logs` and USAGE on
   its identity sequence using the backend's runtime role (an owner check does
   not verify runtime privileges). Use `docs/least-privilege.sql` as the grant
   reference. Keep audit persistence enforced.
5. Ensure frontend and backend deploy the same login contract. Verify
   `/api/login/options`, login, session cookie, `/api/me`, and logout through
   the frontend domain. Production needs `COOKIE_SECURE=true` and
   `APP_ORIGIN=https://frontend-iota-five-52.vercel.app`.

Generate and validate the bundle with `node validation/login-migration.mjs`
after installing the validation dependencies. It tests upgrades from empty,
v4, v5 and v6 databases, repeat application, existing credentials, immutable
audit history, and the current backend's actual activity INSERT statement.
