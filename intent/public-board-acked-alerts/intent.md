# Intent: the public board keeps a fault until it clears

Author: the maintainer (product owner), 2 November 2026. Status: accepted by the owner's request to build it.

## Problem
The public status board leaves out acknowledged alerts. Acknowledging records that someone knows about
a condition; it does not resolve it. So acknowledging a paper jam on a printer that still answers polls
makes the board read Ready while the printer is jammed, and someone walks to a printer that cannot print.

## Proposed outcome
An alert stays on the board until it clears, whether or not anyone has acknowledged it.

## Affected users and systems
People reading the board, and administrators who acknowledge alerts. `internal/api/public.go`, the
settings hint, the wiki. The console's Alerts page and acknowledgement itself do not change.

## Constraints
The board still sends only a severity and a fixed category, never the alert text, and still reads only
the store. Acknowledging stays a staff action that the public cannot see.

## Open questions
None. Alerts on decommissioned printers and boards with the alerts switch off are separate questions.
