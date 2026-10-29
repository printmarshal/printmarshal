[Home](Home.md) · [Configuration reference](Configuration-reference.md) · [API reference](API-reference.md)

# Discovery and inventory

## Candidate discovery versus identification

Discovery finds addresses worth asking; identification determines whether an address is a manageable printer and merges protocol observations into one identity. An open SNMP agent alone need not be a printer. Run discovery through `POST /api/discovery/runs` (operator), inspect the returned job ID, then retrieve `GET /api/jobs/{id}` for per-address results. No discovery path performs a SET, configuration apply or firmware upload.

```bash
PM=http://localhost:8080
curl -s -H "Authorization: Bearer $SESSION" -H 'Content-Type: application/json' \
  -d '{"zeroconf":true,"targets":["192.0.2.0/28"],"allowLarge":false}' \
  "$PM/api/discovery/runs"
```

This is an example for a subnet you control; replace the documentation address with an authorized target. `zeroconf` is the real key (not `zeroConf`). In the HTTP handler its initial value is true, while `targets` and `allowLarge` are empty/false. An explicit request target list is distinct from the scheduled defaults in `discovery.targets`.

## Zero-configuration discovery

mDNS/DNS-SD browses `_ipp._tcp`, `_ipps._tcp`, `_printer._tcp`, `_pdl-datastream._tcp`, `_scanner._tcp`, `_uscan._tcp`, `_uscans._tcp`. It sends PTR queries on multicast-capable IPv4 interfaces and joins `224.0.0.251:5353` with port reuse so it can coexist with Bonjour/Avahi. Responses supply SRV/TXT/A information; where A is absent the response source address may be used. Default collection time is 5 seconds.

WS-Discovery sends a Probe to `239.255.255.250:3702` and collects ProbeMatch responses for 3 seconds by default. These methods need suitable multicast/L2 access. Candidates from all methods are merged by address, preserving discovered hostname/services/TXT hints. Hints can fill missing make/model/UUID/admin URL/location information and mark scanning services as MFP evidence.

The scheduled discovery switch is off by default. Enabling it runs zero-conf plus configured active targets every hour by default; manual runs do not require that switch. The scheduler checks configuration every five seconds, but discovery intervals are clamped to at least five minutes.

## Active scans

| Target syntax | Behavior |
| --- | --- |
| `192.0.2.10` | Literal address. |
| `192.0.2.1-192.0.2.20` | Inclusive IPv4 range. |
| `192.0.2.1-20` | Last-octet IPv4 range. |
| `192.0.2.0/28` | IPv4 CIDR; network/broadcast skipped when prefix is shorter than /31. |
| Hostname without a hyphen | Resolves with the server resolver; first returned address used. |

The configured expansion cap is 4,096 addresses. Hitting it records a skipped-result message with the count; it is not silent truncation. `allowLarge:true` removes that cap for the run. IPv6 literals can be passed, but CIDR scanning is IPv4-only. A hyphen is parsed as range syntax before hostname resolution, so a hyphenated hostname can fail as a bad range; use its address in that case.

The production liveness probe is an SNMP system-group read with 1-second timeout and one retry, using global read credentials. `scanWorkers` defaults to 64. Only responders go to the richer identification stage. An IPP-only device that does not answer SNMP will not be discovered by this active-scan probe; zero-conf may still supply its address. Credentials scoped only to an undiscovered device/group cannot supply a bare-address probe.

## Identity reconciliation

Identifiers (standards and vendor) run during identification and successful results merge by driver priority. Stable data includes serial, UUID, MAC, address, make/model, firmware, device type and enterprise ID. Every device keeps its resolved capability map and the methods that observed it.

Persistence checks identities in this order: nonempty **serial → UUID → lowercased MAC → address**. When an address match is the only match and the incoming and stored records have conflicting nonempty serial, UUID or MAC, the store refuses to merge them: it creates a new device record instead. The old device retains its history, operator fields, groups and credentials. Address reuse can therefore legitimately leave two records with the same address until an operator retires the old one.

Rediscovery fills/refreshed automatic fields without replacing operator display name, location, asset tag or notes. Lifecycle state also survives rediscovery. Do not interpret a matching model name alone as proof of identity. Concurrent discoveries of the same entirely new identity are not protected by a unique serial/UUID/MAC database constraint; avoid overlapping large discovery runs when auditing reconciliation results.

Sources: `internal/store/{sqlite,postgres}_devices.go`, `internal/fleet/service.go`, `internal/engine/engine.go`.

## The device web page

Every device record carries a `webUrl`, the link to the printer's own
administration page, shown as an icon beside the address in the device list
and as the **Device web page** button on the detail page. It is resolved from
three sources, in order:

1. An operator override, set in the **Web page** field of the identity card
   (`PATCH /api/devices/{id}` with `webUrl`; an empty value clears it). The
   URL must be `http` or `https` with a host and must not carry credentials.
2. The URL the device advertises (IPP `printer-more-info`, eSCL `AdminURI`,
   or a DNS-SD `adminurl` record), when it looks like a web page rather than
   a protocol endpoint: URLs on ports 631, 515 or 9100, or under `/ipp` or
   `/eSCL`, are ignored, because they answer with a status page, not the
   administration UI.
3. A probe. After identification, and on a poll while nothing is known yet,
   PrintMarshal requests `https://<host>/` and then `http://<host>/` with a
   3-second limit each and keeps the first that answers with any HTTP
   response. The host is the device's hostname when one is known, else its
   address. The probe is a plain GET of the root page and changes nothing on
   the device; certificates are not verified (printers ship self-signed
   ones), and the operator's browser shows its own warning when the link is
   opened. A probe that finds nothing is remembered (`webUrlProbed` =
   `none`) and repeated only when the device is identified again, so polls
   do not knock on closed ports forever.

Links open in a new tab with `rel="noopener noreferrer"`. Credentials are
never placed in the URL.

## Lifecycle

Lifecycle is operator-controlled and separate from online/offline status. A poll can show a discovered device online or an active device offline; it does not automatically enact lifecycle transitions.

| Current `state` | Allowed target states |
| --- | --- |
| `discovered` | `onboarded`, `active`, `decommissioned` |
| `onboarded` | `active`, `needs_attention`, `decommissioned` |
| `active` | `needs_attention`, `decommissioned` |
| `needs_attention` | `active`, `decommissioned` |
| `decommissioned` | `onboarded` |

Use `POST /api/devices/{id}/state` with `{"state":"active"}`. `PATCH /api/devices/{id}` accepts only `displayName`, `location`, `assetTag`, `notes`. Default fleet listings and scheduled polls exclude decommissioned devices; explicit filters/targets can include them on some read/configuration paths. Firmware excludes them. Deletion is an admin action, removes the device and associated records, and records a sync tombstone; decommissioning preserves the record.

`GET /api/devices` supports `q`, `state`, `make`, `model`, `severity`, `group`, `sort`, `limit`, `offset`, `includeDecommissioned`. Sort values are `name`, `model`, `address`, `lastSeen`, `severity`. `total` is the count of returned/resolved devices, not a promise of the unpaginated database size. CSV endpoints are `/api/devices.csv` and `/api/supplies.csv`.

## Static and rule groups

Static groups persist `memberIds`; use `PUT /api/groups/{id}/members` with `{"deviceIds":["DEVICE_ID"]}` to replace membership. Rule groups contain `kind:"rule"` and a `rule` object. All nonempty tests are ANDed:

| Rule key | Match |
| --- | --- |
| `makeContains`, `modelContains`, `locationContains` | Case-insensitive substring. |
| `subnet` | CIDR contains the device address. Invalid CIDR matches nothing. |
| `deviceType`, `state` | Exact value. |
| `hasCapability` | Key exists in the capability profile. |

An empty rule `{}` matches every device considered by the caller; different operations may exclude decommissioned devices. Static and rule membership also participates in credential resolution. Group-scoped API principals cannot mutate groups, users or tokens, because group membership defines their access boundary. Group scope is described in [Identity and access](Identity-and-access.md).

```json
{"name":"Office MFPs","description":"Active multifunction fleet","kind":"rule","rule":{"deviceType":"mfp","state":"active"}}
```

Catalogue family/support views are separate from inventory identity. See [Vendor plugins and drivers](Vendor-plugins-and-drivers.md) for research evidence versus observed capability and qualification.

## Known discrepancies

A target is treated as a range only when the text before its first hyphen is an IPv4 address, so hostnames such as `print-server-3` resolve as names. The “read-only decommissioned” expectation should not be broadened into a universal write guard: configuration target resolution includes explicitly named decommissioned records. The identity conflict rule is implemented on both stores, but concurrent identity upsert is not fully transactional across independent discoveries.

Sources: `internal/discovery/{targets,scan,mdns,wsd}.go`, `cmd/printmarshal/main.go`, `internal/api/server.go`, `internal/model/model.go`, `internal/store/{sqlite,postgres}_devices.go`.
