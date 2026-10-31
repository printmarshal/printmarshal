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

Every identification also records, per driver that applies to the device, why that driver did not answer (`identifyErrors`, replaced on each scan and empty when everything answered). A vendor plugin that merely belongs to another manufacturer is not an error. The device page shows these under Reachability ("SNMP did not answer on the last scan: request timeout …"), and the discovery log says `no answer from snmp` for the address, so a printer that stops answering SNMP is visible instead of silently keeping its earlier values: a scan never blanks a field it could not read, so the earlier serial, model and firmware stay. Identification asks the generic SNMP driver, every vendor plugin and the scan probe at once, and a printer's SNMP agent can drop that burst; `poll.snmpSessionsPerDevice` (default 2) bounds how many SNMP sessions are open to one device at a time, and `poll.snmpTimeout` / `poll.snmpRetries` now apply to the vendor plugins as well as the generic driver.

Rediscovery fills/refreshed automatic fields without replacing operator display name, location, asset tag or notes. Lifecycle state also survives rediscovery. Do not interpret a matching model name alone as proof of identity. Concurrent discoveries of the same entirely new identity are not protected by a unique serial/UUID/MAC database constraint; avoid overlapping large discovery runs when auditing reconciliation results.

Sources: `internal/store/{sqlite,postgres}_devices.go`, `internal/fleet/service.go`, `internal/engine/engine.go`.

## Import from a CSV file

Use an import when you already have a list of your printers, in a spreadsheet or an asset register, and want them in the inventory without scanning for them, or want to fill in locations, asset tags and notes across printers you already have. On the Discovery page, choose **Import CSV…**, then **Choose a CSV file**. **Download template** saves a CSV holding only the header row.

### What the file needs

Each row needs one key: an address, a hostname that resolves, or the serial of a printer already recorded. A file with no `address`, `hostname` or `serial` column is refused. A row that has none of them usable is skipped:

| Row has | Result |
| --- | --- |
| An address | Matches a recorded printer, or creates one. An address must be an IP address, or the name a printer is already recorded under; otherwise the row is skipped with "address is not an IP address". |
| A hostname, no address | The console looks the name up once, and uses the address it gets. If the lookup fails and no serial matches, the row is skipped with "no address: hostname NAME did not resolve". |
| A serial only | Updates the printer with that serial. If there is none, the row is skipped with "no address; scan first", because a record with no address could not be polled. |

Rows match by address first, then by serial, and decommissioned printers count. A row whose address and serial belong to two printers is skipped with "address and serial belong to different devices". The key columns only find or create the printer: an import does not change the address, hostname or serial of a printer already recorded.

Header names ignore case, spaces, underscores and hyphens, so `Asset tag`, `asset_tag` and `assetTag` are the same column. Naming a column twice, for example with both `name` and `displayName`, refuses the file.

| Column | Also accepted as | Value |
| --- | --- | --- |
| `address` | `ip`, `ipAddress` | Key. |
| `hostname` | `host` | Key, when the row has no address. Otherwise a hint for identification. |
| `serial` | `serialNumber` | Key. Matching ignores case. |
| `displayName` | `name` | Up to 200 characters. |
| `location` | | Up to 200 characters. |
| `assetTag` | | Up to 200 characters. |
| `notes` | `note` | Up to 2,000 characters. Replaces the notes rather than adding to them. |
| `webUrl` | `webAddress` | An `http` or `https` URL with a host and no credentials. |
| `dnsSync` | | `true`, `yes` or `1`; `false`, `no` or `0`. |
| `state` | `lifecycleState` | `discovered`, `onboarded`, `active`, `needs_attention` or `decommissioned`. |
| `group` | `groupName` | The exact name of an existing static group. The printer joins it; no membership is ever removed. |

The console's own device export (`/api/devices.csv`) is accepted, so you can export from one instance and import into another. Its other columns, such as `make`, `model` and `lastSeen`, are ignored. Any column the console does not know is ignored too, and the preview lists it under "Ignored columns".

An invalid value skips the whole row, never part of it, and the reason names the cell: for example "webUrl is not a URL", "state is not a lifecycle state", or `group "Lab" does not exist`. A rule-based group is refused as well, because only static groups take members. A second row for the same address, serial or printer is skipped with "duplicate of row N".

### Limits

A file can have up to 5,000 data rows and be up to 2 MB. It must be UTF-8 text, and a byte-order mark is fine. The separator can be a comma or a semicolon; the console picks whichever the header line uses more. A file over either limit is refused with the limit in the message. Blank rows do not count.

### The preview

Choosing a file shows the preview. It changes nothing and contacts no printer. The only thing it sends out is the DNS lookup for rows that give a hostname and no address, so a long list of hostnames can take a while. Above the table are the totals ("3 to create, 10 to update, 1 skipped"). The table has one line per row, 200 to a page:

- **Row**: the row's line in the file, where the header is line 1.
- **Key**: the address, `host:NAME` or `serial:NUMBER`.
- **Outcome**: **Create**, **Update** or **Skip**.
- **Fields**: the values that would be set.
- **Held back**: the values that would be left alone, each with its reason.
- **Reason**: why the row is skipped.

A row that would change nothing is skipped with "unchanged", or with "nothing to change: every value is held back" if its only changes are held back.

### Values already present

The import fills in what is empty and leaves alone what someone chose. A supplied value is held back according to where the present value came from:

| Present value | Reason shown under **Held back** |
| --- | --- |
| Set by a person in the console | "set by an operator; column not marked replace" |
| Set by DNS sync | "kept in sync with DNS; column not marked replace" |
| Has a value, source unknown | "already has a value; column not marked replace" |
| Set by an earlier import | Not held back. An import may overwrite its own earlier values, so you can correct a typo by importing again. |

An unknown source means an edit from before sources were recorded, or a location read from the printer. A field an operator has deliberately emptied also counts as set by an operator, and is held back.

To overwrite, tick the column under **Replace values an operator set**; the preview updates. The tick applies to the whole column and to all three reasons above, not only operator edits. `group` and the key columns cannot be marked.

Marking a column does not get round the lifecycle. A state the printer cannot move to from its current one is held back with "cannot move from X to Y", whether or not the column is marked. A new printer starts as `discovered`; see [Lifecycle](#lifecycle) for the allowed moves.

An empty cell leaves the field alone, even in a marked column. An import cannot clear a field.

A display name equal to the name the printer already goes by is no change, so importing an export back does not freeze a name that comes from DNS or from the printer itself.

### Who can do what

Operators and administrators can open the dialog and preview. Only an administrator can apply, with **Import N printers**; an operator sees "An administrator applies the import." Viewers do not get the button.

For a group-scoped user, a row for a printer or group outside their groups is skipped with "outside your groups". So is a row that would create a printer without naming, in `group`, one of their groups, since the new printer would land outside them.

### Applying and results

Applying starts a job of type `import`, which works the plan out again from the file, so a change to the inventory since the preview is taken into account. The Discovery page then shows the job: **Rows processed**, a log, and the **Results** card with one line per row (**Created**, **Updated**, **Skipped** or **Failed**) and its counts. It also appears under **Previous runs**, tagged **Import**. The audit trail gets one `devices.import` entry with the counts and the replaced columns, never the file's content, plus `device.created`, `device.patch` and `group.members` entries for each change.

Printers to create are identified first, the same read-only way discovery identifies them, eight at a time and up to 60 seconds each, followed by a read-only first poll. The rows are then applied in file order, so **Rows processed** can stay at 0 for a while. Nothing is written to a printer.

A printer that does not answer is still created, with the address and the file's values, and its result says it is "not reachable yet, so it will be identified when it answers". It is identified when it first answers at a scheduled poll or a discovery. If it then turns out to be a printer already recorded at another address, as shown by its serial, that record takes the address and the address-only one stays for an administrator to remove.

If identification finds that a printer to create is already recorded, the row updates that record by the same rules as any update, and its result reads **Updated**, or **Skipped** with "already recorded; unchanged".

There is no way to cancel an import while it runs. If the server shuts down during one, rows already applied stay applied, and the job fails with "stopped at row N". After a restart, upload the file again.

### Where a value came from

The device page shows where an imported value came from, as "from import, 31 Oct" beside the display name, DNS sync, location, web page, asset tag and notes. A display name kept in sync shows "from DNS". An operator's later edit removes the note, and that field is held back from future imports.

Sources: `internal/importcsv/{parse,plan}.go`, `internal/api/import.go`, `web/src/components/ImportDialog.tsx`.

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

Use `POST /api/devices/{id}/state` with `{"state":"active"}`. `PATCH /api/devices/{id}` accepts only `displayName`, `location`, `assetTag`, `notes`, `dnsSync`. Default fleet listings and scheduled polls exclude decommissioned devices; explicit filters/targets can include them on some read/configuration paths. Firmware excludes them. Deletion is an admin action, removes the device and associated records, and records a sync tombstone; decommissioning preserves the record.

`GET /api/devices` supports `q`, `state`, `make`, `model`, `severity`, `group`, `sort`, `limit`, `offset`, `includeDecommissioned`. Sort values are `name`, `model`, `address`, `lastSeen`, `severity`. `total` is the count of returned/resolved devices, not a promise of the unpaginated database size. CSV endpoints are `/api/devices.csv` and `/api/supplies.csv`.

## Reverse DNS names, name-source setting and explicit rename

Reverse DNS lookup (`discovery.reverseDns.enabled`) is optional and off by default. The switch and its options (DNS server, short or full names, forward-confirm, lookup timeout, refresh interval) are runtime settings: an administrator changes them under Settings → Polling & discovery (the `reverseDns` object in `GET/PATCH /api/settings`) and they apply on the next identification or poll without a restart; the config file and `PM_REVERSE_DNS_*` variables are the defaults, `GET /api/settings` reports under `sources.reverseDns` whether each default came from the environment, the file or the built-in value, and `DELETE /api/settings` returns to those defaults. When the name source is "DNS name" while lookups are off, the Settings page says so next to the dropdown. When enabled (or triggered manually via `POST /api/devices/{id}/refresh-hostname`), PrintMarshal stores the PTR result in `Device.DNSName` (`dnsName`, with timestamp `dnsCheckedAt`) and never overwrites `Device.Hostname` (which remains what mDNS or protocol drivers report). Looked-up names with invalid hostname characters, labels over 63 characters or total length over 253 characters, or IP-encoded octets joined by `-` or `.` (such as `10-0-0-7` or `ip-10-0-0-7`) are marked `dnsNameSuspect: true` and excluded from automatic selection and rename actions.

A looked-up DNS name never silently changes what is displayed. Default name precedence remains `displayName` → `sysName` → `hostname` → `model` → `address`. Administrators can set the preferred display source across the UI via `names.source` in `GET/PATCH /api/settings` (`advertised` default | `dns` | `display`), and operators can explicitly copy names into `displayName` using `POST /api/devices/rename` (`source`: `dns` | `hostname` | `sysName` | `pattern`, dry-run by default, recording `displayNameSource`). Per device, `dnsSync` (`PATCH /api/devices/{id}`) defaults to `false`: when enabled, a refreshed non-suspect DNS name that differs from `displayName` updates `displayName` automatically (audited as `device.patch` with reason `"dns sync"`); when off and `displayNameSource` is `"dns"`, a changed DNS name sets `dnsNameChanged: true` on read so the UI can offer a one-click update.

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


## Editing many devices at once

Select devices in the inventory (a group's devices by filtering on the group, then **Select all**) and use **Edit…** in the bulk toolbar. The dialog sets the operator fields the fleet shares: DNS sync on or off (keep each display name synced with its DNS name), **Rename from DNS now** (devices without a usable DNS name are skipped and say so), a location, and a note appended to each device's notes on its own paragraph, existing notes kept. Every field can be left as it is. Each changed device gets its own audit entry and a `devices.bulk` summary records the action; a group-scoped token can edit only devices in its groups. The **Rename…** dialog offers "Keep these names in sync with DNS from now on" when the source is DNS. Reverse DNS lookups are off by default: the device page says so beside DNS sync, with a link to the setting under Settings, Polling and discovery, Reverse DNS; until it is on, names are looked up only when you press Refresh or rename from DNS, which looks up any device that has no DNS name yet. The list marks devices kept in sync with a **DNS sync** pill under the name; both dialogs say how many of the selected devices already have DNS sync on and when reverse DNS is off, and the bulk result line names the reasons devices were skipped.
