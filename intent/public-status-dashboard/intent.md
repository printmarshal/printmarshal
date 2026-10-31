# Intent: a public status dashboard, configured by administrators

Author: the maintainer (product owner), 31 October 2026. Status: draft. Issue: none yet (open one when accepted).

## Problem
Everything PrintMarshal shows is behind a sign-in. People who use the printers but do not run
them (staff in an office, students in a library, visitors in a print room) have no way to see
whether a printer is up, has paper or has toner before they walk to it or send a job. Today the
only options are to give those people a viewer account, which exposes the whole inventory and
needs a login, or to answer the question by hand. An administrator cannot publish a reduced,
read-only view of the fleet at all.

## Proposed outcome
An administrator can turn on a public status dashboard from Settings and decide exactly what it
shows: which printers appear, and for each kind of information whether it is shown at all,
namely whether the printer is up or down, whether it has paper, whether it has toner, and
whether active alerts are shown. The page is viewable without signing in, at a stable address
the administrator can put on a sign or a wall screen. It is off by default and shows nothing
until an administrator has chosen printers and fields. Everything else in the console stays
behind the sign-in exactly as today.

## Affected users and systems
Administrators (configure it; `web/src/pages/Settings.tsx`, a new section); the public (read
it; a new session-less SPA route and page); `internal/api` (one new unauthenticated endpoint,
an admin settings endpoint, the route registration in `server.go`); the settings store (new
keys); the security event log (configuration changes); `secureHeaders` if the page may be
embedded on a signage screen; the API reference and the wiki's monitoring page.

## Constraints

### The anonymous endpoint is a different kind of surface
Every API route today runs through `require`, which authenticates, checks the role, applies
token restrictions and group scope. The public endpoint is the first data route that does
none of that, so its boundary has to be explicit rather than inherited:

- **Off by default, absent when off.** When the feature is not enabled the endpoint answers
  404 like any unknown path and the public page says the board is not available. Enabling it is
  an administrator action, recorded in the security log with who did it and what was chosen,
  and API tokens may not change it (the same rule as the directory settings).
- **Allow-list in, projection out.** The response is built server-side from the administrator's
  list of device ids and the per-field switches. It is a new, purpose-built structure, never a
  filtered `Device`: a field reaches the public response only because the code puts it there.
  For each shown device: a display name, the location if the administrator chose to include it,
  and only the enabled signals (up/down; paper per tray as ok/low/empty; toner per supply as a
  percentage or ok/low/empty; active alerts). Nothing else leaves: no IP address, hostname,
  serial number, MAC, firmware version, model or vendor unless explicitly enabled, no group,
  tag, asset tag, note, credential reference, compliance or security data, no event history,
  no user or instance information. A test asserts the exact set of JSON keys so that a later
  field added to `Device` cannot leak by accident.
- **Alert text is device-originated free text.** Alert descriptions come from the printers and
  from operators; the public view shows a severity and a short fixed category rather than the
  raw description unless the spec argues otherwise. Acknowledged and cleared alerts are not
  shown. Security assessment findings are never shown publicly, whatever the switches say.
- **Public requests never cause device contact.** The endpoint serves a snapshot that the
  server rebuilds on its own schedule from data the pollers already hold; a flood of public
  requests cannot trigger polls, discovery or any write. The snapshot carries a "last updated"
  time so a stale board is visibly stale.
- **Cheap to serve, hard to abuse.** Responses are small, cached and rate-limited per client
  address; no WebSocket stream and no per-request database fan-out across the fleet. The
  endpoint accepts no parameters that select data (no device id, no filter): one board, one
  response.
- **Separate from the signed-in SPA state.** The public page does not call `/api/auth/me` or
  any other authenticated route, does not show navigation, and keeps the same brand.
- **Scope rules still hold for the administrator.** The device picker shows only devices the
  configuring administrator may see; a group-scoped administrator cannot publish devices
  outside their scope.

### Other constraints
- Read-only end to end; nothing here touches a device.
- Removing a device removes it from the board; the settings never reference devices that no
  longer exist in a way that errors the page.
- No new dependencies.
- Works on a phone and on a wall screen (large type at a distance is a design concern for the
  spec, not an afterthought).

## Open questions
- Where does the configuration live: a new Settings section ("Public status", admin only) or
  inside the dashboard's edit mode? The request places it with the dashboard settings; today
  dashboard layout is edited on the dashboard page itself and Settings has no dashboard section.
- Should the public address be a fixed path (for example `/status`) or an unguessable slug the
  administrator can regenerate, as a middle ground between "anyone on the network" and
  "anyone with the link"?
- May the page be framed by a signage system? `secureHeaders` sends `X-Frame-Options: DENY`
  for everything today; relaxing it for one path only needs a decision.
- Should the administrator be able to give a device a public label distinct from its display
  name (for example "Library, ground floor, colour")?
- One board or several (per building, per group)? The first version assumes one.
- Should "up or down" mean reachable by the poller, or the device's own reported state
  (a printer in sleep mode is reachable but not "ready")?
