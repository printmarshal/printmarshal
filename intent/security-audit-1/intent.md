# Intent: close the authorization and robustness gaps from security audit run 1

Author: the maintainer, via the 30 October 2026 audit (Cloudflare security-audit skill). Status: accepted. Issue: none; the audit record is private.

## Problem
A source-only audit of main at 43a21d9 left fifteen leads unverified. Verification by test showed eight real gaps: alternate readers (the sync API, the live stream) ignored a token's group scope; API tokens could reconfigure directory sign-in; a group-scoped admin token could create, change or delete credentials bound outside its scope; the body scope check could be skipped with a non-JSON Content-Type or trailing data; a discovery range ending at 255.255.255.255 never terminated; an anonymous login attempt's username reached the mail Subject header unescaped; LOGIN and XOAUTH2 ran without TLS; the startup log carried the full database DSN.

## Proposed outcome
Every reader applies the same group scope as the primary lists; tokens cannot touch sign-in providers; credential mutations respect scope; the scope check sees every JSON body the handlers decode; ranges terminate; mail headers are safe; clear-text SASL refuses without TLS; logs carry a redacted DSN.

## Affected users and systems
Scoped API tokens and their integrations (sync mirrors, stream consumers), administrators using tokens for automation, SMTP notifiers without STARTTLS (they now fail with a clear error until `startTls` is enabled).

## Constraints
No change to session users' behaviour; no device I/O involved; failing tests committed before the fixes.

## Open questions
Seven further leads (campaign membership drift, print-provider sync scope, IPP pin ordering, EST chain pinning, SNMP walk cancellation, device identity rebinding) remain unverified and need their own intents.
