# Plan: security audit run 1 fixes (from intent.md 2026-10-31)

## Files that change
internal/api/{scope.go,scopeof.go,sync.go,ws.go,server.go}, internal/discovery/targets.go, internal/config/config.go, cmd/printmarshal/main.go, internal/alerts/{notify.go,smtpauth.go}, docs/{export.md,auth.md,events.md}, tests beside each.

## Order of work
1. Failing tests for each lead, committed first.
2. Scope: `peekBody` regardless of Content-Type; `readJSON` rejects trailing data; tokens denied on `/api/settings/directory*`; credential scope rule (`credentialScopeString`), list filter; `deviceIDOf` shared by sync and the hub; sync drops out-of-scope rows and refuses jobs/audit to scoped principals; hub keeps a per-subscriber filter refreshed on the re-authorization tick.
3. Range loop stops on the end address; `RedactDSN`; `mailMessage`/`headerText`; `requireTLS` for LOGIN and XOAUTH2.
4. Docs.

## Risks
Scoped sync clients no longer receive tombstones (documented). SMTP notifiers configured without STARTTLS stop authenticating until fixed (intended; the error names the setting). The stream filter drops job events for scoped subscribers.

## Proof
`go test ./...` green, including the eight new tests that failed before the fixes.
