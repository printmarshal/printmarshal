# Review instructions

Applies to every pull request. Run three passes and tag each finding with its pass.

## Passes
- **Bugs**: logic errors, broken edge cases, races (the engine, jobs and the firmware campaign
  run concurrently), regressions in behaviour the tests do not cover.
- **Security**: trust boundaries crossed by unauthenticated network peers, printers and
  anything spoofing one, viewer and operator roles, scoped API tokens and identity-provider
  responses; injection, SSRF in device-URL probes, path traversal in uploads, secrets in logs or
  diffs, missing authorization on a device-owned route.
- **Compliance**: the change does what `intent/<slug>/intent.md` asked, matches `spec.md` and
  `plan.md` where they exist (and `plan.md` was updated in the same commit when the
  implementation departed), and keeps the principles: read-only by default, every device write
  dry-run first and read back, unknown is never pass, operator fields never overwritten by scans,
  no product-direction material in the repo.

## What Important means here
Reserve Important for findings that would break behaviour, write to a device without the gate,
leak a secret or credential, let a lower-trust principal cross a boundary, or publish material
that must stay offline. Style, naming and wording are nits.

## Cap the nits
Report at most five nits per review and summarise the rest as a count.

## Do not report
`internal/web/dist/`, `brand/png/`, `brand/icons/png/`, `testdata/fixtures/`, formatting that
`gofmt`, `go vet` and the web typecheck already enforce in CI, and the research reports under
`research/` (engineering evidence, not code).
