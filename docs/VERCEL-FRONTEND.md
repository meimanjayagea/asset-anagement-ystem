# Vercel Frontend Deployment

This repository is a monorepo. Deploy only the Vue frontend to Vercel.

Recommended Vercel settings:

- Root Directory: `frontend`
- Framework Preset: Vite
- Install Command: `npm ci`
- Build Command: `npm run build`
- Output Directory: `dist`

The frontend currently calls the backend with same-origin relative paths such as
`/api` and `/health`. A Vercel static deployment will serve the UI only; the Go
API, PostgreSQL database, migration job, bootstrap job, session cookies, and
reverse proxy rules must be hosted separately.

Backend hosting requirements:

- A long-running Docker-capable host or container platform for `backend`.
- PostgreSQL 17 or compatible managed PostgreSQL.
- A migration/bootstrap process with separate owner/runtime credentials.
- HTTPS termination and a same-origin reverse proxy from the public frontend
  origin to the Go API, or a deliberate API URL refactor plus CORS/cookie review.
- Production secrets in the hosting provider secret manager, never committed.
