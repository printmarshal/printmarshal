# Intent: UI test pass, round 2 fixes

Issues: #64, #65, #66, #67, #68, #69, #70, #71, #72.

## Problem
A second read-through of the UI, on a simulated fleet, found nine problems. Evidence is in the issues.
- Switching report tabs blanks the whole application until the page is reloaded (#64).
- One failed dashboard refresh replaces the dashboard with an error and removes the way to retry (#71).
- The public board shows no up or down label that works without colour (#66).
- The public board calls a low drum or maintenance part "toner low" (#67).
- Public board audit entries do not say what changed (#72).
- An operator who opens the public board settings address silently gets Notifications (#68).
- Device Activity cuts the change data short and offers no full entry, unlike Settings (#69).
- The compliance report says the security feature has not landed, though it has (#70).
- Opening Configuration on a decommissioned printer still tries to read the device (#65). A test
  instance also kept a real address in its inventory, which is a fixture problem.

## Proposed outcome
Every report tab loads with navigation intact. A failed refresh keeps what is on screen and offers a retry.
The board names online state in words and names the right part. Audit entries show what changed, without
exposing the unlisted address. Settings explains refused sections. Activity matches the audit trail. The compliance
message is true. A decommissioned printer is not contacted when someone looks at its record.

## Affected users and systems
Operators and administrators in `web/src`. Anonymous viewers of the public board. `internal/api`, `internal/reports`.

## Constraints
- The public board's security boundary stays as specified: anonymous output stays allow-listed, and alerts stay a
  severity and a fixed category, never free text.
- The slug of an unlisted board never enters the audit log, because operators can read it.
- "Unknown" is never "pass". Nothing here writes to a device.

## Out of scope
#47, #52 and #54, which wait on separate decisions.
