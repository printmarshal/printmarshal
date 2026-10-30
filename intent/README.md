# Intent

This folder is the Plan-stage artifact home from `docs/sdlc.md`: one directory per change,
named by a short slug, holding `intent.md` (what is wanted, why, under which constraints),
then `spec.md` (requirements and design, with flagged concerns) and `plan.md` (files that
change, order of work, risks, proof) as the change moves through the loop. The git history of
these files is the audit trail: who asked, what was decided, who approved.

Rules:
- Link the GitHub issue or PR number in `intent.md`; GitHub stays the record, this folder is the
  working copy (linkage is the minimum bar).
- Engineering intent only. Product direction, pricing, tiers, naming and company matters do not
  go here; they live in the owner's private repository.
- An intent is accepted when the owner merges it; acceptance starts the Design stage. A plan is
  accepted when the owner says so in the PR or the session; acceptance starts implementation.
- When the implementation departs from `plan.md`, update the plan in the same commit.

Templates: `TEMPLATE-intent.md`, `TEMPLATE-spec.md`, `TEMPLATE-plan.md`.
