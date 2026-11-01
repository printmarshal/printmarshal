# Intent: exported CSV cannot run as a spreadsheet formula

Author: the maintainer (product owner), 1 November 2026. Status: accepted by the owner's request to build it.

## Problem
The device, supplies, consumables and report exports write values as they are stored. Several come
from outside the console: a printer's name, model, location and supply descriptions are read from
the device over the network. A value starting with `=`, `+`, `-` or `@` becomes a live formula when
an administrator opens the file in a spreadsheet, so whoever controls a printer's name can reach
the machine of whoever opens the export.

## Proposed outcome
Every CSV the console writes neutralises such values with a leading apostrophe, the common defence.
Real numbers, including negative ones, stay numbers. The console's own importers remove the
apostrophe, so exporting and importing again changes nothing.

## Affected users and systems
Anyone who exports CSV. `internal/api` (four exports, one import), `internal/importcsv`, a new
`internal/csvsafe`.

## Constraints
Stored values do not change, and neither does the JSON API. Standard library only.

## Open questions
None.
