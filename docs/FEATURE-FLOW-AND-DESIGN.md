# Asset Management: Feature Flows and Design

This document defines the user journey and operational boundaries for the ten core capabilities. API authorization and branch scope are enforced by the backend; UI visibility is only a convenience, never an access control.

## Operating model

The core record is one physical asset, identified by an organization-unique tag. An asset belongs to a category and a location; the location determines its branch. Cost, useful life, depreciation method, supplier and acquisition reference are finance-controlled fields. A user session selects one authorized branch context, while central administrators may select all branches. Every write records an audit event. Physical/custodian changes also append an immutable movement event.

The expected navigation is: Overview → Asset register → asset detail/history → request or work order → checker decision → updated register and audit trail. Finance works as a separate workspace so operational users do not see cost or accounting information they are not permitted to access.

## Capability flows

### 1. Registration and classification

1. A permitted user chooses a branch, location and category. The server derives the branch from the location and verifies that the category is shared or belongs to that same branch.
2. The user records a unique tag, name, serial number, acquisition date, supplier/reference, warranty and category policy. Finance users add acquisition cost, salvage value, useful life, depreciation method and depreciation start date.
3. The category supplies the default useful life and depreciation method; the user can override the depreciation method when authorized. The server validates dates, integer IDR bounds, tag uniqueness and branch scope before commit.
4. Registration writes the asset, an immutable `registered` movement and an audit event in one transaction.

### 2. Lifecycle management

The operational states are `available`, `assigned`, `maintenance` and terminal `disposed`. Assignment requires a named custodian; return clears it. An available asset may be proposed for transfer or disposal. The request is not effective until a different user approves it, and its expected asset version must still match. A transfer updates the location and writes a movement; a disposal snapshots proceeds and carrying values, sets the terminal state and writes a disposal movement. Archive is a reversible soft delete and is separate from disposal.

### 3. Depreciation management

Each asset uses one policy: straight-line, double-declining balance with a switch to straight-line when that produces a larger remaining-life charge, or non-depreciable. Calculations use integer rupiah and completed months from the depreciation start date, stop at salvage value, and are estimates for operational reporting. Revaluation resets the depreciable basis and remaining life from its effective date. Finance can inspect opening value, period charge and closing value by month and authorized branch.

The application does not claim tax, IFRS, PSAK or other statutory compliance. An accountant must confirm convention, in-service date, residual value and rounding before financial statements use these estimates.

### 4. Maintenance scheduling

Category interval/instructions seed the next due date. A user schedules a work order against an available asset; start moves the asset to maintenance; completion records actual cost/notes, returns it to available and calculates the next due date from the category interval. Cancellation is only allowed before work starts. Active work and pending approvals block conflicting structural asset changes.

### 5. Location tracking

Location is tracked through explicit registration, assignment, return, same-branch relocation, approved transfer and disposal events. The append-only timeline records actor, timestamp, before/after locations and custodians, branches and note. This is an auditable, user-reported location history, not GPS or continuous real-time tracking. Branch-limited users see only the side of a cross-branch movement within their own access.

### 6. Warranty and service contracts

Warranty expiry is stored per asset. Service contracts may cover one asset or a whole branch and include vendor, reference, start/end date, renewal notice period, optional annual cost and notes. The list flags contracts inside their renewal window; expired warranties and contracts appear in compliance reporting. Contracts are soft-deleted and audited, not erased.

### 7. Valuation and revaluation

An authorized proposer records the new amount, remaining useful life and reason. The effective date is the first day of the current month in Asia/Jakarta. Pending proposals do not affect carrying value. A different authorized checker approves or rejects; approval checks the asset version, increments it, and changes the carrying basis. Rejection requires a reason and does not change the asset. At most one pending valuation per asset is allowed.

### 8. Disposal and write-off

The maker selects disposal and documents the reason; Finance may add proceeds. A different checker approves. The system captures book value, gross basis, accumulated depreciation and proceeds, then makes the asset terminal. Accounting preview balances cash/proceeds, accumulated depreciation, asset cost and gain/loss. Legacy disposed records without a financial snapshot remain visible, but are intentionally excluded from journal export instead of receiving invented accounting values.

### 9. Accounting integration

No ERP has been selected, so the integration is a configurable, generic journal CSV rather than a vendor-specific connector. The organization maps asset cost, accumulated depreciation, depreciation expense, cash, disposal gain/loss, revaluation reserve and impairment accounts. The CSV includes journal date, branch, reference, account code, debit, credit and memo. Generated entries cover period depreciation, approved revaluations and disposals; text fields are neutralized against spreadsheet formula injection and exports are audited.

The chart-code defaults are examples only. Finance must map them to the client's chart of accounts and validate postings before import. Currency is currently IDR, there is no ERP authentication/transport, and no tax or statutory rules are inferred.

### 10. Reporting and compliance

The Finance workspace offers a period depreciation schedule, pending/approved valuation list, contract renewal list, journal preview/export, account mapping and an operational compliance snapshot. Audit trail captures before/after state and actor; user activity captures request outcome and context. Reports accept only a branch the session can access. Financial values require finance capability; auditors/managers without it receive the non-financial compliance summary only.

## Roles and separation of duties

| Role | Intended scope and duties |
|---|---|
| Central administrator | All branches, all product features, access administration and audit. |
| Branch administrator | Full feature access within its assigned branch; no HQ access or branch creation. |
| Manager | Branch operations, requests/checking, maintenance, contract handling, reports and valuation proposals; no finance settings or monetary fields. |
| Finance | Branch-scoped financial records, depreciation, account mapping, journals, valuation decisions and finance exports; no user/asset operational administration. |
| Operator / IT support | Asset operations, maintenance and stocktake according to granted branch scope; no finance data. |
| Staff / employee | Read assigned-scope inventory and create permitted requests. |
| IT developer | Read-only operational inventory, without audit or finance records. |
| Auditor | Read-only permitted operational data, audit and non-financial reports. |

No role changes the row-level branch predicate. The database checks organization/branch relationships, and the API rechecks capabilities on every request. Maker-checker is enforced by actor IDs on the server. Central and branch administrators can soft-delete records within their scopes; permanent deletion is not part of routine UI workflows.

## Interface and localization

The light palette uses a restrained sky blue for primary actions, cool white surfaces and dark marine navigation; green, amber and red remain reserved for positive, due and rejected states. Dark mode uses the same semantic roles with deeper surfaces and accessible contrast. The language selector switches Indonesian/English, persists per browser, and formats dates/timestamps for Asia/Jakarta and money as IDR.

The register remains the high-frequency operational view. Finance uses period controls, compact tables and inline proposal/contract forms. Theme and language controls remain available on login and after sign-in. Responsive layouts collapse tables into horizontal scroll rather than hiding financial/audit columns or changing row semantics.

## Acceptance checks

- An asset cannot be registered into a location or category outside the actor's authorized branch.
- A branch user cannot read another branch's asset, movement details, contracts, valuation or audit payload.
- Failed version checks, same-maker decisions and duplicate pending valuations do not partially apply changes.
- Disposal and valuation decisions update the audit trail and applicable asset movement atomically.
- Journal preview/export balances debit and credit for each disposal/revaluation/depreciation reference; the generic CSV is not represented as a certified ledger.
- Dark mode, language preference, and date/currency formatting survive reload; financial actions remain capability-gated in both themes/locales.
