# Responsive Navigation

## Menu Ownership

| Entry | Children | Reason |
| --- | --- | --- |
| Overview | Direct entry | Daily summary and actions across modules. |
| Assets & inventory | Asset register, Asset lifecycle | The register owns structured records; lifecycle owns photos, borrowing, repair evidence, and field operations. |
| Operations | Approvals, Maintenance, Stocktake | Existing operational queues, approval decisions, and inspection sessions. Their routes and API contracts remain unchanged. |
| Master data | Branches, Locations, Categories | Shared organizational structure and classification used by asset workflows. |
| General Setup | General Code, General Code Detail | Company-scoped lookup and numbering configuration, separate from physical locations and user access. Central administrators only. |
| Control & access | Audit trail, User activity, Team & access | Change accountability, session/request history, and authorization administration. Physical stocktake remains an operational inspection, not a security log. |
| Finance & reports | Direct entry with existing internal tabs | Accounting, depreciation, revaluation, journals, contracts, and compliance already have their own workspace navigation. |

Lifecycle reports remain in the lifecycle workspace as operational exports. Finance remains the accounting workspace. This release groups existing navigation without moving data, deleting routes, or changing permissions.

## Behavior

- Only authorized entries render; groups without authorized children disappear.
- Groups expand independently and the active route's group opens automatically.
- Navigation uses a bounded vertical scroll area; brand and connection status remain outside it. The active entry is brought into view when navigating.
- At widths up to 960px, a menu button opens a drawer. Selection, Escape, the close button, and the backdrop close it.
- The drawer traps keyboard focus, makes underlying content inert, locks background scrolling, and restores focus/scrolling on close. Resizing to desktop clears drawer state.
- Desktop retains a fixed sidebar. Tables, workspace tabs, and galleries scroll within their own containers instead of expanding the page.
- Application-header styles no longer affect calendar, notification, and modal headers. Small-screen notifications use a viewport-bounded panel.

## Verification

Browser fixtures exercise 320, 390, 768, 960, 1024, and 1440px widths, including 844x390 landscape. Checks cover disclosure state, permission filtering, scroll reachability, focus containment/restoration, Escape/backdrop closing, breakpoint resizing, calendar controls, notification bounds, forms, and page overflow. Fixture screenshots contain demonstration data, not production inventory.
