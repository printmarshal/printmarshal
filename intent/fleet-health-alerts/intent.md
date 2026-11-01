# Intent: a printer with an active alert is not Healthy

Author: the maintainer (product owner), from QA on 31 October 2026. Status: draft. Issue: #47.

## Problem
A device's health comes only from what the printer reports about its operating state. Alerts the
console raises itself, such as a supply rule, do not change it. In QA a printer with a black drum
at 5% and an active critical alert showed green Online on its own page, counted as Healthy on the
dashboard, and was missing from Devices needing attention. The separate low-supplies widget did
show it, so the console contradicted itself. The same is true of a warning: "0 warning" can sit
next to "14 active alerts".

## Proposed outcome
A device's severity is the worse of what the printer reports and its active operating alerts.
Fleet health, Devices needing attention, the Critical and warning counts, the inventory's status
pill, severity filter and sort, the CSV export and the metrics all use that one answer. The
attention table says why the device is there: the alert's own text, for example
"Black Drum HP W9054MC at 5%".

## Affected users and systems
Everyone who reads the dashboard or inventory. `internal/store` (both backends), `internal/model`,
`internal/fleet`, `internal/api`, `web/src/components/ui.tsx`, `web/src/dashboard/widgets.tsx`.

## Constraints
- Discovery and polling still never write to a device; this only changes how stored state is read.
- `status.severity` keeps meaning what the printer reported. It is not overwritten.
- An offline device stays Offline, whatever alerts remain from before it stopped answering.
- The public board has its own projection and does not change.

## Open questions
Settled in the plan: which alert sources count, and whether acknowledging an alert restores Healthy.
