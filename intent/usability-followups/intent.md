# Intent: usability follow-ups from the UI test pass

Author: the maintainer (product owner), from QA on 31 October 2026. Status: draft. Issue: #54.

## Problem
QA listed places where the console works but is harder to read or use than it should be: tables
that cannot be sorted by most columns, addresses in text order (.10 before .2), tables that scroll
sideways on a phone, a bulk rename that cannot look up the DNS names it needs, internal terms such
as `opc` and `tonerCartridge`, a confusing "Out of paper: all Any trays are empty" alert, a default
tray shown as `-1`, draft and saved counts mixed on Groups, read-only pages that do not say they are
read-only, faint text and a white scrollbar in the dark theme, a sign-in that gives the same message
for empty fields and a wrong password, Reports counting a decommissioned printer without saying so,
a lone toner percentage on the public board, very long discovery errors, and a Regenerate button
whose immediate effect is only explained after pressing it.

## Proposed outcome
Each of those reads plainly and does what a person would expect. Any column in the device table
sorts from its header, in either direction, and addresses sort numerically.

## Affected users and systems
Everyone using the console. `web/src`, `internal/api` (device sort), `internal/alerts` (paper rule),
`internal/reports` (scope count).

## Constraints
- No new device contact except the DNS lookup a person asks for, which never talks to the printer.
- The default-tray change is display only, so nothing changes in the configuration write path.
- `status.severity`, operator fields and the public board's privacy rules do not change.

## Open questions
None. What was reviewed and left alone is listed in the plan.
