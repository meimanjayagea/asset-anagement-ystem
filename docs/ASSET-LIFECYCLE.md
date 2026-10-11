# Asset Lifecycle Release (Schema 8)

## Business Flow

1. Register an asset with acquisition, supplier, warranty and depreciation data. An empty tag receives a unique automatic tag. Update brand, model, specifications and optional RFID identifier in the visual catalog.
2. Upload multiple catalog photographs. Print a QR/Code 128 label; scan by camera or an uploaded label image. QR links open the asset after authentication. RFID identifiers support lookup; physical reader integration is not implemented without hardware specifications.
3. Request a loan with a due date. A different authorized officer checks out the asset with photographs and condition notes. An officer other than the borrower verifies return photographs. Damaged returns leave the asset unavailable in maintenance status.
4. Submit a repair with damage evidence, or schedule preventive maintenance with a checklist and optional technician. Start the work, then complete every checklist item and record costs, replacement parts and completion photographs. The category interval determines the next preventive date. Planned dates and actual work orders appear in the calendar.
5. Create a physical stocktake snapshot. Compare the official photograph and scan/verify the tag. Missing, damaged and relocated findings require audit photographs; relocated findings also require a discovered location. Every snapshot item must be verified before closure. Acknowledgment is required for discrepancies. Findings do not silently mutate the asset master.
6. Submit a disposal request with final photographs and a write-off, sale or abandonment method. A different checker approves or rejects it. Existing transfer, approval, movement and accounting flows remain in use.
7. Export lifecycle, maintenance-cost and monthly depreciation/book-value data to PDF or Excel. Financial fields remain hidden from roles without financial access. Asset disposal contributes no active net book value to the export.

## Access And Evidence

- Administrator and branch administrator retain their existing boundaries. Managers/operators/IT support manage operational handovers and maintenance. Staff/employees submit requests. Auditors read scoped activity and audit history and verify physical stocktakes.
- Data, photos, lookup, work orders, reports and notifications are scoped by organization and accessible branches.
- Loan check-out and return cannot use the older direct asset-action endpoints to bypass condition evidence.
- Active loans block disposal, maintenance creation, structural editing and asset archiving. Version checks and row locks protect conflicting transitions.
- New transactions require applicable photographs. Historical requests and work orders retain their previous evidence policy instead of inventing attachments.
- Photographs and their transaction links are append-only. Each evidence photograph can be attached to one transaction, not reused as a new inspection.
- Uploads accept JPEG/PNG, validate decoded dimensions, then re-encode to JPEG without source metadata. Private authenticated database storage limits each normalized photo to 512 KiB, each asset to 40 photographs and each organization to 50 MiB. The frontend resizes originals before upload. Increasing long-term storage requires a separate private object-storage design, retention policy and capacity review.
- In-app reminders cover preventive dates within seven days, open work orders within seven days and loan returns within three days, including overdue items. They refresh while the application is open; email, WhatsApp and operating-system push are not part of this release.
- Lists remain paginated. Field workspace retrieval and lifecycle exports are bounded to 5,000 records; select a narrower branch/location when necessary.

## Metadata Migration

Migrations 007 and 008 are additive and bundled in the server and standalone upgrade SQL. All 23 application tables have:

- `created_at`, `created_by`
- `updated_at`, `updated_by`

The transaction wrapper passes the authenticated actor to a transaction-local setting. Database triggers stamp new records and updates while preserving creation metadata. Immutable audit/evidence records still prohibit changes. Unknown legacy actors/timestamps stay NULL, rather than fabricating historical attribution. System migrations and operator recovery without an authenticated actor also remain unattributed.

## Production Rollout

1. Create a database backup/snapshot using the managed database provider before applying schema changes.
2. Prefer a migration job using an owner connection in `MIGRATION_DATABASE_URL`. Run the server `migrate` command or `docs/login-upgrade.sql`. Migrations are transactional and advisory-locked.
3. Apply `docs/least-privilege.sql` for a separate runtime role, including new table/sequence grants. Adjust its database name to the actual deployment.
4. Deploy backend and confirm `/health/ready` succeeds with all nine migrations. Then deploy the matching frontend commit.
5. When the managed owner connection cannot leave the platform, an explicitly authorized temporary deployment can set `AUTO_MIGRATE=true` and `MIGRATION_USE_RUNTIME_DATABASE=true`. After successful readiness verification, set both false and redeploy. Do not enable `ACCOUNT_RECOVERY` or change production passwords for this release.
6. Verify login options, authenticated scope, catalog/image loading, notifications and reports. Perform destructive business-flow UAT in a disposable database, not production inventory.
7. The additive schema supports an application rollback to the previous code without dropping evidence or audit data. Do not run destructive down migrations. A failed migration rolls back as one transaction.

## Verification

- Native PostgreSQL integration tests exercise every registered API endpoint, unauthorized requests, role/branch boundaries, evidence validation, duplicate/conflicting transitions and all-table metadata.
- Upgrade validation covers fresh databases and versions 2, 4, 5, 6 and 8, including repeat execution and preserved credentials/audit records.
- Live browser tests use PostgreSQL-backed login, persistence, photos, specs, QR scan/print, loan handover/return, reminders, repair checklist, physical audit, disposal and PDF/Excel output.
- Desktop/mobile screenshots use generated test data only. A browser camera cannot prove physical camera/RFID hardware behavior in CI.
- The additional document/scanning libraries load on demand. The Excel library emits a large lazy-loaded chunk; it does not enter the initial application bundle.
