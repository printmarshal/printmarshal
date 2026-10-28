# PrintMarshal (PM) — UI design prompt

Paste everything below into a Claude design session.

---

You are designing the complete web UI for **PrintMarshal (PM)**, a self-hosted web
application for managing a fleet of network printers. Design a modern, professional,
information-dense operations console — high-fidelity mockups of the key screens, a
reusable component library, and a cohesive visual system, in both light and dark
themes.

## Product in one paragraph

PrintMarshal lets an IT or print-services team manage a fleet of network printers
(from tens to several thousand devices, across one or more sites) over their whole
lifecycle: **discover, inventory, monitor, configure, secure, update firmware, and
retire.** It manages *devices*, not print jobs — it is **not** a print-accounting,
quota, or pull-printing tool, so there are no cost, user-quota, or job-release
screens. It is vendor-agnostic: it manages printers from any manufacturer, so the UI
must never show a specific brand's logo, product name, or branding.

## Who uses it

- **Operators / fleet admins** (primary): live all day in the dashboard and
  inventory; triage alerts, find a device fast, push configuration, run security
  assessments, schedule firmware. Value speed, density, and bulk actions.
- **Viewers / help desk**: read-only — check a device's status and supplies, look up
  an asset.
- **System admins**: manage users/roles, credentials, notifications, and system
  settings.

Design for the operator first. This is a tool people use for hours, not a landing
page.

## Design principles

1. **Calm operations console.** Trustworthy, efficient, data-forward. Think a modern
   monitoring/observability or device-management dashboard — not a consumer app, not
   marketing. No decorative illustration, no hero imagery.
2. **Density with legibility.** Operators scan large tables and many devices at once.
   Favor compact, scannable layouts with a comfortable/compact density toggle.
3. **Status at a glance.** Health, severity, and compliance must read instantly
   through consistent color + icon + label (never color alone — accessibility).
4. **Safety is visible.** Any action that changes a device (config push, security
   remediation, firmware) is clearly distinguished from read actions, and always flows
   through a review/confirmation step. Show dry-run previews before real writes.
5. **Respect operator data.** Fields a person edits (display name, location, asset
   tag, notes) are visually distinct from auto-discovered fields and are never
   silently overwritten.
6. **Live, not noisy.** Status and job progress update in real time with subtle,
   non-jarring transitions.

## Information architecture (left sidebar nav)

Collapsible sidebar (icon+label ⇄ icon-only), with these sections:

- **Dashboard** — fleet overview and health.
- **Devices** — the inventory (list + detail).
- **Alerts** — active and historical alerts.
- **Groups** — static and rule-based device groups.
- **Configuration** — configuration profiles, apply, and drift.
- **Security** — policies, assessments, compliance, certificates, credentials.
- **Firmware** — version inventory and update campaigns.
- **Jobs** — running and past tasks; the activity/audit trail.
- **Discovery** — run and review discovery.
- **Reports** — utilization and supply forecasting.
- **Settings** — users/roles, notifications, system.

Top bar: global search (jump to any device by name/IP/serial/model), environment/site
switcher, a live connection indicator, notifications bell, current user + role.

## Screens to design

For each, show the populated state plus the empty, loading, and error states where
noted, and reflect role permissions (viewer sees read-only; write actions appear only
for operator/admin).

1. **Dashboard / fleet overview**
   - KPI stat tiles: total devices, online/offline, devices with active alerts,
     critical vs warning counts, low/empty supplies, out-of-compliance devices,
     firmware-updates-available.
   - A fleet health strip or donut (online / warning / critical / offline).
   - Active critical-alert banner(s) at the top.
   - Panels: recent alerts, devices needing attention, supplies running low,
     in-progress jobs, recent activity.
   - Auto-refresh with a countdown + manual refresh; tile ⇄ list affordances.

2. **Device inventory (list)**
   - Toggle between a **data table** and a **card/tile grid**.
   - Table columns: status pill, display name, model, IP/hostname, location, supply
     summary (mini multi-color bar), active alerts, compliance, firmware, last seen.
     Column chooser, sort, sticky header, row density toggle.
   - A filter bar: status, model, vendor, group, location, alert severity, compliance,
     supply level; plus free-text search. Saved filters.
   - **Bulk selection → bulk actions** (poll now, add to group, apply profile, run
     assessment, schedule firmware, export). Show a selection action bar.
   - Design for **thousands of rows** (virtualized list, pagination or infinite
     scroll, result count, and a clear note if results are capped).
   - Empty state (no devices yet → prompt to run discovery).

3. **Device detail**
   - Header: display name (inline-editable), status pill, model, IP/hostname, serial,
     a row of primary actions (Poll now, Edit, Apply profile, Assess, Update firmware),
     and Previous/Next to cycle the current list.
   - **Identity & asset** card: model, serial, MAC, firmware, device type, discovered
     vs operator-set fields clearly marked; editable display name, location, asset tag,
     notes.
   - **Supplies** card: each consumable as a color-matched level bar (e.g. C/M/Y/K,
     drum, fuser, waste) with % and a small history sparkline.
   - **Trays / media** card: per-tray OK/low/empty, size and media type.
   - **Counters** card: total / mono / color / duplex with trend charts over time.
   - **Status & alerts** card: current device message and recent alerts for this
     device.
   - **Configuration** tab: current settings grouped by area; drift vs assigned
     profile.
   - **Security** tab: compliance summary, failing policy items, certificate list with
     expiry.
   - **Firmware** tab: installed version, available update, update history.
   - **Activity** tab: this device's job/audit history.

4. **Alerts**
   - List with severity filter (critical / warning / ok / all), grouped or flat.
   - Each alert: severity pill, device, description, source (polled/trap/push),
     first/last seen, acknowledge and clear actions.
   - Bulk acknowledge. Empty state = "all clear."

5. **Groups**
   - List of groups (name, type: static vs rule-based, member count).
   - Group editor: for rule-based, a condition builder (model is X, subnet in Y,
     capability = Z); live member preview. Static: a device picker.
   - Group detail shows members and the profiles/policies targeting the group.

6. **Configuration**
   - **Profiles** list (name, settings count, assigned groups, last applied).
   - **Profile editor**: settings organized by area (Network & I/O, Protocols &
     discovery, Security & access, Energy/sleep & clock, Trays & media, Control panel).
   - **Apply flow**: pick target (device/group) → **dry-run preview** showing
     per-setting before/after and which devices would change → confirm → **results**
     view with per-device, per-setting success/fail.
   - **Drift**: a device/group compared to its assigned profile, highlighting drifted
     settings, with a re-apply action.

7. **Security**
   - **Policies** list (name, item count, assigned targets, last assessed, compliance).
   - **Policy editor**: policy items (desired security settings) with assess +
     remediate definitions.
   - **Assessment results**: a compliance dashboard — overall score, pass/fail by item,
     by device, by group; a clear per-device compliance meter; filter to failing.
   - **Remediate flow**: same safe-write pattern as configuration (dry-run → confirm →
     per-item results), with a prominent warning that remediation can change access.
   - **Certificates**: fleet certificate inventory with **expiry timeline/indicators**
     (expiring-soon warnings), enroll/renew actions.
   - **Credentials**: managed credential sets (SNMP, SNMPv3, admin passwords, cert
     material) — write-only inputs, never display secret values; scope (device/group);
     last-used.

8. **Firmware**
   - **Versions**: firmware inventory across the fleet (version distribution by model,
     how many devices are behind).
   - **Campaigns**: create a staged rollout — select target, image, schedule,
     maintenance window, concurrency, canary group. A campaign monitor showing
     per-device progress/status, with pause/cancel and clear risk messaging.

9. **Jobs / activity**
   - Running jobs with live progress bars and per-device results; job history.
   - The **audit trail**: who changed what on which device(s), when, and the outcome.
   - Filter by type, actor, device, outcome, time.

10. **Discovery**
    - Start a run: choose methods (zero-config listen, active scan by IP/range/CIDR),
      with credentials to try. Show a live progress view (addresses probed, devices
      found/identified) and the result set with an "onboard selected" action.

11. **Reports**
    - Utilization reports (page volume over time by device/group), supply consumption
      and **low-stock forecasting**, compliance-over-time. Exportable (CSV/PDF).

12. **Settings**
    - **Users & roles** (viewer / operator / admin), invites, role assignment.
    - **Notifications**: webhook (Slack/Teams/Discord/generic) and email, per event
      type, with test-send.
    - **System**: polling cadence, backup/restore, data retention, appearance (theme,
      density), timezone.

13. **Sign in** — a clean, minimal self-hosted login (no marketing).

## Reusable component library (design these as a system)

- **Status pill** (online / warning / critical / offline / unknown) — color + icon +
  label.
- **Severity badge** (critical / warning / ok / info).
- **Supply-level bar** — single and grouped (multi-cartridge), color-matched to the
  consumable, with %, low/empty thresholds, and an optional sparkline.
- **Tray status** indicator (OK / low / empty per tray).
- **KPI stat tile** (value, label, delta/trend, click-through).
- **Data table** — sortable, column chooser, sticky header, row selection, bulk-action
  bar, density toggle, virtualized for large sets, inline status.
- **Device card** (grid view) — name, status, model, supplies mini-bar, alert count.
- **Filter bar** + saved-filter chips.
- **Compliance meter / score** ring and bar.
- **Certificate-expiry indicator** / timeline.
- **Progress / job card** with per-device breakdown.
- **Dry-run / diff preview** (before → after, per-setting, per-device).
- **Confirmation dialog** for destructive or device-changing actions (clear statement
  of blast radius: how many devices, what changes).
- **Alert banner** (page-level and device-level).
- **Forms**: setting rows with current vs desired, validation, write-only secret
  fields, "operator-set vs discovered" field markers.
- **Empty / loading / error / permission-denied** states for lists and detail panels.
- **Sparkline / trend chart** and small time-series charts (counters, supply history).

## Visual system

- **Aesthetic:** clean, neutral, enterprise-grade; a restrained accent color; generous
  use of a neutral gray scale; subtle borders and elevation; no gradients-as-decoration,
  no heavy shadows. Icon set: a single consistent line-icon family.
- **Both light and dark themes**, with a single token set that works in each. Define
  color as tokens: neutral surface/background/border/text scale; semantic **status**
  tokens (good/green, warning/amber, critical/red, info/blue, offline/gray); one
  **accent**; and an accessible **categorical palette** for charts.
- **Color is never the only signal** — always pair with icon and/or text. Target WCAG
  AA contrast.
- **Typography:** a clean UI typeface; a clear type scale (page title, section, body,
  caption); tabular figures for metrics and counters.
- **Spacing/layout:** an 8px spacing system; consistent card and table paddings; a
  comfortable + compact density option.
- **Responsive:** optimized for desktop (wide tables, multi-panel detail); must remain
  usable down to tablet width (stacked panels, horizontally scrollable tables). It is a
  LAN admin tool, so desktop-first is fine, but don't break on smaller widths.

## Constraints

- **Brand-agnostic.** No real manufacturer names, logos, or branding anywhere — not in
  nav, device rows, or sample data. Use neutral placeholder model names
  (e.g. "Model A4-320", "MFP C-560").
- **Device-centric, not job-centric.** No print-accounting, quota, cost, or
  job-release UI.
- **Self-hosted tool.** No cloud-signup, pricing, or marketing surfaces.

## What to deliver

- High-fidelity mockups of at least: Dashboard, Device inventory (table + card),
  Device detail, Alerts, Configuration apply (with dry-run), Security assessment
  results, and a Firmware campaign monitor.
- The component library and the color/type/spacing system, in light and dark.
- A short rationale for the layout and visual decisions.

It will be implemented as a TypeScript single-page app, so keep components and states
realistic and implementable; you do not need to pick a specific component framework.
