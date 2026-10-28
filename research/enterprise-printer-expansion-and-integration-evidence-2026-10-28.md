# Enterprise printer expansion and integration evidence

Research date: **28 October 2026**. This supplements [the first-pass report](/srv/printmarshal/research/printer-models-and-manageability-2026-10-28.md). The companion [JSON catalogue](/srv/printmarshal/research/enterprise-printer-catalogue-2026-10-28.json) contains the model records, evidence scopes, management profiles, source registry and HP product-number examples.

## What changed

The first pass underrepresented managed-channel and high-volume devices. This expansion consolidates **495 distinct base model identifiers in 126 groups across 15 manufacturers**, including **144 HP identifiers, of which 83 are Managed LaserJet/PageWide models**. These totals include models retained from the first pass; they are not 495 newly discovered or physically validated products. Flow, suffix and product-number examples are tracked separately rather than counted as extra base models.

The user’s E8777/E8766 examples are covered under the official **E87740/E87750/E87760/E87770** and **E87640/E87650/E87660** names. The expansion also includes their mono E826/E825 counterparts, lower-speed Managed A3 generations, Managed A4 devices, PageWide fleets, and newer enterprise machines. HP’s named-model management and firmware tables are substantially broader than a retail-store product selection. [NHP01] [NHP02] [NHP04] [NHP05]

The useful result for PrintMarshal is a much larger qualification pool with evidence for separate operations. A vendor tool listing a model establishes a credible integration target. A model administrator guide can establish a protocol or administrative feature. Neither establishes that PrintMarshal has implemented and tested that operation.

## Evidence rules

- **D — Device document:** an exact-model/family guide or specification. Its claim is limited to the features actually described.
- **T — Management-tool table:** the vendor explicitly lists the model for management/settings software. Individual operations can still differ.
- **A — App/feature matrix:** an official platform, firmware-feature or application compatibility list establishes identity or feature eligibility. It does not establish general device administration.
- **I — Identity evidence:** an official launch, product page, support hub or advisory names the model. Detailed administration remains a separate research item.

All catalogue entries remain **research candidates**, with **zero physical devices validated in this work**. The JSON preserves the original evidence category and sources instead of flattening everything to a “supported” boolean. Its vendor profiles are context, not inherited per-model guarantees. The narrower `operation_evidence` records identify the devices and operations for which stronger documentation was found.

**U means unverified.** An unknown launch year stays unknown. A firmware date, management-software support date, current support-page timestamp or online catalogue entry is not a launch date or proof of current sale. Older HP Managed generations are deliberately included because the user requested them; their presence is not a measured claim about today’s installed population.

Sources are primary vendor material or standards. Some HP pages block direct retrieval, and the E877 PDF exceeded the browser’s full-fetch limit; relevant official indexed sections were used and are labelled in the source registry. Earlier verified sources are carried forward with their original access notes. No current complete private-MIB or partner SDK package was obtained, and no protocol implementation was built.

## HP: the missing enterprise and Managed coverage

### E877 and E876 need first-class device records

**E877:** HP publishes both speed-specific model identities and generic E877 product identities. The E877dn **5QK03A** specification explicitly documents SNMPv3, HTTPS, IPsec, 802.1X, certificate handling, secure boot, allow-listing, encrypted storage and security syslog integration. Its listed hardware includes a 10.1-inch panel, 6 GB memory, 500 GB HDD, Gigabit Ethernet, optional wireless accessory and automatic duplex. The base speed is 40 ppm, with 50/60/70 ppm licences. Those are this exact specification’s facts, not defaults for every HP device. [NHP14]

**E876:** the named E87640/50/60 and E82540/50/60 guide documents SNMPv1/v2c/v3, EWS administration, TLS, 802.1X, IPsec/firewall, certificates, protocol controls and firmware upgrade. Relevant printed pages are 119–128 for administration, including General/Firmware Upgrade on page 122 and management protocols on page 128. This is direct device-family evidence, beyond simply finding a vendor software entry. [NHP05]

HP’s developer compatibility table supplies exact product/bundle identities. The JSON retains **23 product-number examples**, including E877 speed-specific dn and Flow z bundles, generic E877 units, and E876/E825 examples. It is an example registry, not a complete regional SKU database. For example, the Workpath table labels X3A87A as E87640 du, while the device guide labels that product number E87640dn. Preserve both observations and confirm the physical device before normalizing the bundle. [NHP03] [NHP05]

### Licensed speed changes identification

HP documents Late Point Differentiation Configuration: a purchased speed licence is associated with the printer and serial number and can be installed through supported vendor routes. Its applicable-device list includes E730/E731, E785/E786, E826/E877 and selected X models. A licence can alter the reported model/speed without replacing the engine. HP also documents a 60 ppm limit when an inner finisher is installed. [NHP13]

**Design inference:** treat a device’s reported name as an observation, not its permanent primary key. Retain manufacturer/badge, product number, serial, controller identity, firmware components, installed licence/speed and options. Map generic E877dn/E877z observations into the appropriate platform family while keeping the actual licensed configuration. Do not infer interchangeable firmware from similar numbers.

### Management and firmware evidence

| Target | Stronger evidence | What it supports researching | Remaining implementation evidence |
| --- | --- | --- | --- |
| E87740/50/60/70; generic E877 | Named family guide; exact E877dn security/specification; WJA and firmware listings | Discovery, SNMP monitoring, EWS administration, policy inspection, updates | Exact private objects, permitted third-party administrative contract and per-firmware behaviour |
| E87640/50/60; E82540/50/60 | Named-model guide with SNMP agents, network/security controls and upgrade pages | Monitoring plus explicit administration/security/update targets | Validate dn/z/du, storage revision, OID coverage and update package |
| E786/E785; E731/E730; E826 | Named WJA, feature and firmware matrices | Additional A3 platform qualification targets | Do not copy every E877 operation without the exact guide and test |
| E40040/E42540/E45028/E47528 | Exact named-model support documents | Enterprise Managed A4 coverage | Distinct formatter/storage/firmware behaviour |
| E500/E501, E525/E526, E600/E601, E625/E626, E650/E651, E675/E676 | Official firmware identities; portfolio/compatibility entries | Earlier and refreshed A4 Managed fleets | Identify exact generation and bundle before selecting an adapter |
| E556/E586, E751/E776, P752/P774/P779 | Named PageWide Managed support and WJA entries | Enterprise inkjet fleets absent from retail LaserJet-only research | Exact PageWide release status, objects and package entitlement |
| 8501/8601/8801 | WJA support plus product/model guide evidence | Newer high-volume enterprise coverage | Controller-specific verification and current package eligibility |

Sources: [NHP01] [NHP02] [NHP04] [NHP05] [NHP06] [NHP09] [NHP11] [NHP12] [NHP15] [NHP16] [NHP17].

The firmware matrix includes E877 and E876 entries with **FutureSmart 5.9.2.3** and date code **20260311**, while some families also have older FutureSmart branches. These are observed source values, not a blanket deployment recommendation. Match product number, branch, full revision, region and prerequisites to the downloaded release notes. A table row that abbreviates E87640/E87660 does not invalidate separately documented E87650. [NHP02]

HP documents `.bdl` updates through EWS and Web Jetadmin. That establishes a vendor-supported remote route; it does not supply a public unattended upload contract for a new fleet tool. Read the installed revision again after reboot and record both transfer and installation outcomes. [H11]

Web Jetadmin documents SNMP Get/Set use and different credential workflows across FutureSmart generations. Its guide makes stronger SHA/AES choices dependent on Manageability Contract versions. Record actual authentication/privacy algorithms and management-contract capabilities instead of reducing SNMPv3 to yes/no. [NHP19] [NHP20]

The port documentation distinguishes device communication from services on the WJA host: HTTPS 7627 is used for some device web-service operations, while OXPm entries also describe WJA-side communication. An open port is not a published API contract. Obtain the correct operation schema, authentication rules and SDK/licence terms before implementing a proprietary adapter. [NHP08]

### New local Workpath integration evidence

HP documents deployment outside HP Command Center through EWS, USB, App Manager’s command line and WJA, and states that it publishes APIs for Workpath app management. Requirements include firmware **5.9.2.3+**, the applicable Workpath licence and model-specific prerequisites. The compatible-device page lists exceptions awaiting firmware. Some app/security paths still require web services or internet access. This is concrete evidence for investigating **local app installation and management**; it does not establish a general device-configuration API. [NHP07] [NHP03]

## Additional enterprise depth across manufacturers

### Ricoh: high-volume devices and additional controller generations

Add IM C6500/C8000 and C6510/C8010, IM 7000/8000/9000, A4 IM 550F/600F/600SRF and P 800/P 801/P C600/P C375 to the earlier IM coverage. The high-volume colour brochure documents 65/80 ppm, 6 GB split mainframe/panel memory and 640 GB storage. The high-volume mono sheet documents 4,300 standard and 8,700 maximum input sheets. These are separate departmental/high-volume test targets. [NRI02] [NRI03] [NRI06] [NRI07] [NRI08] [NRI09] [NRI10]

The IM 7000/8000/9000 network manual explicitly includes SNMPv3 configuration. Ricoh’s ACT page enumerates supported models across A3/A4 and documents feature updates. Its OAuth transition page gives model-dependent firmware-component requirements, including older high-volume MP devices. Those numbers concern a particular feature/component; they must not be treated as a universal system-firmware version or copied across controllers. [NRI04] [NRI01] [NRI05]

RiDP explicitly offers Standard/Private MIB specifications and SmartDeviceMonitor API; the Japanese program names PMAPI and additional MIB interfaces. Those are better management integration leads than assuming SmartSDK workflow REST covers device administration. Obtain the supported-model matrix and operation documentation for monitoring, configuration and update separately. [R08] [R09] [R10]

Retain Savin/Lanier and other proven Ricoh badge aliases with their raw identities. An IKON service relationship alone does not establish the engine. Japanese SD, Cxx11 and regional mono generations remain distinct records. [R23] [R24] [R22] [R06] [R12] [R13]

### Canon: settings-tool evidence reaches well beyond the first shortlist

Canon’s Device Settings Configurator list adds high-volume DX C7700, 6700/6800/6900, 8700/8900 and A4 DX families. It explicitly distinguishes settings retrieval/distribution permissions between generations; a listed older imagePRESS group cannot receive distributed settings through that tool. This is useful operation-specific evidence, not merely product marketing. A future **2026.11** model-data entry on the page was excluded from this October catalogue. [NCA01]

The DX 8900 manual directly documents SNMPv1/v3, MIB access permissions, Remote UI settings, RFC2790 formatting, 802.1X, IPP Everywhere and discovery/network protocols. It does not establish SNMPv2c. The US brochure places 8905i/8995i/8986i at 105/95/86 Letter ppm. A proprietary CPCA port listing is not a third-party implementation specification. [NCA02] [NCA03] [NCA04] [NCA05]

### Xerox: mono AltaLink deserves equal coverage

Add B8145/B8155/B8170 and B8245/B8255/B8270 alongside the colour AltaLink generations. The current B8200 page rates B8270 at **72 ppm**: a model’s last digits are not a reliable speed rule. The shared B/C8100/B/C8200 administrator guide is the stronger source for clone, update and security operations. Keep VersaLink qualification separate. [NXE01] [X02] [X03]

### Konica Minolta: high-volume and A4 generations

The expanded inventory separates C750i from C751i, the older/newer mono i-series and 850i/950i, plus A4 C3350/C4050 and 4050/4750 generations. Official model advisories help establish identities and firmware tracks but do not, by themselves, establish every administration capability. The vendor’s security paper explicitly describes OpenAPI device-information/configuration functions; exact partner schemas and supported-model revisions are still needed. [NKM01] [NKM04] [K03]

Regional specifications matter: the cited C751i Hong Kong sheet reports **75 mono / 70 colour ppm**, while the C651i EU page gives 65/65 ppm. Store source region and speed basis instead of deriving specifications from the model name. [NKM03] [NKM02]

### Kyocera: enterprise A4 and high-volume TASKalfa

Add PA4500x/5000x/5500x/6000x, MA4500/5500/6000 variants and high-volume TASKalfa generations. A named MA6000/PA6000 Command Center RX guide is listed in current support downloads. The exact 7054ci sheet documents SNMPv1/v2c/v3, TLS 1.3, SCEP, TPM, secure boot and 4 GB RAM / 32 GB SSD / 320 GB HDD, with optional storage. This is strong evidence for that model, not a blanket promise for every ECOSYS/MZ controller. [NKY01] [NKY02] [NKY03] [NKY04]

### Lexmark: vendor management evidence for departmental and A3 fleets

The MVE 4.7 table explicitly lists the expanded CS/CX, MS/MX and enterprise XC/XM targets and requires SNMPv3 on the selected models marked with footnote 2. It separately notes that devices moved to End of Firmware Support no longer receive MVE updates/support, even if they still function. A historical compatibility entry therefore needs a current lifecycle check. [NLE01]

MVE documents NPAP, XMLNT and LST as well as SNMP and HTTPS; those are distinct channels. Firmware deployment accepts a file and update window and has a Firmware Updates access-control prerequisite. Certificate automation is **MVE-to-CA enrollment followed by installation to the printer**; its EST connector does not prove that the printer itself implements native EST. These are useful reference behaviours for plugin design while the proprietary operation schema remains unresolved. [NLE02] [NLE03] [NLE04]

### Sharp: an unusually useful model-by-operation matrix

Add BP-70M75/70M90, mono BP-50M/70M and MX-M1056/M1206. Sharp’s IT guide and SRDM matrix distinguish discovery, meters, cloning, security management and updates by model group. The high-volume BP mono specification explicitly documents network and one-to-many firmware updates. [NSH01] [NSH02] [NSH03]

Keep a negative test case: SRDM’s MX-C407P/C507P/C607P row provides monitoring but marks power/security management, cloning and firmware update unsupported. That is a limitation of the documented tool path, not proof that no other update method exists. Do not apply A3 BP capabilities to every Sharp badge. [NSH02]

### Toshiba, Brother, Epson, OKI, Samsung, Fujifilm and RISO

Toshiba’s high-volume colour 6526AC/6527AC/7527AC and mono 6529A/7529A/9029A are included. TopAccess exposes component software versions; preserve that inventory rather than one flattened firmware string. [NTO01] [T06] [T07]

Brother’s enterprise colour network guide names regional HL/MFC and EX variants. They need exact product identities alongside the mono enterprise family, with BRAdmin/Mass Deployment and firmware-tool evidence kept separate from a general API claim. [NBR01] [B02] [B04] [B05]

Epson’s older WF-C21000 remains a useful enterprise target even though the cited European SKU is explicitly discontinued; its support page still lists firmware downloads. The AM-C4000/5000/6000 US availability announcement is **March 2023**. Earlier regional releases, if any, require their own evidence. [NEP01] [NEP02] [E02]

OKI A3 C824/C834/C844 and MC853/MC873/MC883 have their own official manuals; C650 protocols cannot be inherited automatically. Legacy Samsung stays on its SyncThru track. [NOK01] [NOK02] [O02] [G01]

Fujifilm’s evaluated C8180/C7580/C6580 secure-configuration guide instructs administrators to disable SNMP in that evaluated configuration. Distinguish “feature present,” “enabled by policy” and “usable with our credentials.” RISO engines and external controllers similarly need separate identities and tests. [NFU01] [Z01] [Z05]

## Qualification order for building PrintMarshal

This is an engineering-priority order based on the user’s gaps, breadth of family coverage and quality of primary documentation. It is **not an installed-base market ranking**. Prefer devices already accessible in the target customer fleet before buying hardware.

| Priority | Representative device/configuration | Main result to establish |
| --- | --- | --- |
| 1 | HP E87770z or E877dn with known speed licence | Newer Managed A3 identity, monitoring, licensed speed and options |
| 2 | HP E87660dn/z, with an E87650 sample if available | Older Managed A3 compatibility and complete E876 coverage |
| 3 | Ricoh IM C3010/C3510 | Main Ricoh colour controller, private counters and configuration interface |
| 4 | Ricoh IM 7000/8000/9000 | High-volume mono, finisher inventory and component firmware |
| 5 | HP E82660/E82670 | Mono counterpart to newer Managed A3 platform |
| 6 | HP E82550/E82560 | Earlier mono Managed generation |
| 7 | HP E78630/E78635 | Midrange Managed colour A3 |
| 8 | HP E73135/E73140 | Midrange Managed mono A3 |
| 9 | HP E52645 plus E62665 or E62675 | Two Managed A4 mono generations/classes |
| 10 | HP E67660 plus E45028/E47528 | High-volume and compact Managed colour A4 differences |
| 11 | HP PageWide E77650/E77660 | Enterprise inkjet management and update package matching |
| 12 | Canon DX 8905/8995/8986 | High-volume SNMP, settings transfer and certificate/control scope |
| 13 | Xerox B8255/B8270 or C8255 | Explicit standard MIB coverage, clone and firmware lifecycle |
| 14 | Konica Minolta C651i/C751i | OpenAPI administrative operations and firmware compatibility |
| 15 | Kyocera TASKalfa 7054ci | SCEP, policy settings and exact SNMP/MIB coverage |
| 16 | Lexmark CX943/XC9455 and one MX822/MX826 | A3 versus A4 management channels, firmware access controls |
| 17 | Sharp BP-70M90 and MX-C407P negative control | Model-specific positive and unsupported operations |
| 18 | Toshiba e-STUDIO7527AC or 9029A | TopAccess component inventory and administration/update scope |
| 19 | HP 8601/8801 or newer mono 5502/5602 | New enterprise controller qualification |
| 20 | Brother L9670, Epson AM-C6000, Fujifilm C7580 as accessible | Remaining controller diversity and policy-disabled protocols |

A family representative can establish a reusable implementation candidate. Each additional SKU/firmware/option combination still needs evidence for the operations advertised. Flow versus non-Flow, embedded versus external controller, network interface and security mode belong in that record.

## What a standards core can actually establish

The reusable first layer is identification and observation, followed by capability probes. The following are **standard candidate objects**, not promises that all catalogue devices implement them. The row indices, counter units and sentinel values must be read rather than assumed. [S01] [S02] [S05]

| Observation | Standard object/root | Required interpretation |
| --- | --- | --- |
| Agent/platform identity | `sysObjectID.0 = 1.3.6.1.2.1.1.2.0`; `sysDescr.0 = 1.3.6.1.2.1.1.1.0` | An enterprise OID identifies an agent family; corroborate engine/model/product identity |
| Device descriptions | `hrDeviceDescr = 1.3.6.1.2.1.25.3.2.1.3` | Walk device rows; do not hard-code one index |
| Serial candidate | `prtGeneralSerialNumber = 1.3.6.1.2.1.43.5.1.1.17` | Validate presence and uniqueness against the physical device |
| Lifetime meter | `prtMarkerLifeCount = ...43.10.2.1.4`; unit `...43.10.2.1.3` | Preserve marker index and unit; not automatically billable mono/colour impressions |
| Supply values | `prtMarkerSuppliesTable = ...43.11.1` | Preserve description, unit, maximum and level; unknown/negative sentinels are not percentages |
| Alerts | `prtAlertTable = ...43.18.1` | Reconcile active rows and changes; do not treat table order as a permanent event ID |
| IPP printer attributes | IPP Get-Printer-Attributes | Complement SNMP identity/state when enabled; it does not supply universal fleet policy/firmware control |

The abbreviated OIDs above start with `1.3.6.1.2.1`. Exact standard definitions are in Printer MIB, Host Resources and IPP. Discovery can also use DNS-SD/mDNS where policy permits; it does not replace routed subnet discovery or authenticated probes. [S01] [S02] [S03] [S09] [S10]

**Design inference:** keep the standard observation layer usable before proprietary administration is available. Add per-vendor operations only where the wire contract, object definition or supported command path is known. IPP Everywhere, AirPrint, eSCL and embedded workflow SDKs should remain separate capabilities from configuration, security policy and firmware management.

## Acceptance evidence required before advertising support

For each device/firmware combination, save a compact evidence record containing exact identity, source revisions, enabled protocols, credentials/authentication mode, observed objects, operation results and limitations. The useful progression is:

1. **Identify:** repeatable identity across DHCP/address changes; preserve model/product/badge and licence observations.
2. **Monitor:** verify serial, supply semantics, trays, options, uptime, alerts and meter units against the device. Exercise sleep/offline and missing-object responses.
3. **Configure:** use one documented reversible field such as location, verify the read-back and restore it. Record the particular API/object and access requirement.
4. **Secure:** inspect supported protocol/authentication/certificate capabilities; test a documented policy operation on a lab device, including whether it changes management reachability.
5. **Update:** verify the exact permitted package and prerequisites, transfer through a documented route, observe reboot/recovery and verify component versions afterward. Record update permission separately from discovery/monitoring.

This work produces the research inputs for those tests. It does not perform network scans, change printer settings, install firmware, change application code or modify a support manifest.

## Known limitations and specific next evidence to obtain

| Area | Evidence still needed |
| --- | --- |
| HP | Current licensed/private MIB and administrative contract; exact model/MC/firmware operation matrix; update package entitlement; complete regional SKU mapping |
| Ricoh | RiDP PMAPI/SmartDeviceMonitor/private-MIB specifications and exact controller compatibility; customer/service update transport and package rules |
| Canon | Supported administrative SDK/protocol access, DSC file-format/licence terms and precise model firmware-update route |
| Konica Minolta | Current OpenAPI operation schemas, partner terms, model revisions and permitted system-firmware update route |
| Xerox / Sharp / Kyocera | Exact third-party administrative/update contract, beyond documented vendor UI/tool behaviour |
| Lexmark | Current device protocol documentation and entitlements, model update packages, EOFS status and management-security prerequisites |
| Other vendors | Exact model administrator/update guides, MIB revisions and regional firmware rights before capability inheritance |
| Market prevalence | Customer inventories or reliable model-level fleet data; compatibility breadth alone cannot establish popularity |
| Hardware detail | Complete current regional specs for every secondary candidate; this supplement prioritizes identity and management evidence |

Source inconsistencies are preserved rather than silently turned into product facts: malformed model strings, conflicting storage revisions, inconsistent colour/mono labels, regional speeds and abbreviated firmware family rows all require corroboration. The initial two research documents were preserved. No guessed numeric range was expanded into model numbers.

## Expanded model inventory

The tables below expose the catalogue’s concrete base identifiers. Prefixes such as Canon DX, Sharp BP-/MX- and Toshiba e-STUDIO are retained in the JSON. Year **U** means the release year was not verified; the JSON separately records lifecycle and qualification notes. Sources in a row establish its stated evidence class, not every capability in the vendor profile.

### HP — 144 base identifiers

Managed-channel coverage is listed first; Enterprise retail/channel generations follow. The E877/E876 operation findings above are narrower than this entire inventory.

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| HP01 | E87740, E87750, E87760, E87770 | A3 colour laser MFP | U | T | [NHP01] [NHP02] [NHP04] [NHP14] |
| HP02 | E87640, E87650, E87660 | A3 colour laser MFP | U | T | [NHP01] [NHP05] |
| HP03 | E78625, E78630, E78635 | A3 colour laser MFP | U | T | [NHP01] [NHP09] |
| HP04 | E78523, E78528 | A3 colour laser MFP | U | T | [NHP01] [NHP09] |
| HP05 | E78223, E78228 | A3 colour laser MFP | U | A | [NHP03] [NHP09] |
| HP06 | E78323, E78325, E78330 | A3 colour laser MFP | U | D | [NHP10] |
| HP07 | E77822, E77825, E77830 | A3 colour laser MFP | U | T | [NHP01] [NHP10] |
| HP08 | E77422, E77428 | A3 colour laser MFP | U | T | [NHP01] [NHP09] |
| HP09 | E78122 | A3 colour laser MFP | U | A | [NHP09] |
| HP10 | E82650, E82660, E82670 | A3 mono laser MFP | U | T | [NHP01] [NHP02] [NHP13] |
| HP11 | E82540, E82550, E82560 | A3 mono laser MFP | U | T | [NHP01] [NHP05] |
| HP12 | E73130, E73135, E73140 | A3 mono laser MFP | U | T | [NHP01] [NHP09] |
| HP13 | E73025, E73030 | A3 mono laser MFP | U | A | [NHP02] [NHP09] |
| HP14 | E72725, E72830 | A3 mono laser MFP | U | A | [NHP09] |
| HP15 | E72525, E72530, E72535 | A3 mono laser MFP | U | T | [NHP01] [NHP10] |
| HP16 | E72425, E72430 | A3 mono laser MFP | U | T | [NHP01] [NHP02] |
| HP17 | E40040 | A4 mono laser printer | U | I | [NHP11] [NHP06] |
| HP18 | E42540 | A4 mono laser MFP | U | I | [NHP11] [NHP06] |
| HP19 | E45028 | A4 colour laser printer | U | I | [NHP11] |
| HP20 | E47528 | A4 colour laser MFP | U | I | [NHP11] [NHP02] |
| HP21 | E50045, E50145 | A4 mono laser printer | U | A | [NHP02] [NHP06] |
| HP22 | E52545, E52645 | A4 mono laser MFP | U | A | [NHP02] [NHP03] |
| HP23 | E60055, E60065, E60075, E60155, E60165, E60175 | A4 mono laser printer | U | A | [NHP02] [NHP06] |
| HP24 | E62555, E62565, E62575, E62655, E62665, E62675 | A4 mono laser MFP | U | A | [NHP02] [NHP06] |
| HP25 | E55040 | A4 colour laser printer | U | A | [NHP02] |
| HP26 | E57540 | A4 colour laser MFP | U | A | [NHP02] [NHP03] |
| HP27 | E65050, E65060, E65150, E65160 | A4 colour laser printer | U | A | [NHP02] |
| HP28 | E67550, E67560, E67650, E67660 | A4 colour laser MFP | U | A | [NHP02] [NHP03] |
| HP29 | E75245, E85055 | A3 colour laser printer | U | A | [NHP09] [NHP02] |
| HP30 | E55650 | A4 colour page-array inkjet printer | U | I | [NHP12] |
| HP31 | E58650 | A4 colour page-array inkjet MFP | U | I | [NHP12] [NHP03] |
| HP32 | E75160, P75250 | A3 colour page-array inkjet printer | U | I | [NHP12] |
| HP33 | E77650, E77660 | A3 colour page-array inkjet MFP | U | T | [NHP01] [NHP12] |
| HP34 | P77440, P77940, P77950, P77960 | A3 colour page-array inkjet MFP | U | T | [NHP01] [NHP12] |
| HP35 | M406, M407, M507, M607, M608, M609, M610, M611, M612 | A4 mono laser printer | U | A | [NHP09] [H06] [H07] |
| HP36 | M430, M431, M528, M631, M632, M633, M634, M635, M636 | A4 mono laser MFP | U | A | [H03] [H07] [NHP03] |
| HP37 | M455, M554, M555, M652, M653 | A4 colour laser printer | U | A | [NHP09] [H04] [H07] |
| HP38 | M480, M577, M578, M681, M682 | A4 colour laser MFP | U | A | [NHP09] [NHP03] |
| HP39 | M751, M856 | A3 colour laser printer | U | A | [NHP09] [H07] |
| HP40 | M776 | A3 colour laser MFP | U | A | [NHP09] [NHP03] |
| HP41 | 5700, 6700, 6701, X55745, X65455, X65465 | A4 colour laser printer | U | T | [NHP01] [NHP09] |
| HP42 | 5800, 6800, 6801, X57945, X58045, X67755, X67765 | A4 colour laser MFP | U | T | [NHP01] [NHP09] |
| HP43 | 8501 | A3 mono laser printer | U | T | [NHP01] [NHP17] |
| HP44 | 8601 | A3 mono laser MFP | U | T | [NHP01] [NHP15] |
| HP45 | 8801 | A3 colour laser MFP | U | T | [NHP01] [NHP16] |
| HP46 | 5501, 5502, 6500, 6501, X50352, X50452, X60257, X60357 | A4 mono laser printer | U | I | [NHP18] |
| HP47 | 5601, 5602, 6600, X52952, X53052, X62757 | A4 mono laser MFP | U | I | [NHP18] |

### Ricoh — 66 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| RI01 | IM C2000, IM C2500, IM C3000, IM C3500, IM C4500, IM C5500, IM C6000 | A3 colour laser MFP | 2019 | I | [R03] [NRI01] |
| RI02 | IM C2010, IM C2510, IM C3010, IM C3510, IM C4510, IM C5510, IM C6010 | A3 colour laser MFP | 2023 | D | [R04] [R05] |
| RI03 | IM C7010, IM C6510, IM C8010 | A3 colour laser MFP | U | I | [NRI01] [NRI09] |
| RI04 | IM C3010SD, IM C4510SD, IM C6010SD | A3 colour electrophotographic MFP | 2025 | I | [R22] |
| RI05 | IM C2011, IM C2511, IM C3011, IM C3511, IM C4511, IM C5511, IM C6011 | A3 colour electrophotographic MFP | 2026 | I | [R06] |
| RI06 | IM C6500, IM C8000 | A3 high-volume colour laser MFP | U | D | [NRI02] [NRI01] |
| RI07 | IM 2500, IM 3000, IM 3500, IM 4000, IM 5000, IM 6000 | A3 mono laser MFP | U | I | [R07] [R11] |
| RI08 | IM 2510, IM 3010, IM 3510, IM 4510, IM 5510, IM 6010, IM 7010 | A3 mono electrophotographic MFP | 2026 | I | [R12] [R13] |
| RI09 | IM 7000, IM 8000, IM 9000 | A3 high-volume mono laser MFP | U | D | [NRI03] [NRI04] [NRI01] |
| RI10 | IM C300, IM C300F, IM C400F, IM C400SRF | A4 colour laser MFP | U | I | [NRI01] [R19] |
| RI11 | IM C320F, IM C401F, IM C401SRF | A4 colour laser MFP | U | I | [NRI01] [R14] |
| RI12 | IM 370F, IM 460F | A4 mono laser MFP | U | D | [NRI01] [R17] |
| RI13 | IM 550F, IM 600F, IM 600SRF | A4 mono laser MFP | U | A | [NRI05] [NRI01] |
| RI14 | P 800, P 801 | A4 mono laser printer | U | D | [NRI06] [NRI10] |
| RI15 | P C600, P C375 | A4 colour laser printer | U | D | [NRI07] [NRI08] |
| RI16 | MP 6503, MP 7503, MP 9003 | A3 high-volume mono laser MFP | U | A | [NRI05] |
| RI17 | MP 501, MP 601 | A4 mono laser MFP | U | A | [NRI05] |

### Canon — 67 base identifiers

DX and imageFORCE prefixes are separated in each model group in the JSON; regional i/iF suffixes are not extra base-model counts.

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| CA01 | DX C3730, C3725, C3720, C3835, C3830, C3826, C3822, C3935, C3930, C3926, C3922 | A3 colour laser MFP | U | T | [NCA01] [C01] [C09] |
| CA02 | DX C5760, C5750, C5740, C5735, C5870, C5860, C5850, C5840 | A3 colour laser MFP | U | T | [NCA01] [C04] [C05] |
| CA03 | DX C7780, C7770, C7765 | A3 high-volume colour laser MFP | U | T | [NCA01] |
| CA04 | DX 4751, 4745, 4735, 4725, 4845, 4835, 4825, 4945, 4935, 4925 | A3 mono laser MFP | U | T | [NCA01] |
| CA05 | DX 6780, 6765, 6755, 6855, 6870, 6860, 6980 | A3 high-volume mono laser MFP | U | T | [NCA01] |
| CA06 | DX 8705, 8795, 8786, 8905, 8995, 8986 | A3 high-volume mono laser MFP | U | T | [NCA01] [NCA02] [NCA03] [NCA04] [NCA05] |
| CA07 | DX C357, C257, C359, C259, C477, C478 | A4 colour laser MFP | U | T | [NCA01] |
| CA08 | DX 717, 617, 527, 719, 619, 529 | A4 mono laser MFP | U | T | [NCA01] |
| CA09 | imageFORCE C5140, C5150, C5160, C5170 | A3 colour electrophotographic MFP | U | T | [NCA01] [C07] [C08] |
| CA10 | imageFORCE 6155, 6160, 6170, 8105, 8195, 8186 | A3 mono electrophotographic MFP | U | T | [NCA01] |

### Xerox — 22 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| XE01 | C8130, C8135, C8145, C8155, C8170 | A3 colour laser MFP | U | D | [X01] [X02] |
| XE02 | C8230, C8235, C8245, C8255, C8270 | A3 colour laser MFP | U | D | [X02] [X03] [X04] |
| XE03 | B8145, B8155, B8170 | A3 mono laser MFP | U | D | [X02] [NXE01] |
| XE04 | B8245, B8255, B8270 | A3 mono laser MFP | U | D | [NXE01] [X02] |
| XE05 | C620, B620 | A4 laser printer, colour C / mono B | U | D | [X05] [X06] [X07] |
| XE06 | C625, B625, C415, B415 | A4 laser MFP, colour C / mono B | U | D | [X05] [X06] |

### Konica Minolta — 36 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| KM01 | C250i, C300i, C360i, C450i, C550i, C650i, C750i | A3 colour laser MFP | U | I | [K02] [K03] [NKM01] |
| KM02 | C251i, C301i, C361i, C451i, C551i, C651i, C751i | A3 colour laser MFP | U | I | [K04] [NKM01] [NKM02] [NKM03] |
| KM03 | 300i, 360i, 450i, 550i, 650i, 750i | A3 mono laser MFP | U | I | [NKM01] |
| KM04 | 301i, 361i, 451i, 551i, 651i, 751i | A3 mono laser MFP | U | I | [NKM01] [K09] |
| KM05 | 850i, 950i | A3 high-volume mono laser MFP | U | I | [NKM01] [NKM04] |
| KM06 | C3350i, C4050i, C3351i, C4051i | A4 colour laser MFP | U | I | [NKM01] |
| KM07 | 4050i, 4750i, 4051i, 4751i | A4 mono laser MFP | U | I | [NKM01] |

### Kyocera — 30 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| KY01 | 2554ci, 3554ci, 4054ci, 5054ci, 6054ci, 7054ci | A3 colour laser MFP | U | I | [Y05] [NKY01] |
| KY02 | 5004i, 6004i, 7004i, 7003i, 8003i, 9003i | A3 mono laser MFP | U | I | [Y05] [NKY04] |
| KY03 | MZ2501ci, MZ3501ci, MZ4001ci, MZ5001ci, MZ6001ci, MZ7001ci | A3 colour laser MFP | U | I | [Y03] [Y04] |
| KY04 | MZ5001i, MZ6001i, MZ7001i | A3 mono laser MFP | U | I | [Y03] |
| KY05 | PA4500x, PA5000x, PA5500x, PA6000x | A4 mono laser printer | U | I | [NKY02] [NKY03] |
| KY06 | MA4500ix, MA4500ifx, MA5500ifx, MA6000ifx | A4 mono laser MFP | U | I | [NKY02] [NKY03] [Y07] |
| KY07 | MA4000cix | A4 colour laser MFP | U | I | [Y06] |

### Lexmark — 43 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| LE01 | CS730, CS735, CS737, CS820, CS827 | A4 colour laser printer | U | T | [NLE01] |
| LE02 | CX730, CX735, CX737, CX820, CX825, CX827, CX830, CX833, CX860 | A4 colour laser MFP | U | T | [NLE01] |
| LE03 | CX930, CX931, CX942, CX943, CX944 | A3 colour laser MFP | U | T | [NLE01] |
| LE04 | MS725, MS821, MS822, MS823, MS824, MS825, MS826 | A4 mono laser printer | U | T | [NLE01] |
| LE05 | MX721, MX722, MX725, MX822, MX824, MX826 | A4 mono laser MFP | U | T | [NLE01] |
| LE06 | MX931 | A3 mono laser MFP | U | T | [NLE01] |
| LE07 | XC9325, XC9335, XC9445, XC9455, XC9465 | A3 colour laser MFP | U | T | [NLE01] |
| LE08 | XM5365, XM5370 | A4 mono laser MFP | U | T | [NLE01] |
| LE09 | CX532, CX635, MX632 | A4 laser MFP, colour CX / mono MX | U | T | [NLE01] |

### Sharp — 29 base identifiers

BP groups use the BP- prefix and MX groups use MX-; the operation matrix must match the exact group.

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| SH01 | BP-70M75, BP-70M90 | A3 high-volume mono laser MFP | U | T | [NSH01] [NSH03] |
| SH02 | BP-50M26, BP-50M31, BP-50M36, BP-50M45, BP-50M55, BP-50M65, BP-70M31, BP-70M36, BP-70M45, BP-70M55, BP-70M65 | A3 mono laser MFP | U | T | [NSH01] [NSH02] |
| SH03 | BP-50C26, BP-50C31, BP-50C36, BP-50C45, BP-50C55, BP-50C65, BP-70C31, BP-70C36, BP-70C45, BP-70C55, BP-70C65 | A3 colour laser MFP | U | T | [NSH02] [A01] |
| SH04 | MX-M1056, MX-M1206 | A3 high-volume mono laser MFP | U | T | [NSH01] |
| SH05 | MX-C407P, MX-C507P, MX-C607P | A4 colour laser printer | U | T | [NSH02] |

### Toshiba — 18 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| TO01 | 2525AC, 3025AC, 3525AC, 4525AC, 5525AC, 6525AC | A3 colour laser MFP | U | D | [T01] [T02] |
| TO02 | 6526AC, 6527AC, 7527AC | A3 high-volume colour laser MFP | U | D | [NTO01] |
| TO03 | 2528A, 3028A, 3528A, 4528A, 5528A, 6528A | A3 mono laser MFP | U | D | [T03] [T07] |
| TO04 | 6529A, 7529A, 9029A | A3 high-volume mono laser MFP | U | D | [T06] |

### Brother — 11 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| BR01 | HL-L9410CDN, HL-L9430CDN, HL-L9470CDN, HL-EX470W | A4 enterprise colour laser printer | U | D | [NBR01] |
| BR02 | MFC-L9610CDN, MFC-L9630CDN, MFC-L9670CDN, MFC-EX670W | A4 enterprise colour laser MFP | U | D | [NBR01] [B04] |
| BR03 | HL-L6415DW, MFC-L6810DW, MFC-L6915DW | A4 enterprise mono laser printer / MFP | U | D | [B01] [B02] |

### Epson — 7 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| EP01 | AM-C4000, AM-C5000, AM-C6000 | A3 colour enterprise inkjet MFP | 2023 | D | [E02] [E03] |
| EP02 | AM-C10000 | A3 high-volume colour enterprise inkjet MFP | 2026 | I | [E06] |
| EP03 | WF-C21000 | A3 high-volume colour page-array inkjet MFP | U | A | [NEP01] [NEP02] |
| EP04 | WF-C5390, WF-C5890 | A4 colour inkjet printer / MFP | 2023 | I | [NEP03] [E07] |

### OKI — 8 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| OK01 | C650dn, ES6450 | A4 colour LED printer | 2021 | D | [O01] [O02] [O03] [O05] |
| OK02 | C824, C834, C844, MC853, MC873, MC883 | A3 colour LED printer / MFP | U | I | [NOK01] [NOK02] |

### Samsung — 3 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| SA01 | X4220RX, X4250LX, X4300LX | A3 colour laser MFP | U | D | [G01] [G02] [G03] |

### Fujifilm — 9 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| FU01 | C3570, C4570, C5570, C7070 | A3 colour laser MFP | U | D | [F01] [F02] |
| FU02 | C6580, C7580, C8180 | A3 high-volume colour laser MFP | U | I | [NFU01] |
| FU03 | C3530, C4030 | A4 colour laser MFP | U | D | [F04] |

### RISO — 2 base identifiers

| Group | Base model identifiers | Segment | Year | Evidence | Sources |
| --- | --- | --- | --- | --- | --- |
| RS01 | GL9730, GL7430 | A3 enterprise inkjet printer | U | D | [Z01] [Z05] |

## Source registry

New sources use the **N** prefix. Other IDs preserve the first-pass source numbering. Access descriptions distinguish retrieved sections, indexed primary-source extracts and material carried forward from the first pass. The JSON retains these access notes alongside every URL. Multiple language/locale paths to the same document are not independent corroborating sources.

- **NHP01** — [Web Jetadmin 10.6 late breaking news, edition 63, September 2026][NHP01] — official search index.
- **NHP02** — [HP FutureSmart firmware version and revision matrix][NHP02] — official search index.
- **NHP03** — [Workpath exact device and product-number compatibility][NHP03] — full page.
- **NHP04** — [E877 user guide, management chapter][NHP04] — official indexed sections full fetch too large.
- **NHP05** — [E82540/50/60 and E87640/50/60 user guide][NHP05] — full pdf and targeted sections.
- **NHP06** — [HP A3 and A4 managed portfolio][NHP06] — official search index.
- **NHP07** — [Workpath alternative deployment FAQ][NHP07] — official search index.
- **NHP08** — [Web Jetadmin ports and direction of communication][NHP08] — official search index.
- **NHP09** — [Universal Print support in FutureSmart 5.8.1.1; named enterprise/managed models][NHP09] — official search index.
- **NHP10** — [E725/E778/E783 model specifications and variants][NHP10] — official search index.
- **NHP11** — [Managed A4 E40040/E42540/E45028/E47528 exact model identities][NHP11] — official search index.
- **NHP12** — [PageWide Managed model identities in HP support matrix][NHP12] — official search index.
- **NHP13** — [Managed speed licence registration and LPDC][NHP13] — official search index.
- **NHP14** — [E877dn 5QK03A specifications][NHP14] — full page.
- **NHP15** — [LaserJet Enterprise MFP 8601dn specifications][NHP15] — official search index.
- **NHP16** — [Color LaserJet Enterprise 8801 user guide, management protocols][NHP16] — official search index.
- **NHP17** — [LaserJet Enterprise 8501 specifications][NHP17] — official search index.
- **NHP18** — [Enterprise 5502 guide hub naming new mono enterprise model families][NHP18] — official search index.
- **NHP19** — [SNMPv3 management best practices for Web Jetadmin][NHP19] — official search index.
- **NHP20** — [Web Jetadmin user guide; Manageability Contract and SNMP crypto requirements][NHP20] — official search index.
- **NRI01** — [Always Current Technology named-model applicability][NRI01] — full page.
- **NRI02** — [IM C6500 / C8000 specification brochure][NRI02] — official search index.
- **NRI03** — [IM 7000 / 8000 / 9000 specification sheet][NRI03] — official search index.
- **NRI04** — [IM 7000 / 8000 / 9000 network configuration manual (Japanese)][NRI04] — official search index.
- **NRI05** — [OAuth 2.0 availability and firmware-component requirements by Ricoh model][NRI05] — official search index.
- **NRI06** — [P 800 A4 mono printer and management software][NRI06] — official search index.
- **NRI07** — [P C600 specifications and Web Image Monitor][NRI07] — official search index.
- **NRI08** — [P C375 specifications and Web Image Monitor][NRI08] — official search index.
- **NRI09** — [Ricoh regional configurator, including C6510/C8010][NRI09] — official search index.
- **NRI10** — [Ricoh official manuals inventory including P 801][NRI10] — official search index.
- **NCA01** — [Device Settings Configurator 2.1.1 supported devices and operation restrictions][NCA01] — full page.
- **NCA02** — [DX 8900 network settings, RFC2790 and management permissions][NCA02] — official search index.
- **NCA03** — [DX 8900 SNMPv1/v3 monitoring and controlling][NCA03] — official search index.
- **NCA04** — [DX 8900 network protocols and ports][NCA04] — official search index.
- **NCA05** — [DX 8900 US brochure][NCA05] — official search index.
- **NXE01** — [AltaLink B8245 / B8255 / B8270 specifications][NXE01] — official search index.
- **NXE02** — [AltaLink B82xx/C82xx security disclosure][NXE02] — official index full fetch failed.
- **NKM01** — [Konica Minolta named-model security advisory, June 2025][NKM01] — official search index.
- **NKM02** — [C651i regional specifications][NKM02] — official search index.
- **NKM03** — [C751i Hong Kong datasheet][NKM03] — official search index.
- **NKM04** — [AccurioPrint 850i/950i high-volume office and light-production positioning][NKM04] — official search index.
- **NKY01** — [TASKalfa 7054ci exact protocols, security and storage][NKY01] — official search index.
- **NKY02** — [PA6000/MA6000 series official named-model support notice, June 2023][NKY02] — official search index.
- **NKY03** — [MA6000ifx official support hub and July 2026 Command Center RX guide][NKY03] — official search index.
- **NKY04** — [Command Center RX advisory with explicit affected-model list][NKY04] — official search index.
- **NLE01** — [Markvision Enterprise 4.7 supported printers and footnotes][NLE01] — full page.
- **NSH01** — [Sharp IT Reference Guide; model-by-operation device management matrix][NSH01] — official search index.
- **NSH02** — [Sharp Remote Device Manager capability matrix][NSH02] — official search index.
- **NSH03** — [BP-70M75 / BP-70M90 exact specifications][NSH03] — official search index.
- **NTO01** — [Toshiba 6526AC/6527AC/7527AC global product page][NTO01] — official search index.
- **NBR01** — [Brother 2022 enterprise colour model group, network configuration][NBR01] — official search index.
- **NEP01** — [Epson WF-C21000 European specs, explicitly discontinued regional SKU][NEP01] — official search index.
- **NEP02** — [Epson WF-C21000 firmware and network management downloads][NEP02] — official search index.
- **NFU01** — [Apeos C8180/C7580/C6580 evaluated secure-configuration guide][NFU01] — official search index.
- **NLE02** — [Markvision 4.7 server-to-printer ports][NLE02] — full target section.
- **NLE03** — [Markvision 4.7 firmware update procedure and access-control prerequisites][NLE03] — full target section.
- **NLE04** — [Markvision 4.7 certificate automation architecture][NLE04] — full target section.
- **NOK01** — [C824/C834/C844 official specifications][NOK01] — official search index.
- **NOK02** — [MC853/MC873/MC883 official specifications][NOK02] — official search index.
- **NEP03** — [WF-C5390 and WF-C5890 January 2023 launch][NEP03] — official search index.
- **S01** — [RFC 3805, Printer MIB v2][S01] — carried from first pass; read..
- **S02** — [RFC 2790, Host Resources MIB][S02] — carried from first pass; read..
- **S03** — [RFC 8011, IPP model and semantics][S03] — carried from first pass; read..
- **S05** — [RFC 3414, SNMPv3 USM][S05] — carried from first pass; read..
- **S09** — [RFC 6762, mDNS][S09] — carried from first pass; read..
- **S10** — [RFC 6763, DNS-SD][S10] — carried from first pass; read..
- **R23** — [IPA/JISEC Ricoh and regional-badge certification][R23] — carried from first pass; indexed; exact named alias set..
- **R24** — [Ricoh history / acquired sales organizations PDF][R24] — carried from first pass; indexed..
- **H09** — [HP enterprise eSCL configuration guidance][H09] — carried from first pass; indexed; platform scope..
- **H12** — [HP WXP certificate policies / EST Connector][H12] — carried from first pass; indexed; manager scope..
- **C06** — [DX C5800 network menu / IPP Everywhere / RFC 2790][C06] — carried from first pass; indexed..
- **K06** — [C360i/C361i firmware release information PDF][K06] — carried from first pass; indexed; Fleet RMM/NDES conditions..
- **A04** — [Sharp new BP-series launch, October 2025][A04] — carried from first pass; read..
- **A09** — [Sharp evaluated guidance/firmware archive][A09] — carried from first pass; indexed; not necessarily latest firmware..
- **T04** — [EU 25AC brochure PDF][T04] — carried from first pass; indexed..
- **R21** — [Firmware Update Tool model applicability][R21] — carried from first pass; read shell/applicability; transport not available..
- **R27** — [Always Current Technology deployment/update explanation][R27] — carried from first pass; indexed..
- **H06** — [Enterprise/Managed model equivalents][H06] — carried from first pass; indexed..
- **H07** — [FutureSmart version/model table][H07] — carried from first pass; indexed; dynamic, not a pinned update recommendation..
- **H03** — [M528 series datasheet][H03] — carried from first pass; indexed; direct retrieval timeout..
- **H04** — [M555 series December 2020 datasheet][H04] — carried from first pass; indexed; direct retrieval timeout..
- **R03** — [Ricoh Australia 2019 IM C launch][R03] — carried from first pass; read/indexed..
- **R04** — [Ricoh AP 2023 IM C launch][R04] — carried from first pass; indexed..
- **R05** — [US IM C series brochure PDF][R05] — carried from first pass; indexed; regional members and specs..
- **R22** — [Japan IM C SD variants launch][R22] — carried from first pass; indexed..
- **R06** — [Japan IM Cxx11 launch, 17 August 2026][R06] — carried from first pass; indexed; sale date 24 August..
- **R07** — [IM 3500 AP product/specification page][R07] — carried from first pass; indexed..
- **R11** — [2023 MFP/printer security white paper PDF][R11] — carried from first pass; read; TPM, encryption, TLS, port controls..
- **R12** — [Japan new mono IM launch][R12] — carried from first pass; indexed; January 2026 availability..
- **R13** — [AP mono IM xx10 launch, April 2026][R13] — carried from first pass; indexed..
- **R19** — [Ricoh intelligent-device range brochure PDF][R19] — carried from first pass; indexed; differing regional capacity..
- **R14** — [AP IM C320F / M C320FW / P C375 launch][R14] — carried from first pass; indexed..
- **R17** — [IM 460F AP specification page][R17] — carried from first pass; read..
- **C01** — [Canon DX C3900 and other 2023 launches][C01] — carried from first pass; indexed..
- **C09** — [DX platform launch, March 2020][C09] — carried from first pass; indexed..
- **C04** — [DX C5800 / 6800 2021 launch][C04] — carried from first pass; indexed..
- **C05** — [DX C5800 brochure PDF][C05] — carried from first pass; read..
- **C07** — [imageFORCE C5100 / 6100 launch, July 2025][C07] — carried from first pass; indexed..
- **C08** — [imageFORCE C5100 certificate/import/export manual][C08] — carried from first pass; indexed..
- **X01** — [AltaLink 8100 launch, July 2020][X01] — carried from first pass; indexed..
- **X02** — [B/C8100 and B/C8200 administrator guide PDF, July 2025][X02] — carried from first pass; read; SNMP, eSCL, cloning and firmware..
- **X03** — [AltaLink C8200 detailed specification PDF][X03] — carried from first pass; read; explicit RFC MIBs and protocols..
- **X04** — [AltaLink 8200 launch, July 2024][X04] — carried from first pass; indexed..
- **X05** — [VersaLink C625/C620 brochure PDF][X05] — carried from first pass; indexed..
- **X06** — [Xerox secure-installation guide index][X06] — carried from first pass; indexed; exact-series guide availability..
- **X07** — [VersaLink C625 detailed specifications, Spanish PDF][X07] — carried from first pass; indexed; 650/2850 capacity..
- **K02** — [C360i/C300i/C250i SNMP administration manual][K02] — carried from first pass; indexed..
- **K03** — [Konica Minolta security technical support paper][K03] — carried from first pass; indexed; OpenAPI administration scope..
- **K04** — [Japan 2024 bizhub 1i launch][K04] — carried from first pass; indexed..
- **K09** — [OAuth firmware model/version applicability][K09] — carried from first pass; indexed; secondary model families..
- **Y05** — [TASKalfa family Asia brochure PDF][Y05] — carried from first pass; indexed; regional storage/options..
- **Y03** — [Japan TASKalfa MZ launch schedule][Y03] — carried from first pass; indexed; 2025 availability..
- **Y04** — [MZ family reliability/security and Net Viewer][Y04] — carried from first pass; indexed..
- **Y07** — [ECOSYS MA4500ifx specification PDF][Y07] — carried from first pass; indexed; Printer MIB/WS-Discovery..
- **Y06** — [ECOSYS MA4000cix product page][Y06] — carried from first pass; indexed..
- **A01** — [BP-70C31/36/45 specification PDF][A01] — carried from first pass; indexed; LED, protocols and firmware management..
- **T01** — [Toshiba global 25AC product/security specifications][T01] — carried from first pass; indexed..
- **T02** — [US 25AC specification PDF, June 2026 revision][T02] — carried from first pass; indexed..
- **T03** — [Toshiba quick-start guide, October 2022 PDF][T03] — carried from first pass; indexed; family existence by 2022..
- **T07** — [TopAccess guide / Current Software List PDF][T07] — carried from first pass; indexed..
- **T06** — [e-STUDIO6529A/7529A/9029A brochure PDF][T06] — carried from first pass; indexed..
- **B04** — [MFC-L9670CDN firmware and management-tool downloads][B04] — carried from first pass; indexed..
- **B01** — [MFC-L6915DW product/specification page][B01] — carried from first pass; read..
- **B02** — [MFC-L6915DW family protocol/security specifications][B02] — carried from first pass; read..
- **E02** — [US AM-C4000/C5000/C6000 launch][E02] — carried from first pass; indexed..
- **E03** — [AM series administrator/user guide PDF][E03] — carried from first pass; indexed; SNMP and firmware procedure..
- **E06** — [US AM-C10000 launch, June 2026][E06] — carried from first pass; indexed..
- **E07** — [WF-C5890 product specifications][E07] — carried from first pass; indexed..
- **O01** — [OKI C650 launch, March 2021][O01] — carried from first pass; indexed..
- **O02** — [C650/ES6450 specifications manual][O02] — carried from first pass; read..
- **O03** — [OKI SNMPv3 and supplied MIB-file location][O03] — carried from first pass; indexed..
- **O05** — [C650dn official firmware/updater page][O05] — carried from first pass; indexed..
- **G01** — [Samsung X4300 SyncThru administration manual, HP-hosted][G01] — carried from first pass; indexed, extensive official extract..
- **G02** — [HP Samsung SL-X4300LX specifications][G02] — carried from first pass; indexed..
- **G03** — [HP Samsung firmware-version checking][G03] — carried from first pass; indexed..
- **F01** — [Fujifilm Apeos C5570/4570/3570/3070 brochure PDF][F01] — carried from first pass; indexed..
- **F02** — [Fujifilm US Apeos C7070-family brochure PDF][F02] — carried from first pass; indexed..
- **F04** — [Apeos C4030/C3530 brochure PDF][F04] — carried from first pass; indexed..
- **Z01** — [RISO global GL specifications][Z01] — carried from first pass; indexed..
- **Z05** — [ComColor GL administrator guide PDF][Z05] — carried from first pass; indexed..
- **H11** — [HP Enterprise/Managed firmware-update procedure][H11] — carried from first pass; indexed; `.bdl`, EWS/WJA..
- **R08** — [RiDP device interfaces][R08] — carried from first pass; read; standard/private MIB and SmartDeviceMonitor API..
- **R09** — [Ricoh Japan developer-program contact/interface list][R09] — carried from first pass; indexed; PMAPI/Common/Private/GJ MIB..
- **R10** — [Ricoh SmartSDK overview][R10] — carried from first pass; indexed; REST workflow/API scope..
- **K10** — [Fleet RMM user guide PDF][K10] — carried from first pass; indexed; communication checks..
- **Y08** — [TASKalfa Command Center RX guide PDF][Y08] — carried from first pass; indexed..
- **L07** — [Fleet Tracker server Open API][L07] — carried from first pass; indexed..
- **L08** — [Document Distributor REST/SOAP job API][L08] — carried from first pass; indexed; jobs, not device administration..
- **B05** — [Brother secure SNMP modes / older utility compatibility][B05] — carried from first pass; indexed..
- **E04** — [AM series US user guide PDF][E04] — carried from first pass; indexed; security and Web Config firmware..
- **E05** — [Epson Device Admin and supported devices][E05] — carried from first pass; indexed..
- **O04** — [MC853 official Configuration Tool/download information][O04] — carried from first pass; indexed..
- **F03** — [Fujifilm Internet Services SNMPv3 help][F03] — carried from first pass; indexed; Apeos 5570 help scope..
- **F05** — [Fujifilm Internet Services setup explanation][F05] — carried from first pass; indexed; C3567-family scope..
- **F06** — [Apeos C325 audit-log guide PDF][F06] — carried from first pass; indexed..

[NHP01]: https://support.hp.com/emea_middle_east-en/document/ish_3017251-3017406-16
[NHP02]: https://support.hp.com/ph-en/document/ish_4025423-4025481-16
[NHP03]: https://developers.hp.com/workpath/compatible-devices
[NHP04]: https://kaas.hpcloud.hp.com/pdf-public/pdf_3075457_en-US-1.pdf
[NHP05]: https://h10032.www1.hp.com/ctg/Manual/c05387361.pdf
[NHP06]: https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=c08740367
[NHP07]: https://support.hp.com/at-de/document/ish_13877154-13872747-16?fallbackLocale=us-en&validated=true
[NHP08]: https://support.hp.com/hk-en/document/ish_3007439-11769595-16
[NHP09]: https://kaas.hpcloud.hp.com/pdf-public/pdf_11219519_en-US-1.pdf
[NHP10]: https://support.hp.com/sg-en/document/c05443354
[NHP11]: https://support.hp.com/us-en/document/ish_3245298-3245359-16
[NHP12]: https://support.hp.com/gb-en/document/ish_8516332-8516384-16
[NHP13]: https://support.hp.com/au-en/document/ish_5609980-5610028-16
[NHP14]: https://www.hp.com/emea_africa-en/products/printers/product-details/product-specifications/38348453
[NHP15]: https://www.hp.com/us-en/shop/pdp/hp-laserjet-enterprise-mfp-8601dn
[NHP16]: https://kaas.hpcloud.hp.com/pdf-public/pdf_11513794_en-US-1.pdf
[NHP17]: https://support.hp.com/gb-en/product/product-specs/hp-laserjet-enterprise-8501-printer-series/2102657284
[NHP18]: https://support.hp.com/us-en/product/setup-user-guides/hp-laserjet-enterprise-5502dn-printer-series/2103151462
[NHP19]: https://kaas.hpcloud.hp.com/pdf-public/pdf_6892698_en-US-1.pdf
[NHP20]: https://kaas.hpcloud.hp.com/pdf-public/pdf_3017438_en-US-1.pdf
[NRI01]: https://www.ricoh-usa.com/en/products/always-current-technology/always-current-technology-instructions-and-versions
[NRI02]: https://www.ricoh-ap.com/products/office-printing/mfp-colour/-/media/ra/files/pdfs/products_brochure/office_solutions/printers_and_copiers/mfp_colour/ricoh-im-c6500-c8000-brochure.pdf
[NRI03]: https://assets.ricoh-usa.com/j2jqn9lauv41/1CdwVWGQXlWPUvDB7EZqqr/387ce4601d87d7cb760faf613c87ae31/IM_7000_8000_9000-spec-sheet_RUS_EN_FINAL_V1.pdf
[NRI04]: https://manuals.ricoh.com/mfp/p_manual/IM9000JPN/ja/setup/int/network.htm
[NRI05]: https://www.ricoh-ap.com/support/enabling-oauth-2-authentication-for-ricoh-products
[NRI06]: https://www.ricoh-ap.com/products/office-printing/black-and-white-printer/p-800
[NRI07]: https://www.ricoh-ap.com/products/office-printing/colour-printer/p-c600
[NRI08]: https://www.ricoh-ap.com/products/office-printing/colour-printer/p-c375
[NRI09]: https://www.ricoh-ap.com/configurators
[NRI10]: https://www.ricoh-ap.com/downloads/manual-for-office-solutions
[NCA01]: https://oip.manual.canon/USRMB-0556-zz-DS-enUV/
[NCA02]: https://oip.manual.canon/USRMA-8320-zz-CS-8900-enAU/contents/devu-set_reg-pref-nw.html
[NCA03]: https://oip.manual.canon/USRMA-8319-zz-CS-8900-enUS/contents/devu-setup-nw-pref_fit-snmp_moni_control.html
[NCA04]: https://oip.manual.canon/USRMA-8318-zz-CS-8900-enGB/contents/devu-apdx-sys_spec-nw.html
[NCA05]: https://www.usa.canon.com/bin/canon/downloadasset?path=%2Fcontent%2Fdam%2Fcanon-assets%2Fbrochures%2Fbusiness%2Fcopiers-and-mfp%2Fimagerunner-advance-dx%2FimageRUNNER-ADV-DX-8900-Series-ProductBrochure.pdf
[NXE01]: https://www.shop.xerox.com/altalink-b8200-series
[NXE02]: https://securitydocs.business.xerox.com/wp-content/uploads/2024/09/Security-Guide-Information-Assurance-Disclosure-Xerox%C2%AE-AltaLink%C2%AE-B82XX-and-C82XX.pdf
[NKM01]: https://www.konicaminolta.eu/eu-en/support/vulnerability-information/pass-back-attack-vulnerability-in-konica-minolta-multifunction-printers
[NKM02]: https://www.konicaminolta.eu/eu-en/digital-workplace/printing-devices/bizhub-c651i
[NKM03]: https://www.konicaminolta.hk/hk/wp-content/uploads/2025/03/bizhub-C751i-datasheet_low-1.pdf
[NKM04]: https://www.konicaminolta.eu/eu-en/news/konica-minolta-launches-accurioprint-850i-and-950i-series-to-help-customers-rethink-efficiency
[NKY01]: https://www.kyoceradocumentsolutions.us/content/download-center-americas/us/documents/brochures-and-specs/TASKalfa_7054ci_Spec_Sheet_pdf.download.pdf
[NKY02]: https://www.kyoceradocumentsolutions.com/global/en/support-and-download/support-information/info_20230622.html
[NKY03]: https://www.kyoceradocumentsolutions.us/en/support/downloads.name-L3VzL2VuL21mcC9FQ09TWVNNQTYwMDBJRlg%3D.html
[NKY04]: https://www.kyoceradocumentsolutions.eu/en/about-us/contact-us/press/kyocera-command-center-rx-%28ccrx%29-security-vulnerability.html
[NLE01]: https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-96923c7e-f5e1-4123-87f2-d4baf2ddfbcf-en.html
[NSH01]: https://business.sharpusa.com/portals/0/downloads/guides/IT_Reference_Guide.pdf
[NSH02]: https://business.sharpusa.com/Portals/0/downloads/Literature/SRDM_Data_Sheet.pdf
[NSH03]: https://business.sharpusa.com/portals/0/downloads/Literature/BP-70M75_70M90-Specification-Sheet.pdf
[NTO01]: https://www.toshibatec.com/cnt/products_overseas/MFP/eS7527AC_series/
[NBR01]: https://support.brother.com/g/b/faqend.aspx?c=us_ot&faqid=faq00000027_035&lang=en&prod=mfcl9670cdn_all
[NEP01]: https://www.epson.eu/en_EU/products/printers/inkjet/business-inkjet/workforce-enterprise-wf-c21000/p/29267
[NEP02]: https://www.epson.eu/en_EU/support/sc/epson-workforce-enterprise-wf-c21000/s/s1780
[NFU01]: https://support-fb.fujifilm.com/driver_downloads/security_function_AC8180_C7580_C6580_gm1043e2-1.pdf
[NLE02]: https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-0cdeb48e-9b19-491c-b31e-2b30b2e4488c-en.html
[NLE03]: https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-3776b84f-3e48-45d7-b122-9d04fe0b5f7b-en.html
[NLE04]: https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-0f35f6c8-7344-4ea5-a3ab-2f349be62d38-en.html
[NOK01]: https://www.oki.com/printing/online-manuals-18/470955/id/contents/contents/ap_000012.html
[NOK02]: https://www.oki.com/printing/online-manuals-12/459925/id/contents/contents/am_000039.html
[NEP03]: https://news.epson.com/news/workforce-pro-wf-c5890-c5390
[S01]: https://www.rfc-editor.org/rfc/rfc3805
[S02]: https://www.rfc-editor.org/rfc/rfc2790
[S03]: https://www.rfc-editor.org/rfc/rfc8011
[S05]: https://www.rfc-editor.org/rfc/rfc3414
[S09]: https://www.rfc-editor.org/rfc/rfc6762
[S10]: https://www.rfc-editor.org/rfc/rfc6763
[R23]: https://www.ipa.go.jp/en/security/jisec/software/certified-cert/c0638_it8682.html
[R24]: https://www.ricoh.com/-/Media/Ricoh/Sites/com/IR/financial_data/annual_report/ar/10/ar12.pdf
[H09]: https://support.hp.com/ro-en/document/c04945509
[H12]: https://learn.workforceexperience.hp.com/docs/installing-and-managing-certificates-using-printer-policies
[C06]: https://oip.manual.canon/USRMA-5488-zz-CS-5800-enUS/contents/devu-set_reg-pref-nw.html
[K06]: https://cscsupportftp.mykonicaminolta.com/DownloadFile/Download.ashx?fileversionid=39530&productid=2175
[A04]: https://global.sharp/corporate/news/251009-a.html
[A09]: https://global.sharp/products/copier/certified/download/cc-certified.html
[T04]: https://fi.toshibatec.eu/publicsite-service/resource/download/pseu/en/93e9c5fb-a118-4378-87c0-20891df166d3/030ef8eb494c8943c444bb6adf66b2cc/BR_eSTUDIO6525AC_series.pdf
[R21]: https://kb.gsd.ricoh.com/app/answers/ans_product/a_id/293886
[R27]: https://www.ricoh.com.my/dynamic-workplace-intelligence/ricoh-always-current-technology/ract-learn-more
[H06]: https://support.hp.com/be-fr/document/ish_13770559-13697423-16
[H07]: https://support.hp.com/sg-en/document/ish_4025423-4025481-16
[H03]: https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=c08015961
[H04]: https://h20195.www2.hp.com/v2/getpdf.aspx/4aa7-8159enuc.pdf
[R03]: https://www.ricoh.com.au/news/ricoh-unveils-new-intelligent-devices
[R04]: https://www.ricoh-ap.com/news/2023/05/09/ricoh-introduces-new-im-c-series-a3-multifunction-printers
[R05]: https://assets.ricoh-usa.com/j2jqn9lauv41/1zi0tYuWi9f18Oz7SEKVY3/960afe1aeb53bafdb38a2df058041a7b/IM_C_Series_Brochure_RUSA.pdf
[R22]: https://jp.ricoh.com/release/2025/0203_1
[R06]: https://jp.ricoh.com/release/2026/0817_1
[R07]: https://www.ricoh-ap.com/products/office-printing/mfp-black-and-white/im-3500
[R11]: https://www.ricoh.com/-/Media/Ricoh/Sites/com/security/pdf/mfp-security-whitepaper.pdf?230929=
[R12]: https://jp.ricoh.com/release/2025/1222_1
[R13]: https://www.ricoh-ap.com/news/2026/04/24/ricoh-introduces-new-im-xx10-series-a3-black-and-white-multifunction-printers
[R19]: https://www.ricoh-ap.com/en/products/office-printing/mfp-colour/-/media/ra/files/pdfs/products_brochure/office_solutions/printers_and_copiers/range_brochure/ricoh_intelligent_devices_brochure.pdf
[R14]: https://www.ricoh-ap.com/news/2024/09/21/ricoh-unveils-new-a4-colour-laser-mfps
[R17]: https://www.ricoh-ap.com/products/office-printing/mfp-black-and-white/im-460f
[C01]: https://www.usa.canon.com/newsroom/2023/20230523-printer
[C09]: https://www.usa.canon.com/newsroom/2020/20200310-iradv-dx
[C04]: https://www.usa.canon.com/newsroom/2021/20210601-products
[C05]: https://www.usa.canon.com/bin/canon/downloadasset?path=%2Fcontent%2Fdam%2Fcanon-assets%2Fbrochures%2Fbusiness%2Fcopiers-and-mfp%2Fimagerunner-advance-dx%2FEnterprise_iR-ADV-DX-5800-Series_Brochure.pdf
[C07]: https://www.usa.canon.com/newsroom/2025/20250722-printer
[C08]: https://oip.manual.canon/USRMA-9286-zz-CS-5100-enUS/contents/devu-set_menu_list-impt_expt-mng-dev-certificate.html
[X01]: https://www.news.xerox.com/news/xerox-launches-connectkey-enabled-altalink-digital-workplace-assistant
[X02]: https://download.support.xerox.com/pub/docs/ALB81XX/userdocs/any-os/en_GB/Xerox_AltaLink_B8100_C8100_AltaLink_B8200_C8200_series_mfp_sag_en-US.pdf
[X03]: https://www.office.xerox.com/latest/AC8SS-04UA.PDF
[X04]: https://www.news.xerox.com/news/xerox-transforms-document-processing-with-new-ai-assisted-xerox-altalink-8200-series-multi-function-printers
[X05]: https://www.office.xerox.com/latest/V6CBR-012.pdf
[X06]: https://security.business.xerox.com/en-usdocuments/installation-guides/
[X07]: https://www.office.xerox.com/latest/V6CSS-01L.pdf
[K02]: https://manuals.konicaminolta.eu/bizhub-C360i-C300i-C250i/EN/bizhub-c360i-c300i-c250i/contents/id08-_104516893.html
[K03]: https://www.konicaminolta.com/global-en/security/mfp/setting/pdf/SecurityTechnicalSupportPaper_en.pdf
[K04]: https://www.konicaminolta.com/jp-ja/newsroom/2024/0515-01-01.html
[K09]: https://www.konicaminolta.eu/eu-en/support/oauth2.0-information
[Y05]: https://www.kyoceradocumentsolutions.com/vn/vi/products/catalog/pdf/Brochure_TASKalfa_2554ci_3554ci_4054ci_5054ci_6054ci_7054ci_5004i_6004i_7004i_AS.pdf
[Y03]: https://www.kyoceradocumentsolutions.com/global/ja/news/rls_2024/rls_20241202.html
[Y04]: https://www.kyoceradocumentsolutions.com/asia/en/products/mfp/taskalfa-mz7001ci/features-security.html
[Y07]: https://www.kyoceradocumentsolutions.us/content/download-center-americas/us/documents/brochures-and-specs/KDA_ECOSYS_MA4500ifx_pdf.download.pdf
[Y06]: https://www.kyoceradocumentsolutions.us/en/products/mfp/ECOSYSMA4000CIX.html
[A01]: https://business.sharpusa.com/portals/0/downloads/Literature/BP-70C31_70C36_70C45-Specification-Sheet.pdf
[T01]: https://www.toshibatec.com/cnt/products_overseas/MFP/eS6525AC_series/
[T02]: https://business.toshiba.com/app/uploads/2026/06/2525AC-3025AC-3525AC-4525AC-Spec-Sheet-EN.pdf
[T03]: https://business.toshiba.com/downloads/KB/f1Ulds/20151/eS6528A_QS_EN_0001.pdf
[T07]: https://business.toshiba.com/downloads/KB/f1Ulds/20992/eS6528A_TAG_EN_0003.pdf
[T06]: https://business.toshiba.com/app/uploads/2023/04/6529A-7529A-9029A-Brochure-EN.pdf
[B04]: https://support.brother.com/g/b/downloadlist.aspx?c=us&lang=en&os=10012&prod=mfcl9670cdn_all
[B01]: https://www.brother-usa.com/p/laser-printers/MFCL6915DW
[B02]: https://support.brother.com/g/s/id/htmldoc/mfc/cv_mfcl6915dw/use/html/GUID-6CD1909F-A0FF-4E39-8C38-E80D9857564E_1.html?lang=en&prod=dcpl5610dn_us
[E02]: https://news.epson.com/news/workforce-enterprise-am-c4000-c5000-c6000
[E03]: https://files.support.epson.com/docid/other/npd7005-00_en.pdf
[E06]: https://news.epson.com/news/workforce-enterprise-am-c10000-mfp
[E07]: https://epson.com/For-Work/Printers/Inkjet/WorkForce-Pro-WF-C5890-Color-MFP/p/C11CK23201?blaid=5961692
[O01]: https://www.oki.com/sg/printing/about-us/news-room/press-releases/2021/210316_ODSP_C650_OKI_Launches_World_Smallest_High_Performance_A4_Colour_Printer/index.html
[O02]: https://www.oki.com/printing/online-manuals-Z002/EE8001-1215/id/contents/contents/33916965.html
[O03]: https://www.oki.com/printing/online-manuals-Z002/EE8001-6614/id/contents/contents/241796849.html
[O05]: https://www.oki.com/es/printing/support/firmware/colour/09006144/
[G01]: https://printersetup.ext.hp.com/ts/manual/Samsung%20X4300%20Series/English/manual/sws.htm
[G02]: https://support.hp.com/ro-en/product/product-specs/samsung-multixpress-sl-x4300-color-laser-multifunction-printer-series/model/16449692
[G03]: https://support.hp.com/au-en/document/c05584061
[F01]: https://asset.fujifilm.com/www/in/files/2024-07/7c4415f3b39e90052e7787cc50562056/apeos-c5570-c4570-c3570-brochure_ind.pdf
[F02]: https://asset.fujifilm.com/www/us/files/2024-11/f06ebb5723fb04af46f09583c1cd7212/apeos_c7070_brochure.pdf
[F04]: https://asset.fujifilm.com/www/in/files/2025-04/0d239230c247f480ec5fe82dc7feb2fd/apeos-c4030-c3530-brochure_in.pdf
[Z01]: https://www.riso.co.jp/english/product/comcolor/gl/spec/index.html
[Z05]: https://www.riso.com/download/manual/gl/GL_AdministratorsGuide_ENG.pdf
[H11]: https://support.hp.com/au-en/document/ish_4158468-4158516-16
[R08]: https://ricoh-ridp.com/ricoh-device-interfaces
[R09]: https://www.mlsvc.ricoh.co.jp/regist/is?SMPFORM=lap-sik-87c49836449aea2ce532de713da3aafa
[R10]: https://emea.ricoh-developer.com/about-us/membership/smart-membership-benefits/smartsdk-overview
[K10]: https://konicaminolta.ca/wps/wcm/connect/bca/a5e9861a-cc04-4360-a46e-484aa26f3209/FleetRMMUserGuide.pdf?MOD=AJPERES
[Y08]: https://dam.kyoceradocumentsolutions.com/content/dam/gdam_dc/dc_global/document/manual/product_080/commandcenterrx_taskalfa7054ci_en_03.pdf
[L07]: https://publications.lexmark.com/publications/lexmark_solutions/Fleet_Manager/AG/html/en/openAPI-group.html
[L08]: https://publications.lexmark.com/publications/lexmark_solutions/Document_Distributor/SDK_Guide/v5.4/html/en/understanding-job-submission-web-service-api-topic.html
[B05]: https://support.brother.com/g/b/sp/faqend.aspx?c=nz&faqid=faq00100232_002&lang=en&prod=mfcl8690cdw_eu_as&src=ipas
[E04]: https://files.support.epson.com/docid/cpd6/cpd62840.pdf
[E05]: https://epson.com/Accessories/Printer-Accessories/Epson-Device-Admin/p/DEVICEADMIN
[O04]: https://www.oki.com/de/printing/support/drivers-and-utilities/colour-multifunction/45850404/
[F03]: https://opencds-fb.fujifilm.com/gen/product2_aux/prod/ews_help/ap/apeos_5570/en/contents/07_protocol/pro_setting_snmp_v3.html
[F05]: https://opencds-fb.fujifilm.com/gen/product2_aux/prod/manual/ap_c3567_ug/solution/en/000005.html
[F06]: https://support-fb.fujifilm.com/driver_downloads/gm2160e5-1.pdf

