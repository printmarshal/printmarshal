# How PrintMarshal is built: the AI-native SDLC loop

Adopted 30 October 2026 from Anthropic's *AI-Native SDLC Playbook*. The six stages run as a
loop, each stage ends by committing an artifact the next stage reads, and the git history of
those artifacts is the audit trail: who asked for what, what the agent produced, who approved.
Human judgement sits above the loop at the gates; the agent acts up to each gate and never past
it. The owner (the repository's maintainer) is the product owner and the merge authority.

| Stage | Artifact committed | Gate | Who decides |
| --- | --- | --- | --- |
| Plan | `intent/<slug>/intent.md` | Intent accepted (merged) | Owner |
| Design | `intent/<slug>/spec.md` with flagged concerns | Spec accepted | Owner, with a technical lead for higher-risk work |
| Build | `intent/<slug>/plan.md`, then the diff and its tests | Plan accepted before code; plan updated when the diff departs | Owner or the engineer running the session |
| Test | Test output, typecheck, screenshots in the PR | Verification shown before "done" | The session (self-check), then the reviewer |
| Deploy | The PR with its review findings | Review against `REVIEW.md`; merge | Owner, through branch protection |
| Maintain | A new `intent.md` from an incident or measurement, plus a regression test | Triage: fix now, schedule, dismiss | Owner |

## Plan
An idea, a GitHub issue, a support case or a measurement becomes `intent/<slug>/intent.md`
(template in `intent/`): the problem with evidence, the proposed outcome, affected users and
systems, constraints, open questions, and the issue number. It is written in the originator's
words and accepted by merging. GitHub issues remain the record; the folder is the working copy,
and each file links the issue (linkage is the minimum bar). Product direction, pricing, tiers,
naming and company matters never enter this repository; they live in the owner's private
research repository.

## Design
From an accepted intent the agent writes `spec.md` in one session with the project skills
loaded (`device-write-safety`, `steel-and-signal-brand`, `sdlc-artifacts`, `frontend-design`
for anything visual): numbered testable requirements, the design, flagged concerns where a
policy strains. The owner reviews the spec against the intent and resolves the concerns first.
A one-file fix may skip the spec; the plan says so.

## Build
Sessions start in plan mode with the intent and spec, and iterate `plan.md` (files that change,
order of work, risks, proof) until an engineer who never saw the conversation could implement
from it. The accepted plan is committed before code. When the implementation departs, the plan
is updated in the same commit. `CLAUDE.md` is the new-joiner page every session reads; when
the agent makes a mistake twice, the correction goes there. Institutional knowledge that must
apply consistently is a skill under `.claude/skills/`; anything that must hold without
exception is a hook in `.claude/settings.json`:

- `gate.sh` blocks pushes to `main`, force-pushes of `main`, history rewrites, merges from the
  agent, and device writes (SNMP SET, IPP operations, firmware posts) to anything but the
  simulator on loopback.
- `protect-paths.sh` blocks edits to generated output (`internal/web/dist/`, rendered brand
  PNGs), to dependency lockfiles outside an upgrade task (`PM_ALLOW_DEPS=1`), and to test files
  during a fix task (`PM_FIX_TASK=1`).
- `format.sh` runs `gofmt` on every Go file the agent edits.

Independent work runs in parallel sessions, each in its own worktree on its own branch; tasks
that share files run in one session, one after another. A verifier subagent
(`.claude/agents/verifier.md`) runs the app in a fresh context and reports whether the change
matches the plan.

## Test
Every session has a feedback loop: `make test` (vet and the Go suite), `npm run typecheck` in
`web/`, `scripts/e2e.sh` for the server and simulator together, and a screenshot for anything
visual. The quantified target comes from the plan's proof section. Bug fixes write the failing
test first, commit it, then fix without touching the test (`PM_FIX_TASK=1`). Verification output
is pasted into the report and the PR. Continuous evals of the agent configuration (a suite that
runs when `CLAUDE.md`, skills or hooks change) are not set up yet; they need an API key with a
budget and 20–50 recorded tasks, and are the next step once this loop has run for a while.

## Deploy
Every change arrives as a PR from a branch; there is no path to `main` for the agent. PRs are
reviewed against `REVIEW.md` (bugs, security, compliance with intent, spec, plan and the
principles; Important versus Nit; at most five nits) and the owner merges. Review findings that
recur go into `CLAUDE.md`. The website deploys from `main` through the Pages workflow and the
wiki is synced from `wiki/` after a merge; both are the owner's gate because the merge is.
There is no production deployment of the server itself from this repository (operators run
their own), so the release gate is the merge plus a tagged release when one is cut.

## Maintain
Incidents, measurements from operators (for example a fleet scan that still drops printers) and
review findings come back into the loop as a new `intent.md`, and each fixed incident adds a
regression test. Automated control bands on a metric (CI failure rate, PR cycle time) with a
deterministic detector that invokes the agent read-only at 2σ and lets it open a PR at 3σ are
the playbook's closing step; they are not set up yet.

## Measuring it
From git and PR history: time from first conversation to a committed intent; time from intent
to spec; share of changes merged from the first implementation pass; how often the merged diff
still matches `plan.md`; first-pass CI success for agent-written changes; review time per PR;
review findings that cite a policy a skill should have applied (should fall toward zero).
