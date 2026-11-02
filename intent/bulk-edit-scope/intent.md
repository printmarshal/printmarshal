# Intent: group-scoped users can bulk edit the devices in their groups

Author: the maintainer (product owner), 1 November 2026. Status: accepted by the owner's request to build it.

## Problem
The bulk edit spec says a group-scoped principal may edit devices in its groups and gets 403 for one
outside them. In practice every scoped principal gets 403, even for its own devices. The scope check
reads `POST /api/devices/bulk` as a device whose id is "bulk", so the request never reaches the check
that was meant to apply, and the existing test passes without testing it.

## Proposed outcome
A scoped principal's bulk edit of its own devices works. Naming any device outside its groups is
still refused, with nothing changed.

## Affected users and systems
Group-scoped users and API tokens. `internal/api/scope.go`.

## Constraints
No widening of what a scoped principal can reach. No device contact.

## Open questions
None.
