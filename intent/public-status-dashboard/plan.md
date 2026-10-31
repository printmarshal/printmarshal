# Plan: a public status dashboard, configured by administrators

From `spec.md` (accepted in PR 44). Status: draft. Requirement numbers (R1…R18) are the spec's.

## Files that change
Server (`internal/api`)
- `public.go` (new): the configuration type `publicStatusConfig` and its validation (R8, R10,
  R13 limits, origins validated with `url.Parse` as scheme+host only); `loadPublicStatus` /
  `savePublicStatus` on the settings key `public.status` through `Store.GetSetting` /
  `Store.SetSetting`; the response types `publicBoard`, `publicDevice`, `publicTray`,
  `publicSupply`, `publicAlert` (R2); `buildPublicBoard(cfg, devices, alerts, now, pollInterval)`
  as a pure function (R2, R3, R4, `stale`); `alertCategory(source, key)` with the fixed mapping
  (R4); a `boardCache` (value, built-at, 15 s, one in-flight build at a time, R6); a
  `publicLimiter` built on the `loginLimiter` pattern keyed by `clientIP(r)` with 30 per minute
  and `Retry-After: 60` (R7); handlers `getPublicStatus` (both paths, 404 via `writeErr` with the
  mux's "no such endpoint" text when disabled or slug mismatch, R1), `getPublicStatusSettings`,
  `putPublicStatusSettings` (admin; security log entries R9; `allowedAll` on the device ids R10;
  slug regeneration with `crypto/rand`, 22 base64url characters).
- `public_test.go` (new), written first and committed failing for the handlers that do not exist
  yet (they fail to compile until stubs exist, so step 1 adds the route stubs returning 501 in the
  same commit as the tests, and the tests assert the real behaviour):
  `TestPublicStatusIs404WhenOff` (R1), `TestPublicStatusUnlistedSlugOnly` (R1),
  `TestPublicBoardKeySet` (R2: marshal with every switch on, compare the exact key sets per object,
  and with every switch off: only `label`), `TestPublicBoardNeverCarriesIdentity` (R3: the JSON text
  contains none of the device's address, serial, hostname, model or id),
  `TestAlertCategoryMapping` (R4, table), `TestPublicStatusReadsStoreOnly` (R5: a `Server` whose
  `Fleet` is nil and whose engine would panic on use serves the board),
  `TestPublicStatusCacheSharesBuilds` (R6), `TestPublicStatusRateLimit` (R7: 31st request in a
  minute is 429 with the header), `TestPublicStatusSettingsRefuseTokens` (R9, as
  `TestTokensCannotManageDirectorySettings`), `TestPublicStatusSettingsScope` (R10),
  `TestPublicStatusFrameHeaders` (decision 3: `/status` with origins listed gets
  `frame-ancestors`, `/devices` keeps `X-Frame-Options: DENY`).
- `server.go`: register `GET /api/public/status`, `GET /api/public/status/{slug}` without
  `require`; `GET|PUT /api/settings/public-status` with `require(RoleAdmin, …)`; add
  `/api/settings/public-status` to the token-denied paths beside the directory settings;
  `secureHeaders` takes the server so that for `r.URL.Path == "/status"` or a `/status/` prefix
  it consults the saved frame origins (cached with the board) and sends
  `Content-Security-Policy: frame-ancestors …` instead of `X-Frame-Options` when the list is
  non-empty.
- `scopeof.go`: the two settings routes are admin-only already; no scope change.

Web (`web/src`)
- `types-public.ts` (new): `PublicBoard` mirroring R2, and `PublicStatusConfig`.
- `pages/PublicStatus.tsx` (new): the session-less page (R11, R12, R16–R18); reads the path to
  pick the plain or slug endpoint; 30 s refresh with "Updated N s ago"; three failures →
  "Status unavailable"; `?wall=1` mode with 10 s paging over 12 per page; the states of R18.
- `App.tsx`: before `api.me()` runs, if `location.pathname` is `/status` or starts with `/status/`,
  render `PublicStatus` and nothing else (no `Shell`, no settings fetch).
- `pages/settings/PublicStatus.tsx` (new) and `pages/Settings.tsx`: the admin section (R13–R15)
  with the device picker (reuse the list fetch from `Devices.tsx` filtered by search and group,
  up/down ordering), public label inputs, switches, origins textarea, title, unlisted toggle
  with the address shown and a Regenerate button, preview link, summary line, empty-board warning.
- `dashboard/EditBar.tsx`: one link "Public board…" to `/settings/publicstatus` for admins.
- `styles.css`: `.pub-` rules for the page (type scale for wall mode, pills, grid, single column
  under 600 px), reusing the existing tone variables.

Docs
- `wiki/API-reference.md` (the two public endpoints, the settings route, the 404/429 semantics),
  `wiki/Monitoring-and-alerts.md` (a "Public status board" section: enable, choose printers and
  signals, the address, wall mode, framing), `wiki/Security.md` (the boundary in six bullets).
- `intent/public-status-dashboard/design.md` (new, from the design pass): layout, type scale,
  pill vocabulary, wall mode, the three states, with a sketch; the web step implements it.

## Order of work
1. **Design pass** (Fable, `frontend-design` and `steel-and-signal-brand` skills): write
   `design.md` and commit it. Half a day at most; it fixes the public page's look before any web
   code so the web worker has a target.
2. **Server tests and stubs** (careful-tier worker): `public_test.go` plus 501 stubs and the
   route registrations; `go test ./internal/api -run Public` shows every new test failing on
   behaviour; commit.
3. **Server** (same worker, `PM_FIX_TASK=1`): `public.go`, `server.go`, `secureHeaders`;
   `make test` green; commit.
4. **Public page** (medium-tier worker): `PublicStatus.tsx`, `App.tsx`, `types-public.ts`,
   styles, from `design.md`; `npm run typecheck && npm run build`; restore
   `internal/web/dist/index.html`; commit.
5. **Settings section** (medium-tier worker): the section, `Settings.tsx`, the edit-bar link;
   typecheck and build; commit.
6. **Wiki** (light-tier worker): the three pages; commit.
7. **Verification** (verifier agent, fresh context): the proof below on loopback with `pm-sim`
   devices only; paste outputs and screenshots into the PR description.

Steps 4 and 5 can run in parallel in separate worktrees after step 3, since they touch
different files except `Settings.tsx` (step 5 only) and `App.tsx` (step 4 only); they merge into
the branch in that order.

## Risks
- **What it could break.** `secureHeaders` wraps every request: a wrong prefix check would drop
  the DENY header from console pages. Covered by `TestPublicStatusFrameHeaders` on `/devices` and
  `/status-not-the-board` style paths. `App.tsx` gains a path switch before the auth gate: a typo
  there blanks the console; the typecheck plus the verifier's sign-in check cover it.
- **Riskiest step.** Step 3's projection and the key-set test: if the test is written against the
  struct rather than the marshalled JSON, an `omitempty` or embedded field could leak. The test
  must assert on decoded JSON maps.
- **Rate limiter memory.** The login limiter keeps a map by address; the public one must prune
  entries older than a minute on each hit (the login one already does) and cap the map at, say,
  10,000 addresses, dropping the oldest.
- **Cache and configuration changes.** A `PUT` must invalidate the board cache and the frame
  origins cache, or a disabled board keeps serving for up to 15 s; the test for R1 toggles off and
  requests immediately.
- **Options not taken.** A background snapshot goroutine (does work when nobody looks); a
  separate static bundle for the public page (one SPA is simpler, the gate just routes earlier);
  WebSocket updates (public connections to the hub are out of scope).

## Proof
- Step 2: the `go test` output with the new tests failing on behaviour (not compile errors).
- Step 3: `make test` with 43 packages `ok` (42 today plus none new; the count is a check that
  nothing was dropped), and `go test -race ./internal/api`.
- Steps 4 and 5: `npm run typecheck` and `npm run build` clean.
- Step 7, on `127.0.0.1:<port>` with `scripts/e2e.sh`-style setup (server plus `pm-sim`, never a
  real address): sign in as admin, enable the board with two simulated devices, one label set,
  all switches on; `curl` the public endpoint without cookies and paste the JSON (check the key
  set by eye against R2); sign out and open `/status` in a private window: screenshot at desktop,
  at 400 px, and with `?wall=1`; turn unlisted on, confirm `/status` says not available and the
  slug address works; turn the board off, confirm 404 and the "not available" page; confirm
  `/devices` still sends `X-Frame-Options: DENY` (`curl -I`); 31 quick requests show one 429.

## Departures made in step 3 (server)
- `secureHeaders(next)` keeps its signature (`reports_test.go` calls it); the board's frame check
  is `secureHeadersWith(s.boardFrameOrigins, mux)`, which `Handler` uses. Same behaviour as
  planned: `/status` and `/status/…` only, `frame-ancestors` when the board is on and origins
  are listed, `X-Frame-Options: DENY` everywhere else.
- The token denial for `/api/settings/public-status` lives in `scope.go` (`authorize`), beside
  the directory settings, not in `server.go`.
- `buildPublicBoard` takes one more argument, the low-toner threshold, from
  `Cfg.Alerts.SupplyWarningPercent` (toner `status`: `empty` at 0 %, `low` at or under it).
- `internal/events` catalogues `public.status.changed` and `public.status.enabled` as security
  events (not notifiable), so the security log entries are exported and not logged as unknown.
- `Handler` initialises the HTTP stats before serving: their lazy init raced under concurrent
  first requests, which `TestPublicStatusCacheSharesBuilds` exposed under `-race`.
- The `PUT` body is the configuration shape plus `regenerateSlug` (boolean); a `slug` in the
  body is ignored (the server owns it). A slug is issued on the first unlisted save and kept
  across saves until regenerated.
- Projection choices the spec left open: toner names come from the colorant ("Black"), never
  the supply description (it carries vendor and part numbers, R3); manual-feed trays are left
  out of `paper[]` as in the paper alert rule; a device without a public label or display name
  is "Printer N", never `Device.Name()` (which falls back to hostname, model, address); alerts
  are de-duplicated per device by severity and category, critical first; decommissioned devices
  stay listed if the administrator listed them; `updatedAt` is the newest of `lastSeen`, status
  time and `lastPolled` among listed devices (null with `stale: true` when none was ever
  polled, the build time when the board lists no device).
- Proof count: `make test` prints 42 packages `ok`. The "43" above was a miscount: `go list`
  finds 42 packages with tests on the base commit and on this branch (none added or dropped).
