# General Setup and DEMO

## Design

Migration 009 is additive. Existing organization login codes, employee IDs,
credentials, HQ, and operational data are preserved.

General Code is an organization-scoped namespace: stable key, name, company prefix
and description. General Code Detail is a classification and numbering rule:
stable key, name, type (hq/branch/reference), prefix and 3-8 sequence digits.
Example: MJ-CBG-000001. Uniqueness is organization-scoped; integrations must also
send organization ID or the existing organization login code.

Branch creation can select a detail instead of manual code. HQ/legacy branch edit
can allocate business_code once; its old code remains stable. HQ must remain HQ
because existing access checks depend on it. Reference details are lookup values;
they do not allocate standalone numbers. A UI preview never consumes a number.

Configuration and allocation lock parent then detail. Branch write, counter and
append-only issued registry commit atomically. Issued numbers are never recycled,
even after soft deletion. The counter cannot be reset via API. Once issued,
prefixes/type/width are immutable; names/descriptions may still change. Create a
new rule for a new format. Collisions and exhausted counters fail without partial
writes or truncation.

Archive active details before their parent; restore parent before its details.
Archived rules cannot allocate, but issued branches retain their identifiers.
Only central administrators manage company-wide setup and seed operations.

## Master Editing

- Branch: name, address, one-time business-code allocation; stable branch code.
- Location: name, building, floor, room; stable branch ownership.
- Category: name, default useful life; existing asset financial snapshots stay
  unchanged. Maintenance/depreciation policy uses the existing dedicated form.
- User: name/email; stable employee ID and unchanged password. Existing access
  form manages role, activation and branches and revokes sessions.

PUT edits require the version from the list API; stale versions return 409.
Branch/location/user changes including archive/restore increment versions.
Existing reference, branch scope, HQ, self-account and last-admin protections
remain. User archive/access changes share the organization advisory lock.

Audit, user activity, photo evidence and issued registries stay append-only.
Control & Access edit/delete applies to accounts, not security history.
All new tables have created_at, created_by, updated_at and updated_by triggers.

## Samples

One DEMO branch, 26 assets, 42 labeled raster illustrations, three locations
(one archived), four scoped categories (one archived), and nine fictional role
records (one archived).

- Catalog, multi-photo, specifications, RFID lookup, three depreciation methods.
- Available, assigned, maintenance, disposed and archived assets.
- Four maintenance statuses; preventive/repair jobs, checklist, parts, costs,
  damage/repair evidence.
- Six loan cases: pending, overdue checkout, returned good/damaged, rejected,
  cancelled, with before/after proof.
- Seven transfer/disposal cases: pending/approved/rejected transfers; pending
  disposal and approved write-off/sale/abandonment, independent decision actors.
- Open/closed stocktakes: present/missing/damaged/relocated, with finding proof.
- Three valuation statuses and expiring/expired/archived contracts.
- Movement/seed audit history and overdue in-app reminders.
- Active/archived General Setup examples and issued DEMO-CBG-000001 branch alias.

Images are explicitly illustrations, not operational photographs. DEMO enters
company-wide reports: select an operational branch to exclude it from real
accounting. Actions performed on samples persist normally.

No sample credentials exist: password hashes are intentionally invalid. Only
the fictional staff record is active for admin-operated loan checkout; it still
cannot log in. Other sample accounts are inactive. No extra central admin is
created, no real password is reset and no global accounting profile is replaced.

## Execution and Release

POST /api/demo-data with confirmation SEED_DEMO_KEEP_EXISTING as central admin,
or use the General Code page's confirmed sample action.

Server CLI: seed-demo with DATABASE_URL, DEMO_SEED_ORG_CODE and
DEMO_SEED_CONFIRM=SEED_DEMO_KEEP_EXISTING. Temporary startup deployment uses the
same two DEMO variables; clear confirmation after verifying success.

All samples and manifest commit in one transaction. Replays return the manifest
without duplicates or credential changes. A pre-existing DEMO branch without
the manifest aborts; it is never overwritten.

Deploy backend first with owner migration 8 to 9, apply narrow runtime grants
from docs/least-privilege.sql, verify /health/ready, then deploy frontend.
If temporarily using an owner-capable managed runtime connection, explicitly
enable AUTO_MIGRATE and MIGRATION_USE_RUNTIME_DATABASE and disable both after
rollout. ACCOUNT_RECOVERY must remain false.
