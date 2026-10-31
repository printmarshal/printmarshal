# API reference

[Home](Home.md) · [Identity and access](Identity-and-access.md) · [Configuration reference](Configuration-reference.md)

This route inventory was generated from the literal `mux.Handle`/`HandleFunc` registrations in `internal/api/*.go` on 2026-10-29, then paired with each handler's decoded body and response. Minimum roles are **viewer < operator < admin**. A role does not bypass token/session or group-scope restrictions. Public means no `require` wrapper; protocol callbacks still validate their own flow state.

JSON requests use `Content-Type: application/json`. Shared decoding rejects unknown fields and caps JSON at 1 MiB. Errors normally use `{"error":"message"}` with 400 invalid input, 401 no usable authentication, 403 role/scope/principal refusal, 404 missing resource, 409 lifecycle/conflict, or 500 store/internal failure. Some errors have feature-specific statuses noted in the table/guides. Read responses are 200 unless marked otherwise. Objects below use JSON field names; `?` means omitted/optional, not a literal key suffix. Named request schemas are expanded after the route tables.

**Effect legend:** Read = no device mutation, but fresh reads may contact devices and persist observations. PM = mutate application records (no preview). **DR** = dry-run default; only explicit `dryRun:false` permits the corresponding device/host action. Device = already-issued certificate installation without a dry-run flag. Host = immediate CUPS queue removal without a dry-run flag. Auth = session/SSO flow. Firmware confirm/pause/resume/cancel control the mode saved at start, not a fresh dry-run decision.

Use `Authorization: Bearer <session>` or a `pm_session` cookie for human sessions; service tokens use a bearer `pmt_...` value and never cookies/query parameters. API-token principals cannot use protected `/api/auth/*` routes or mutate users/tokens. Scoped principals cannot mutate groups/users/tokens. See the complete scope limits under [Identity and access](Identity-and-access.md).


## Core, sessions, inventory, groups and jobs

Source: `internal/api/server.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/healthz` | public | — | 200 `{ok,version}` | Read |
| GET | `/readyz` | public | — | 200 `{ok,version}`; 503 `{ok:false,database}` | Read |
| GET | `/metrics` | viewer | — | 200 Prometheus text | Read |
| POST | `/api/auth/login` | public | `{username,password}` | 200 `{user,token}` + session cookie | Auth |
| POST | `/api/auth/logout` | public | — | 200 `{ok:true}` + cleared cookie | Auth |
| GET | `/api/auth/me` | viewer | — | 200 User | Auth |
| POST | `/api/auth/password` | viewer | `{current,new}` | 200 `{ok:true}` | Auth |
| GET | `/api/stream` | viewer | — | 101 WebSocket; event envelopes | Read |
| GET | `/api/dashboard` | viewer | — | 200 `{summary,alerts[],jobs[],attention[],consumables,security,layout}` — `security` is the compliance roll-up over the caller's visible devices (`unknown` items kept apart from the score, `notAssessed` devices); `layout` is `{effective,personal,default}`, each a `DashboardLayout` or null | Read |
| PUT | `/api/dashboard/layout` | viewer (sessions only; API tokens get 403) | `{version:1,widgets:[{id,hidden?,collapsed?}]}` | 200 the normalised layout (unknown ids dropped) | PM |
| DELETE | `/api/dashboard/layout` | viewer (sessions only) | — | 200 `{ok:true}`; the caller follows the instance default again | PM |
| PUT | `/api/dashboard/layout/default` | admin (sessions only) | same body | 200 the normalised default for everyone without a personal layout | PM |
| DELETE | `/api/dashboard/layout/default` | admin (sessions only) | — | 200 `{ok:true}`; built-in order again | PM |
| GET | `/api/devices` | viewer | query: `q,state,make,model,severity,group,sort,limit,offset,includeDecommissioned` | 200 `{devices[],total}` | Read |
| GET | `/api/devices.csv` | viewer | query: `device-list filters` | 200 devices CSV | Read |
| GET | `/api/supplies.csv` | viewer | query: `device-list filters` | 200 installed supplies CSV | Read |
| GET | `/api/devices/{id}` | viewer | — | 200 `{device,groups,alerts[]}` | Read |
| PATCH | `/api/devices/{id}` | operator | `{displayName?,location?,assetTag?,notes?,dnsSync?}` | 200 Device | PM |
| DELETE | `/api/devices/{id}` | admin | — | 200 `{ok:true}` | PM |
| POST | `/api/devices/{id}/poll` | operator | — | 200 `{device,servedBy,errors,reachable}` | PM |
| POST | `/api/devices/{id}/refresh-hostname` | operator | — | 200 Device | PM |
| POST | `/api/devices/rename` | operator | `{deviceIds:string[],source:"dns"\|"hostname"\|"sysName"\|"pattern",pattern?,dryRun?}` | 200 `{dryRun,source,pattern?,updated,skipped,results:[{id,from,to,skipped,reason?}]}` | **DR** |
| POST | `/api/devices/bulk` | operator | `{deviceIds[],renameFromDns?,set:{dnsSync?,location?,appendNote?}}` (at most 2,000 ids; at least one change) | 200 `{changed,skipped,results[{id,changed[],skipped?}]}`; group scope enforced; one `device.patch` audit row per changed device plus a `devices.bulk` summary | PM |
| POST | `/api/devices/{id}/state` | operator | `{state}` | 200 Device | PM |
| GET | `/api/devices/{id}/alerts` | viewer | query: `all,limit (200)` | 200 Alert[] | Read |
| GET | `/api/devices/{id}/telemetry` | viewer | query: `metric,label,from,to,limit` | 200 TelemetrySample[] | Read |
| GET | `/api/telemetry` | viewer | query: `device (required),metric,label,from,to,limit` | 200 TelemetrySample[] | Read |
| GET | `/api/groups` | viewer | — | 200 Group[] | Read |
| POST | `/api/groups` | operator | `{name,description?,kind?,rule?,memberIds?}` | 201 Group | PM |
| GET | `/api/groups/{id}` | viewer | — | 200 Group | Read |
| PATCH | `/api/groups/{id}` | operator | `{name?,description?,kind?,rule?}` | 200 Group | PM |
| DELETE | `/api/groups/{id}` | operator | — | 200 `{ok:true}` | PM |
| PUT | `/api/groups/{id}/members` | operator | `{deviceIds: string[]}` | 200 Group | PM |
| GET | `/api/groups/{id}/devices` | viewer | — | 200 Device[] | Read |
| POST | `/api/discovery/runs` | operator | `{targets?,zeroconf?,allowLarge?}`; absent body: zeroconf=true | 202 Job (direct) | PM |
| GET | `/api/discovery/runs` | viewer | query: `limit (50)` | 200 Job[] | Read |
| POST | `/api/poll` | operator | `{deviceIds?,groupId?}`; absent body allowed | 202 Job (direct) | PM |
| GET | `/api/jobs` | viewer | query: `type,status,limit (100)` | 200 Job[] | Read |
| GET | `/api/jobs/{id}` | viewer | — | 200 `{job,results[]}` | Read |
| GET | `/api/alerts` | viewer | query: `device,all,severity,limit (500)` | 200 Alert[] | Read |
| POST | `/api/alerts/{id}/ack` | operator | — | 200 Alert | PM |
| POST | `/api/alerts/{id}/clear` | operator | — | 200 Alert | PM |
| GET | `/api/credentials` | operator | — | 200 Credential[] (metadata only) | Read |
| POST | `/api/credentials` | admin | `{name,type,scope?,username?,priority?,purpose?,secret:Secret}` | 201 Credential (metadata only) | PM |
| PATCH | `/api/credentials/{id}` | admin | `{purpose}` only | 200 Credential | PM |
| DELETE | `/api/credentials/{id}` | admin | — | 200 `{ok:true}` | PM |
| GET | `/api/users` | admin | — | 200 User[] | Read |
| POST | `/api/users` | admin | `{username,password,role}` | 201 User | PM |
| DELETE | `/api/users/{id}` | admin | — | 200 `{ok:true}` | PM |
| GET | `/api/audit` | operator | query: `limit (200)` | 200 AuditEntry[] | Read |
| GET | `/api/notifications` | operator | query: `limit (200)` | 200 NotificationLog[] | Read |
| GET | `/api/settings` | viewer | — | 200 `{version,poll,discovery,trap,alerts,telemetry,names,runtime,defaults,notifiers}` | Read |
| PATCH | `/api/settings` | admin | RuntimeSettings partial overlay | 200 RuntimeSettings | PM |
| DELETE | `/api/settings` | admin | — | 200 RuntimeSettings | PM |
| GET | `/api/drivers` | viewer | — | 200 driver descriptors[] | Read |

## Provider discovery and OIDC

Source: `internal/api/oidc.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/auth/providers` | public | — | 200 `{local,oidc,providers[]}` | Auth |
| GET | `/api/auth/oidc/start` | public | — | 302 IdP redirect + flow cookie; failure redirects to UI | Auth |
| GET | `/api/auth/oidc/callback` | public | query: `code,state or error,error_description` | 303 `/` + session cookie; failure redirects with oidc_error | Auth |

## SAML

Source: `internal/api/saml.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/auth/saml/metadata` | public | — | 200 SAML XML metadata; 503 unavailable | Auth |
| GET | `/api/auth/saml/start` | public | — | 302 IdP redirect + flow cookie | Auth |
| POST | `/api/auth/saml/acs` | public | application/x-www-form-urlencoded `SAMLResponse` (IdP POST) | 303 `/` + session cookie; failure redirects with sso_error | Auth |

## API tokens

Source: `internal/api/tokens.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/tokens` | admin | — | 200 APIToken[] | Read |
| POST | `/api/tokens` | admin | `{name,role?,scope?:string[],expiresAt?,ttl?}` | 201 `{token,secret}` (secret once) | PM |
| DELETE | `/api/tokens/{id}` | admin | — | 200 APIToken (revoked) | PM |

## Configuration management

Source: `internal/api/config.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/profiles` | viewer | — | 200 `{profiles[],total}` | Read |
| POST | `/api/profiles` | operator | `{name,description?,settings:[{key,value}]}` | 201 Profile | PM |
| GET | `/api/profiles/{id}` | viewer | — | 200 `{profile,drift[],writes[]}` | Read |
| PATCH | `/api/profiles/{id}` | operator | `{name?,description?,settings?}` | 200 Profile | PM |
| DELETE | `/api/profiles/{id}` | operator | — | 200 `{ok:true}` | PM |
| POST | `/api/profiles/{id}/apply` | operator | `{target:ConfigTarget,dryRun?}` | 202 `{job}` | **DR** |
| POST | `/api/profiles/{id}/drift` | viewer | `{target:ConfigTarget,refresh?}` | 200 `{reports[],devices,drifted}` | PM |
| GET | `/api/devices/{id}/config` | viewer | query: `refresh=true` | 200 `{document,fresh,snapshotId?,at?}` | Read |
| GET | `/api/devices/{id}/config/snapshots` | viewer | query: `limit` | 200 `{snapshots[],total}` | Read |
| POST | `/api/devices/{id}/config` | operator | `{changes:[{key,value}],dryRun?}` | 202 `{job}` | **DR** |
| GET | `/api/config/catalog` | viewer | — | 200 `{areas[],settings[]}` | Read |
| GET | `/api/config/diff` | viewer | query: `a,b (required device IDs),refresh` | 200 ConfigDiff `{a,b,items[],same}` | Read |
| GET | `/api/config/writes` | viewer | query: `device,profile,job,limit` | 200 `{writes[],total}` | Read |
| GET | `/api/config/writes/{id}` | viewer | — | 200 `{write,reversible}` | Read |
| POST | `/api/config/writes/{id}/revert` | operator | `{dryRun?}`; absent body allowed | 202 `{job}` | **DR** |

## Security and PKI

Source: `internal/api/security.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/policies` | viewer | — | 200 Policy[] | Read |
| POST | `/api/policies` | operator | `{name,description?,items:PolicyItem[]}` | 201 Policy | PM |
| GET | `/api/policies/{id}` | viewer | — | 200 Policy | Read |
| PATCH | `/api/policies/{id}` | operator | `{name?,description?,items?}` | 200 Policy | PM |
| DELETE | `/api/policies/{id}` | operator | — | 200 `{ok:true}` | PM |
| POST | `/api/policies/{id}/assess` | operator | `{deviceIds?,groupId?,dryRun?}` (flat target; dryRun unused for assessment) | 202 Job (direct) | PM |
| POST | `/api/policies/{id}/remediate` | operator | `{deviceIds?,groupId?,dryRun?}` (flat target) | 202 Job (direct) | **DR** |
| GET | `/api/compliance` | viewer | query: `policy,group,device` | 200 `{results[],summary}` | Read |
| GET | `/api/compliance/history` | viewer | query: `policy,from,to` | 200 `{policy,points[]}` | Read |
| GET | `/api/devices/{id}/security` | viewer | query: `policy` | 200 `{assessments[],certificates,alerts[]}` | Read |
| GET | `/api/devices/{id}/certificates` | viewer | query: `live=true` | 200 `{deviceId,installed[],observed[],issued[],canInstall,drivers?,tls?}` | Read |
| POST | `/api/devices/{id}/certificates/enroll` | operator | EnrollRequest | 202 Job (direct) | **DR** |
| POST | `/api/devices/{id}/certificates/{certId}/install` | operator | — | 202 Job (direct) | **Device** |
| GET | `/api/ca-profiles` | viewer | — | 200 CAProfile[] | Read |
| POST | `/api/ca-profiles` | admin | CAProfile (name/protocol/url required) | 201 CAProfile | PM |
| DELETE | `/api/ca-profiles/{id}` | admin | — | 200 `{ok:true}` | PM |
| POST | `/api/security/rotate` | operator | `{kind,target:Target,dryRun?}` | 202 Job (direct) | **DR** |
| GET | `/api/security/catalog` | viewer | — | 200 `{settings,operators,severities,managementPaths}` | Read |

## Firmware

Source: `internal/api/firmware.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/firmware/inventory` | viewer | query: `make,model,group,q` | 200 `{entries:[{make,model,version,count,latest?,latestPackageId?,outdated,deviceIds[]}],devices,outdated,unknown}` | Read |
| GET | `/api/firmware/outdated` | viewer | — | 200 outdated devices[] | Read |
| GET | `/api/firmware/packages` | viewer | — | 200 FirmwarePackage[] | Read |
| POST | `/api/firmware/packages` | admin | multipart `file,vendor,models,version,minVersion,notes` | 201 FirmwarePackage | PM |
| GET | `/api/firmware/packages/{id}` | viewer | — | 200 FirmwarePackage | Read |
| DELETE | `/api/firmware/packages/{id}` | admin | — | 204 empty | PM |
| GET | `/api/firmware/campaigns` | viewer | query: `status,package,active,limit` | 200 campaign View[] | Read |
| POST | `/api/firmware/campaigns` | operator | campaignRequest | 201 campaign View | PM |
| GET | `/api/firmware/campaigns/{id}` | viewer | — | 200 `{campaign,devices[]}` | Read |
| PATCH | `/api/firmware/campaigns/{id}` | operator | campaignRequest (draft only) | 200 campaign View | PM |
| DELETE | `/api/firmware/campaigns/{id}` | operator | — | 204 empty | PM |
| POST | `/api/firmware/campaigns/{id}/start` | operator | `{dryRun?}`; absent body allowed | 200 campaign View | **DR** |
| POST | `/api/firmware/campaigns/{id}/confirm` | operator | — | 200 campaign View | Campaign control |
| POST | `/api/firmware/campaigns/{id}/pause` | operator | — | 200 campaign View | Campaign control |
| POST | `/api/firmware/campaigns/{id}/resume` | operator | — | 200 campaign View | Campaign control |
| POST | `/api/firmware/campaigns/{id}/cancel` | operator | — | 200 campaign View | Campaign control |
| GET | `/api/firmware/campaigns/{id}/devices` | viewer | — | 200 campaign device rows[] with name/address/model | Read |
| GET | `/api/devices/{id}/firmware` | viewer | query: `limit (50)` | 200 `{deviceId,firmware,history[]}` | Read |

## Consumables and supply links

Source: `internal/api/consumables.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/consumables` | viewer | query: `q,type,color,status,model` | 200 `{items[],total}` (decorated stock rows) | Read |
| POST | `/api/consumables` | operator | Consumable (name required; server identity/times generated) | 201 Consumable | PM |
| GET | `/api/consumables.csv` | viewer | — | 200 CSV | Read |
| POST | `/api/consumables/import` | operator | raw text/csv (not multipart) | 200 `{created,updated,errors[]}` | PM |
| GET | `/api/consumables/settings` | viewer | — | 200 ConsumableSettings | Read |
| PUT | `/api/consumables/settings` | admin | ConsumableSettings | 200 ConsumableSettings | PM |
| GET | `/api/consumables/forecast` | viewer | — | 200 ItemForecast[] | Read |
| GET | `/api/consumables/installed` | viewer | query: `device-list filters; forecast=false; unmatched=true` | 200 Installed[] | Read |
| GET | `/api/consumables/replacements` | viewer | query: `all,limit (200)` | 200 StockEvent[] | Read |
| POST | `/api/consumables/replacements/{id}/confirm` | operator | — | 200 `{item,event}` | PM |
| POST | `/api/consumables/replacements/{id}/dismiss` | operator | — | 200 `{ok:true}` | PM |
| POST | `/api/consumables/bulk-stock` | operator | `{items:[{id,onHand}],reason?}` | 200 `{updated,errors[]}` | PM |
| GET | `/api/consumables/{id}` | viewer | — | 200 `{item,status,devices[],forecast,events[]}` | Read |
| PATCH | `/api/consumables/{id}` | operator | `{name?,type?,color?,partNumber?,compatibleModels?,unit?,warningAt?,criticalAt?,location?,notes?,reorderUrl?,unitCost?,clearUnitCost?}` | 200 Consumable | PM |
| DELETE | `/api/consumables/{id}` | operator | — | 200 `{ok:true}` | PM |
| POST | `/api/consumables/{id}/adjust-stock` | operator | `{set?:int,delta?:int,reason?:string}` (set or delta required) | 200 `{item,status,event}` | PM |
| GET | `/api/consumables/{id}/devices` | viewer | — | 200 Installed[] | Read |
| GET | `/api/consumables/{id}/forecast` | viewer | — | 200 ItemForecast | Read |
| GET | `/api/consumables/{id}/events` | viewer | query: `limit (500)` | 200 StockEvent[] | Read |
| GET | `/api/devices/{id}/supplies` | viewer | — | 200 `{supplies[],pendingReplacements[]}` | Read |
| PUT | `/api/devices/{id}/supplies/{slot}/link` | operator | `{consumableId}` | 200 SupplyLink | PM |
| DELETE | `/api/devices/{id}/supplies/{slot}/link` | operator | — | 200 `{ok:true}` | PM |
| POST | `/api/devices/{id}/supplies/{slot}/mark-replaced` | operator | `{decrement:bool,consumableId?,reason?}` | 200 `{item,status,event}` | PM |

## Catalogue and support

Source: `internal/api/catalog.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/catalog/summary` | viewer | — | 200 `{catalogue,fleet,overrides,brands,segments}` | Read |
| GET | `/api/catalog/families` | viewer | query: `brand,q,segment` | 200 `{families[],total}` | Read |
| GET | `/api/catalog/families/{id}` | viewer | — | 200 family detail/evidence/source/fleet/qualification object | Read |
| PUT | `/api/catalog/families/{id}/qualification` | operator | `{status,note}` | 200 Qualification | PM |
| GET | `/api/catalog/profiles` | viewer | — | 200 management profiles[] | Read |
| GET | `/api/catalog/match` | viewer | query: `make,model,sysDescr,enterpriseId` | 200 Match | Read |
| GET | `/api/devices/{id}/support` | viewer | — | 200 `{match,support}` | Read |

## Print provider

Source: `internal/api/printprovider.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/print-provider` | viewer | — | 200 `{settings,availability,queues[],inventoryError}` | Read |
| PUT | `/api/print-provider` | admin | printprovider.Settings (full replacement) | 200 Settings | PM |
| GET | `/api/print-provider/plan` | viewer | — | 200 Plan | Read |
| POST | `/api/print-provider/sync` | operator | `{dryRun?}`; absent body allowed | 202 `{job}` | **DR** |
| DELETE | `/api/print-provider/queues/{deviceId}` | operator | — | 200 `{removed,deviceId}` | **Host** |

## Events and notifications

Source: `internal/api/events.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/events/catalog` | viewer | — | 200 event definitions[] | Read |
| GET | `/api/notifications/outbox` | operator | query: `state,limit (200)` | 200 Outbox[] | Read |
| POST | `/api/notifications/outbox/{id}/requeue` | admin | — | 200 `{ok:true}` | PM |
| POST | `/api/notifications/drain` | admin | — | 200 `{ok:true}` | PM |
| POST | `/api/notifiers/{name}/test` | admin | — | 200 `{ok:true}` | PM |

## SIEM, device logs and sync

Source: `internal/api/export.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/export/status` | viewer | — | 200 SIEM Status (including ingest and syncCollections) | Read |
| GET | `/api/devices/{id}/logs` | viewer | query: `limit (200, max 2000)` | 200 device-log rows[] with severityName/facilityName | Read |
| GET | `/api/sync/{collection}` | viewer; audit collection: operator | query: `since,limit (200, max 1000); If-None-Match` | 200 `{collection,items[],cursor,hasMore}` + ETag; 304 unchanged | Read |

## Reports

Source: `internal/api/reports.go`.

| Method | Path | Minimum role | Body / query | Response | Effect |
| --- | --- | --- | --- | --- | --- |
| GET | `/api/reports` | viewer | — | 200 `{available,reports,maxRangeDays}` | Read |
| GET | `/api/reports/utilisation` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |
| GET | `/api/reports/fleet-health` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |
| GET | `/api/reports/supplies` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |
| GET | `/api/reports/alerts` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |
| GET | `/api/reports/firmware` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |
| GET | `/api/reports/compliance` | viewer | query: `from,to,days,group,device,top,format,table` | 200 report JSON or CSV; compliance report is unavailable placeholder | Read |

## Request body schemas

These are the exact decoder declarations with their JSON tags. For anonymous handler bodies, the Go field names are not wire keys: use the `json` names. Embedded targets flatten in `targetBody`, while `applyBody` and rotation wrap a target under the literal `target` key. Pointer fields distinguish absent from zero; the dry-run helper maps nil to true. The schemas document accepted decoder shapes, not permission to override generated IDs/timestamps; handlers generate or reset server-owned fields. Field-level validations are described in the corresponding feature pages.

### Common target examples

```json
{"deviceIds":["DEVICE_ID"]}
```

```json
{"groupId":"GROUP_ID"}
```

Configuration apply wraps one under `target`. Security assess/remediate uses the flat form. For firmware a group takes precedence over an explicit list; configuration combines and deduplicates explicit IDs and group members.


### adjustStock

Source: `internal/api/consumables.go`.

```go
var body struct {
		Set    *int   `json:"set"`
		Delta  *int   `json:"delta"`
		Reason string `json:"reason"`
	}
```

### applyDeviceConfig

Source: `internal/api/config.go`.

```go
var body struct {
		Changes []model.SettingChange `json:"changes"`
		DryRun  *bool                 `json:"dryRun"`
	}
```

### applyProfile

Source: `internal/api/config.go`.

```go
var body applyBody
```

### assessPolicy

Source: `internal/api/security.go`.

```go
var body targetBody
```

### bulkStock

Source: `internal/api/consumables.go`.

```go
var body struct {
		Items []struct {
			ID     string `json:"id"`
			OnHand int    `json:"onHand"`
		} `json:"items"`
		Reason string `json:"reason"`
	}
```

### catalogSetQualification

Source: `internal/api/catalog.go`.

```go
var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
```

### changePassword

Source: `internal/api/server.go`.

```go
var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
```

### createCAProfile

Source: `internal/api/security.go`.

```go
var p model.CAProfile
```

### createCampaign

Source: `internal/api/firmware.go`.

```go
var req campaignRequest
```

### createConsumable

Source: `internal/api/consumables.go`.

```go
var c model.Consumable
```

### createCredential

Source: `internal/api/server.go`.

```go
var body struct {
		Name     string                  `json:"name"`
		Type     model.CredentialType    `json:"type"`
		Scope    string                  `json:"scope"`
		Username string                  `json:"username"`
		Priority int                     `json:"priority"`
		Purpose  model.CredentialPurpose `json:"purpose"` // read (default) | write | both
		Secret   model.Secret            `json:"secret"`
	}
```

### createGroup

Source: `internal/api/server.go`.

```go
var g model.Group
```

### createPolicy

Source: `internal/api/security.go`.

```go
var body policyBody
```

### createProfile

Source: `internal/api/config.go`.

```go
var body struct {
		Name        string                `json:"name"`
		Description string                `json:"description"`
		Settings    []model.SettingChange `json:"settings"`
	}
```

### createToken

Source: `internal/api/tokens.go`.

```go
var body struct {
		Name      string     `json:"name"`
		Role      model.Role `json:"role"`
		Scope     []string   `json:"scope"`     // group ids; empty = unscoped
		ExpiresAt *time.Time `json:"expiresAt"` // RFC 3339, optional
		TTL       string     `json:"ttl"`       // Go duration alternative, e.g. "720h"
	}
```

### createUser

Source: `internal/api/server.go`.

```go
var body struct {
		Username string     `json:"username"`
		Password string     `json:"password"`
		Role     model.Role `json:"role"`
	}
```

### enrollCertificate

Source: `internal/api/security.go`.

```go
var req security.EnrollRequest
```

### linkSupply

Source: `internal/api/consumables.go`.

```go
var body struct {
		ConsumableID string `json:"consumableId"`
	}
```

### login

Source: `internal/api/server.go`.

```go
var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
```

### markReplaced

Source: `internal/api/consumables.go`.

```go
var body struct {
		Decrement    bool   `json:"decrement"`
		ConsumableID string `json:"consumableId"`
		Reason       string `json:"reason"`
	}
```

### patchCampaign

Source: `internal/api/firmware.go`.

```go
var req campaignRequest
```

### patchConsumable

Source: `internal/api/consumables.go`.

```go
var p struct {
		Name             *string               `json:"name"`
		Type             *model.ConsumableType `json:"type"`
		Color            *string               `json:"color"`
		PartNumber       *string               `json:"partNumber"`
		CompatibleModels *[]string             `json:"compatibleModels"`
		Unit             *string               `json:"unit"`
		WarningAt        *int                  `json:"warningAt"`
		CriticalAt       *int                  `json:"criticalAt"`
		Location         *string               `json:"location"`
		Notes            *string               `json:"notes"`
		ReorderURL       *string               `json:"reorderUrl"`
		UnitCost         *float64              `json:"unitCost"`
		ClearUnitCost    *bool                 `json:"clearUnitCost"`
	}
```

### patchCredential

Source: `internal/api/server.go`.

```go
var body struct {
		Purpose model.CredentialPurpose `json:"purpose"`
	}
```

### patchDevice

Source: `internal/api/server.go`.

```go
var body struct {
		DisplayName *string `json:"displayName"`
		Location    *string `json:"location"`
		AssetTag    *string `json:"assetTag"`
		Notes       *string `json:"notes"`
		DNSSync     *bool   `json:"dnsSync"`
	}
```

### renameDevices

Source: `internal/api/server.go`.

```go
var body struct {
		DeviceIDs []string `json:"deviceIds"`
		Source    string   `json:"source"` // dns | hostname | sysName | pattern
		Pattern   string   `json:"pattern"`
		DryRun    *bool    `json:"dryRun"`
	}
```

### patchPolicy

Source: `internal/api/security.go`.

```go
var body policyBody
```

### patchProfile

Source: `internal/api/config.go`.

```go
var body struct {
		Name        *string                `json:"name"`
		Description *string                `json:"description"`
		Settings    *[]model.SettingChange `json:"settings"`
	}
```

### patchSettings

Source: `internal/api/server.go`.

```go
// Decodes into the already-loaded fleet.RuntimeSettings value.
```

### pollAll

Source: `internal/api/server.go`.

```go
var body struct {
		DeviceIDs []string `json:"deviceIds"`
		GroupID   string   `json:"groupId"`
	}
```

### printProviderSync

Source: `internal/api/printprovider.go`.

```go
var req struct {
		DryRun *bool `json:"dryRun"`
	}
```

### profileDrift

Source: `internal/api/config.go`.

```go
var body struct {
		Target  model.ConfigTarget `json:"target"`
		Refresh bool               `json:"refresh"`
	}
```

### putConsumableSettings

Source: `internal/api/consumables.go`.

```go
var cs model.ConsumableSettings
```

### putPrintProvider

Source: `internal/api/printprovider.go`.

```go
var in printprovider.Settings
```

### remediatePolicy

Source: `internal/api/security.go`.

```go
var body targetBody
```

### revertConfigWrite

Source: `internal/api/config.go`.

```go
var body struct {
		DryRun *bool `json:"dryRun"`
	}
```

### rotateCredentials

Source: `internal/api/security.go`.

```go
var body struct {
		Kind   string          `json:"kind"`
		Target security.Target `json:"target"`
		DryRun *bool           `json:"dryRun"`
	}
```

### setDeviceState

Source: `internal/api/server.go`.

```go
var body struct {
		State model.LifecycleState `json:"state"`
	}
```

### setGroupMembers

Source: `internal/api/server.go`.

```go
var body struct {
		DeviceIDs []string `json:"deviceIds"`
	}
```

### startCampaign

Source: `internal/api/firmware.go`.

```go
var req struct {
		DryRun *bool `json:"dryRun"`
	}
```

### startDiscovery

Source: `internal/api/server.go`.

```go
var p fleet.DiscoveryParams
```

### updateGroup

Source: `internal/api/server.go`.

```go
var patch struct {
		Name        *string          `json:"name"`
		Description *string          `json:"description"`
		Kind        *string          `json:"kind"`
		Rule        *model.GroupRule `json:"rule"`
	}
```

### campaignRequest

Source: `internal/api/firmware.go`.

```go
type campaignRequest struct {
	Name                    *string                  `json:"name"`
	Description             *string                  `json:"description"`
	PackageID               *string                  `json:"packageId"`
	GroupID                 *string                  `json:"groupId"`
	DeviceIDs               *[]string                `json:"deviceIds"`
	CanaryCount             *int                     `json:"canaryCount"`
	Concurrency             *int                     `json:"concurrency"`
	Window                  *model.MaintenanceWindow `json:"window"`
	FailureThresholdPercent *int                     `json:"failureThresholdPercent"`
	ScheduledAt             *time.Time               `json:"scheduledAt"`
	ClearScheduledAt        bool                     `json:"clearScheduledAt"`
	VerifyTimeoutSecs       *int                     `json:"verifyTimeoutSecs"`
	AllowDowngrade          *bool                    `json:"allowDowngrade"`
}
```

### applyBody

Source: `internal/api/config.go`.

```go
type applyBody struct {
	Target model.ConfigTarget `json:"target"`
	DryRun *bool              `json:"dryRun"`
}
```

### policyBody

Source: `internal/api/security.go`.

```go
type policyBody struct {
	Name        *string             `json:"name"`
	Description *string             `json:"description"`
	Items       *[]model.PolicyItem `json:"items"`
}
```

### targetBody

Source: `internal/api/security.go`.

```go
type targetBody struct {
	security.Target
	DryRun *bool `json:"dryRun"`
}
```

### DiscoveryParams

Source: `internal/fleet/service.go`.

```go
type DiscoveryParams struct {
	Targets    []string `json:"targets"`
	ZeroConf   bool     `json:"zeroconf"`
	AllowLarge bool     `json:"allowLarge"`
}
```

### RuntimeSettings

Source: `internal/fleet/runtime.go`.

```go
type NameSettings struct {
	Source string `json:"source"` // advertised (default) | dns | display
}

type RuntimeSettings struct {
	PollInterval          config.Duration `json:"pollInterval"`
	PollConcurrency       int             `json:"pollConcurrency"`
	DiscoveryEnabled      bool            `json:"discoveryEnabled"`
	DiscoveryInterval     config.Duration `json:"discoveryInterval"`
	DiscoveryTargets      []string        `json:"discoveryTargets"`
	SupplyWarningPercent  int             `json:"supplyWarningPercent"`
	SupplyCriticalPercent int             `json:"supplyCriticalPercent"`
	OfflineAfterFailures  int             `json:"offlineAfterFailures"`
	TelemetryRetention    config.Duration `json:"telemetryRetention"`
	Names                 NameSettings    `json:"names"`
}
```

## Device renaming and name-source preference

`POST /api/devices/rename` (operator) previews or applies bulk display-name updates across `deviceIds` (subject to group-scope checks via `allowedAll`). `source` must be `"dns"`, `"hostname"`, `"sysName"`, or `"pattern"`. When `source` is `"pattern"`, `pattern` expands `{serial}`, `{model}`, `{make}`, `{address}`, `{hostname}`, `{dnsName}`, `{sysName}`, `{location}`, and `{assetTag}` along with literal text. Omitting `dryRun` (or passing `dryRun: true`) returns a preview without writing; `dryRun: false` sets `displayName` and `displayNameSource` for non-skipped devices, records one `devices.rename` audit entry plus `device.patch` per updated device, and publishes `device.updated`. Devices are skipped when the selected source is empty, when a referenced DNS name is flagged `dnsNameSuspect`, or when the computed name equals the current name.

The runtime setting `names.source` (`GET/PATCH /api/settings`, admin to modify) selects the preferred name source shown in the UI (`"advertised"` default, `"dns"`, or `"display"`), falling back to the standard precedence (`displayName` → `sysName` → `hostname` → `model` → `address`) when a device lacks the preferred source or its DNS name is suspect. Per-device `dnsSync` (`PATCH /api/devices/{id}`) optionally keeps `displayName` synchronized when `DNSName` changes on refresh (audited as `device.patch` with reason `"dns sync"`).

### Target

Source: `internal/security/service.go`.

```go
type Target struct {
	DeviceIDs []string `json:"deviceIds,omitempty"`
	GroupID   string   `json:"groupId,omitempty"`
}
```

### EnrollRequest

Source: `internal/security/certs.go`.

```go
type EnrollRequest struct {
	CAProfileID string   `json:"caProfileId"`
	Use         string   `json:"use"` // identity (default) | ca
	CommonName  string   `json:"commonName,omitempty"`
	SANs        []string `json:"sans,omitempty"`
	DryRun      bool     `json:"dryRun"`
}
```

### Group

Source: `internal/model/model.go`.

```go
type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Kind        string     `json:"kind"` // static | rule
	Rule        *GroupRule `json:"rule,omitempty"`
	MemberIDs   []string   `json:"memberIds"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}
```

### GroupRule

Source: `internal/model/model.go`.

```go
type GroupRule struct {
	MakeContains     string     `json:"makeContains,omitempty"`
	ModelContains    string     `json:"modelContains,omitempty"`
	LocationContains string     `json:"locationContains,omitempty"`
	Subnet           string     `json:"subnet,omitempty"` // CIDR
	DeviceType       string     `json:"deviceType,omitempty"`
	State            string     `json:"state,omitempty"`
	HasCapability    Capability `json:"hasCapability,omitempty"`
}
```

### Secret

Source: `internal/model/model.go`.

```go
type Secret struct {
	Community    string `json:"community,omitempty"`
	AuthProtocol string `json:"authProtocol,omitempty"` // MD5 | SHA | SHA256 ...
	AuthPassword string `json:"authPassword,omitempty"`
	PrivProtocol string `json:"privProtocol,omitempty"` // DES | AES | AES256 ...
	PrivPassword string `json:"privPassword,omitempty"`
	Password     string `json:"password,omitempty"`
	PEM          string `json:"pem,omitempty"`
}
```

### Consumable

Source: `internal/model/model.go`.

```go
type Consumable struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             ConsumableType `json:"type"`
	Color            string         `json:"color"`      // "" | cyan | magenta | yellow | black | <named>
	PartNumber       string         `json:"partNumber"` // SKU; the primary match key against installed supplies
	CompatibleModels []string       `json:"compatibleModels"`
	Unit             string         `json:"unit"` // each | pack | ...
	OnHand           int            `json:"onHand"`
	WarningAt        int            `json:"warningAt"`
	CriticalAt       int            `json:"criticalAt"`
	Location         string         `json:"location"`
	Notes            string         `json:"notes"`
	ReorderURL       string         `json:"reorderUrl"`
	UnitCost         *float64       `json:"unitCost,omitempty"` // reference only
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}
```

### ConsumableSettings

Source: `internal/model/model.go`.

```go
type ConsumableSettings struct {
	DefaultWarningAt     int    `json:"defaultWarningAt"`
	DefaultCriticalAt    int    `json:"defaultCriticalAt"`
	AutoDecrement        string `json:"autoDecrement"`        // off | prompt | automatic
	ReplacementJumpPct   int    `json:"replacementJumpPct"`   // level rise that counts as a replacement
	ForecastMinDays      int    `json:"forecastMinDays"`      // minimum history before a forecast is given
	ReorderLeadDays      int    `json:"reorderLeadDays"`      // "reorder soon" when stock lasts no longer than this
	AutoSuggestFromScans bool   `json:"autoSuggestFromScans"` // offer unmatched installed supplies as new items
}
```

### SettingChange

Source: `internal/model/config.go`.

```go
type SettingChange struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}
```

### ConfigTarget

Source: `internal/model/config.go`.

```go
type ConfigTarget struct {
	DeviceIDs []string `json:"deviceIds,omitempty"`
	GroupID   string   `json:"groupId,omitempty"`
}
```

### PolicyItem

Source: `internal/model/security.go`.

```go
type PolicyItem struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Operator compares the current value with Value: eq | ne | gte | lte | in | notIn | set | unset
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	// Severity weights the compliance score: critical=3, high=2, medium=1, low=0.5.
	Severity string `json:"severity"`
	// Remediate: when false the item is assess-only (e.g. requires a manual step).
	Remediate bool `json:"remediate"`
	// Remediation is the change applied when the item fails (defaults to Key=Value for eq).
	Remediation *SettingChange `json:"remediation,omitempty"`
	// ManagementPath marks items whose remediation could remove the channel we
	// manage the device through (e.g. disabling SNMP, HTTP); the engine must keep
	// at least one working path (spec §11).
	ManagementPath string `json:"managementPath,omitempty"` // snmp | ipp | http | ""
}
```

### CAProfile

Source: `internal/model/security.go`.

```go
type CAProfile struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Protocol EnrollmentProtocol `json:"protocol"`
	URL      string             `json:"url"`
	// CredentialID references a credential holding the challenge/password/account key.
	CredentialID string `json:"credentialId,omitempty"`
	// CAFingerprint pins the enrollment server's TLS certificate (SHA-256 hex of
	// the leaf or any chain certificate); empty = system trust roots.
	CAFingerprint string `json:"caFingerprint,omitempty"`
	ValidityDays  int    `json:"validityDays,omitempty"`
	// KeyType of the keys generated for devices: rsa2048 (default, widest
	// printer support) | rsa3072 | ecdsa-p256.
	KeyType string `json:"keyType,omitempty"`
	// Options are protocol-specific, non-secret settings: ACME "email",
	// "dnsProvider" (manual) and "dnsWait" (e.g. "5m"); EST "label" (RFC 7030
	// §3.2.2 path segment); SCEP "caIdent" (GetCACert message).
	Options   map[string]string `json:"options,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}
```

### MaintenanceWindow

Source: `internal/model/firmware.go`.

```go
type MaintenanceWindow struct {
	Days  []string `json:"days,omitempty"`  // mon..sun; empty = any
	Start string   `json:"start,omitempty"` // "22:00"
	End   string   `json:"end,omitempty"`   // "06:00"
	TZ    string   `json:"tz,omitempty"`
}
```

### Settings

Source: `internal/printprovider/settings.go`.

```go
type Settings struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // cups | ipp-everywhere-list
	// QueuePrefix is prepended to every queue name; it is also how the module
	// recognises queues it owns, so it must never be empty.
	QueuePrefix string `json:"queuePrefix"`
	// DefaultOptions are lpadmin -o key=value pairs applied to every queue,
	// whitespace separated (e.g. "media=iso_a4_210x297mm sides=two-sided-long-edge").
	DefaultOptions string `json:"defaultOptions"`
	// AutoCreateForGroupID restricts queue creation to one group; empty means
	// every eligible device.
	AutoCreateForGroupID string `json:"autoCreateForGroupId"`
	// Driverless uses the CUPS "everywhere" model (IPP Everywhere); when false
	// a raw queue is created and clients supply the driver.
	Driverless bool `json:"driverless"`
}
```

## Query conventions and practical limits

- Device filters: `q`, `state`, `make`, `model`, `severity`, `group`, `sort`, `limit`, `offset`, `includeDecommissioned=true`; rule groups are resolved in the API. `total` on device list is the returned list length. Empty filters do not mean an API-token scope is applied to every other aggregate endpoint.
- Telemetry accepts RFC 3339 `from`/`to`; default 5,000 samples ordered oldest first. Reports additionally accept plain UTC dates; plain `to` includes that date by converting to the next midnight, while explicit timestamps remain exclusive. Maximum report span 400 days; default top 10 and last 30 days. `table` selects a CSV table named by `GET /api/reports`.
- `all=true` includes cleared alerts or nonpending replacement events, depending on endpoint. An acknowledgment does not clear an alert.
- Firmware upload body is capped at 512 MiB by the handler, with multipart parsing memory threshold 32 MiB; `models` can repeat and each value can contain comma-separated substrings. `version` and nonempty `file` are required. `StoragePath` is hidden from JSON. Package deletion is 409 while an unfinished campaign references it.
- Consumables import is at most 4 MiB raw CSV, matched by existing ID, then part number, then name. Batch/import HTTP 200 can contain `errors`; inspect it. `compatibleModels` export format is semicolon-separated.
- Sync's opaque cursor must be stored and passed back unchanged. `limit` defaults 200 and is capped at 1000. The weak ETag applies to the same requested page. This is incremental current-record synchronization, not an immutable change log. Device deletion tombstones exist; deleted device alerts can disappear without alert tombstones.
- `/api/devices/{id}/certificates/{certId}/install` requires the issued certificate to belong to that device. It ignores any preview body and queues a real installation. Private keys are never included in certificate responses.
- Unknown `/api/` paths return JSON 404. The `/` handler serves the SPA with index fallback for client routes. GET route registrations also handle HEAD under Go's method-pattern routing; no extra custom HEAD endpoints are registered.

## Response models and source guide

Domain models, including `Device`, `Status`, `Supply`, `Tray`, `Counters`, `User`, `Credential`, `Alert`, `Job`, `JobResult`, `AuditEntry`, `TelemetrySample`, `NotificationLog`, `Outbox`, `StockEvent` and `SupplyLink`, are defined with JSON tags in `internal/model/model.go`. Configuration models live in `internal/model/config.go`; security/PKI in `internal/model/security.go`; firmware in `internal/model/firmware.go`. Feature-enriched views are declared beside the handler/service: `internal/firmware/inventory.go` and `campaign.go`, `internal/consumables/{service,forecast}.go`, `internal/catalog/`, `internal/printprovider/`, `internal/siem/`, `internal/reports/`. A JSON object or array is exactly the table's wrapper: do not assume every submission returns `{job}`—several return a direct Job.

An event stream sends `{id,version,type,at,origin,data}` envelopes. A job fetch returns `{job,results}`; per-device results carry `status`, `message` and feature-specific `data`. A successful HTTP submission is not evidence that a device operation later succeeded. Poll `GET /api/jobs/{id}` and inspect all results/outcomes.

## Known discrepancies

There is no generic job resume/cancel endpoint, administrative password-reset endpoint, user PATCH, certificate-private-key download or report-compliance implementation beyond its unavailable placeholder. Some source guides abbreviate certificate install's parameter as `issuedId`; the registered path uses `{certId}`. Scope checks apply only where implemented, not universally to query-based aggregates/sync/metrics. Do not infer an endpoint from UI terminology or from the original design specification.
