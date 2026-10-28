# Authentication and authorization

PrintMarshal has three kinds of principal and two layers of authorization.

| Principal | How it signs in | Where it is managed |
| --- | --- | --- |
| Local user | username + password → session cookie / bearer session token | Settings → Users & roles |
| OIDC user | "Sign in with …" → identity provider → session | provisioned on first sign-in; role from the IdP |
| API token (service account) | `Authorization: Bearer pmt_…` | Settings → API tokens, `/api/tokens` |

Authorization is the role rank (viewer < operator < admin) plus an optional
**group scope** that limits a principal to the devices in named groups.

## Sessions

- `POST /api/auth/login` returns a session token and sets the `pm_session`
  cookie (HttpOnly, SameSite=Strict, `Secure` when `auth.secureCookies` /
  `PM_SECURE_COOKIES=true`). Sessions are stored as SHA-256 hashes and expire
  after `auth.sessionTtl` (30 days by default).
- The bearer form (`Authorization: Bearer <session token>`) is for scripts
  that sign in with a password; integrations should use an API token instead.
- A password change ends every other session of the account. Deleting a user
  ends all of them.
- Failed sign-ins are rate limited per source address
  (`auth.loginFailuresPerMinute`, default 10) and published as `auth.failed`.

## API tokens (service accounts)

An API token is a first-class principal for machines: a name, a role, an
optional group scope and an optional expiry. It is created once by an admin
and the secret (`pmt_` + 64 hex characters) is shown exactly once; the server
keeps only its SHA-256, the same way it keeps sessions.

```bash
# create (admin session)
curl -s -H "Authorization: Bearer $SESSION" -H 'Content-Type: application/json' \
  -d '{"name":"monitoring-exporter","role":"viewer","scope":["<group id>"],"ttl":"720h"}' \
  http://localhost:8080/api/tokens
# → {"token":{...},"secret":"pmt_…"}

# use
curl -s -H "Authorization: Bearer pmt_…" http://localhost:8080/api/devices

# list / revoke (admin)
curl -s -H "Authorization: Bearer $SESSION" http://localhost:8080/api/tokens
curl -s -X DELETE -H "Authorization: Bearer $SESSION" http://localhost:8080/api/tokens/<id>
```

Fields on create: `name` (required), `role` (`viewer` default, `operator`,
`admin`), `scope` (list of group ids; empty = every device), and either
`ttl` (Go duration, e.g. `720h`) or `expiresAt` (RFC 3339).

Rules:

- A token acts with its role everywhere a user of that role could, with
  these exceptions: it cannot call anything under `/api/auth/*` (no sessions,
  password changes or SSO) and cannot create or revoke tokens, whatever its
  role. Those requests answer 403 and are recorded as `auth.denied`.
- The secret is accepted in the `Authorization` header only, never as a
  cookie or query parameter.
- Revocation is immediate: the next request answers 401. Expired tokens
  behave the same. `lastUsedAt` is updated at most once a minute.
- Every creation and revocation is audited (`token.create`, `token.revoke`)
  and published as `auth.token.created` / `auth.token.revoked` security
  events; the secret never appears in either.
- In audit entries a token's actor is `token:<name>`, so it can never be
  confused with a human account of the same name.

## Group scope

A principal with a scope (today: API tokens; the field also exists on users
for a future admin UI) only reaches devices in those groups. Static groups
use their member list; rule groups are evaluated against every device.

What the scope enforces:

- `GET /api/devices`, the CSV exports, `GET /api/groups/{id}/devices`, the
  dashboard's attention list and `GET /api/alerts` return only in-scope
  devices / alerts.
- Every route under `/api/devices/{id}/…` (status, alerts, telemetry, config,
  security, certificates, firmware, supplies, poll, state, patch, delete)
  answers **403 `outside your group scope`** for a device outside the scope,
  whichever feature file serves it; the check sits in the shared `require`
  middleware.
- JSON bodies naming devices (`deviceId`, `deviceIds`, `devices`) on bulk
  endpoints (poll, configuration apply, firmware campaigns, security
  remediation, supply operations) are checked the same way, and
  `POST /api/poll` without an explicit device list (whole fleet) is refused.
- Each violation is recorded as `auth.denied` with the path and device.

What it does not do (by design, documented rather than hidden): the live
stream (`/api/stream`), job lists and results, consumables, credentials and
settings are fleet-wide resources and stay governed by the role alone. Give
scoped tokens the lowest role that does the job.

Session users have no scope unless one is set on the account; existing
behaviour for people is unchanged.

## OIDC single sign-on

PrintMarshal is a standard OpenID Connect relying party: authorization-code
flow with PKCE (S256), `state` and `nonce`, discovery from
`<issuer>/.well-known/openid-configuration`, JWKS fetched and cached (refreshed
hourly, and on an unknown key id at most once a minute), ID tokens verified
with the Go standard library (RS256 and ES256 only; anything else, including
`none` and HMAC, is rejected). No third-party library is involved.

### Configuration

```json
"auth": {
  "oidc": {
    "issuer": "https://login.microsoftonline.com/<tenant-id>/v2.0",
    "clientId": "<application (client) id>",
    "clientSecretEnv": "PM_OIDC_CLIENT_SECRET",
    "redirectUrl": "https://printmarshal.example.com/api/auth/oidc/callback",
    "displayName": "Corp SSO",
    "scopes": ["profile", "email"],
    "roleClaim": "roles",
    "roleMap": { "PrintMarshal.Admin": "admin", "PrintMarshal.Operator": "operator" },
    "defaultRole": "viewer",
    "allowedDomains": ["example.com"]
  }
}
```

Environment overrides: `PM_OIDC_ISSUER`, `PM_OIDC_CLIENT_ID`,
`PM_OIDC_CLIENT_SECRET_ENV` (the *name* of the variable holding the secret),
`PM_OIDC_REDIRECT_URL`. The secret itself lives only in the environment
variable `clientSecretEnv` names; a public client (PKCE only) leaves it
unset. The issuer must be `https://` (plain `http://` is accepted on loopback
for development only).

SSO is on as soon as `issuer` and `clientId` are set: the login page shows
"Sign in with <displayName>" above the local form, and `GET
/api/auth/providers` reports it. Local accounts keep working, so an admin
can always recover access if the IdP is unavailable.

### Redirect URL

Register exactly `https://<your host>/api/auth/oidc/callback` with the
provider (`redirectUrl` must match it character for character). Flow:

1. `GET /api/auth/oidc/start` stores state, nonce and the PKCE verifier in a
   10-minute `pm_oidc` cookie (HttpOnly, SameSite=Lax, path-limited to
   `/api/auth/oidc/`) and redirects to the provider.
2. The provider sends the browser to `/api/auth/oidc/callback?code=…&state=…`.
3. PrintMarshal checks the state, redeems the code at the token endpoint with
   the verifier (and the client secret, `client_secret_post`), verifies the ID
   token (signature, issuer, audience/`azp`, expiry, nonce), applies the
   domain filter, provisions or updates the user, issues a normal session and
   redirects to `/`. Failures go back to `/?oidc_error=<message>` and are
   published as `auth.failed`; successes as `auth.login`.

### Users and roles

- Username = `preferred_username`, else `email`, else the subject.
- First sign-in creates a local user with a random, unknown password; the
  account can only be used through the IdP (an admin can still reset the
  password to turn it into a local account). The creation is audited with
  `source: oidc`.
- Role: the values of `roleClaim` (a string or list; dot paths such as
  `realm_access.roles` work) are looked up in `roleMap`; the highest mapped
  role wins; otherwise `defaultRole` (viewer). When `roleClaim` and `roleMap`
  are set, the role is **re-applied on every sign-in**, so removing someone
  from the IdP group demotes them at their next login. Without a role
  mapping, roles are assigned once and managed locally.
- `allowedDomains` rejects sign-ins whose `email` domain is not listed (and
  any token without an email when the list is set).

### Provider notes

**Microsoft Entra ID.** App registration → Web platform, redirect URI as
above. Issuer `https://login.microsoftonline.com/<tenant-id>/v2.0`. Create a
client secret and export it as `PM_OIDC_CLIENT_SECRET`. For roles, define
app roles on the registration (e.g. `PrintMarshal.Admin`), assign users /
groups, and use `"roleClaim": "roles"`. `preferred_username` is the UPN.

**Okta.** Applications → Create App Integration → OIDC, Web Application,
sign-in redirect URI as above. Issuer is your authorization server, e.g.
`https://<org>.okta.com/oauth2/default` (or `https://<org>.okta.com` for the
org server). Add a `groups` claim to the ID token (Security → API →
authorization server → Claims, filter e.g. `starts with printmarshal-`) and
use `"roleClaim": "groups"`. Okta puts the login in `email` /
`preferred_username` depending on the profile mapping.

**Keycloak.** Clients → Create client → OpenID Connect, Client authentication
on, valid redirect URI as above. Issuer
`https://<host>/realms/<realm>`. Realm roles arrive as
`realm_access.roles`, client roles as `resource_access.<client>.roles`; use
either as `roleClaim`. `preferred_username` is set by default.

Any other compliant provider (Google Workspace, Auth0, Authentik, Dex,
Zitadel) works the same way; only the claim that carries roles differs.

## Security notes

- Secrets at rest: passwords are bcrypt; session and API-token secrets are
  SHA-256 digests; the OIDC client secret is read from the environment at
  use time and never stored.
- Every denial (`auth.denied`), failed sign-in (`auth.failed`), sign-in,
  sign-out, session revocation and token change is a security event in the
  catalogue (`GET /api/events/catalog`) and is mirrored into the audit log,
  so it reaches the SIEM export like any other audit entry.
- API tokens are deliberately unable to mint tokens or touch `/api/auth`: a
  leaked token cannot be used to make itself permanent or to take over a
  human account.
- Put PrintMarshal behind TLS before enabling SSO or issuing tokens
  (docs/security.md); set `PM_SECURE_COOKIES=true` so the session and SSO
  cookies are `Secure`.
- The OIDC flow cookie is single use and path-limited; a replayed callback
  (no cookie, reused state) fails. Failed callbacks count towards the login
  rate limit.
- Clock skew of up to one minute is tolerated on `exp`/`iat`; keep the
  server's clock synchronised.
