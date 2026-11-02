# Intent: the console serves its own fonts

Author: the maintainer (product owner), 1 November 2026. Status: accepted by the owner's request to build it.

## Problem
The console and its public status board load their fonts from Google. Every person who opens either
one tells a third party that they did, which a self-hosted product should not need to do. On a network
without internet access the fonts do not load at all and the page falls back to system fonts.

## Proposed outcome
The fonts ship inside the server. Opening the console or the public board contacts nothing but the
server itself.

## Affected users and systems
Everyone opening the console. `web/index.html`, `web/src/main.tsx`, `web/package.json`, the lockfile.

## Constraints
The same typefaces and weights, so nothing changes visually. The fonts' licences travel with them.
The public website in `site/` is a separate static site and is not part of this.

## Open questions
None.
