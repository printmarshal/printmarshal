# Vendor plugins (spec §6, §7 "vendor device web services", roadmap phase 7)

Every sub-package here is an `engine.Driver` for one manufacturer. The core never
asks "is this a Ricoh?"; each plugin decides that itself in `Identify` from the SNMP
`sysObjectID` enterprise number (or, when the agent reports none, from badge text in
`sysDescr` / `hrDeviceDescr`, or from the IPP `printer-device-id` MFG field) and returns
`vendorkit.ErrNotVendor` for everything else. Registering all of them is therefore safe
for any fleet. `plugins.RegisterAll` registers them in the research's priority order;
`plugins.List()` returns each plugin's descriptor (badges, families, management paths,
capability/evidence rows) for `/api/drivers`.

The shared scaffolding lives in [`vendorkit`](vendorkit): matching, identity from the
standard groups, the IPP fallback, a read-only `read_config` / `read_security` base
(`identity.*` from the system group plus `vendor.<name>.*` facts), fixture transports
for tests, and the descriptor types.

Sources are cited with the ids of the two research reports
(`research/printer-models-and-manageability-2026-10-28.md` = `[Rxx]`, `[Hxx]`, `[Cxx]`,
`[Xxx]`, `[Kxx]`, `[Yxx]`, `[Bxx]`, `[Lxx]`, `[Exx]`, `[Axx]`, `[Txx]`, `[Oxx]`, `[Gxx]`,
`[Fxx]`, `[Sxx]`, `[Mxx]`; `research/enterprise-printer-expansion-and-integration-evidence-2026-10-28.md`
= `[Nxxnn]`). The management-path column comes from the catalogue JSON's
`management_profiles`; it describes the vendor's documented routes, not what PrintMarshal
implements.

## Evidence legend

| Status | Meaning |
| --- | --- |
| **observed** | exercised read-only against real hardware by this build and cross-checked |
| **documented** | the research or primary vendor documentation describes the interface used |
| **unverified** | relies on community MIB knowledge the research could not confirm; the code is probe-gated and only reports when the device answers sensibly |
| **unsupported** | not implemented: no documented, concrete interface (the standards drivers still serve the capability where they can) |

## Capability / evidence matrix

Capabilities not listed for a plugin are served by the standards drivers (`snmp`,
`ipp`) exactly as before; a plugin never takes a capability away from them.

| Plugin | Enterprise(s) | identify | read_status | read_counters | firmware_read | read_config | read_security | write_config / write_security | manage_certs | firmware_update |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **ricoh** | 367 | documented [R23] [R25] | — | **unverified** engine counter table `.1.3.6.1.4.1.367.3.2.1.2.19.5.1` (community OID; gated on a `Total` row) [R08] [R09] | — | documented: identity.* + badge/family [R08] | unsupported: Web Image Monitor only [R10] [R11] | unsupported: PMAPI / Device Interfaces need RiDP access [R08] [R09] | unsupported: native SCEP/EST/ACME U [R11] | unsupported: no public endpoint may be inferred from the Firmware Update Tool / ACT [R21] [R27] |
| **hp** | 11, 236 (Samsung legacy, identity only) | **observed** PML id objects + Jetdirect sysDescr [H06] [H07] [NHP05] | **observed** HR/Printer MIB + PML panel line + LEDM alerts [H08] | **observed** PML total/mono/colour/duplex [H08] | **observed** PML date code + component table [H07] [H11] | **observed** LEDM `/DevMgmt` → canonical keys [H08] [H09] | **observed** protocols.* / adminPassword.set from LEDM [H14] [NHP05] | unsupported: EWS / Web Jetadmin are vendor tools; no public write contract [H08] [H10] [NHP08] [NHP19] | unsupported: EST Connector is manager-side (WXP) [H12] | unsupported: `.bdl` only through EWS / Web Jetadmin [H11] [NHP02] |
| **canon** | 1602 | documented [C03] [C06] [NCA02] [NCA03] | — | **unverified** counter table `.1.3.6.1.4.1.1602.1.11.1.3.1.4.<id>` keyed by Canon's documented Check Counter ids 101/108/113/122/501 (community OID; gated) [C03] | — | documented | unsupported: Remote UI only [C03] | unsupported: DSC / iW MC are vendor tools [NCA01] | unsupported: device-side SCEP documented (imageFORCE C5100) but no client API [C08] [C12] | unsupported: CDS / eMaintenance [C11] |
| **xerox** | 253 | documented [X01] [X02] [X03] [NXE01] | — | unsupported: meters via EWS/clone only | — | documented | unsupported [X02] [X06] | unsupported: clone files are a vendor path [X02] | unsupported: SCEP/EST/ACME U in the shared guide [X02] | unsupported: `.dlm` via EWS / Fleet Orchestrator / FTP directory only [X02] |
| **konicaminolta** | 18334 (Develop, Olivetti badges) | documented [K02] [K08] [NKM01] | — | unsupported: meters via bizhub OpenAPI, partner schema not obtained [K03] | — | documented | unsupported [K03] | unsupported: OpenAPI needs partner specs [K03] | unsupported: NDES/SCEP is Fleet RMM-mediated [K06] | unsupported [K10] [K11] |
| **kyocera** | 1347 | documented [Y01] [Y07] [NKY01] | — | unsupported | — | documented | unsupported [Y01] [NKY01] | unsupported: CCRX / Net Viewer / KFS [Y08] | unsupported: SCEP is a device feature, no API [Y01] [Y04] | unsupported [Y04] [Y09] |
| **brother** | 2435 | documented [B01] [B02] [NBR01] | — | unsupported | — | documented | unsupported [B02] | unsupported: Mass Deployment Tool / BRAdmin [B02] [B04] | unsupported [B01] | unsupported: Firmware Update Tool fetches from Brother's server [B01] [B04] |
| **lexmark** | 641 | documented [L02] [L05] [L06] [NLE01] | — | unsupported [NLE02] | — | documented | unsupported [L01] [L02] | unsupported: MVE over LST/NPAP/XMLNT (proprietary) [L09] [NLE02] | unsupported: MVE enrols and installs; not native EST [NLE04] | unsupported: MVE upload [NLE03] |
| **epson** | 1248 | documented [E01] [E03] | — | unsupported | — | documented | unsupported [E03] [E04] | unsupported: Device Admin / Web Config [E05] | unsupported [E04] | unsupported: Web Config / Device Admin; USB recovery [E03] [E04] |
| **sharp** | 2385 | documented [A01] [NSH01] [NSH02] | — | unsupported: SRDM meters [NSH01] | — | documented | unsupported [A01] | unsupported: web clone / SRDM [NSH02] | unsupported [A01] | unsupported: SRDM one-to-many [A01] [NSH03] |
| **toshiba** | 1129 | documented [T01] [T02] [T07] | — | unsupported | — | documented | unsupported [T01] [T04] | unsupported: TopAccess [T07] | unsupported: SCEP feature, no API [T01] | unsupported [T07] |
| **oki** | 2001 | documented [O01] [O02] [O03] | — | unsupported: MIB files on supplied media [O03] | — | documented | unsupported [O04] | unsupported: Configuration Tool [O04] | unsupported [O05] | unsupported: Firmware Update Tool [O05] |
| **fujifilm** | 297 (Fuji Xerox badge) | documented [F01] [F03] [F05] | — | unsupported | — | documented | unsupported [F03] | unsupported: Internet Services [F05] | unsupported [F03] | unsupported [F02] |

Priorities: `identify` 60 for every plugin (so canonical make, serial and firmware from
the vendor objects win the merge); HP's observed read capabilities 60; the unverified
Ricoh/Canon counter readers 60 but only declared for devices whose probe answered;
the base `read_config` 20. Each `doc.go` repeats its plugin's table.

Every `read_config` document is read-only (`Writable: false`, `ReadOnlyReason` set):
no plugin implements `ConfigWriter`, `SecurityWriter`, `CertManager` or
`FirmwareUpdater`, because the research found no public wire-level write contract for
any vendor. Firmware delivery mechanisms that are documented (HP `.bdl` via EWS/WJA,
Xerox `.dlm` via EWS/Fleet Orchestrator/FTP, Sharp SRDM, Lexmark MVE) are all
vendor-tool paths, so `firmware_update` stays unsupported and says so in each `doc.go`.

## Documented management paths (catalogue `management_profiles`)

| Vendor | Read | Configuration | Firmware | Admin API | Sources |
| --- | --- | --- | --- | --- | --- |
| HP (FutureSmart) | SNMP + EWS; LEDM XML on LaserJet Pro / OfficeJet class | EWS, Web Jetadmin (SNMP Get/Set, HTTPS 7627 device services) | `.bdl` via EWS / WJA repository; match product number, branch, revision, date code | none public; Workpath APIs are for app deployment | [NHP19] [NHP20] [H11] [NHP07] [NHP08] |
| Ricoh | Web Image Monitor; Standard/Private MIB + SmartDeviceMonitor API via RiDP; PMAPI | Device Interfaces / PMAPI / private MIB (program access, model matrix) | ACT, Firmware Update Tool; third-party upload unverified | partner interfaces only | [R08] [R09] [R10] [R11] [NRI01] [NRI05] |
| Canon | Remote UI; SNMPv1/v3 (DX C3900, DX 8900) | Device Settings Configurator, iW MC / EMC | CDS / eMaintenance | settings-tool compatibility is not an API | [NCA01] [NCA02] [NCA03] [C08] |
| Xerox | SNMP + EWS; AltaLink names RFC 2790/3805 | EWS clone files / Fleet Orchestrator (AltaLink) | `.dlm` via EWS / Fleet Orchestrator / FTP; restricted downgrades | EIP coverage unverified | [X02] [X03] |
| Konica Minolta | Web Connection + SNMP | bizhub OpenAPI (TLS) device info / configuration / meters | Fleet RMM / CS Remote Care | partner OpenAPI | [K02] [K03] [K10] [NKM01] |
| Kyocera | SNMP + Command Center RX | CCRX, Net Viewer / Device Manager / KFS | Net Viewer; package/transport unverified | none verified | [NKY01] [NKY03] [Y07] [Y08] |
| Lexmark | SNMPv3-gated MVE support; MIB pack = Printer MIB v1 | EWS; MVE over SNMP/HTTPS + LST/NPAP/XMLNT | MVE upload with access-control prerequisite | Fleet Tracker REST / LDD JSAPI are server/job APIs | [NLE01] [NLE02] [NLE03] [NLE04] [L07] [L08] |
| Sharp | SRDM discovery/status/meters per matrix row | web management / cloning, SRDM | SRDM one-to-many on supported rows | OSA is an app framework | [NSH01] [NSH02] [NSH03] |
| Toshiba | TopAccess + SNMP; component software list | TopAccess | component versions; upload rights unconfirmed | none verified | [T01] [T04] [T07] |
| Brother | SNMP / Web Based Management | BRAdmin / Mass Deployment Tool | Firmware Update Tool (vendor server) | none verified | [B02] [B04] [B05] [NBR01] |
| Epson | SNMP + Web Config | Epson Device Admin batch settings | Web Config / Device Admin; USB recovery | Open Platform is workflow | [E03] [E04] [E05] [NEP02] |
| OKI | SNMPv1/v3 (C650); MIB files on media | Configuration Tool / EWS | official C650 updater | none verified | [O02] [O03] [O04] [O05] |
| Fujifilm | SNMPv3 + Internet Services; secure mode may disable SNMP | Internet Services | regional route only | workflow only | [F03] [F05] [F06] [NFU01] |
| Samsung legacy | SyncThru Web Service | SyncThru tools | legacy entitlement check | none | [G01] [G02] [G03] |

## Regional badges and aliases

| Plugin | Badge labels resolved to the canonical make | Rule |
| --- | --- | --- |
| ricoh | Savin, Lanier, Gestetner, Nashuatec, Rex-Rotary, NRG, IKON, infotec | Savin/Lanier/Gestetner/Nashuatec/Rex-Rotary are enumerated for the same evaluated product set by JISEC [R23]. NRG/IKON/infotec are labels only: an IKON dealer record never selects the plugin without enterprise 367 [R24]. `Identity.Make` = "Ricoh"; the presented badge stays in the model text and in `vendor.ricoh.badge`. |
| hp | Hewlett-Packard, Samsung | Samsung (enterprise 236) is identified by the HP plugin with `Make` "Samsung" and receives **no** HP enrichment: SyncThru is a separate legacy generation [G01] [M06]. |
| konicaminolta | Develop (ineo), Olivetti (d-Color / d-Copia) | Develop ineo identity is declared by the official download centre [K08]; Olivetti/Muratec/Sindoh need explicit identity evidence, so the label is informational and the enterprise OID (18334) decides. |
| fujifilm | Fuji Xerox | Enterprise 297 is Fuji Xerox's; Fujifilm identifiers must never select the Xerox plugin or AltaLink firmware [F03]. |
| canon, xerox, kyocera, brother, lexmark, epson, sharp, toshiba, oki | — | No research-backed rebrands (Kyocera Copystar/UTAX/TA, Xerox-badged Fuji devices etc. are deliberately not aliased until evidence exists). |

Badge text alone (without an enterprise match) is only consulted when the agent reports
no `sysObjectID` enterprise at all; a badge word on another vendor's agent is rejected
(`TestBadges` in `ricoh`).

## Real-hardware validation: HP Color LaserJet MFP M283fdw, 198.51.100.5 (2026-10-28)

Read-only only: SNMP GET/walk (`snmpwalk -v2c`), IPP Get-Printer-Attributes and HTTP GET
of LEDM pages. Nothing was set, posted or uploaded. Recordings:
`testdata/fixtures/hp-m283fdw-enterprise.snmp` (system, ifPhysAddress, HR, Printer MIB,
HP enterprise objects; operator contact scrubbed) and
`testdata/fixtures/hp-m283fdw-ledm/*.xml` (LEDM pages; contact, SSID and BSSID scrubbed).
`PM_LIVE_HP=198.51.100.5 go test ./internal/plugins/hp -run TestLiveHP -v` re-runs the
validation (identify, counters, status, firmware, config) against the device.

What answered and how it cross-checks:

| Object | OID / page | Value | Cross-check |
| --- | --- | --- | --- |
| sysObjectID | `.1.3.6.1.2.1.1.2.0` | `.1.3.6.1.4.1.11.2.3.9.1` | enterprise 11 |
| Jetdirect sysDescr | `.1.3.6.1.2.1.1.1.0` | `HP ETHERNET MULTI-ENVIRONMENT,SN:VNB0TEST01,FN:0000001,SVCID:10084,PID:HP Color LaserJet MFP M283fdw` | SN = prtGeneralSerialNumber; PID = hrDeviceDescr |
| PML device id | `.1.3.6.1.4.1.11.2.3.9.1.1.7.0` | `MFG:HP;CMD:...;MDL:HP ColorLaserJet MFP M282-M285;...;PRN:7KW75A;...eSCL:...` | identical to IPP `printer-device-id` |
| PML panel line | `.1.3.6.1.4.1.11.2.3.9.1.1.3.0` | `Sleep mode is on.` | also `.1.3.6.1.4.1.11.2.4.3.1.2.0` |
| product number / model / serial / FW date code | `.1.3.6.1.4.1.11.2.3.9.4.2.1.1.3.{1,2,3,5}.0` (PML strings carry a 2-byte `01 15` header) | `7KW75A` / `HP Color LaserJet MFP M283fdw` / `VNB0TEST01` / `20260622` | LEDM ProductConfigDyn `ProductNumber`, `SerialNumber`, `Version/Date 2026-06-22` |
| firmware components | `.1.3.6.1.4.1.11.2.3.9.4.2.1.1.20.{6,7}.N.0` | Formatter FW 20260622, DC Controller FW 10100, Duplexer FW `?? hardcoded` (dropped) | LEDM BoardInfo ModelNumber 10100 |
| total engine pages | `.1.3.6.1.4.1.11.2.3.9.4.2.1.4.1.2.5.0` | 1396 | = prtMarkerLifeCount 1396 = LEDM `TotalImpressions` |
| mono / colour pages | `...4.1.2.6.0` / `...4.1.2.7.0` | 312 / 1084 | = LEDM `MonochromeImpressions` 312 / `ColorImpressions` 1084 (**some community templates label 6/7 the other way round**; the reader also requires mono+colour = total) |
| duplex pages | `...4.1.2.22.0` | 263 | = LEDM `DuplexSheets` 263 |
| supply part numbers / serials | `...4.1.10.1.1.56.N.0` / `...4.1.10.1.1.3.N.0` | W2210A/W2211A/W2213A/W2212A | = Printer MIB supply descriptions and LEDM ConsumableConfigDyn `ProductNumber` |
| LEDM resource list | `GET /DevMgmt/DiscoveryTree.xml` | 46 resources (ConsumableConfigDyn, ProductStatusDyn, ProductConfigDyn, ProductUsageDyn, SecurityDyn, NetAppsDyn, IOConfigDyn, Fax*, Copy*, EventTable, `/ipp/print`) | `NetAppsSecureDyn.xml` and `/IoMgmt/Adapters` answer although unlisted; `FirmwareUpdateDyn.xml` 404 |
| LEDM NetAppsDyn | protocols | SNMP enabled/readWrite/publicAllowed, mDNS, WSD, LPD, LLMNR, SLP, 9100, IPP enabled; HTTPS redirect disabled; DHCPv4 | mapped to `protocols.*`, `network.ipv4.mode` |
| LEDM ProductConfigDyn | settings | PasswordStatus notSet, PowerSaveTimeout 1minute, ShutDownDelay 4hours, Duplex disabled, language en | `security.adminPassword.set=false`, `energy.sleepDelayMinutes=1`, `energy.autoOffMinutes=240`, `print.defaultDuplex=false` |
| LEDM ProductStatusDyn | status | `ready` + four `genuineHP` Info alerts | translated ("Genuine HP supply installed (Yellow toner)") |

Operational note: the embedded web server took > 10 s to answer the first GET while the
device was in sleep (later requests answered in well under a second), so
`vendorkit.DefaultHTTPOptions` uses a 25 s timeout and `ReadStatus` bounds its optional
LEDM fetch to 8 s. SNMP answered immediately throughout.

Not observed (and therefore not claimed): FutureSmart Enterprise/Managed objects beyond
the shared PML lineage, Jetdirect protocol flags under `.1.3.6.1.4.1.11.2.4.3.7`, the
scanner counters under `...4.2.1.2.2.1.{44,73,74}`, and any LEDM `PUT`.

## Fixtures and tests

- `testdata/fixtures/hp-m283fdw-enterprise.snmp`, `hp-m283fdw-ledm/` — real recordings (above).
- `testdata/fixtures/hp-m283fdw.snmp` — the earlier recording, which has **no system
  group**; every plugin rejects it over SNMP and the HP plugin identifies the device from
  the companion `hp-m283fdw.ipp` (`TestIdentifyViaIPPWhenSNMPHasNoSysObjectID`).
- `testdata/fixtures/<vendor>-<model>.snmp` for the other twelve plugins — synthetic but
  structurally faithful (system group with the right enterprise, HR, Printer MIB
  tables). Ricoh and Canon fixtures carry the **unverified** enterprise rows, marked as
  such in the file comments.
- `internal/plugins/all_test.go` — contract test (every plugin is an `engine.Driver`,
  implements the typed interface for every declared capability, and its evidence matrix
  agrees with its declared set) and the fixture matrix (each fixture is accepted by
  exactly its own plugin and rejected by the other twelve with `ErrNotVendor`).

## Adding a vendor

Fill a `vendorkit.Spec` (name, canonical vendor, enterprise numbers, badges, IPP
manufacturers, declared capabilities, evidence rows, families, management paths, an
optional `Enrich` hook that probes vendor objects and appends only the capabilities the
device answered for, an optional `Config` hook), embed `*vendorkit.Base`, implement the
typed interfaces for the extra capabilities, add a fixture and tests, and append the
constructor to `plugins.New`. Keep the honesty rule: declare less rather than guess.

## Verification backlog (what a real-hardware test should verify next)

Ordered by the research's qualification order [first-pass "Prioritized support shortlist"
and the expansion report's "Qualification order"]:

1. **Ricoh IM C3010 / C3510** — confirm the engine counter table OID, column numbers and
   row names (`Total`, `Copier: B & W`, …); obtain the RiDP Standard/Private MIB
   specifications and PMAPI docs; then promote `read_counters` from unverified
   [R08] [R09] [R01].
2. **Ricoh IM C3000** (2019, HDD) and **IM 3500** (mono) — controller/firmware
   differences, counter naming on mono engines [R02] [R07].
3. **HP E87770z / E877dn and E87660** (FutureSmart Managed A3) — confirm the PML
   identity/counter objects answer on FutureSmart (the M283fdw is LaserJet Pro); confirm
   whether PML `.6.0`/`.7.0` are mono/colour there; record licensed-speed changes in the
   reported model [NHP13] [NHP14] [NHP05].
4. **HP M507dn / M528dn** — Enterprise A4 generation; no LEDM expected, EWS only [H01] [H14].
5. **Canon DX C3930i / DX 8905** — confirm the counter table OID and the id mapping
   (101/108/113/122/501) [C03] [NCA02].
6. **Xerox AltaLink C8145 / B8255** — standard MIB coverage, clone/`.dlm` lifecycle [X02] [X03].
7. **Konica Minolta C300i / C651i** — OpenAPI operations once partner specs exist [K03].
8. **Kyocera TASKalfa 3554ci / 7054ci** — SCEP, CCRX, exact MIB coverage [Y01] [NKY01].
9. **Brother MFC-L6915DW**, **Lexmark CX635adwe** (eSCL, MIB v1 caveat), **Epson
   AM-C6000**, **Sharp BP-70C36** (+ MX-C407P negative control), **Toshiba
   e-STUDIO3525AC** (TopAccess component list), **OKI C650dn**, **Fujifilm Apeos C3570**
   — identity, SNMP version/MIB conformance, policy-disabled protocols
   [B02] [L02] [L06] [E01] [NSH02] [T07] [O02] [NFU01].
10. **Samsung SL-X4300LX** — only if a real inventory contains SyncThru devices [G01].

## Research gaps a second pass should fill

- **Ricoh**: the licensed Private MIB (counter table, serial, firmware component objects),
  PMAPI/SmartDeviceMonitor operation specs, supported-model matrix; Web Image Monitor
  XML/JSON endpoints are not documented at all.
- **HP**: FutureSmart PML object list (mono/colour counter semantics on Enterprise), the
  EWS `.bdl` upload contract and Manageability Contract versions [NHP20]; LEDM `PUT`
  semantics for LaserJet Pro configuration writes.
- **Canon**: counter table OID and Remote UI / DSC file format.
- **Xerox**: EWS meter/clone endpoints and `.dlm` upload contract per family.
- **Konica Minolta**: bizhub OpenAPI operation schemas and licence terms.
- **Kyocera / Sharp / Toshiba / Brother / Epson / OKI / Fujifilm**: any private MIB
  counter objects (mono/colour/duplex/scan), EWS endpoints, firmware transports.
- **Lexmark**: NPAP/XMLNT/LST protocol documentation; per-model MIB v2 conformance.
- **All**: native SCEP/EST/ACME enrolment APIs (every vendor is U or manager-mediated).
