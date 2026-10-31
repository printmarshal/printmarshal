# Plan: dashboard layout and security-posture widget (from spec.md 2026-10-31)

## Files that change
- `internal/security/assess.go` (+ `assess_test.go`): `ComplianceSummary.Unknown`, `.NotAssessed`; `Summarize(as, inScope int)`.
- `internal/api/security.go`: pass the scoped device count to `Summarize`.
- `internal/store/store.go`, `sqlite.go`, `postgres.go` (+ `store_conformance_test.go`): `DeleteSetting`.
- `internal/api/dashboard.go` (new; the handler moves out of `server.go`): dashboard payload with `security` and `layout`; layout routes; validation; `internal/api/dashboard_test.go` (new).
- `internal/api/scope.go`: tokens denied on `/api/dashboard/layout`.
- `internal/api/server.go`: route registration; `deleteUser` removes the personal layout.
- `web/src/types.ts`; `web/src/dashboard/{catalogue.ts,layout.ts,EditBar.tsx,WidgetFrame.tsx}`; `web/src/dashboard/widgets/*.tsx` (one file per widget, moved from `Dashboard.tsx`); `web/src/dashboard/widgets/SecurityPosture.tsx`; `web/src/pages/Dashboard.tsx`; `web/src/pages/Security.tsx` (show unknown and not-assessed in the summary); `web/src/styles.css` (edit-mode toolbar, hidden tray, collapsed card).
- `wiki/API-reference.md`, `wiki/Monitoring-and-alerts.md`.

## Order of work
1. Backend summary: `Summarize` with `inScope`, `Unknown`, `NotAssessed`; tests; `GET /api/compliance` passes the count. (`make test` green.)
2. Store: `DeleteSetting` on SQLite and PostgreSQL with a conformance test.
3. API: `dashboard.go` with the layout types, normalisation against a server-side copy of the catalogue ids (the SPA owns titles and sizes; the server validates ids and booleans), the four routes, token denial, user-deletion cleanup, `security` and `layout` in the dashboard payload; tests for: personal and default precedence, admin-only default, token 403, unknown ids dropped, scoped principal's posture numbers, user deletion cleanup.
4. Web: catalogue and layout helpers with the merge rules from the spec; split today's dashboard JSX into widget components without visual change; verify with the typecheck and a screenshot that the page is pixel-identical before adding edit mode.
5. Web: edit mode (WidgetFrame controls, EditBar, hidden tray, save/reset/default), then the posture widget, then the Security page summary numbers. Screenshots at desktop and phone width, light and dark.
6. Docs; `plan.md` updated if anything above moved.

## Risks
- Splitting `Dashboard.tsx` into widgets can change spacing or lose the live-refresh behaviour; step 4 verifies pixel parity before anything else changes. This is the riskiest step.
- Scope: the posture summary must use the resolved device ids, not all assessments; the test with a scoped token guards it.
- Layout churn when widgets are renamed: the normalisation rule (unknown ids dropped, missing ids appended) is tested on the server and in the client helper.
- Options not taken: a `users.prefs` column (needs a migration and couples prefs to the auth table); drag-and-drop (dependency, pointer-only); storing layouts in browser storage (would not follow the user between browsers and could not have an admin default).

## Proof
- `make test` green, including new tests in `internal/security`, `internal/store` and `internal/api`.
- `cd web && npm run typecheck && npm run build` green.
- Screenshots: dashboard before and after the widget split (identical); edit mode with one widget hidden and one moved; the posture widget with simulator data showing unknown items; phone width in edit mode.
- A scoped API token's dashboard payload carries posture numbers for its devices only.

## Departures recorded during implementation (2026-10-31)
- The widgets live in one file, `web/src/dashboard/widgets.tsx`, with a renderer per widget, not one file per widget: the components are short and share imports.
- Today's page used four different CSS grids; a reorderable list needs one grid, so `.dash-grid` is a 12-column grid with spans by widget size (kpi 2, third 4, half 6, full 12) and breakpoints at 1100, 900 and 640 px. The page is visually equivalent, not pixel-identical; screenshots at 1440 px of main and the branch were compared.
- `Summarize` keeps its signature; `SummarizeFor(as, devicesInView)` adds the view size, so existing callers are untouched.
- `GET /api/compliance` gained group-scope filtering and a 403 on a foreign `?device=`: an audit-class gap found while wiring the posture summary.
