# UAT and release evidence

Business owner and IT owner complete these in staging and record actor, timestamp, request ID, screenshots and expected/actual outcome. No production data in automated test DB.

| ID | Scenario | Expected |
|---|---|---|
| IAM-01 | Bootstrap empty DB, bootstrap again | First success, second refuses |
| IAM-02 | Incorrect credentials, repeated failures | 401; after threshold 429 |
| IAM-03 | Logout/password/access revocation | Existing cookie rejected |
| IAM-04 | Auditor tries register/approve | 403 despite direct API access |
| IAM-05 | Operator tries users/audit endpoint | 403 |
| TEN-01 | Other organization references location/category | 422; no row created |
| TEN-02 | Other organization lists/mutates asset | No leakage; mutate 404 |
| REG-01 | Duplicate tag/serial | 409 |
| REG-02 | Invalid money/life/warranty | 422 |
| REG-03 | Search/status/page/export current page | Correct filter; CSV safe values |
| AST-01 | Assign then return | Custodian populated then cleared; version increments |
| AST-02 | Stale version/concurrent assign | One success, conflict on other |
| REQ-01 | Transfer request then assign attempt | Assignment blocked while pending |
| REQ-02 | Maker attempts own approval as admin | 403 |
| REQ-03 | Other checker approves transfer | Location + request + audit atomically committed |
| REQ-04 | Reject without reason, with reason | Invalid rejected; valid terminal rejected |
| REQ-05 | Dispose assigned asset | Denied until returned |
| REQ-06 | Approve twice | Second 409 |
| MNT-01 | Schedule/start/complete | Correct asset/job state and actual cost |
| MNT-02 | Complete before start / duplicate job | Denied |
| MNT-03 | Scheduled due yesterday | Dashboard overdue increments |
| STK-01 | Open stocktake snapshot | Expected assets captured |
| STK-02 | Observe known/unknown/duplicate tag | Known accepted, others rejected |
| STK-03 | Missing/changed then close without acknowledgement | 409 |
| STK-04 | Close with acknowledgement; observe after close | Close succeeds; further observe denied |
| AUD-01 | Read timeline | Correct actor, entity, before/after |
| AUD-02 | UPDATE/DELETE audit as runtime | Denied |
| SEC-01 | No CSRF header / foreign Origin | 403 |
| SEC-02 | Oversized body / malformed JSON trailing content | Rejected |
| SEC-03 | TLS session cookie | HttpOnly/Secure/SameSiteStrict |
| OPS-01 | DB offline | Readiness 503, writes do not silently succeed |
| OPS-02 | Restart API/DB | Data persists; graceful shutdown |
| OPS-03 | Restore backup on isolated DB | Counts, relationships and user login verified |
| OPS-04 | Load at planned dataset/concurrency | Document measured latency and DB capacity |
| UX-01 | Desktop/mobile workflow | Tables scroll, actions visible, errors actionable |

Include data correction SOP, lost asset investigation, approval delegation and offboarding checklist in organizational training. Missing accounting/SSO/attachment features must be accepted as explicit scope limits, not inferred complete.

## Tambahan v2

| ID | Scenario | Expected |
|---|---|---|
| BR-01 | Create JKT/BDG branches and branch-specific locations | Company FK valid; duplicate branch code rejected |
| BR-02 | Scoped operator lists assets/dashboard | Only assigned branches; foreign branch query 403 |
| BR-03 | Direct foreign asset assign/maintenance/stocktake | 404/403 with activity denial captured |
| BR-04 | Transfer by one-branch user to foreign destination | Denied; multi-branch maker + other checker accepted |
| BR-05 | Reassign user scope | Old sessions revoked; new login enforces revised scope |
| AUTH-01 | Enter organization code and select an active branch | Only branches under that organization code are listed; branch is required |
| AUTH-02 | Log in with valid credentials on an unassigned branch | Login denied with an explicit branch-membership error |
| AU-03 | Successful/failed login, logout, GET and denied mutation | All events recorded; no password/token/body |
| AU-04 | Read activity with branch auditor | Scoped events, no company failed-login leakage |
| AU-05 | Activity DB permission failure | HTTP 503, request ID in server log; refresh mutation result |
| CAT-01 | New category interval 90 days; register purchase 2026-01-01 | Next maintenance 2026-04-01 |
| CAT-02 | Complete work order | Next due completion_date + current interval |
| CAT-03 | Change policy with stale version | 409 |
| CAT-04 | Apply policy to existing assets | Eligible recalculated; pending/jobs skipped |
| MIG-01 | v1 data upgrade | Assets preserved, HQ mapping, legacy audit unchanged |
