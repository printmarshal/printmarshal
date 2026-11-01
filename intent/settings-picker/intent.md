# Intent: a searchable settings picker with plain explanations

Author: the maintainer (product owner), from building a configuration profile, 31 October 2026. Status: accepted by the owner's request to build it.

## Problem
Adding a setting to a configuration profile means scrolling a single dropdown of about 80 entries.
It is hard to find anything in it, and the names alone do not tell a normal user what most settings do.

## Proposed outcome
In the owner's words: instead of a dropdown, something we can drag and drop, that is searchable, where
each one has an explanation of what it does.

## Affected users and systems
Operators writing profiles. `internal/model`, `internal/api/config.go`, `web/src/components/config/`.

## Constraints
Secret settings stay out of profiles. Choosing a setting only edits the draft; applying a profile is
still a dry run first. No change to the configuration write path.

## Open questions
None.
