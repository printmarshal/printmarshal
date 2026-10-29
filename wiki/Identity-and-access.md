[Home](Home.md) · [Configuration reference](Configuration-reference.md) · [API reference](API-reference.md)

# Identity and access

PrintMarshal has the following principal types and two layers of authorization.

| Principal | How it signs in | Where it is managed |
| --- | --- | --- |
| Local user | username + password → session cookie / bearer session token | Settings → Users & roles |
| OIDC / SAML user | "Sign in with …" → identity provider → session | provisioned on first sign-in; role from the IdP |
| LDAP / RADIUS user | username + password on the same login form → directory / RADIUS server → session | provisioned on first sign-in; role from groups / reply attributes |
| API token (service account) | `Authorization: Bearer pmt_…` | Settings → API tokens, `/api/tokens` |

Every external method is optional and off until configured, and all of them
share one rule: the local account is bound to the identity the provider
reports (**issuer + subject**), never to the username. `GET
/api/auth/providers` lists what is enabled (`providers[]` with `kind`,
`name`, `method` = `redirect` or `password`, and `start` for the redirect
kinds); the login page builds itself from that list.

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
  password changes or SSO), cannot create or delete users and cannot create
  or revoke tokens, whatever its
  role. Those requests answer 403 and are recorded as `auth.denied`.
- The API-token secret is accepted in the `Authorization` header only, never as a
  cookie or query parameter.
- Revocation is immediate: the next request answers 401. Expired tokens
  behave the same. `lastUsedAt` is updated at most once a minute.
- Every creation and revocation is audited (`token.create`, `token.revoke`)
  and published as `auth.token.created` / `auth.token.revoked` security
  events; the secret never appears in either.
- In audit entries a token's actor is `token:<name>`, so it can never be
  confused with a human account of the same name.

## Group scope

For the device-targeted paths listed below, a principal with a scope (today: API tokens; the field also exists on users
for a future admin UI) is restricted to devices in those groups. This is not an isolation guarantee for all collection endpoints. Static groups
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
- JSON bodies naming devices or groups (`deviceId`, `deviceIds`, `devices`,
  `groupId`, `groupIds`, at the top level or under `target`) on bulk
  endpoints (poll, configuration apply and drift, firmware campaigns,
  security assessment and remediation, rotation, supply operations) are
  checked the same way: a group is resolved to its current devices first.
  `POST /api/poll` without an explicit device list (whole fleet) is refused.
- Resources that belong to a device are checked through their own id:
  `/api/alerts/{id}` (ack, clear), `/api/config/writes/{id}` (read, revert),
  `/api/firmware/campaigns/{id}` (read, patch, start, confirm, pause, resume,
  cancel, devices) and `/api/consumables/replacements/{id}` resolve to the
  device (or the campaign's group) behind them and answer 403 outside the
  scope. A missing resource is a plain 404.
- A scoped principal cannot change groups, users or tokens (`POST`, `PATCH`,
  `PUT`, `DELETE` under `/api/groups`, `/api/users`, `/api/tokens` answer
  403): groups are what define its boundary (a rule group with an empty rule
  matches the whole fleet), and the other two would let it mint an unscoped
  principal. Reads stay allowed.
- Each violation is recorded as `auth.denied` with the path and device.

API tokens, scoped or not, additionally cannot create or delete users
(`POST`/`DELETE /api/users` answer 403 `api tokens cannot manage
authentication or users`): a service account must not be able to turn
itself into a password login that outlives its expiry or revocation.

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
{
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

- Accounts are bound to the identity, not the name: the user row stores the
  issuer and the token's `sub`, and every sign-in resolves the account by
  that pair (`externalIssuer` / `externalSubject` in `GET /api/users`). A
  provider that lets someone choose the username `admin` therefore cannot
  sign them in as the local administrator, and a username change at the
  provider cannot move an account to another person.
- Username = `preferred_username`, else `email`, else the subject. On the
  first sign-in the username must be free: if a local account (or one bound
  to another provider) already uses it, the sign-in is refused with a clear
  message and an `auth.failed` event. Use a distinct provider username or a planned account migration; there is no account-linking or rename API. Existing local users can continue local login.
- First sign-in creates a local user with a random, unknown password; the
  account can only be used through the IdP (the current API has no administrative password-reset endpoint). The creation is audited with
  `source: oidc` and the subject.
- Role: the values of `roleClaim` (a string or list; dot paths such as
  `realm_access.roles` work) are looked up in `roleMap`; the highest mapped
  role wins above the `defaultRole` floor (viewer when omitted). When `roleClaim` and `roleMap`
  are set, the role is **re-applied on every sign-in**, so removing someone
  from the IdP group demotes them at their next login. Without a role
  mapping, roles are assigned once; current routes do not offer role editing.
- `allowedDomains` rejects sign-ins whose `email` domain is not listed (and
  any token without an email when the list is set). A token whose
  `email_verified` claim is `false` is rejected too, because the domain is
  then not a trustworthy boundary; a token without the claim (Entra ID
  omits it) is accepted.

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

## LDAP / Active Directory

For sites whose identities live in a directory and that have no web SSO.
Users type their directory credentials into the ordinary login form; the
server binds to the directory on their behalf. Implemented with
`github.com/go-ldap/ldap/v3`.

### Configuration

```json
{
"auth": {
  "ldap": {
    "url": "ldaps://dc1.corp.example.com:636",
    "startTls": false,
    "caFile": "/etc/printmarshal/corp-root-ca.pem",
    "bindDN": "CN=printmarshal,OU=Service Accounts,DC=corp,DC=example,DC=com",
    "bindPasswordEnv": "PM_LDAP_BIND_PASSWORD",
    "baseDN": "OU=People,DC=corp,DC=example,DC=com",
    "userFilter": "(|(sAMAccountName=%s)(userPrincipalName=%s))",
    "usernameAttr": "sAMAccountName",
    "subjectAttr": "objectGUID",
    "groupAttr": "memberOf",
    "roleMap": {
      "CN=PrintMarshal Admins,OU=Groups,DC=corp,DC=example,DC=com": "admin",
      "PrintMarshal Operators": "operator"
    },
    "defaultRole": "viewer",
    "allowedGroups": ["PrintMarshal Users"],
    "timeout": "10s",
    "displayName": "Corp AD"
  }
}
}
```

Without a service account, bind directly as the user instead:

```json
{
"ldap": {
  "url": "ldap://ldap.example.com:389",
  "startTls": true,
  "userDNTemplate": "uid=%s,ou=people,dc=example,dc=com",
  "baseDN": "dc=example,dc=com",
  "subjectAttr": "entryUUID",
  "roleMap": { "cn=printer-admins,ou=groups,dc=example,dc=com": "admin" }
}
}
```

(`"userDNTemplate": "%s@corp.example.com"` binds with an AD user principal
name.) Environment overrides: `PM_LDAP_URL`, `PM_LDAP_START_TLS`,
`PM_LDAP_CA_FILE`, `PM_LDAP_BIND_DN`, `PM_LDAP_BIND_PASSWORD_ENV` (the *name*
of the variable holding the password), `PM_LDAP_USER_DN_TEMPLATE`,
`PM_LDAP_BASE_DN`, `PM_LDAP_USER_FILTER`. The service-account password lives
only in the variable `bindPasswordEnv` names. LDAP is on as soon as `url`
is set.

| Field | Meaning | Default |
| --- | --- | --- |
| `url` | `ldap://host:389` or `ldaps://host:636` | required |
| `startTls` | upgrade an `ldap://` connection with StartTLS before any bind | `false` |
| `caFile` / `insecureSkipVerify` | PEM bundle that verifies the server / skip verification (lab only) | system roots |
| `bindDN` + `bindPasswordEnv` | read-only service account that finds the user's entry | none → direct bind |
| `userDNTemplate` | bind name when there is no service account; `%s` is the login name (DN-escaped) | — |
| `baseDN` | search base for the user entry | required with `bindDN`; optional with a template |
| `userFilter` | RFC 4515 filter; every `%s` is the escaped login name | `(\|(sAMAccountName=%s)(uid=%s)(userPrincipalName=%s)(mail=%s))` |
| `usernameAttr` | attribute that becomes the local username | the attribute that matched the login name, then `sAMAccountName`, `uid`, the typed name (direct bind), the DN |
| `subjectAttr` | stable identifier the account is bound to | `objectGUID` (rendered as the canonical GUID string), then `entryUUID`, then the DN |
| `groupAttr` | multi-valued attribute listing the user's groups | `memberOf` |
| `roleMap` | group **DN or CN** → `viewer` / `operator` / `admin` (case-insensitive) | none |
| `defaultRole` | initial role and minimum role selected by mapping | `viewer` |
| `allowedGroups` | DNs or CNs; users in none of them are refused | any |
| `timeout` | per sign-in, covering connect, binds and search | `10s` |
| `displayName` | name shown on the login page and in audit entries | the server host |

### Flow

`POST /api/auth/login` always tries the local account first. The directory
is consulted only when **no local account owns that username**, or the
account that does is already bound to this LDAP issuer. A local account
(the built-in `admin`, anyone created in Settings) therefore keeps its
local password as the only way in, whatever the directory says. Then:

1. Connect (TLS for `ldaps://`, StartTLS when asked; an empty password is
   refused before anything is sent, so an "unauthenticated bind" can never
   succeed).
2. With a service account: bind as it, search `baseDN` (subtree) with
   `userFilter`; exactly one entry must match, then bind as that entry with
   the user's password. Without one: bind as `userDNTemplate` and read the
   entry (through `userFilter` under `baseDN`, or the bind DN itself when
   `baseDN` is empty).
3. Read `groupAttr`, check `allowedGroups`, map the role (highest wins).
4. Provision through the same path as OIDC with issuer `ldap:<url>` and
   the `subjectAttr` value as the subject; audit `user.create` with
   `source: ldap` on first sign-in, `auth.login` ("signed in via <name>")
   on every one.

Wrong password, unknown user, several matching entries and a login name
with filter metacharacters all answer `401 invalid credentials` with the
same body and delay, and count towards the login rate limit. A directory
that cannot be reached (or an empty `bindPasswordEnv` variable) answers
`503` with a generic message; the cause is in the server log and in an
`auth.failed` event with result `error`.

### Roles

`roleMap` keys may be full group DNs or just the first RDN value (the CN),
compared case-insensitively. When a `roleMap` is set the role is re-applied
on every sign-in; without one, roles are assigned once (`defaultRole`) and
managed locally.

### Security notes

- Prefer `ldaps://` or `startTls: true` with `caFile`: the user's password
  travels in the simple bind. Plain `ldap://` without StartTLS sends it in
  clear text and should only be used on an isolated management network.
- The service account needs **read access to the user base only** (the
  user attributes above and `memberOf`); it never writes. Give it a long
  random password, no interactive logon, and keep it in `bindPasswordEnv`.
- Login names are escaped for both filters (RFC 4515) and DNs (RFC 4514)
  before use; the search is limited to two entries so an over-broad filter
  cannot be used to enumerate the directory.
- A renamed directory user keeps their PrintMarshal account (bound by GUID /
  UUID); a deleted-and-recreated one gets a new account.

## RADIUS

For sites that front their authentication with a RADIUS server (NPS,
FreeRADIUS, a token appliance). Implemented with the standard library only:
RFC 2865 PAP `Access-Request` over UDP with the `User-Password` hidden under
the shared secret, a `Message-Authenticator` (RFC 2869, HMAC-MD5) on every
request, and the reply's Response Authenticator and Message-Authenticator
verified before it is trusted. `Access-Challenge` (two-step / OTP prompts)
is not supported and is treated as a rejection.

### Configuration

```json
{
"auth": {
  "radius": {
    "servers": ["nps1.corp.example.com:1812", "nps2.corp.example.com:1812"],
    "secretEnv": "PM_RADIUS_SECRET",
    "timeout": "3s",
    "retries": 2,
    "nasIdentifier": "printmarshal",
    "roleAttribute": "Filter-Id",
    "roleMap": { "printmarshal-admin": "admin", "printmarshal-operator": "operator" },
    "defaultRole": "viewer",
    "displayName": "Corp RADIUS"
  }
}
}
```

Environment overrides: `PM_RADIUS_SERVERS` (comma separated),
`PM_RADIUS_SECRET_ENV` (the *name* of the variable holding the secret),
`PM_RADIUS_NAS_IDENTIFIER`. RADIUS is on as soon as `servers` has an entry.

| Field | Meaning | Default |
| --- | --- | --- |
| `servers` | `host:port`, tried in order; each gets `retries` retransmissions | required |
| `secretEnv` | env var holding the shared secret | required |
| `timeout` / `retries` | per attempt / retransmissions per server | `3s` / `2` |
| `nasIdentifier` | `NAS-Identifier` attribute (`NAS-IP-Address` 127.0.0.1 is sent as well) | `printmarshal` |
| `roleAttribute` | reply attribute looked up in `roleMap`: `Filter-Id` or `Class` | `Filter-Id` |
| `roleMap` / `defaultRole` | value → role; highest wins; re-applied on every sign-in when set | none / `viewer` |

### Flow and identity

The login handler tries the local account, then LDAP (when configured),
then RADIUS — the same rules as above: a free username is offered to each
in turn, a bound account only to its own provider, a local account to
none. A reject from any server is final; only a server that does not
answer is skipped.

**RADIUS has no stable user identifier.** The subject is therefore the
**lowercased username** and the issuer `radius:<first server>`, which
means: renaming a user at the RADIUS backend creates a new PrintMarshal
account, and giving the old name to someone else hands them the old
account's role. Use `roleMap` so roles follow the reply attributes rather
than the account, and prefer LDAP or SAML when a stable identifier exists.

### Security notes

- **PAP sends the password to the RADIUS server protected only by the
  shared secret** (an MD5-based stream cipher). Use RadSec (RADIUS over
  TLS, through a local proxy such as radsecproxy) or IPsec on any network
  you do not fully trust, and a long random secret.
- Replies are accepted only with a matching identifier, a correct Response
  Authenticator and, when present, a correct Message-Authenticator; forged
  or truncated packets are ignored like lost ones.
- Change `nasIdentifier` if your policy keys on it; register PrintMarshal's
  source address as a RADIUS client at the server.

## SAML 2.0

For identity providers that only speak SAML (ADFS, older Shibboleth /
Okta / Entra ID configurations). PrintMarshal is a standard **service
provider** with the HTTP-Redirect binding for the request and HTTP-POST
for the response, built on `github.com/crewjam/saml` for metadata,
XML-DSIG verification and assertion decryption.

### Configuration

```json
{
"auth": {
  "saml": {
    "idpMetadataUrl": "https://login.microsoftonline.com/<tenant-id>/federationmetadata/2007-06/federationmetadata.xml?appid=<app-id>",
    "baseUrl": "https://printmarshal.example.com",
    "signRequests": false,
    "usernameAttr": "http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name",
    "subjectAttr": "http://schemas.microsoft.com/identity/claims/objectidentifier",
    "roleAttr": "http://schemas.microsoft.com/ws/2008/06/identity/claims/groups",
    "roleMap": { "<admins group object id>": "admin", "<operators group object id>": "operator" },
    "defaultRole": "viewer",
    "allowedGroups": [],
    "displayName": "Corp SSO (SAML)"
  }
}
}
```

Environment overrides: `PM_SAML_IDP_METADATA_URL`, `PM_SAML_IDP_METADATA_FILE`,
`PM_SAML_BASE_URL`, `PM_SAML_ENTITY_ID`, `PM_SAML_ACS_URL`,
`PM_SAML_SP_CERT_FILE`, `PM_SAML_SP_KEY_FILE`. SAML is on as soon as one of
the two metadata sources is set.

| Field | Meaning | Default |
| --- | --- | --- |
| `idpMetadataUrl` / `idpMetadataFile` | the IdP's metadata (`https://` only, except loopback), fetched once at first use, or a local copy | one required |
| `baseUrl` | PrintMarshal's public origin | required |
| `entityId` | the SP entity ID / audience | `<baseUrl>/api/auth/saml/metadata` |
| `acsUrl` | the assertion consumer service | `<baseUrl>/api/auth/saml/acs` |
| `spCertFile` / `spKeyFile` | the SP signing pair (PEM, RSA) | generated under `<dataDir>/saml-sp.crt` / `.key` (mode 0600) on first use; the log says where |
| `signRequests` | sign AuthnRequests (RSA-SHA256) | `false` |
| `usernameAttr` | assertion attribute (Name or FriendlyName) used as the username | the NameID |
| `subjectAttr` | attribute used as the stable subject | the NameID when its format is `persistent`, `unspecified` or absent; a `transient` or `emailAddress` NameID requires `subjectAttr` |
| `roleAttr` / `roleMap` / `defaultRole` | groups attribute → role; highest wins; re-applied on every sign-in when both are set | — |
| `allowedGroups` | users with none of these `roleAttr` values are refused | any |
| `displayName` | login button label | the IdP host |

### Routes and flow

- `GET /api/auth/saml/metadata` — the SP metadata to register at the IdP
  (entity ID, ACS URL, the signing certificate, `WantAssertionsSigned`).
- `GET /api/auth/saml/start` — builds an AuthnRequest, keeps its ID in a
  10-minute `pm_saml` cookie (HttpOnly, path-limited to `/api/auth/saml/`;
  `Secure; SameSite=None` behind TLS with `auth.secureCookies`) and
  redirects to the IdP. RelayState is empty: the landing page is always `/`.
- `POST /api/auth/saml/acs` — validates the response: XML round-trip safety,
  the signature on the Response and/or Assertion against the IdP metadata
  certificates (an unsigned response with an unsigned assertion is
  rejected), the Issuer, `Destination` = the ACS URL, the audience =
  entity ID, `NotBefore` / `NotOnOrAfter` on the conditions and the bearer
  confirmation (three minutes of clock skew), `InResponseTo` = the ID in the
  `pm_saml` cookie (IdP-initiated sign-in is deliberately not accepted),
  and that the assertion ID has not been presented before within its
  validity window. Encrypted assertions are decrypted with the SP key. Then
  the user is provisioned with issuer `saml:<IdP entity ID>`, the role is
  mapped, a normal session is issued and the browser lands on `/`. Failures
  go back to `/?sso_error=<message>` and are published as `auth.failed`.

### Provider notes

**Microsoft Entra ID.** Enterprise applications → New application → Create
your own → *Integrate any other application*. Single sign-on → SAML:
Identifier = `https://<host>/api/auth/saml/metadata`, Reply URL =
`https://<host>/api/auth/saml/acs`. Use the *App Federation Metadata Url*
as `idpMetadataUrl`. Entra's default NameID is the UPN with format
`emailAddress` or `unspecified` depending on the template, so set
`subjectAttr` to `http://schemas.microsoft.com/identity/claims/objectidentifier`
(stable object id) and `usernameAttr` to
`http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name`. Add a groups
claim (Attributes & Claims → Add a group claim → groups assigned to the
application, or security groups) and map the group **object ids** in
`roleMap`; with "Customize the name of the group claim" you can emit
display names instead, in which case map those.

**Okta.** Applications → Create App Integration → SAML 2.0. Single sign-on
URL = `https://<host>/api/auth/saml/acs`, Audience URI =
`https://<host>/api/auth/saml/metadata`, Name ID format *Persistent*
(then the NameID is the subject) or *Unspecified* with `user.login`. Add a
group attribute statement (e.g. name `groups`, filter *Starts with*
`printmarshal-`) and use `"roleAttr": "groups"`. The IdP metadata URL is
on the app's Sign On tab.

**ADFS.** Add Relying Party Trust → *Import data about the relying party
published online* with `https://<host>/api/auth/saml/metadata` (or the
downloaded file). Issuance transform rules: *Send LDAP Attributes as
Claims* with `objectGUID` (or `SAM-Account-Name`) → Name ID
(**persistent** or unspecified format) or → a custom claim used as
`subjectAttr`; *Send Group Membership as a Claim* or *Token-Groups —
Unqualified Names* → `http://schemas.xmlsoap.org/claims/Group` and use that
as `roleAttr` with the group names in `roleMap`. `idpMetadataUrl` is
`https://<adfs>/FederationMetadata/2007-06/FederationMetadata.xml`. ADFS
signs with SHA-256 by default and encrypts assertions when the SP metadata
carries an encryption certificate (PrintMarshal's does); both work.

**Keycloak.** Clients → Create client → SAML, Client ID =
`https://<host>/api/auth/saml/metadata`, Valid redirect URIs / Master SAML
Processing URL = `https://<host>/api/auth/saml/acs`. In *Keys* turn *Client
signature required* off unless you set `signRequests: true` (then upload
the certificate from the SP metadata). Name ID format *persistent* (or
*username*, which is stable in Keycloak). Add a *Group list* or *Role list*
mapper (single attribute, e.g. `groups`) and use it as `roleAttr`. The
realm's IdP metadata is at
`https://<host>/realms/<realm>/protocol/saml/descriptor`.

### Security notes

- Use `idpMetadataUrl` over `https` or a file you obtained out of band: the
  metadata carries the certificates every assertion is verified against.
  Metadata is fetched once per process start; restart after an IdP key
  rollover (or point at a file you update).
- The SP key only signs AuthnRequests (when enabled) and decrypts
  assertions; losing it costs you a metadata re-upload at the IdP, not any
  secrets. Back it up with the data directory all the same.
- An IdP that cannot provide a stable NameID must provide an attribute for
  `subjectAttr`; without one the sign-in is refused rather than bound to a
  value that may change hands.

## Security notes

- Secrets at rest: passwords are bcrypt; session and API-token secrets are
  SHA-256 digests; the OIDC client secret, LDAP service-account password
  and RADIUS shared secret are read from the environment at use time and
  never stored or logged.
- All four external methods bind accounts by (issuer, subject) and refuse
  a first sign-in whose username is already taken, so no provider can hand
  out the local `admin` account or move an account between people.
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
- The OIDC and SAML flow cookies are single use and path-limited; a
  replayed callback (no cookie, reused state / request ID) fails, and a
  SAML assertion ID is remembered until it expires. Failed callbacks and
  failed LDAP / RADIUS attempts count towards the login rate limit.
- Clock skew of up to one minute is tolerated on `exp`/`iat`; keep the
  server's clock synchronised.


## Role and administration reference

| Minimum role | Main permissions |
| --- | --- |
| `viewer` | Inventory, device/configuration/security/firmware reads, reports, stream, metrics; change own password with current password. |
| `operator` | Discovery/poll jobs, device metadata/state, group/profile/policy CRUD, stock adjustments, configuration applies/reverts, remediation/rotation/enrollment, firmware campaign controls, print-provider sync/removal, audit and notification logs. |
| `admin` | User creation/deletion, credential creation/purpose change/deletion, API-token creation/revocation, CA-profile creation/deletion, firmware package upload/deletion, runtime/consumable/print-provider settings, outbox requeue/drain and notifier tests; also all lower-role operations. |

These are minimum role ranks, not independent grants. Consult the [API reference](API-reference.md) for each route. Tokens cannot invoke protected auth routes even if their role is admin; login/logout and the SSO flow routes are public handlers, not API-token administration operations. A local password must be at least eight characters. `POST /api/users` requires an explicit valid role. User deletion refuses to delete the caller's own account. Secrets never appear in `GET /api/users`; external bindings do (`externalIssuer`, `externalSubject`).

Cookie sessions and bearer sessions share the same lookup and expiry. Logout deletes the presented session and clears the cookie; it does not revoke an API token. A WebSocket recheck is once per minute rather than immediate disconnect on revocation. Failed-sign-in throttling uses the TCP peer, not forwarded client headers; configure rate limiting at a reverse proxy too.

## Exact scope limitations in this build

The enforced cases listed above are implemented in `internal/api/scope.go`; list filtering is performed by the individual handlers. Scope is not a tenant boundary. In addition to the explicitly fleet-wide stream, jobs, stock, credential and settings resources, the current report handlers, sync collections, metrics, compliance aggregate/history and configuration-write listing are not filtered by the authenticated principal's group set. `GET /api/config/diff` also resolves query parameters directly rather than through the scoped device path. The collection firmware inventory and campaign list are not equivalent to a scoped single-campaign lookup. Use dedicated low-role integrations with appropriate visibility; do not promise confidentiality of other groups through these aggregate surfaces.

## Known discrepancies

- `docs/auth.md` describes roles as locally managed and mentions administrative password resets, but the current routes provide user create/list/delete and self password change only: no user PATCH, role-change or administrative password-reset route. Do not invent those endpoints. External role synchronization updates roles at sign-in when enabled.
- Role mapping calls `oidc.MapRole`: it starts with `defaultRole` (or viewer), then accepts only mappings at least as high as the current best. Thus a configured operator/admin default is also a floor, rather than merely a fallback. Use `defaultRole: "viewer"` when lower mapped roles must be possible. LDAP maps group keys case-insensitively; OIDC, SAML and RADIUS role-map lookups use exact strings.
- LDAP's timeout is applied to dialing and individual LDAP operations; the comment/table in `docs/auth.md` describing one timeout covering the entire exchange is stronger than the implementation. Several sequential operations can consume several timeout budgets.
- SAML `baseUrl` can be omitted if both `entityId` and `acsUrl` are supplied, per `internal/auth/saml/saml.go`; prefer setting the public base URL. When metadata comes from a file and `displayName` is omitted, the label is `SAML single sign-on`, not an IdP hostname.
- RADIUS `roleAttribute` also accepts `Reply-Message` and numeric attribute numbers 1–255, beyond the two named attributes emphasized above. `retries: 0` selects the default two retransmissions; a negative value selects zero retransmissions in the client.

Sources: `internal/api/{server,scope,providers,oidc,saml,ldap,radius,tokens}.go`, `internal/auth/auth.go`, `internal/auth/tokens.go`, `internal/auth/{oidc,saml,ldap,radius}/`, and `internal/config/config.go`. Configuration defaults and every environment override are in [Configuration reference](Configuration-reference.md).
