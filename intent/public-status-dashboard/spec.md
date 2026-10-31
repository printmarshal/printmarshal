# Spec: a public status dashboard, configured by administrators

From `intent.md` (accepted in PR 42). Status: draft.

## Decisions proposed for the intent's open questions
1. **Configuration lives in Settings**, in a new admin-only section "Public status". The dashboard's
   edit bar gets one link to it ("Public board…"). Reason: it needs a device picker and switches
   that apply to everyone, which is Settings' job; the dashboard's edit mode is personal layout.
2. **Address: a fixed path by default, an unguessable one on request.** The board is at `/status`.
   An administrator can switch to "unlisted": the board moves to `/status/<slug>` with a 22-character
   random slug they can regenerate, and `/status` then shows "not available". Off is the default.
3. **Framing is off unless listed.** The console keeps `X-Frame-Options: DENY`. For the board's
   path only, the administrator may list origins allowed to frame it; the server then sends
   `Content-Security-Policy: frame-ancestors <list>` instead of the DENY header for that path.
4. **Public labels, yes.** Each shown device may carry a public label ("Library ground floor,
   colour"); it is stored in the board configuration, not on the device, and used instead of the
   display name when set.
5. **One board** in this version. The stored shape is a single object; a later version can hold a
   list without changing this one.
6. **"Up" means the poller reached the device** (`status.online`). The device's own state word
   (ready, sleeping, printing) is a separate switch, off by default, so a sleeping printer never
   shows as down.

## Requirements
Each is testable; R-numbers are referenced by the plan and the tests.

Boundary
- R1. `GET /api/public/status` (and `/api/public/status/{slug}` when unlisted) needs no session and
  no token. While the board is disabled, both answer 404 with the generic "no such endpoint" body.
  While unlisted, `/api/public/status` answers 404 and only the current slug answers.
- R2. The response is built from a purpose-built struct with exactly these keys and no others:
  `title`, `updatedAt`, `stale` (true when `updatedAt` is older than three poll intervals), and
  `devices[]` with `label` always, `location` only when the location switch is on, `online` only
  when the up/down switch is on, `state` only when the state switch is on, `paper[]` only when the
  paper switch is on (`name`, `status` in ok/low/empty/unknown), `toner[]` only when the toner switch
  is on (`name`, `colorant`, `percent` or null, `status` in ok/low/empty/unknown), `alerts[]` only
  when the alerts switch is on (`severity`, `category`). A test marshals a board with every switch
  on and asserts the exact JSON key set per object, so a field added to `model.Device` cannot leak.
- R3. Never present, whatever the switches: address, hostname, DNS name, MAC, serial, UUID, make,
  model, firmware, groups, tags, asset tag, notes, credentials, compliance or security data, user or
  instance data, device ids. Devices are listed in the configured order with no id.
- R4. Alert `category` is one of offline, paper, toner, door, jam, maintenance, other, derived from
  the alert's source and key by a fixed mapping in code; the free-text description is never sent.
  Acknowledged and cleared alerts are excluded. Security assessment findings are never alerts here.
- R5. Serving the board never contacts a device: the handler reads the store only. A test runs the
  handler with a fleet service whose drivers would fail on any call and sees no call.
- R6. The response is cached in memory for 15 seconds per board; concurrent requests during a
  rebuild share one build (singleflight by hand). The handler accepts no query parameters that
  select data; unknown parameters are ignored.
- R7. Per client address, more than 30 requests in a minute answer 429 with `Retry-After: 60`. The
  limiter reuses the login limiter's shape (`internal/api/authguard.go`) and takes the address from
  the connection, not from forwarded headers, unless `PM_TRUST_PROXY` (existing behaviour) applies.
- R8. A board lists at most 200 devices; `PUT` refuses more. The response is at most about 100 KB.
- R9. Changing the configuration is `PUT /api/settings/public-status` (admin role, API tokens
  refused as for `/api/settings/directory*`); reading it is `GET` on the same path (admin). Every
  change writes a security log entry `public.status.changed` with the username, enabled flag,
  unlisted flag, device count and the switches; enabling writes `public.status.enabled`.
- R10. `PUT` validates that every device id exists and is visible to the caller under group scope,
  and refuses otherwise; a device deleted later simply disappears from the board.
- R11. The public page (`/status`, `/status/<slug>`) renders without a session: it never calls
  `/api/auth/me` or any authenticated route, shows no navigation, and keeps the brand.
- R12. The public page re-fetches every 30 seconds and shows "Updated N s ago"; after three failed
  fetches it shows "Status unavailable" in place of stale data.

Administration
- R13. The Settings section shows: enabled switch; unlisted switch with the current address and a
  "Regenerate" button; board title (default "Printer status", 60 characters max); framing origins
  (one per line, validated as origins); the device picker (search, group filter, add and remove,
  drag to reorder is not required, up and down buttons are); per device an optional public label
  (80 characters max); switches for location, up/down, state, paper, toner, alerts.
- R14. The section shows a live preview link that opens the board in a new tab, and a one-line
  summary of what the public will see ("12 printers; location, up/down, paper, toner").
- R15. Saving with zero devices is allowed but shows a warning that the board will be empty.

Public page design (apply `steel-and-signal-brand`; a design pass is a plan step)
- R16. Light surface (Mist/white, Graphite ink), Cobalt for the one accent, status colours from the
  console's existing tones (success, warning, critical) and never brass as identity. Barlow for the
  title and device labels, IBM Plex Sans for the rest.
- R17. One row or card per device: label, optional location, then the enabled signals as compact
  pills or small bars; paper per tray and toner per supply. Phone width stacks to one column; a
  wall mode (`?wall=1`) enlarges type for reading at three metres and pages through devices when
  more than 12 are listed, 10 seconds per page.
- R18. Empty, disabled and error states each have a short sentence and the icon; no stack traces,
  no instance details.

## Design
- **Packages.** `internal/api/public.go` (routes, projection, cache, limiter, category mapping),
  `internal/api/public_test.go`; `internal/api/server.go` registers the routes outside `require`;
  `secureHeaders` gains a path check for the board (R-decision 3). No change to `internal/fleet`,
  `internal/store` or drivers. No migration: the configuration is the settings key `public.status`
  via `Store.SetSetting`, the same pattern as `dashboard.layout.default`.
- **Configuration shape** (`public.status`): `{ enabled, unlisted, slug, title, frameOrigins[],
  devices: [{ id, label }], show: { location, online, state, paper, toner, alerts } }`.
- **Routes.** `GET /api/public/status`, `GET /api/public/status/{slug}` (no auth);
  `GET|PUT /api/settings/public-status` (admin). `GET /status`, `GET /status/{slug}` are SPA paths
  served by the embedded UI as today.
- **Projection.** `buildPublicBoard(cfg, devices, alerts) publicBoard` is a pure function from
  loaded records to the response struct; the handler loads devices by the configured ids (any order,
  then re-ordered to the configuration) and active alerts for them, then calls it.
- **UI.** `web/src/pages/PublicStatus.tsx` mounted in `App.tsx` before the auth gate when the path
  starts with `/status`; `web/src/pages/settings/PublicStatus.tsx` as the new section; styles in
  `web/src/styles.css` under a `.pub-` prefix. The web `Device` type is not used by the public page;
  it gets its own `PublicBoard` type matching R2.
- **Wiki.** `API-reference.md` (two endpoints and the settings route), `Monitoring.md` or the
  dashboard page (how to set up the board), a security note on the boundary.

## Flagged concerns
- **First unauthenticated data route.** Mitigated by R1 to R8 and the key-set test; the review
  should read `public.go` against `REVIEW.md`'s security section specifically.
- **Framing override** touches `secureHeaders` for every request; the path check must be exact
  (`/status` and `/status/` prefix only) and covered by a test that the console pages keep DENY.
- **Unlisted mode is obscurity, not authentication.** The Settings copy says so in one sentence.
- **Preview before save** would need the unsaved configuration to be rendered; this version
  previews the saved state only (R14).

## Out of scope
Several boards; authentication for the board; history or charts; links from the board into the
console; per-group boards; embedding security or compliance; push updates over WebSocket;
localisation beyond the console's language.
