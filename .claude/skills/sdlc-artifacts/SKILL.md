---
name: sdlc-artifacts
description: Write the Plan/Design/Build artifacts of PrintMarshal's AI-native SDLC loop. Use when starting any new change, feature, bug fix or incident response, when asked for an intent, spec or plan, or before implementing anything that touches more than one file.
---

# SDLC artifacts

The loop is in `docs/sdlc.md`. Each stage ends by committing a file the next stage reads.

1. **intent.md** (`intent/<slug>/intent.md`, from `intent/TEMPLATE-intent.md`): problem with
   evidence, proposed outcome, affected users and systems, constraints, open questions, the
   issue number. Written in the originator's words; the owner accepts it by merging.
2. **spec.md** (same folder): numbered testable requirements, the design (packages, migration
   number, routes, UI states), flagged concerns where a policy strains, out of scope. Apply the
   `device-write-safety` and `steel-and-signal-brand` skills while writing it. Skip for a
   one-file fix; say so in the plan.
3. **plan.md**: files that change, order of work, risks (what it could break, the riskiest
   step, options not taken), proof (tests, commands, screenshots; for a bug the failing test
   that exists first). Start in plan mode, iterate until an engineer who never saw the
   conversation could implement from the plan alone, then commit it before coding.
4. When the implementation departs from the plan, update `plan.md` in the same commit.
5. Verification is part of done: `make test`, the web typecheck for UI changes, a screenshot
   for visual changes, pasted into the report.

Keep engineering material only in `intent/`; product direction, pricing, tiers, naming and
company matters go to the owner's private repository.
