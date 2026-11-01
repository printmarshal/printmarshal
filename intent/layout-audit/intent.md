# Intent: personal dashboard layout changes appear in the audit trail

Author: the maintainer (product owner), from QA on 31 October 2026. Status: draft. Issue: #52.

## Problem
Saving or resetting a personal dashboard layout leaves no audit entry, while the same actions on the
default layout do. QA expected a record, or a statement that personal preferences are excluded, and
found neither.

## Proposed outcome
Saving and resetting a personal layout each record `dashboard.layout.personal` against the person
who did it, as the default layout already does.

## Affected users and systems
Everyone who customises a dashboard; `internal/api/dashboard.go`.

## Constraints
The entry holds counts only, as the default layout's does. Layout behaviour does not change.

## Open questions
None.
