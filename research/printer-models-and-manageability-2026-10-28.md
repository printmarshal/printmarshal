# Business printer models and network manageability

Research date: **28 October 2026**. Scope: office network printers and MFPs, approximately 2020–2026, with selected earlier deployed generations. Ricoh is prioritized. This is a desk-research report for a device fleet platform; no printers were contacted, configured, scanned, or updated.

## Evidence and interpretation

The supplied research prompt is the subject brief. This report adds findings only; it does not change the application or the prompt.

**This is not a measured ranking of installed devices.** Public manufacturer specifications establish capabilities and product positioning, not model-level installed-base counts. IDC's public tracker describes shipments, which include consumer products and do not establish an enterprise installed base. Consequently, the shortlist below ranks **support implementation value**, using deployment-era plausibility, business segment, reuse across related models, technical coverage, and the requested Ricoh emphasis. Model prevalence remains a hypothesis until compared with real customer inventories. [M01]

Evidence labels used throughout:

- **Y**: explicitly documented for the named model or a manual that explicitly includes it.
- **F**: documented at a related family/platform level; exact SKU and firmware still need confirmation.
- **P**: a vendor/partner interface exists; public operation coverage, access entitlement, or exact model applicability is incomplete.
- **I**: an inference, stated as such; never a compatibility guarantee.
- **U**: not confirmed in the sources used. It does **not** mean unsupported.
- **—**: inapplicable, such as scanning on a printer without a scanner.
- **C**: present in an official regional lineup/product source; this is not a worldwide sales or support-lifecycle guarantee.
- **E**: earlier generation retained for deployed-fleet coverage. **L**: legacy line, especially Samsung. Continued documentation or firmware availability is not proof of continued sale.
- **≤year**: documented by that year; the original release year was not established. A brochure revision, certification, or support-page date is not silently treated as a launch date.

Specifications are regional and configuration dependent. Speeds are simplex A4 unless **Ltr** is stated. A3 engines often advertise their A4/Letter speed. “Maximum duty cycle” is distinct from recommended monthly volume. Marketing “equivalent” resolution is distinguished from native print resolution where the source does so. Unresolved fields are retained as U instead of filled from reseller listings.

## Executive summary

Build a **standards-based discovery and monitoring core**, then add vendor and controller-specific configuration/update adapters. Printer MIB and Host Resources MIB expose useful identity, supplies, trays, alerts, and printer state; IPP adds capabilities and operational state. Neither establishes a uniform API for address books, certificates, security policy, storage sanitization, or firmware deployment. The standards and vendor documentation support this division. [S01] [S02] [S03] [X02] [K03]

Treat **SNMP version**, **MIB coverage**, **IPP support**, **IPP Everywhere**, **AirPrint**, **Mopria Print**, and **eSCL** as separate capabilities. SNMPv3 says nothing about which MIB objects exist or are writable. IPP support and an AirPrint logo do not independently prove PWG IPP Everywhere certification. A print certification does not prove driverless scanning. The PWG registry's model rows did not load in the research browser, so this report does not invent certifications from it. [S04] [S05] [S06] [S07]

Ricoh should receive the first substantial plugin: the 2019 IM C generation, 2023 IM Cxx10 generation, A4 Smart Operation Panel models, and newer regional variants offer useful coverage. Ricoh explicitly offers standard/private MIB specifications and management interfaces through its developer program. SmartSDK WebAPI is a REST interface for device workflows; its existence is **not evidence of an unrestricted REST API for every administrative setting**. [R08] [R09] [R10]

Brand consolidation does not eliminate controller differences. Xerox completed its Lexmark acquisition on 1 July 2025; retain separate Xerox and Lexmark protocol profiles. Ricoh/Toshiba's ETRIA engine venture, subsequently joined by OKI, also does not make existing device controllers or firmware interchangeable. Toshiba's own investor material explicitly depicts brand-specific controllers on common engines. HP's acquisition of Samsung printers closed on 1 November 2017; Samsung SyncThru devices remain a separate legacy management generation. [M02] [M03] [M04] [M05] [M06]

The largest outstanding uncertainties are exact per-SKU MIB conformance, downloadable enterprise MIB revisions, partner SDK entitlements, model-specific eSCL/IPP Everywhere evidence, update transport details, and release/end-of-sale dates for several regional lines. These are listed explicitly in the verification backlog.

## Master comparison

The API column concerns **device administration**, not print-job submission. P/F entries require the qualifications in the brand sections. Each row's source column identifies the hardware source; management evidence is linked in the corresponding brand section. Model lists in one row are test siblings, not assertions of identical firmware.

| Brand / concrete model(s) | Release evidence; status | Type / technology | ppm | SNMPv3 | IPP Everywhere | eSCL | Administrative API / framework | Security highlights and qualification | Hardware source |
|---|---|---|---:|---|---|---|---|---|---|
| Ricoh IM C3000 / C3500 | 2019; E | A3 color laser MFP | 30 / 35 | F | U | U | P: private MIB / PMAPI; SmartSDK workflow REST | Storage encryption and protocol controls are platform features; check firmware | [R02] [R03] |
| Ricoh IM C3010 / C3510 | 2023; C in AP/NA, newer JP generation exists | A3 color laser MFP | 30 / 35 | Y | U | U | P: Ricoh Device Interfaces; SmartSDK | TPM 2.0, encrypted storage; TLS capability depends on firmware/service | [R01] [R04] [R11] |
| Ricoh IM C4510 / C6010 | 2023; C regional | A3 color laser MFP | 45 / 60 | Y | U | U | P: same interface program; operation scope U | TPM 2.0; security white paper explicitly names models | [R04] [R05] [R11] |
| Ricoh IM C3011 / C3511 / C4511 / C6011 | 24 Aug 2026, JP; C | A3 color electrophotographic MFP | 30 / 35 / 45 / 60 | U | U | U | U exact generation; do not inherit Cxx10 claims | New regional generation; obtain exact administrator manual | [R06] |
| Ricoh IM 3500 / 5000 / 6000 | ≤2023; E; approximately 2021 generation, launch not verified here | A3 mono laser MFP | 35 / 50 / 60 | F | U | U | P: Ricoh interface program | TLS 1.3 generation documented; HDD configuration matters | [R07] [R11] |
| Ricoh IM 3510 / 4510 / 6010 / 7010 | 2026, regional launches; C | A3 mono electrophotographic MFP | 35 / 45 / 60 / 70 | U | U | U | U exact SKU coverage | TPM 2.0/WPA3 in AP launch; obtain model guide | [R12] [R13] |
| Ricoh IM C320F | Sep 2024 AP; C | A4 color laser MFP | 32 | F | U | U | P: sGW / SmartSDK; Web Image Monitor | TLS 1.3, WPA3, authentication; optional SSD | [R14] [R15] [R16] |
| Ricoh IM 460F | Release U; C regional | A4 mono laser MFP, A3 bypass/ADF capability | 46 | U | U | U | P: platform interfaces; exact support U | eMMC encryption, 802.1X; HDD overwrite requires optional HDD | [R17] |
| HP LaserJet Enterprise M507dn / M528dn | By 2019; E, firmware maintained | A4 mono laser printer / MFP | 43 default; up to 50 high-speed | F | U | — / F | P: OXP ecosystem / Web Jetadmin; public generic REST U | SNMPv3/HTTPS/IPsec platform; storage differs by SKU | [H01] [H02] [H03] |
| HP Color LaserJet Enterprise M555dn | 2020 datasheet; E | A4 color laser printer | 38 | Y | U | — | P: OXP / Web Jetadmin | Enterprise network/security policy support | [H04] |
| HP Color LaserJet Enterprise MFP 5800dn | Release U, ≤2025 source; C | A4 color laser MFP | 43 / 45 Ltr | Y | U | F | P: FutureSmart / OXP / Web Jetadmin | 802.1X, certificate management, SNMPv3 | [H05] [H15] |
| Canon imageRUNNER ADVANCE DX C3930i / C3935i | May 2023; E in US after 2026 replacement | A3 color laser MFP | 30 / 35 Ltr | Y | F | U | General admin REST U; Remote UI / MEAP ecosystem | TLS 1.3, 802.1X, IPsec, SSD encryption | [C01] [C02] [C03] |
| Canon imageRUNNER ADVANCE DX C5850i / C5860i | 2021; E | A3 color laser MFP | 50 / 60 Ltr | F | Y | U | General admin REST U; iW console / Remote UI | Startup verification, embedded control, TPM, encrypted SSD | [C04] [C05] [C06] |
| Canon imageFORCE C5150 / C5160 / C5170 | Jul 2025; C | A3 color OLED-exposure electrophotographic MFP | 50 / 60 / 70 Ltr | U | U | U | U general API; Remote UI configuration documented | SCEP/certificate settings documented on this exact family | [C07] [C08] |
| Xerox AltaLink C8145 / C8155 | Jul 2020; E | A3 color laser MFP | 45 / 55 | Y | U | Y | P: EIP; documented cloning / Fleet Orchestrator | 802.1X/IPsec; options and software release affect controls | [X01] [X02] |
| Xerox AltaLink C8245 / C8255 | Jul 2024; C | A3 color laser MFP | 45 / 55 | Y | U | Y | P: EIP; Fleet Orchestrator / clone files | TPM 2.0, NVMe SSD, network and firmware policy controls | [X02] [X03] [X04] |
| Xerox VersaLink C625 | ≤2024 brochure; C | A4 color laser MFP | 50 / 52 Ltr | F | U | U | P: ConnectKey; confirm operation coverage separately | Exact secure-install guide required; not inferred from AltaLink | [X05] [X06] |
| Konica Minolta bizhub C300i / C360i | 2019 generation; E | A3 color laser MFP | 30 / 36 | Y | U | U | Y/P: bizhub OpenAPI; partner specifications | SNMPv3 / TLS; optional security functions vary | [K01] [K02] [K03] |
| Konica Minolta bizhub C301i / C361i | May 2024 JP; C | A3 color laser MFP | 30 / 36 | F | U | U | P: OpenAPI / Fleet RMM; exact firmware gates | Certificate issuance through Fleet RMM is firmware dependent | [K04] [K05] [K06] |
| Kyocera TASKalfa 3554ci | 2021 generation; E/C regional | A3 color laser MFP | 35 Ltr | Y | U | U | General REST U; HyPAS / Net Viewer / KFS | SCEP, secure boot, TPM, TLS 1.3, encrypted HDD | [Y01] [Y02] |
| Kyocera TASKalfa MZ3501ci / MZ5001ci | Jan 2025 JP; C | A3 color laser MFP | 35 / 50 | F | U | U | General REST U; HyPAS / KFS | SCEP and fleet monitoring/update tools documented for family | [Y03] [Y04] |
| Brother MFC-L6915DW | By Nov 2023; C | A4 mono laser MFP | 50 / 52 Ltr | Y | U | U | General REST U; Mass Deployment Tool / BRAdmin | 802.1X, IPsec, TLS, WPA3; USB job storage differs from internal disk | [B01] [B02] [B03] |
| Lexmark CX635adwe | By Apr 2023; C | A4 color laser MFP | 40 / 42 Ltr | Y | U | Y | Direct admin REST U; MVE / eSF; Fleet Tracker server API | 802.1X, IPsec, certificates; optional intelligent storage | [L01] [L02] [L03] |
| Epson WorkForce Enterprise AM-C6000 | Nov 2022 EU / Mar 2023 US; C | A3 color page-width inkjet MFP | 60 | Y | U | I: Mopria Scan documented | Direct admin REST U; Epson Device Admin; Open Platform workflow | IPsec/802.1X, startup verification, CA certificates | [E01] [E02] [E03] [E04] [E08] |
| Sharp BP-70C36 | Apr 2022 US; E/C regional | A3 color **LED** MFP | 36 | Y | U | U | P: Sharp OSA; full admin REST U | TLS 1.3, WPA3, port filtering, integrity/self-recovery | [A01] [A02] |
| Sharp BP-51C36 / BP-71C65 | 2025 generation; C regional | A3 color electrophotographic MFP | 36 / 65 | U | U | U | P: OSA generation; exact administrative scope U | BIOS/firmware recovery in new series; obtain exact security guide | [A03] [A04] [A05] |
| Toshiba e-STUDIO3525AC / 4525AC | By Oct 2022 manual; C regional | A3 color laser MFP | 35 / 45 Ltr | Y | U | U | General REST U; TopAccess / e-BRIDGE Next | SCEP, TPM 2.0, secure boot, encrypted SSD; IPsec regional option | [T01] [T02] [T03] |
| OKI C650dn / ES6450 | Mar 2021 regional launch; C | A4 color LED printer | 35 | Y | U | — | General REST U; Configuration Tool | 802.1X, IPsec, HTTPS; SNMPv2c not established | [O01] [O02] [O03] |
| Samsung MultiXpress SL-X4300LX | Pre-acquisition generation; L; exact launch U | A3 color laser MFP | 30 | Y | U | U | General REST U; SyncThru / XOA workflow | SNMPv3, certificates, IPsec and access controls; inspect legacy firmware | [G01] [G02] |
| Fujifilm Apeos C3570 / C4570 | Release U; ≤2024 US/India sources; C regional | A3 color electrophotographic MFP | 35 / 45 | F | U | U | General REST U; Internet Services / custom services | Do not copy Xerox controller/security support | [F01] [F02] [F03] |
| RISO ComColor GL9730 / GL7430 | 2021 JP announcement / 2022 ComColor history; C | A3 color line inkjet printer; optional scanner | 165 / 140 | U; native JP spec lists v1 | U | U if scanner fitted | General REST U; RISO Console | HTTPS/IPsec documented; native protocol list is not SNMPv3 proof | [Z01] [Z02] [Z03] [Z04] |

## Hardware and connectivity reference

These are concrete configurations, not a single spec applied to an entire manufacturer. **All rows below have documented automatic duplex**, except where the exact configuration remains U. Input capacity excludes the ADF. `G` means 10/100/1000 Ethernet; `opt` means optional. Storage entries distinguish RAM, controller flash, and user/job storage. Unknown duty-cycle values are not replaced with a recommended monthly volume.

| Model / source configuration | Max print resolution, dpi | Input sheets std / max | Max monthly duty cycle | RAM std / max | Onboard storage | Ethernet / Wi-Fi / USB | Evidence |
|---|---|---|---|---|---|---|---|
| Ricoh IM C3010 / C3510, Canadian sheet | 1200×1200 | 1200 / 4700 | U | 4 GB main + 4 GB panel; max same | 256 GB SSD | G / opt / A+B | [R01] |
| Same names, AP specification | 1200×1200 | 1200 / 4850 | U | 2 GB main + 4 GB panel | 256 GB SSD | G / opt / A+B | [R18] |
| Ricoh IM C3000 / C3500, older US-hosted sheet | 1200×1200 | 1200 / 2300 in this sheet; conflicts with other regional brochures | U | 2 / 4 GB as stated; panel allocation not separated | 320 GB HDD | G / opt / A+B | [R02] [R19] |
| Ricoh IM 3500, AP | 1200×1200 | 1200 / 4700 | U | 2 GB main + 2 GB panel | 320 GB storage; HDD in configurator | G / opt / USB; verify exact accessory | [R07] |
| Ricoh IM C320F, Canadian sheet | 1200×1200 | 300 / 800 | U | 2 GB main + 4 GB panel | 64 GB eMMC; opt 256 GB SSD | G / opt / A+B | [R15] [R16] |
| Ricoh IM 460F, AP | 1200×1200 | 600 / 2100 | U | 2 GB main + 4 GB panel | 64 GB eMMC; optional HDD/SSD configurations | G / opt / A+B | [R17] |
| HP M507dn, current support guide / UK store | 1200×1200 | 650 / 2300 | 150,000 | 1 / 2 GB in current guide; check physical revision | Optional encrypted HDD in guide; base user storage U | G / opt / USB | [H01] [H02] [H13] |
| HP M528dn | 1200×1200 | 650 / max U here | 150,000 | 1.25 GB printer + 512 MB scanner; max 3.75 GB stated | U; Flow and non-Flow options differ | G / Wi-Fi opt / USB | [H03] [H14] |
| HP M555dn / x | 1200×1200 fine-line mode; ImageREt 3600 also advertised | dn 650 / 2300; x 1200 / 2300 | U | 1 / 2 GB | Optional HP secure HDD; base user storage U | G / optional dn, included x dual-band wireless in EU sheet / USB | [H04] [H16] |
| HP MFP 5800dn, 6QN29A datasheet | 1200×1200 | U / U | 80,000 | 6 / 6 GB | 32 GB eMMC across formatter/engine; opt ≥500 GB HDD or 256 GB SSD via CTO | G / opt / USB 2+3 | [H15] |
| Canon DX C3930i / C3935i, US hardware / India resolution | 1200×1200, half-speed in India specification | 1200 / 2300 | U | 3.5 GB | 256 GB SSD; opt 1 TB | G / opt / USB A+B | [C02] [C11] |
| Canon DX C5850i / C5860i, US | 1200×1200 | 1200 / 6350 | U | 5 GB | 256 GB SSD; opt 1 TB | G / standard b/g/n in cited US brochure / USB A+B | [C05] |
| Xerox AltaLink C8245 / C8255, high-capacity tandem configuration | 1200×2400 | 3140 / 6140 | 200,000 / 300,000 respectively | 4 GB; C8270 is 8 GB | ≥256 GB NVMe SSD; opt 1 TB | G / opt / USB | [X03] |
| Xerox VersaLink C625 | 1200×1200 | 650 / 2850 | 150,000 | 4 GB | 32 GB eMMC; opt ≥500 GB HDD | G / opt / USB | [X05] [X07] |
| Konica Minolta C300i / C360i | 1800-equivalent×600; 1200×1200 mode documented | 1150 / max U | U | 8 GB | 256 GB SSD; 1 TB option in lineup brochure | G / opt F / USB 2 | [K01] [K07] |
| Konica Minolta C301i / C361i | 1800-equivalent×600; 1200×1200 mode documented | U / U | U | 8 GB | 256 GB | Connectivity needs exact regional sheet | [K05] |
| Kyocera TASKalfa 3554ci, US | Up to 4800×1200 advertised | 1150 / 7150 | 125,000 | 4 GB | 32 GB SSD + 320 GB HDD std US; opt 1 TB HDD | G / standard short-range Wi-Fi in US sheet; opt adapter / USB | [Y01] |
| Brother MFC-L6915DW, US | 1200×1200 | 620 / 2700 with tower-tray option | 160,000 | 2 GB | Rear USB flash supports job storage; internal persistent storage U | G / standard dual-band / USB A+B | [B01] [B02] |
| Lexmark CX635adwe | 1200×1200; 4800 Color Quality also advertised | 251 / 2001 | 125,000 | 2 / 2 GB | Optional intelligent storage / magnetic HDD | G / standard / USB A+B | [L01] |
| Epson AM-C6000, Europe | 600×2400 | 1150 / 5150 | 384,000 | 8192 MB; maximum U | 320 GB AES-256 encrypted HDD listed; verify fitted option | G / opt / USB 2 A + USB 3.2 | [E01] |
| Sharp BP-70C36 | 1200×1200 | 650 base / 6300 EU range brochure | U | 5 GB EU | 256 GB EU | G F / WLAN / USB F | [A01] [A06] |
| Toshiba e-STUDIO3525AC, EU | 1200×1200 color; enhanced mono modes vary | 1200 / 5200 | U | 4 GB low-speed; 6 GB for higher-speed family members | 128 GB security SSD | G / opt / USB | [T02] [T04] |
| OKI C650 / ES6450 | 1200×1200 | U / 1940 at 80 gsm | U | 1 GB | Persistent user-storage capacity U | G / wireless listed in C650/ES6450 manual; verify dn/dnw option / USB | [O01] [O02] |
| Samsung SL-X4300LX | U | U / U | U | U | 320 GB HDD, 279 GB user-available | Exact regional connectivity U here | [G02] |
| Fujifilm Apeos C3570 / C4570 | 1200×2400 text/photo; 600×600 text/map | U / U | U | 4 / 4 GB | 128 GB storage device; medium type not established in retained extract | G F / opt F / USB F | [F01] |
| RISO GL9730, native controller | K 600×600; color fine 300×600; smoothing 600×600 | 2500 JP / max U | U | 4 GB | 512 GB SSD; ~370 GB usable | 2×G / U / USB 2+3 | [Z01] [Z03] |

The intentional U entries are a material limit of this research pass. The table is suitable for planning test coverage, not a completed purchasing database. In particular, HP variant-level capacity/storage, newer Ricoh Cxx11 details, and exact regional firmware transports need further documentation.

### Specification conflicts worth preserving

1. **Ricoh C3010/C3510:** the Canadian sheet says mainframe 4 GB + panel 4 GB and maximum 4700 input sheets; AP sources say 2 GB + 4 GB and 4850. Another US family brochure also lists 4850. Record source region/revision and interrogate the installed device; do not choose one as globally correct. [R01] [R05] [R18]
2. **Ricoh IM C320F:** Canadian sources and the US product page support 300/800 sheets, while the AP page has 250/1300 and simultaneously describes a 250-sheet tray plus 50-sheet bypass. This may mix counting conventions, configurations, or page errors; it is unresolved. [R15] [R16] [R20]
3. **Kyocera 3554ci:** US material includes a 320 GB HDD as standard; Asia material lists it as an option. The core SSD size also differs across speed classes. [Y01] [Y05]
4. **Toshiba 25AC family:** global and US documents differ in security/storage options. IPsec and certified storage must be matched to the regional controller and fitted options. [T01] [T04]

## Prioritized support shortlist

These ranks are an **engineering recommendation (I)**, not sales rankings. The deployment signal is official business positioning plus time in market/support continuity. Shared family names are a reuse hypothesis, to be tested before expanding compatibility.

| Rank | First physical test target | Why prioritize it / ranking signal | Next siblings to test |
|---:|---|---|---|
| 1 | Ricoh IM C3010 | Requested Ricoh priority; 2023 departmental platform with explicit interfaces/security evidence | C3510, C2510, Savin/Lanier equivalents |
| 2 | Ricoh IM C3000 | 2019 deployed-generation coverage, HDD and older controller/firmware behavior | C3500, C4500, C6000 |
| 3 | Ricoh IM 3500 | Mono A3 coverage rather than assuming color-platform equivalence | 2500, 5000, 6000 |
| 4 | HP LaserJet Enterprise M507dn | Established workgroup mono generation with an active firmware listing | M528dn, Managed E50145/E52645 |
| 5 | Canon DX C3930i | Documented departmental generation and explicit SNMP/Remote UI controls | C3935i, C3830i |
| 6 | Xerox AltaLink C8145 | 2020 enterprise generation; detailed shared administration documentation | C8155, B8145 |
| 7 | Konica Minolta bizhub C300i | A3 office i-Series, documented SNMP and OpenAPI administration | C360i, Develop ineo+ 300i |
| 8 | Kyocera TASKalfa 3554ci | Mainstream departmental segment; strong certificate and firmware security documentation | 2554ci, 4054ci, 5054ci |
| 9 | Brother MFC-L6915DW | Enterprise A4 mono target; inspect BRAdmin/MDT integration and USB storage | HL-L6415DW, MFC-L6810DW, regional 69xx variants |
| 10 | Lexmark CX635adwe | A4 color target with explicit eSCL documentation and current security controls | CX532adwe, MX632adwe |
| 11 | Epson AM-C6000 | Page-width inkjet coverage; documented enterprise management and network protocols | AM-C4000, AM-C5000, AM-M5500 |
| 12 | Ricoh IM C320F | A4 sGW/eMMC coverage and regional-specification edge cases | M C320FW and P C375 require separate profiles |
| 13 | Ricoh IM 3510 | Newly deployed 2026 mono generation; catch controller differences early | IM 4510, IM 6010, IM 7010 |
| 14 | Xerox AltaLink C8245 | 2024 refresh, NVMe SSD and newer security/firmware-policy behavior | C8255, B8255 |
| 15 | Canon imageFORCE C5150 | 2025 current-generation controller and certificate regression target | C5160/C5170; C3150 separately |
| 16 | Konica Minolta bizhub C301i | 2024 successor tests OpenAPI/firmware assumptions against earlier i-Series | C361i, C451i |
| 17 | Sharp BP-70C36 | 2022 departmental LED platform; official SRDM firmware management evidence | BP-50C31; newer BP-51C36/BP-71C36 separately |
| 18 | Toshiba e-STUDIO3525AC | Major A3 controller family with SCEP and exact TopAccess documentation | 4525AC, 5525AC, mono 3528A separately |
| 19 | Fujifilm Apeos C3570 | Regional enterprise breadth; prevents confusing Fuji Xerox/Fujifilm with Xerox firmware | C4570, C5570, Apeos C3530 separately |
| 20 | OKI C650dn | LED printer-only coverage, official MIB distribution and firmware updater | ES6450, A3 MC883 separately |

**Conditional additions:** prioritize Samsung SL-X4300LX if an actual inventory contains Samsung legacy devices; add RISO GL9730 for education/high-volume inkjet customers; add Pantum BM5100ADW for regions with meaningful Pantum deployments. Add Ricoh IM C3011 immediately for Japan-facing customers. These conditional choices should outrank the generic list when real inventory warrants it.

## Per-brand findings

### Ricoh, Savin, Lanier, Gestetner, Nashuatec, Rex-Rotary / NRG, and IKON provenance

**Model coverage:** start with IM C2000/C2500/C3000/C3500/C4500/C5500/C6000 (2019); IM C2010/C2510/C3010/C3510/C4510/C5510/C6010 (2023, regional availability differs); IM C3010SD/C4510SD/C6010SD (2025 JP); IM C2011/C2511/C3011/C3511/C4511/C5511/C6011 (Aug 2026 JP); IM 2500/3000/3500/4000/5000/6000 (earlier mono generation); IM 2510/3010/3510/4510/5510/6010/7010 (2026 regional). A4 coverage includes IM C300/C300F/C400F, IM C320F, IM 370F/460F. Treat older MP C3004ex/C3504ex/C4504ex/C5504ex/C6004ex as conditional legacy, not new 2020 models. The Ricoh updater applicability list still names these generations. [R03] [R04] [R06] [R12] [R13] [R21] [R22]

**Alias evidence:** the Japanese government certification for IM C2000–C3500 explicitly enumerates Ricoh, Savin, Lanier, Nashuatec, Rex Rotary and Gestetner names for the same evaluated product set. This supports a canonical Ricoh family with regional aliases. It does not establish every historical model mapping. Ricoh's history identifies IKON as an acquired sales company: an IKON service/dealer record alone should not determine the hardware plugin. Keep the presented badge, reported model, serial, controller identity, and canonical family separately. [R23] [R24]

**Monitoring and APIs:** Ricoh's developer site explicitly lists Standard MIB Specification, Private MIB Specification and SmartDeviceMonitor API. The Japanese program separately lists Common MIB, PMAPI, Private MIB and GJ MIB. Obtain the correct licensed revision and supported-model list through RiDP; no public, current, unauthenticated binary MIB download was verified. Do not substitute an old third-party MIB mirror. SmartSDK/WebAPI provides REST-based print/scan/copy/fax and authentication functions; verify administrative operation coverage through the appropriate management interface rather than assuming workflow REST covers everything. [R08] [R09] [R10]

**Network baseline:** Ricoh's IM C2010-era dual-interface article explicitly lists IPP, Bonjour/mDNS, HTTP/HTTPS and SNMPv1/v2/v3, with an optional controller WLAN board. Its scope is a particular direct-connection configuration, so use the device manual to confirm availability on each corporate interface. Neither current Printer MIB v2/Host Resources RFC conformance nor eSCL/IPP Everywhere certification was established for every model. Ricoh RC Gate can discover RFC 3805/1759 and RFC 2790 devices, but this is a **manager capability**, not evidence that every Ricoh agent implements every object. [R25] [R26]

**Security:** the 2023 security paper explicitly names Cxx10 models for TPM 2.0, describes firmware validation, storage encryption, admin authentication and unused-port restrictions, and identifies TLS 1.3 in the IM 2500–6000 generation onward. It distinguishes HDD-only overwrite behavior. Keep storage medium and sanitization method in the profile. Native SCEP, EST and ACME support for the shortlisted Ricoh models remains **U** in the checked evidence; certificate import is not equivalent to enrollment. [R11]

**Firmware:** the Canadian C3010/C3510 sheet documents automatic firmware updates, and Always Current Technology documents feature/security upgrades. The official Firmware Update Tool page confirms named model applicability, but its readable page returned an applicability shell rather than a full transport/API procedure. For a self-hosted plugin, obtain authenticated update documentation, package/region compatibility rules, service entitlements, component-version enumeration and recovery requirements before implementing uploads. Do not infer a public update endpoint from the existence of a Windows tool or @Remote. [R01] [R21] [R27]

**Special cases:** IM 460F is marketed as an A4 device yet supports A3 through the bypass and ADF; a size classification should retain those exceptions. M C320FW and P C375 share a launch grouping with IM C320F but are not evidence of identical SmartSDK capabilities. Japanese Cxx11 and SD variants require distinct firmware/model records. [R17] [R14] [R22]

### HP

**Key targets:** M507/M528; M554/M555/M578; M455/M480; M610/M611/M612 and M634/M635/M636; newer 5700/5800/6700/6800. Include Managed equivalents such as E50145/E52645 and X57945/X67755/X67765 where HP explicitly maps them. That mapping supports identification; it does not authorize cross-flashing. [H06] [H07]

**Management:** Embedded Web Server, Web Jetadmin and the OXP ecosystem are relevant; the public material used here does not establish an unrestricted third-party administrative REST contract. Web Jetadmin supports third-party Printer-MIB devices for baseline monitoring. HP's enterprise eSCL guidance confirms the capability at platform level; record the exact MFP/firmware and whether eSCL is enabled. [H08] [H09] [H10]

**Firmware/security:** FutureSmart bundles use `.bdl`; documented remote routes include EWS and Web Jetadmin's firmware repository. Compare exact model/product number, FutureSmart level, bundle/revision/date code and release prerequisites; query again after reboot. HP's maintained table shows why a single “latest version” string is insufficient. Certificate deployment through fleet software must be distinguished from device-native enrollment: HP WXP documents an **EST Connector** as a certificate-enrollment method, not proof that every printer implements a native EST client. [H11] [H07] [H12]

The M528dn's own product page explicitly documents signed-code checking, SureStart self-healing BIOS, allow-listing, storage encryption and secure erase. These are model-specific evidence, rather than capabilities inferred solely from the FutureSmart name. **Gaps:** obtain the current Jetdirect/enterprise MIB package, exact OXP administrative SDK terms and remaining per-SKU storage specifications. Do not copy these capabilities to LaserJet Pro, OfficeJet or legacy Samsung firmware. [H14]

### Canon

**Key families:** DX C3730i (2020), C3830i/C3835i and C5840i/C5850i/C5860i/C5870i (2021), C3926i/C3930i/C3935i and mono 4925i/4935i/4945i (2023). Current-generation coverage includes imageFORCE C5140/C5150/C5160/C5170 and 6100 series (2025), plus C3100/4100 series (2026). Canon explicitly says the latter replace DX C3900/4900 in the US. A4 candidates include DX C359iF/C259iF and business imageCLASS/i-SENSYS fleets, but the latter need their own protocol profile. [C01] [C04] [C07] [C09] [C10]

**Protocols:** the C3930i/C3935i guide documents SNMP; Canon's C5800 network menu explicitly distinguishes SNMP, RFC 2790 formatting, AirPrint, Mopria and IPP Everywhere. Do not treat SNMPv2c as confirmed just because v1/v3 are present. Remote UI and iW Management Console/Enterprise Management Console are administration tools; MEAP is an embedded application platform, not proof of an open fleet-configuration REST service. [C03] [C06] [C11]

**Security/certificates:** C3900 regional specifications document TLS 1.3, 802.1X, IPsec and encrypted SSD. The current imageFORCE C5100 manual explicitly documents key/CA/CRL management and SCEP settings. Canon's security white paper describes device-side SCEP; exact older-DX applicability should still be checked. EST/ACME remain U. [C11] [C08] [C12]

**Firmware:** Content Delivery System/eMaintenance appear in the product specification. Exact customer-accessible update procedure, model/firmware prerequisites and whether a service contract is needed remain to be established for each controller. An EWS admin page is not, by itself, an officially documented scripting API. [C11]

### Xerox

**Key families:** AltaLink C8130/C8135/C8145/C8155/C8170 and B8100 (2020); C8230/C8235/C8245/C8255/C8270 and B8200 (2024). A4 coverage: VersaLink C405/B405 deployed generation, newer C415/B415/C625/B625 and C620/B620. A3 VersaLink C7120/C7125/C7130 and B7100 require a separate profile from AltaLink; exact launch and management evidence for these secondary targets remains a gap. [X01] [X04] [X06]

**Standards:** AltaLink C8200 specifications explicitly name SNMPv1/v2c/v3, RFC 2790 and RFC 3805, mDNS, WSD and SLPv2. The shared B/C8100 and B/C8200 administrator guide explicitly documents eSCL, separate EIP scanning and clone/update functions. EIP is a vendor interface; full public administrative API scope was not verified. [X03] [X02]

**Firmware/configuration:** `.dlm` upgrades can be installed through EWS/Fleet Orchestrator; the guide also documents FTP-directory scheduling and USB. Verify software version/status afterward. Clone files can produce exceptions across software versions, hardware and models. The guide documents restricted downgrades. SCEP/EST/ACME were not found in the checked shared guide; absence from that document is recorded as U, not a universal negative. [X02]

Xerox's Lexmark acquisition does not justify one firmware adapter. Even similarly branded VersaLink and AltaLink generations need their own verified compatibility records. [M02]

### Konica Minolta, Develop, and other OEM badges

**Key families:** bizhub C250i/C300i/C360i and C450i/C550i/C650i; newer C251i/C301i/C361i/C451i/C551i/C651i/C751i (2024 JP); mono 301i/361i/451i/551i/651i/751i. A4 C3350i/C4050i and 4050i/4750i are useful follow-on targets, but should not inherit all A3 features. Official download-center identity documents pair C250i/C300i/C360i with Develop ineo+ 250i/300i/360i. Other Develop/Olivetti/Muratec mappings need explicit identity evidence before aliasing. [K04] [K08] [K09]

**Management:** Web Connection documents SNMPv1/v2c/v3, trap destinations and separate read/write users. The security paper explicitly describes **bizhub OpenAPI** acquiring/configuring device information over TLS, including network settings, address books, authentication information and meter counts. This is stronger administrative evidence than a generic workflow SDK claim. Obtain partner operation specifications/version compatibility before coding a general adapter. [K02] [K03]

**Certificates/updates:** Fleet RMM documentation distinguishes OpenAPI, HTTP and SNMP access checks. A firmware release note describes CSR additions for Windows NDES/SCEP integration with Fleet RMM. That is **manager-mediated certificate integration with firmware dependencies**, not blanket proof of native SCEP on every bizhub. CS Remote Care offers vendor remote management; exact unrestricted third-party firmware-update transport, SCEP/EST/ACME native support, and current MIB downloads remain U. [K06] [K10] [K11]

### Kyocera

**Key families:** TASKalfa 2554ci/3554ci/4054ci/5054ci/6054ci/7054ci and 5004i/6004i/7004i; newer MZ2501ci/MZ3501ci/MZ4001ci/MZ5001ci/MZ6001ci/MZ7001ci and MZ5001i/MZ6001i/MZ7001i (2025 JP). A4 targets include ECOSYS MA4000cix, MA4500ifx/MA5500ifx and PA-series printers. Regional MA/PA suffixes must be preserved. [Y02] [Y03] [Y05] [Y06] [Y07]

**Management/security:** Command Center RX is the embedded UI; Net Viewer, Device Manager and KFS are fleet tools. HyPAS is an application platform, not evidence of an unrestricted remote admin REST API. The 3554ci sheet documents SNMPv1/v2c/v3, WSD print/scan, AirPrint/Mopria, SCEP, TPM, secure boot, runtime checks, TLS 1.3 and encrypted storage. The MA4500ifx sheet explicitly names Printer MIB and Bonjour/WS-Discovery, but does not establish complete RFC 3805/RFC 2790 implementation on every TASKalfa. [Y01] [Y07] [Y08]

**Firmware:** the MZ family page says Net Viewer can monitor, maintain and update locally/remotely. Obtain the device-specific firmware upgrade guide and packages from the channel; transport/API remains unverified here. Be careful with similarly named tools: Kyocera's download page says its Software Upgrade Tool guide for IB-53/54/55 cannot update other devices. Native EST/ACME and exact private-MIB distribution remain U. [Y04] [Y09]

### Brother

**Key families:** mono HL-L6200DW/L6400DW and MFC-L6900DW earlier fleets; newer enterprise HL-L6415DW and MFC-L6915DW/L6810DW; color HL-L8360CDW/MFC-L8900CDW/MFC-L9570CDW earlier fleets and HL-L9470CDN/MFC-L9670CDN. Model-family prevalence is not quantified here; the latter's official downloads confirm current management-tool support. [B01] [B04] [B05]

**Protocols/configuration:** MFC-L6915DW documentation lists SNMPv1/v2c/v3, IPP/IPPS, mDNS, LLMNR, Web Services print/scan, TLS, IPsec and wired/wireless 802.1X. BRAdmin Professional and Web Based Management support administration; Mass Deployment Tool changes settings on multiple Brother network devices. No general device REST interface or exact current MIB-file download was verified. Earlier Brother documentation confirms SNMPv3-only management modes can affect older utilities. [B01] [B02] [B04] [B05]

**Firmware/certificates:** the official Firmware Update Tool obtains firmware from Brother's internet server and sends it to the printer. Do not assume an offline, redistributable firmware API or undocumented transport. Current download entries identify updater and firmware versions separately. The L6915DW page lists Secure BIOS, but detailed signed-boot/allow-list behavior, native SCEP/EST/ACME and secure-erasure guarantees remain U. [B01] [B04]

### Epson

**Key families:** WorkForce Enterprise AM-C4000/5000/6000; A4 AM-C400/550/550z; mono AM-M5500; flagship AM-C10000 (June 2026 US); earlier WF-C20600/WF-C20750/WF-C21000 and WorkForce Pro WF-C5290/C5790/C579R, WF-C5390/C5890. Prioritize enterprise AM plus one A4 ink-pack family; do not assume all EcoTank/SOHO models share enterprise administration. [E02] [E05] [E06] [E07]

**Management:** Web Config is the embedded admin UI; Epson Device Admin supports grouping and batch configuration. AM-C6000 specifications enumerate SNMPv1/v2c/v3, mDNS, SLP, WSD, LLMNR, IPP/IPPS and WSD Scan. The current guide explicitly lists Mopria Scan, providing a reason to investigate eSCL, but this report marks that inference separately. Epson Open Platform availability is not evidence of a general administrative REST API. Current enterprise MIB definitions and complete standard-MIB conformance remain U. [E01] [E03] [E04] [E05]

**Firmware/security:** the AM guide documents Web Config checking/downloading an update when the printer has internet access, scheduled automatic updates, and Epson Device Admin firmware management. CA certificate handling, IPsec/IP filtering and startup verification are documented. Recovery mode may require USB, so a remote update plugin needs a recovery/escalation path. Native SCEP/EST/ACME was not established. [E03] [E04]

### Lexmark

**Key families:** CX625/CX622 and MX622/MX521/MX522 earlier fleets; CX532adwe/CX635adwe and MX632adwe newer A4; MS/MX and CS/CX workgroup printers; CX930/CX931/CX942/CX943/CX944 A3 candidates need separate controller/OEM validation. Exact launch evidence for every secondary model remains U. [L01] [L02] [L04]

**Management:** Embedded Web Server and Markvision Enterprise (MVE) cover deployment/configuration; eSF is the embedded application ecosystem. The CX532/CX635 guide explicitly exposes eSCL scanning. The older CX625 guide separately documents SNMPv1/v2c and v3 controls. Lexmark's official MIB-pack article explicitly says Printer MIB version 1 for the products it discusses: do **not** relabel that as full RFC 3805 support on all newer devices. [L02] [L05] [L06]

**API and firmware distinctions:** Fleet Tracker has a documented REST API for querying collected inventory/history; it is a **server API**, not a printer firmware/configuration API. Lexmark Document Distributor's REST JSAPI is for document/profile jobs. MVE uses several proprietary device channels; the vendor port list names LST, NPAP and XMLNT alongside SNMP/HTTPS. Treat these as evidence of proprietary management, not a complete implementation specification. Exact CX635 firmware-upload/package rules and native enrollment require further confirmation. [L07] [L08] [L09]

### Sharp

**Key families:** BP-50C26/31/36/45/55/65 and BP-70C31/36/45/55/65 (2022 generation); newer BP-51C26/31/36/45 and BP-71C families (2025). Earlier MX-3071/3571/4071 and MX-2651/3051/3551/4051 remain useful compatibility candidates; Sharp's comparison chart identifies the predecessor relationships. Mono BP-50M/BP-70M and A4 BP-C/BP-B need separate models. [A02] [A03] [A04] [A05] [A07]

**Management/security:** BP-70C documentation lists SNMPv3, IPP/IPP over TLS, 802.1X, IPsec, TLS 1.3, user/admin web management and port/protocol controls. Sharp OSA is an application framework with optional modules on some BP-50 models, not proof of a universal administrative REST interface. Obtain the correct private MIB and OSA/admin SDK terms through Sharp. [A01] [A08]

**Firmware:** the BP-70C sheet explicitly documents network updates and one-to-many updates through Sharp Remote Device Manager. The new generation advertises boot/firmware recovery. Sharp's evaluated firmware archive warns that those files are not necessarily latest; match a security evaluation to its exact firmware/options. Native SCEP/EST/ACME remains U. [A01] [A04] [A09]

### Toshiba

**Key families:** e-STUDIO2525AC/3025AC/3525AC/4525AC/5525AC/6525AC; mono 2528A/3028A/3528A/4528A/5528A/6528A; higher-volume 6527AC/7527AC and 6529A/7529A/9029A. These are e-BRIDGE controller targets; verify older 2515AC/3515AC and 2508A-era devices separately when present. [T01] [T03] [T05] [T06]

**Management/security:** TopAccess provides remote administration. The global 25AC specification explicitly lists SNMPv3, 802.1X, SCEP, TPM 2.0, secure boot and self-encrypting SSD, with IPsec and certified HDD options varying by region. AirPrint/Mopria and WS Scan are documented, but exact eSCL and IPP Everywhere remain U. Current private MIB files, complete RFC 3805/2790 conformance and a general admin REST API were not verified. [T01] [T02] [T04]

**Firmware:** TopAccess exposes a component-level Current Software List including engine, NIC, scanner and accessory versions. Preserve the list rather than a single system string. The checked guide does not establish a general customer-accessible unattended firmware upload API; obtain service/update documentation. A card-reader firmware installation entry is not proof of a device-system firmware route. [T07]

### OKI

**Key targets:** C650dn/dnw and ES6450; A3 C824/C834/C844 and MC853/MC873/MC883 as regional/deployed candidates. The older A3 models may fall outside the time window and must not be represented as recent introductions. C650 is explicitly a 2021 LED model. [O01] [O02] [O04]

**Management:** C650/ES6450 specifications list SNMPv1/v3, IPP/IPPS, Bonjour, WSD, TLS, IPsec and 802.1X; SNMPv2c is not confirmed. The official SNMPv3 guide points to the supplied DVD's `Misc/MIB Files` for model MIB definitions. Configuration Tool discovers devices and changes/backs up settings, with explicit exceptions for passwords/network settings in the MC853 download description. No general administrative REST contract was verified. [O02] [O03] [O04]

**Firmware:** the official C650 support page offers a Firmware Update Tool, requires the device administrator password, and publishes a model-specific current firmware string. Exact network transport and offline package rights remain U. Certificate enrollment and storage erasure require model-specific evidence; do not extrapolate ETRIA ownership into Ricoh management compatibility. [O05] [M03] [M04]

### Legacy Samsung

**Key candidates:** ProXpress M4020/M4070/M4080FX; MultiXpress X4220RX/X4250LX/X4300LX and X7400/X7500/X7600; mono K4xxx/K7xxx. These are conditional legacy targets, mostly predating the requested window. Their installed prevalence was not measured. HP owns the acquired printer business, but firmware management remains SyncThru-generation behavior. [M06] [G01]

**Verified X4300 capabilities:** SyncThru Web Service documents status/supplies/counters, admin configuration, backup/restore/cloning, SNMPv1/v2/v3, IPP, WSD, SLP, mDNS, certificate management, IPsec, access controls and firmware/version functions. XOA application support is not evidence of a complete fleet administrative API. HP's separate support article explains firmware-version checking. Exact enterprise MIB download, native enrollment, current TLS algorithms, boot validation and support lifecycle remain U. Use exact packages and avoid treating the 2017 acquisition as a release date. [G01] [G02] [G03]

### Fujifilm / legacy Fuji Xerox

**Key families:** Apeos C2570/C3070/C3570/C4570/C5570/C6570/C7070; A4 Apeos C3530/C4030 and C325z/dw; retain legacy ApeosPort/DocuCentre identifiers where observed. US/India brochure dates establish regional documentation, not the first global launch. [F01] [F02] [F04]

**Management:** Internet Services displays status/history and changes settings; its SNMPv3 help and C325 audit guide confirm the existence of SNMPv3/security controls in those named families. Do not promote that into exact SNMP/MIB/eSCL support for every C3570 variant. Custom services/embedded integrations are distinct from an open admin API. Exact private-MIB access, firmware update process and enrollment matrix remain gaps. Fujifilm/Fuji Xerox identifiers must not automatically select Xerox AltaLink firmware. [F03] [F05] [F06]

### RISO and Pantum

**RISO:** GL9730/GL7430 are relevant high-volume A3 line-inkjet targets, with RISO Console and an optional scanner. Native Japanese specs explicitly list SNMPv1, IPP, HTTPS and IPsec; the English global specs list SNMP without a version. Therefore SNMPv3 remains U, not Y. An optional external controller may change exposed protocols and must be discovered as its own component. The administrator guide is the next source for detailed configuration; update transport, MIB distribution and certificate enrollment remain unverified. [Z01] [Z02] [Z03] [Z04] [Z05]

**Pantum:** BM5100ADW/BM5100FDN/BM5100FDW and BP5100-series are regional enterprise/workgroup candidates. Pantum's official product page positions BM5100 variants for 1000–10,000 recommended monthly pages, but this pass did not establish SNMPv3, standard-MIB conformance, a current admin API, enrollment or firmware automation. Keep them in discovery/qualification, not in a claimed management-support list. No model-level market-share ranking is asserted. [P01]

### Light-production coverage

These are additional controller/engine test candidates, not part of the office-first top 20:

| Family | Dated evidence and type | Fleet-management implication |
|---|---|---|
| Ricoh Pro C7500 | 2023; SRA3 color electrophotographic press, 85 ppm or 95 with upgrade license; 2400×4800 VCSEL in Canadian brochure | Browser remote operation and a Fiery-co-developed DFE are documented. Inventory the DFE and engine separately; do not assume office IM firmware/API parity. [Q01] [Q02] |
| Ricoh Pro C5400S / C5410S | May 2025 Japanese product listing; light-production color family | Current-generation qualification candidate. Exact SNMP/MIB, API, security, speed and update behavior remain U in this pass. [Q03] |
| Xerox PrimeLink C9265 / C9275 / C9281 | Announced Sep 2024; regional orders from Nov 2024; entry-production color family, up to 81 ppm | Xerox and Fiery server choices are explicitly documented. Keep controller-specific protocol and firmware records. [Q04] |

RISO GL9730/GL7430 and Epson AM-C10000, covered above, also exercise high-volume line-inkjet monitoring. Light-production popularity and enterprise-MFP protocol equivalence are not established by these launch sources.

## Standards-based core versus vendor plugins

| Capability | Reusable core | What still varies / needs a plugin | Standard or evidence |
|---|---|---|---|
| Identity and reachability | DNS/IP inventory, SNMP system group, IPP attributes, advertised services | Badge aliases, controller identity, multiple interfaces and external DFEs | [S01] [S02] [S03] |
| Consumables, trays, basic counters and alerts | Printer MIB groups plus Host Resources printer state | Missing objects, vendor counters, units, color/duplex/A3 billing semantics, waste containers | [S01] [S02] |
| SNMP security | v3 USM credentials, engine discovery, auth/privacy handling | Supported hashes/ciphers, read/write scopes, traps, firmware-dependent choices | [S05] [K02] [L05] |
| Capabilities and state | IPP Get-Printer-Attributes; store supported operations/versions | Optional extensions; administrator policy and firmware are not universally standardized | [S03] [S04] |
| Driverless scanning | Optional eSCL capability/status adapter | Scanner absent, policy disabled, authentication, firmware and endpoint differences | [S07] [S08] [X02] [L02] |
| Discovery | mDNS/DNS-SD and WS-Discovery adapters; directed SNMP polling | VLAN boundaries, disabled multicast, SLP/LLMNR availability and separate interface visibility | [S09] [S10] [R25] [E01] |
| Configuration | Selected documented writable MIB objects, where supported and authorized | Address books, users, certificates, security policy, clone formats, admin SDKs | [R08] [K03] [X02] |
| Firmware inventory | Normalize model and component-version observations | Version semantics, component lists, region, boot prerequisites, package signing and transport | [H07] [H11] [T07] |
| Firmware installation | Shared orchestration, scheduling, audit and verification | Entire device transfer/install/recovery contract | [X02] [E03] [O05] |
| Certificate lifecycle | Expiry inventory, trust-chain handling, renewal jobs | Native SCEP vs manager-mediated enrollment vs manual import; firmware-specific APIs | [S11] [S12] [S13] [H12] [K06] |
| Storage sanitization | Inventory medium/options; record approved operation/result | HDD overwrite vs SSD cryptographic erase, scope and prerequisites | [R11] [T04] |

**Universal means reusable logic, not guaranteed availability on every device.** A firewalled or policy-disabled endpoint must not be reported as an unsupported protocol merely because it timed out. A printer can implement older RFC 1759-style Printer MIB objects without proving full RFC 3805 conformance. Keep evidence and observed behavior separate.

### Useful standard objects

Use these as table/group roots, not a hardcoded assumption that every device uses index `1`:

| Object / group | Numeric OID | Use / qualification |
|---|---|---|
| SNMP system group | `1.3.6.1.2.1.1` | sysDescr, sysObjectID, sysName and uptime; vendor identity is evidence, not a firmware selector |
| Host Resources devices | `1.3.6.1.2.1.25.3` | Device table and printer status/error state |
| Printer general | `1.3.6.1.2.1.43.5` | General printer information and serial-related fields where implemented |
| Printer input | `1.3.6.1.2.1.43.8` | Tray descriptions and levels |
| Printer markers | `1.3.6.1.2.1.43.10` | Marker life count and counter units |
| Printer supplies | `1.3.6.1.2.1.43.11` | Supply type, unit, maximum and current levels |
| Printer alerts | `1.3.6.1.2.1.43.18` | Alert severity/code/description; vendor errors may need translation |

These standard roots come from Printer MIB and Host Resources definitions. Preserve raw units, sentinel values and device/table indices; do not render a negative sentinel as a toner percentage or assume a marker counter equals billable printed sheets. [S01] [S02]

For IPP, retain `printer-make-and-model`, `printer-uuid`, `printer-state`, `printer-state-reasons`, `ipp-versions-supported` and `operations-supported` when returned. IPP is valuable even for a device-management platform that does not submit print jobs. [S03]

### Relevant standards and programs

- **RFC 3805 / Printer MIB v2**, replacing RFC 1759; **RFC 2790 / Host Resources MIB**, replacing RFC 1514. [S01] [S02]
- **RFC 8011 / IPP model and semantics**; PWG **IPP Everywhere** profile and certification are distinct from basic IPP. [S03] [S04] [S06]
- **SNMPv3 USM, RFC 3414**; cipher/hash availability must still be checked on the device. [S05]
- **mDNS RFC 6762** and **DNS-SD RFC 6763**; Bonjour is the associated vendor implementation/branding. [S09] [S10]
- **eSCL / Mopria Scan**: Mopria publishes the specification under its download/license process; Google’s scanner specification also requires eSCL. This is separate from Mopria Print and the AirPrint printing program. [S07] [S08] [S14]
- **SCEP RFC 8894**, **EST RFC 7030**, **ACME RFC 8555**: distinct certificate-enrollment protocols; support for one does not imply another. [S11] [S12] [S13]
- **IEEE 802.1X**, IPsec, TLS/HTTPS and X.509 certificate handling appear in many vendor profiles, but presence, algorithms and remote configurability are model dependent. **WS-Discovery/WSD**, **SLP** and **LLMNR** should also be separate discovery flags; a generic “network discovery supported” field loses useful information. [Y01] [B02] [E01]

## Implications for the fleet platform

The following are **design recommendations inferred from the research**, not changes made to the application:

1. Maintain a capability record per **canonical family + regional SKU + controller + firmware range + installed options**. Retain the reported badge/model. Alias matching must never determine firmware compatibility on its own.
2. Store `documented`, `observed`, `enabled`, `authorized`, `unsupported` and `unknown` separately. Attach source URL, source date, region, manual revision and observation date to each capability. Unknown must not become false during import.
3. Start with read-only SNMP/IPP discovery and TLS inventory. Add vendor counters and alert dictionaries after comparing them with configuration reports and known physical device states.
4. Expose configuration coverage by operation: identity/location, network, protocols, address book, authentication, certificate import/enrollment, security policy, firmware, erase. A single `has_api=true` is insufficient.
5. Implement one shared update state machine, with vendor-specific transfer/install/version adapters. Preserve component versions, package identity, prerequisites, reboot state and final verification. A successful upload is not a successful update.
6. Identify external print controllers/Fiery DFEs as separate components. Their version, IP, management API, scan behavior and security policy can differ from the MFP engine. Do not merge them merely because they share a display or a model brochure.
7. Use actual discovery samples to revise the shortlist. Anonymous family counts by customer/region would be much stronger evidence than shipments, reseller popularity claims or a list of currently sold devices.

## Verification backlog and research limits

| Priority | Unresolved item | Concrete evidence needed |
|---|---|---|
| 1 | Actual most-common models | Representative customer inventory counts with geography, age and sample bias; no public model-level installed-base census was found |
| 1 | Ricoh standard/private MIB and management APIs | Current licensed RiDP MIB files, PMAPI/management operation specs, supported-model/firmware matrix and redistribution terms |
| 1 | Exact writable configuration coverage | Per-operation API documentation and controlled test-device results; no endpoint reverse engineering is represented as official support |
| 1 | Firmware automation | Official package sources, region/serial/product-code prerequisites, signing/version checks, transfer protocol, service entitlement, recovery and downgrade rules |
| 1 | Ricoh 2026 Cxx11 / IM xx10 | Exact regional administrator/security guides and controller differences; new-model launches alone do not establish protocol parity |
| 2 | Printer MIB v2 / Host Resources conformance | Per-model official conformance statement plus recorded table/object coverage; manager interoperability is insufficient |
| 2 | IPP Everywhere | Exact model/firmware registry entry or manual statement; PWG registry model rows were unavailable to the browser |
| 2 | eSCL | Exact model manual or authenticated read-only capability response; AirPrint/Mopria Print logos alone are insufficient |
| 2 | Certificate enrollment | Native vs manager-mediated SCEP/EST/ACME, key generation, SAN/EKU support, renewal behavior, certificate stores and firmware requirements |
| 2 | Hardware gaps | Variant-level RAM/max RAM, native resolution, input capacity, storage medium and true maximum monthly duty cycle for U cells |
| 2 | Release / current / lifecycle dates | Original regional launch plus current official sale/support statements; indexed revision dates are not launch evidence |
| 3 | Legacy/OEM aliases | Exact identity declarations and management evidence for Olivetti, Develop, Muratec, Sindoh, Samsung and regional OEM variants |

Several PDFs were available only through indexed extracts or failed direct retrieval. In particular, direct HP datasheet requests timed out, a historical Brother brochure URL returned 404 despite indexed text, the Ricoh NIAP PDF could not be fetched, the Ricoh Essentials Security Guide exceeded the browser size limit, and a Ricoh HTML manual link failed. Claims based only on broader documentation are marked F/P/U. Those inaccessible documents are **not** treated as fully inspected sources.

## Sources actually used

All links below are primary manufacturer/program/standards sources, except IDC (the market-research source) and IPA/JISEC (the government certification authority). “Indexed” means the relevant text was visible in official-source search extracts; it does not claim full-document inspection. “Read” means page/PDF text was successfully retrieved. Access date for this list is 28 October 2026. Dynamic documents can change after access.

### Market, ownership, and standards

- **M01** — [IDC hardcopy peripheral market insights][M01] — read/indexed; shipment context only.
- **M02** — [Xerox completes Lexmark acquisition, 1 July 2025][M02] — indexed.
- **M03** — [ETRIA / OKI integration announcement][M03] — indexed; planned transaction and engine scope.
- **M04** — [Ricoh completion notice, 1 October 2025][M04] — indexed; completion rather than announcement.
- **M05** — [Toshiba Tec FY2024 results][M05] — indexed; brand controllers/common engines diagram.
- **M06** — [HP completes Samsung printer-business acquisition][M06] — indexed; 1 November 2017.
- **S01** — [RFC 3805, Printer MIB v2][S01] — read.
- **S02** — [RFC 2790, Host Resources MIB][S02] — read.
- **S03** — [RFC 8011, IPP model and semantics][S03] — read.
- **S04** — [PWG IPP Everywhere overview][S04] — read.
- **S05** — [RFC 3414, SNMPv3 USM][S05] — read.
- **S06** — [PWG IPP Everywhere printer registry][S06] — read shell only; model results unavailable.
- **S07** — [Mopria eSCL specification download][S07] — indexed.
- **S08** — [Google ChromeOS scanner specification][S08] — indexed; eSCL requirement.
- **S09** — [RFC 6762, mDNS][S09] — read.
- **S10** — [RFC 6763, DNS-SD][S10] — read.
- **S11** — [RFC 8894, SCEP][S11] — read.
- **S12** — [RFC 7030, EST][S12] — read.
- **S13** — [RFC 8555, ACME][S13] — read.
- **S14** — [Apple AirPrint developer overview][S14] — indexed; printing scope.

### Ricoh

- **R01** — [IM C3010/C3510 Canadian specification PDF][R01] — read; hardware and automatic updates.
- **R02** — [IM C3000/C3500 specification PDF][R02] — indexed; older US-hosted sheet.
- **R03** — [Ricoh Australia 2019 IM C launch][R03] — read/indexed.
- **R04** — [Ricoh AP 2023 IM C launch][R04] — indexed.
- **R05** — [US IM C series brochure PDF][R05] — indexed; regional members and specs.
- **R06** — [Japan IM Cxx11 launch, 17 August 2026][R06] — indexed; sale date 24 August.
- **R07** — [IM 3500 AP product/specification page][R07] — indexed.
- **R08** — [RiDP device interfaces][R08] — read; standard/private MIB and SmartDeviceMonitor API.
- **R09** — [Ricoh Japan developer-program contact/interface list][R09] — indexed; PMAPI/Common/Private/GJ MIB.
- **R10** — [Ricoh SmartSDK overview][R10] — indexed; REST workflow/API scope.
- **R11** — [2023 MFP/printer security white paper PDF][R11] — read; TPM, encryption, TLS, port controls.
- **R12** — [Japan new mono IM launch][R12] — indexed; January 2026 availability.
- **R13** — [AP mono IM xx10 launch, April 2026][R13] — indexed.
- **R14** — [AP IM C320F / M C320FW / P C375 launch][R14] — indexed.
- **R15** — [IM C320F Canadian specification PDF][R15] — indexed.
- **R16** — [IM C320F US product/admin overview][R16] — read.
- **R17** — [IM 460F AP specification page][R17] — read.
- **R18** — [IM C3010 AP specification page][R18] — read/indexed.
- **R19** — [Ricoh intelligent-device range brochure PDF][R19] — indexed; differing regional capacity.
- **R20** — [IM C320F AP specification page][R20] — indexed; unresolved capacity discrepancy.
- **R21** — [Firmware Update Tool model applicability][R21] — read shell/applicability; transport not available.
- **R22** — [Japan IM C SD variants launch][R22] — indexed.
- **R23** — [IPA/JISEC Ricoh and regional-badge certification][R23] — indexed; exact named alias set.
- **R24** — [Ricoh history / acquired sales organizations PDF][R24] — indexed.
- **R25** — [IM C2010-era simultaneous network-interface configuration][R25] — indexed; explicit protocol list and WLAN requirement.
- **R26** — [RC Gate discovery MIB requirements][R26] — indexed; management-software scope.
- **R27** — [Always Current Technology deployment/update explanation][R27] — indexed.

### HP and Canon

- **H01** — [HP M507dn specifications][H01] — indexed.
- **H02** — [M507 printer-specification guide][H02] — indexed; memory/accessories.
- **H03** — [M528 series datasheet][H03] — indexed; direct retrieval timeout.
- **H04** — [M555 series December 2020 datasheet][H04] — indexed; direct retrieval timeout.
- **H05** — [5800dn datasheet][H05] — indexed; direct retrieval timeout.
- **H06** — [Enterprise/Managed model equivalents][H06] — indexed.
- **H07** — [FutureSmart version/model table][H07] — indexed; dynamic, not a pinned update recommendation.
- **H08** — [Web Jetadmin third-party Printer-MIB support][H08] — indexed.
- **H09** — [HP enterprise eSCL configuration guidance][H09] — indexed; platform scope.
- **H10** — [Web Jetadmin user guide PDF][H10] — indexed.
- **H11** — [HP Enterprise/Managed firmware-update procedure][H11] — indexed; `.bdl`, EWS/WJA.
- **H12** — [HP WXP certificate policies / EST Connector][H12] — indexed; manager scope.
- **H13** — [HP UK M507dn store specifications][H13] — indexed; 650/2300 input capacity.
- **H14** — [HP US M528dn specifications and device security][H14] — read.
- **H15** — [HP 5800dn 6QN29A detailed datasheet][H15] — indexed; direct retrieval failed.
- **H16** — [HP EU M555 series datasheet][H16] — indexed; variant capacities and wireless options.
- **C01** — [Canon DX C3900 and other 2023 launches][C01] — indexed.
- **C02** — [Canon DX C3900 brochure PDF][C02] — indexed.
- **C03** — [C3935i/C3930i/C3926i SNMP manual][C03] — indexed.
- **C04** — [DX C5800 / 6800 2021 launch][C04] — indexed.
- **C05** — [DX C5800 brochure PDF][C05] — read.
- **C06** — [DX C5800 network menu / IPP Everywhere / RFC 2790][C06] — indexed.
- **C07** — [imageFORCE C5100 / 6100 launch, July 2025][C07] — indexed.
- **C08** — [imageFORCE C5100 certificate/import/export manual][C08] — indexed.
- **C09** — [DX platform launch, March 2020][C09] — indexed.
- **C10** — [imageFORCE C3100 / 4100 launch, June 2026][C10] — indexed; US replacement relationships.
- **C11** — [DX C3900 regional detailed specifications][C11] — read; security and vendor management tools.
- **C12** — [Canon zero-trust security white paper PDF][C12] — indexed; SCEP and certificate handling.

### Xerox, Konica Minolta, and Kyocera

- **X01** — [AltaLink 8100 launch, July 2020][X01] — indexed.
- **X02** — [B/C8100 and B/C8200 administrator guide PDF, July 2025][X02] — read; SNMP, eSCL, cloning and firmware.
- **X03** — [AltaLink C8200 detailed specification PDF][X03] — read; explicit RFC MIBs and protocols.
- **X04** — [AltaLink 8200 launch, July 2024][X04] — indexed.
- **X05** — [VersaLink C625/C620 brochure PDF][X05] — indexed.
- **X06** — [Xerox secure-installation guide index][X06] — indexed; exact-series guide availability.
- **X07** — [VersaLink C625 detailed specifications, Spanish PDF][X07] — indexed; 650/2850 capacity.
- **K01** — [C360i/C300i/C250i machine specification manual][K01] — read.
- **K02** — [C360i/C300i/C250i SNMP administration manual][K02] — indexed.
- **K03** — [Konica Minolta security technical support paper][K03] — indexed; OpenAPI administration scope.
- **K04** — [Japan 2024 bizhub 1i launch][K04] — indexed.
- **K05** — [C361i/C301i/C251i specification manual][K05] — indexed.
- **K06** — [C360i/C361i firmware release information PDF][K06] — indexed; Fleet RMM/NDES conditions.
- **K07** — [Konica Minolta office lineup brochure PDF][K07] — indexed; SSD option.
- **K08** — [C250i download center / Develop identity declaration][K08] — indexed.
- **K09** — [OAuth firmware model/version applicability][K09] — indexed; secondary model families.
- **K10** — [Fleet RMM user guide PDF][K10] — indexed; communication checks.
- **K11** — [Konica Minolta security paper / CS Remote Care][K11] — same document as K03, cited for remote-management scope.
- **Y01** — [TASKalfa 3554ci US specification PDF][Y01] — read.
- **Y02** — [TASKalfa 2554ci January 2021 datasheet PDF][Y02] — indexed.
- **Y03** — [Japan TASKalfa MZ launch schedule][Y03] — indexed; 2025 availability.
- **Y04** — [MZ family reliability/security and Net Viewer][Y04] — indexed.
- **Y05** — [TASKalfa family Asia brochure PDF][Y05] — indexed; regional storage/options.
- **Y06** — [ECOSYS MA4000cix product page][Y06] — indexed.
- **Y07** — [ECOSYS MA4500ifx specification PDF][Y07] — indexed; Printer MIB/WS-Discovery.
- **Y08** — [TASKalfa Command Center RX guide PDF][Y08] — indexed.
- **Y09** — [Kyocera downloads / scope restriction for IB upgrade utility][Y09] — indexed.

### Brother, Epson, and Lexmark

- **B01** — [MFC-L6915DW product/specification page][B01] — read.
- **B02** — [MFC-L6915DW family protocol/security specifications][B02] — read.
- **B03** — [Historical November 2023 L6915DW brochure][B03] — indexed only; direct URL returned 404, not a fully inspected PDF.
- **B04** — [MFC-L9670CDN firmware and management-tool downloads][B04] — indexed.
- **B05** — [Brother secure SNMP modes / older utility compatibility][B05] — indexed.
- **E01** — [AM-C6000 European detailed specifications][E01] — read.
- **E02** — [US AM-C4000/C5000/C6000 launch][E02] — indexed.
- **E03** — [AM series administrator/user guide PDF][E03] — indexed; SNMP and firmware procedure.
- **E04** — [AM series US user guide PDF][E04] — indexed; security and Web Config firmware.
- **E05** — [Epson Device Admin and supported devices][E05] — indexed.
- **E06** — [US AM-C10000 launch, June 2026][E06] — indexed.
- **E07** — [WF-C5890 product specifications][E07] — indexed.
- **E08** — [EU AM series launch, November 2022][E08] — indexed.
- **L01** — [CX635adwe brochure PDF, Spanish][L01] — read.
- **L02** — [CX532/CX635 user guide PDF][L02] — indexed; explicit eSCL.
- **L03** — [CX635/XC2342 declaration, original issue April 2023][L03] — indexed; documentation date, not launch proof.
- **L04** — [MX622adhe product specifications][L04] — indexed.
- **L05** — [CX625-family SNMP settings][L05] — indexed.
- **L06** — [Lexmark MIB-pack support article][L06] — indexed; older product scope and MIB v1 caveat.
- **L07** — [Fleet Tracker server Open API][L07] — indexed.
- **L08** — [Document Distributor REST/SOAP job API][L08] — indexed; jobs, not device administration.
- **L09** — [Lexmark management protocol/port list][L09] — indexed.

### Sharp, Toshiba, OKI, Samsung, and additional manufacturers

- **A01** — [BP-70C31/36/45 specification PDF][A01] — indexed; LED, protocols and firmware management.
- **A02** — [Sharp April 2022 workgroup-model launch][A02] — indexed.
- **A03** — [BP-51C26/31/36/45 brochure PDF][A03] — read.
- **A04** — [Sharp new BP-series launch, October 2025][A04] — read.
- **A05** — [Sharp 2025 A3 lineup PDF][A05] — indexed.
- **A06** — [Sharp EU range brochure PDF][A06] — indexed; memory/storage/capacity.
- **A07** — [Sharp old/new model comparison PDF][A07] — indexed.
- **A08** — [OSA options on BP-50/70 family][A08] — indexed.
- **A09** — [Sharp evaluated guidance/firmware archive][A09] — indexed; not necessarily latest firmware.
- **T01** — [Toshiba global 25AC product/security specifications][T01] — indexed.
- **T02** — [US 25AC specification PDF, June 2026 revision][T02] — indexed.
- **T03** — [Toshiba quick-start guide, October 2022 PDF][T03] — indexed; family existence by 2022.
- **T04** — [EU 25AC brochure PDF][T04] — indexed.
- **T05** — [e-STUDIO6527AC/7527AC specification PDF][T05] — indexed.
- **T06** — [e-STUDIO6529A/7529A/9029A brochure PDF][T06] — indexed.
- **T07** — [TopAccess guide / Current Software List PDF][T07] — indexed.
- **O01** — [OKI C650 launch, March 2021][O01] — indexed.
- **O02** — [C650/ES6450 specifications manual][O02] — read.
- **O03** — [OKI SNMPv3 and supplied MIB-file location][O03] — indexed.
- **O04** — [MC853 official Configuration Tool/download information][O04] — indexed.
- **O05** — [C650dn official firmware/updater page][O05] — indexed.
- **G01** — [Samsung X4300 SyncThru administration manual, HP-hosted][G01] — indexed, extensive official extract.
- **G02** — [HP Samsung SL-X4300LX specifications][G02] — indexed.
- **G03** — [HP Samsung firmware-version checking][G03] — indexed.
- **F01** — [Fujifilm Apeos C5570/4570/3570/3070 brochure PDF][F01] — indexed.
- **F02** — [Fujifilm US Apeos C7070-family brochure PDF][F02] — indexed.
- **F03** — [Fujifilm Internet Services SNMPv3 help][F03] — indexed; Apeos 5570 help scope.
- **F04** — [Apeos C4030/C3530 brochure PDF][F04] — indexed.
- **F05** — [Fujifilm Internet Services setup explanation][F05] — indexed; C3567-family scope.
- **F06** — [Apeos C325 audit-log guide PDF][F06] — indexed.
- **Z01** — [RISO global GL specifications][Z01] — indexed.
- **Z02** — [RISO GL Japanese launch announcement][Z02] — indexed.
- **Z03** — [RISO GL9730 Japanese native specifications][Z03] — indexed.
- **Z04** — [RISO corporate product history][Z04] — indexed; ComColor launch timing.
- **Z05** — [ComColor GL administrator guide PDF][Z05] — indexed.
- **P01** — [Pantum BM5100 family official product page][P01] — indexed; business positioning only.
- **Q01** — [Ricoh Pro C7500 US/Canada 2023 launch][Q01] — indexed; DFE and browser remote operation.
- **Q02** — [Ricoh Pro C7500 Canadian brochure PDF][Q02] — indexed; speed/resolution.
- **Q03** — [Ricoh production-product announcement archive][Q03] — indexed; Pro C5400S/C5410S listing in May 2025.
- **Q04** — [Xerox PrimeLink C9200 2024 launch][Q04] — indexed; specific models and controller options.

[M01]: https://www.idc.com/promo/hardcopy-peripherals/
[M02]: https://www.news.xerox.com/news/xerox-completes-the-acquisition-of-lexmark-uniting-two-industry-leaders
[M03]: https://www.oki.com/global/press/2025/z24062e.html
[M04]: https://jp.ricoh.com/release/2025/1001_1
[M05]: https://www.toshibatec.com/file/FY2024_Consolidated_Business_Results.pdf
[M06]: https://www.hp.com/us-en/newsroom/press-releases/2017/hp-completes-acquisition-of-samsung-electronics-co---ltd--printe.html
[S01]: https://www.rfc-editor.org/rfc/rfc3805
[S02]: https://www.rfc-editor.org/rfc/rfc2790
[S03]: https://www.rfc-editor.org/rfc/rfc8011
[S04]: https://www.pwg.org/ipp/everywhere.html
[S05]: https://www.rfc-editor.org/rfc/rfc3414
[S06]: https://www.pwg.org/printers/
[S07]: https://mopria.org/spec-download
[S08]: https://developers.google.com/chromeos/peripherals/cc-scanner-v1
[S09]: https://www.rfc-editor.org/rfc/rfc6762
[S10]: https://www.rfc-editor.org/rfc/rfc6763
[S11]: https://www.rfc-editor.org/rfc/rfc8894
[S12]: https://www.rfc-editor.org/rfc/rfc7030
[S13]: https://www.rfc-editor.org/rfc/rfc8555
[S14]: https://developer.apple.com/airprint/
[R01]: https://assets.ricoh-usa.com/j2jqn9lauv41/27FUfh82HOFixuKRrcJuVW/822d883181af52962e53fe6c08be7fcc/IM_C3010_C3510_Spec_Sheet_RCA_EN.pdf
[R02]: https://assets.ricoh-usa.com/j2jqn9lauv41/6MrL4kDitHAhx5l3ejEkcW/f5f72732903876fed596025d46b6f3ab/im-c3000-c3500-spec-sheet.pdf
[R03]: https://www.ricoh.com.au/news/ricoh-unveils-new-intelligent-devices
[R04]: https://www.ricoh-ap.com/news/2023/05/09/ricoh-introduces-new-im-c-series-a3-multifunction-printers
[R05]: https://assets.ricoh-usa.com/j2jqn9lauv41/1zi0tYuWi9f18Oz7SEKVY3/960afe1aeb53bafdb38a2df058041a7b/IM_C_Series_Brochure_RUSA.pdf
[R06]: https://jp.ricoh.com/release/2026/0817_1
[R07]: https://www.ricoh-ap.com/products/office-printing/mfp-black-and-white/im-3500
[R08]: https://ricoh-ridp.com/ricoh-device-interfaces
[R09]: https://www.mlsvc.ricoh.co.jp/regist/is?SMPFORM=lap-sik-87c49836449aea2ce532de713da3aafa
[R10]: https://emea.ricoh-developer.com/about-us/membership/smart-membership-benefits/smartsdk-overview
[R11]: https://www.ricoh.com/-/Media/Ricoh/Sites/com/security/pdf/mfp-security-whitepaper.pdf?230929=
[R12]: https://jp.ricoh.com/release/2025/1222_1
[R13]: https://www.ricoh-ap.com/news/2026/04/24/ricoh-introduces-new-im-xx10-series-a3-black-and-white-multifunction-printers
[R14]: https://www.ricoh-ap.com/news/2024/09/21/ricoh-unveils-new-a4-colour-laser-mfps
[R15]: https://assets.ricoh-usa.com/j2jqn9lauv41/5Yd6DbCJFznG4sSCFe51Lz/445895aa951d0aeb8c3a038a0d489f45/IMC320F_v3_EN_CA.pdf
[R16]: https://www.ricoh-usa.com/en/products/pd/equipment/printers-and-copiers/multifunction-printers-copiers/im-c320f-color-laser-multifunction-printer
[R17]: https://www.ricoh-ap.com/products/office-printing/mfp-black-and-white/im-460f
[R18]: https://www.ricoh-ap.com/products/office-printing/mfp-colour/im-c3010
[R19]: https://www.ricoh-ap.com/en/products/office-printing/mfp-colour/-/media/ra/files/pdfs/products_brochure/office_solutions/printers_and_copiers/range_brochure/ricoh_intelligent_devices_brochure.pdf
[R20]: https://www.ricoh-ap.com/products/office-printing/mfp-colour/im-c320f
[R21]: https://kb.gsd.ricoh.com/app/answers/ans_product/a_id/293886
[R22]: https://jp.ricoh.com/release/2025/0203_1
[R23]: https://www.ipa.go.jp/en/security/jisec/software/certified-cert/c0638_it8682.html
[R24]: https://www.ricoh.com/-/Media/Ricoh/Sites/com/IR/financial_data/annual_report/ar/10/ar12.pdf
[R25]: https://kb.gsd.ricoh.com/app/answers/detail/a_id/289061/kw/install%20finisher
[R26]: https://kb.gsd.ricoh.com/app/answers/detail/a_id/225873/~/how-to-configure-auto-discovery-using-network-search-and-broadcast
[R27]: https://www.ricoh.com.my/dynamic-workplace-intelligence/ricoh-always-current-technology/ract-learn-more
[H01]: https://support.hp.com/us-en/product/product-specs/hp/19203856
[H02]: https://support.hp.com/emea_africa-en/document/c06348079
[H03]: https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=c08015961
[H04]: https://h20195.www2.hp.com/v2/getpdf.aspx/4aa7-8159enuc.pdf
[H05]: https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=c08623799
[H06]: https://support.hp.com/be-fr/document/ish_13770559-13697423-16
[H07]: https://support.hp.com/sg-en/document/ish_4025423-4025481-16
[H08]: https://support.hp.com/us-en/document/ish_3007439-2783223-16
[H09]: https://support.hp.com/ro-en/document/c04945509
[H10]: https://kaas.hpcloud.hp.com/pdf-public/pdf_3017438_en-US-1.pdf
[H11]: https://support.hp.com/au-en/document/ish_4158468-4158516-16
[H12]: https://learn.workforceexperience.hp.com/docs/installing-and-managing-certificates-using-printer-policies
[H13]: https://www.hp.com/gb-en/shop/products/printers/hp-laserjet-enterprise-m507dn-1pv87a-b19
[H14]: https://www.hp.com/us-en/shop/pdp/hp-laserjet-enterprise-mfp-m528dn
[H15]: https://h20195.www2.hp.com/v2/GetPDF.aspx/c08487949
[H16]: https://h20195.www2.hp.com/v2/getpdf.aspx/4AA7-7948EEP.pdf
[C01]: https://www.usa.canon.com/newsroom/2023/20230523-printer
[C02]: https://www.usa.canon.com/bin/canon/downloadasset?path=%2Fcontent%2Fdam%2Fcanon-assets%2Fbrochures%2Fbusiness%2Fcopiers-and-mfp%2Fimagerunner-advance-dx%2FEnterprise_iR-ADV-DX-C3900-Brochure.pdf
[C03]: https://oip.manual.canon/USRMA-8181-zz-CS-3900-enUS/contents/devu-setup-nw-pref_fit-snmp_moni_control.html
[C04]: https://www.usa.canon.com/newsroom/2021/20210601-products
[C05]: https://www.usa.canon.com/bin/canon/downloadasset?path=%2Fcontent%2Fdam%2Fcanon-assets%2Fbrochures%2Fbusiness%2Fcopiers-and-mfp%2Fimagerunner-advance-dx%2FEnterprise_iR-ADV-DX-5800-Series_Brochure.pdf
[C06]: https://oip.manual.canon/USRMA-5488-zz-CS-5800-enUS/contents/devu-set_reg-pref-nw.html
[C07]: https://www.usa.canon.com/newsroom/2025/20250722-printer
[C08]: https://oip.manual.canon/USRMA-9286-zz-CS-5100-enUS/contents/devu-set_menu_list-impt_expt-mng-dev-certificate.html
[C09]: https://www.usa.canon.com/newsroom/2020/20200310-iradv-dx
[C10]: https://www.usa.canon.com/newsroom/2026/20260603-printer
[C11]: https://in.canon/en/business/imagerunner-advance-dx-c3900i-series/main/specification
[C12]: https://oip.manual.canon/USRMA-9934-zz-CS-enUS.pdf?webmnl=
[X01]: https://www.news.xerox.com/news/xerox-launches-connectkey-enabled-altalink-digital-workplace-assistant
[X02]: https://download.support.xerox.com/pub/docs/ALB81XX/userdocs/any-os/en_GB/Xerox_AltaLink_B8100_C8100_AltaLink_B8200_C8200_series_mfp_sag_en-US.pdf
[X03]: https://www.office.xerox.com/latest/AC8SS-04UA.PDF
[X04]: https://www.news.xerox.com/news/xerox-transforms-document-processing-with-new-ai-assisted-xerox-altalink-8200-series-multi-function-printers
[X05]: https://www.office.xerox.com/latest/V6CBR-012.pdf
[X06]: https://security.business.xerox.com/en-usdocuments/installation-guides/
[X07]: https://www.office.xerox.com/latest/V6CSS-01L.pdf
[K01]: https://manuals.konicaminolta.eu/bizhub-C360i-C300i-C250i-UD/EN/contents/id13-_104514277.html
[K02]: https://manuals.konicaminolta.eu/bizhub-C360i-C300i-C250i/EN/bizhub-c360i-c300i-c250i/contents/id08-_104516893.html
[K03]: https://www.konicaminolta.com/global-en/security/mfp/setting/pdf/SecurityTechnicalSupportPaper_en.pdf
[K04]: https://www.konicaminolta.com/jp-ja/newsroom/2024/0515-01-01.html
[K05]: https://www.btapac.konicaminolta.com/users_guide/bizhub_C361i_series/2_03_00/jp/contents/DE_05.html
[K06]: https://cscsupportftp.mykonicaminolta.com/DownloadFile/Download.ashx?fileversionid=39530&productid=2175
[K07]: https://m.konicaminolta.eu/rs/697-GBV-750/images/Konice%20MInolta%20Office_Line-Up_Brochure_FINAL.pdf
[K08]: https://dl.konicaminolta.eu/en/?productName=bizhub+C250i
[K09]: https://www.konicaminolta.eu/eu-en/support/oauth2.0-information
[K10]: https://konicaminolta.ca/wps/wcm/connect/bca/a5e9861a-cc04-4360-a46e-484aa26f3209/FleetRMMUserGuide.pdf?MOD=AJPERES
[K11]: https://www.konicaminolta.com/global-en/security/mfp/setting/pdf/SecurityTechnicalSupportPaper_en.pdf
[Y01]: https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/us/documents/brochures-and-specs/TASKalfa_3554ci_Spec_Sheet_pdf.download.pdf
[Y02]: https://www.kyoceradocumentsolutions.eu/content/download-center/eu/documents/datasheet/DS_TASKalfa_2554ci__pdf.download.pdf
[Y03]: https://www.kyoceradocumentsolutions.com/global/ja/news/rls_2024/rls_20241202.html
[Y04]: https://www.kyoceradocumentsolutions.com/asia/en/products/mfp/taskalfa-mz7001ci/features-security.html
[Y05]: https://www.kyoceradocumentsolutions.com/vn/vi/products/catalog/pdf/Brochure_TASKalfa_2554ci_3554ci_4054ci_5054ci_6054ci_7054ci_5004i_6004i_7004i_AS.pdf
[Y06]: https://www.kyoceradocumentsolutions.us/en/products/mfp/ECOSYSMA4000CIX.html
[Y07]: https://www.kyoceradocumentsolutions.us/content/download-center-americas/us/documents/brochures-and-specs/KDA_ECOSYS_MA4500ifx_pdf.download.pdf
[Y08]: https://dam.kyoceradocumentsolutions.com/content/dam/gdam_dc/dc_global/document/manual/product_080/commandcenterrx_taskalfa7054ci_en_03.pdf
[Y09]: https://www.kyoceradocumentsolutions.us/en/support/downloads.name-L3VzL2VzL21mcC9UQVNLQUxGQTMwOENJ.html
[B01]: https://www.brother-usa.com/p/laser-printers/MFCL6915DW
[B02]: https://support.brother.com/g/s/id/htmldoc/mfc/cv_mfcl6915dw/use/html/GUID-6CD1909F-A0FF-4E39-8C38-E80D9857564E_1.html?lang=en&prod=dcpl5610dn_us
[B03]: https://www.brother-usa.com/-/media/brother/product-catalog-media/documents/2023/11/21/21/26/mfc-l6915dw_2pgbrochure.pdf
[B04]: https://support.brother.com/g/b/downloadlist.aspx?c=us&lang=en&os=10012&prod=mfcl9670cdn_all
[B05]: https://support.brother.com/g/b/sp/faqend.aspx?c=nz&faqid=faq00100232_002&lang=en&prod=mfcl8690cdw_eu_as&src=ipas
[E01]: https://www.epson.eu/en_EU/products/printers/inkjet/business-inkjet/workforce-enterprise%E2%80%8B-am-c6000%E2%80%8B-printer/p/34981
[E02]: https://news.epson.com/news/workforce-enterprise-am-c4000-c5000-c6000
[E03]: https://files.support.epson.com/docid/other/npd7005-00_en.pdf
[E04]: https://files.support.epson.com/docid/cpd6/cpd62840.pdf
[E05]: https://epson.com/Accessories/Printer-Accessories/Epson-Device-Admin/p/DEVICEADMIN
[E06]: https://news.epson.com/news/workforce-enterprise-am-c10000-mfp
[E07]: https://epson.com/For-Work/Printers/Inkjet/WorkForce-Pro-WF-C5890-Color-MFP/p/C11CK23201?blaid=5961692
[E08]: https://press.epson.eu/de_AT/news/drei-neue-epson-mid-range-business-inkjet-drucker-vervollstaendigen-din-a3-portfolio/
[L01]: https://www.lexmark.com/content/dam/lexmark/collateral/brochures/lad/es_xl/CX635adwe-Brochure.pdf
[L02]: https://support.lexmark.com/content/dam/support/collateral/lexmark_hardware/cx53x_cx63x/ug/pdf/Lexmark_CX532_CX635_XC2335_XC2342_UserGuide_en.pdf
[L03]: https://www.lexmark.com/content/dam/lexmark/documents/compliance/ecma-370/ECMA-370-Annex-B1-rev6-CX635adwe-XC2342.pdf
[L04]: https://www.lexmark.com/en_us/printers/printer/11850/Lexmark-MX622adhe
[L05]: https://publications.lexmark.com/publications/lexmark_hardware/CX421_CX522_CX622_CX625_MC2325_MC2425_MC2535_MC2640_XC2235_XC4240/UG/html/en/network-overview-topic.html
[L06]: https://support.lexmark.com/en_us/printers/printer/E462/article/FA615.html
[L07]: https://publications.lexmark.com/publications/lexmark_solutions/Fleet_Manager/AG/html/en/openAPI-group.html
[L08]: https://publications.lexmark.com/publications/lexmark_solutions/Document_Distributor/SDK_Guide/v5.4/html/en/understanding-job-submission-web-service-api-topic.html
[L09]: https://support.lexmark.com/en_us/printers/printer/X464/article/FA642.html
[A01]: https://business.sharpusa.com/portals/0/downloads/Literature/BP-70C31_70C36_70C45-Specification-Sheet.pdf
[A02]: https://news.sharpusa.com/2022-04-12-Sharp-Launches-New-Line-of-A3-Color-Workgroup-Document-Systems
[A03]: https://business.sharpusa.com/portals/0/downloads/Literature/Brochure-BP-51C26-51C31-51C36-51C45.pdf
[A04]: https://global.sharp/corporate/news/251009-a.html
[A05]: https://business.sharpusa.com/Portals/0/downloads/Infographics/Sharp-A3-Product-Lineup.pdf
[A06]: https://docs.aws.sharp.eu/Marketing/Brochure/BRO_A3_Range_Brochure_21628_eng.pdf
[A07]: https://business.sharpusa.com/portals/0/downloads/Literature/BP-Color-Advanced-Essentials-Series-Feature-Comparison-Chart.pdf
[A08]: https://global.sharp/restricted/print/manuals/4/bp70c65/en/contents_09-07_012.html
[A09]: https://global.sharp/products/copier/certified/download/cc-certified.html
[T01]: https://www.toshibatec.com/cnt/products_overseas/MFP/eS6525AC_series/
[T02]: https://business.toshiba.com/app/uploads/2026/06/2525AC-3025AC-3525AC-4525AC-Spec-Sheet-EN.pdf
[T03]: https://business.toshiba.com/downloads/KB/f1Ulds/20151/eS6528A_QS_EN_0001.pdf
[T04]: https://fi.toshibatec.eu/publicsite-service/resource/download/pseu/en/93e9c5fb-a118-4378-87c0-20891df166d3/030ef8eb494c8943c444bb6adf66b2cc/BR_eSTUDIO6525AC_series.pdf
[T05]: https://business.toshiba.com/app/uploads/2023/04/6527AC_7527AC_Spec_Sheet_EN.pdf
[T06]: https://business.toshiba.com/app/uploads/2023/04/6529A-7529A-9029A-Brochure-EN.pdf
[T07]: https://business.toshiba.com/downloads/KB/f1Ulds/20992/eS6528A_TAG_EN_0003.pdf
[O01]: https://www.oki.com/sg/printing/about-us/news-room/press-releases/2021/210316_ODSP_C650_OKI_Launches_World_Smallest_High_Performance_A4_Colour_Printer/index.html
[O02]: https://www.oki.com/printing/online-manuals-Z002/EE8001-1215/id/contents/contents/33916965.html
[O03]: https://www.oki.com/printing/online-manuals-Z002/EE8001-6614/id/contents/contents/241796849.html
[O04]: https://www.oki.com/de/printing/support/drivers-and-utilities/colour-multifunction/45850404/
[O05]: https://www.oki.com/es/printing/support/firmware/colour/09006144/
[G01]: https://printersetup.ext.hp.com/ts/manual/Samsung%20X4300%20Series/English/manual/sws.htm
[G02]: https://support.hp.com/ro-en/product/product-specs/samsung-multixpress-sl-x4300-color-laser-multifunction-printer-series/model/16449692
[G03]: https://support.hp.com/au-en/document/c05584061
[F01]: https://asset.fujifilm.com/www/in/files/2024-07/7c4415f3b39e90052e7787cc50562056/apeos-c5570-c4570-c3570-brochure_ind.pdf
[F02]: https://asset.fujifilm.com/www/us/files/2024-11/f06ebb5723fb04af46f09583c1cd7212/apeos_c7070_brochure.pdf
[F03]: https://opencds-fb.fujifilm.com/gen/product2_aux/prod/ews_help/ap/apeos_5570/en/contents/07_protocol/pro_setting_snmp_v3.html
[F04]: https://asset.fujifilm.com/www/in/files/2025-04/0d239230c247f480ec5fe82dc7feb2fd/apeos-c4030-c3530-brochure_in.pdf
[F05]: https://opencds-fb.fujifilm.com/gen/product2_aux/prod/manual/ap_c3567_ug/solution/en/000005.html
[F06]: https://support-fb.fujifilm.com/driver_downloads/gm2160e5-1.pdf
[Z01]: https://www.riso.co.jp/english/product/comcolor/gl/spec/index.html
[Z02]: https://www.riso.co.jp/release/20211008_1.html
[Z03]: https://www.riso.co.jp/product/orphis/gl/spec/gl9730.html
[Z04]: https://www.riso.co.jp/english/company/history/now.html
[Z05]: https://www.riso.com/download/manual/gl/GL_AdministratorsGuide_ENG.pdf
[P01]: https://global.pantum.com/product-center/1517382458899738625.html
[Q01]: https://www.ricoh-usa.com/en/newsroom/print-service-providers-expand-application-creativity-with-versatile-ricoh-pro-c7500-digital-press
[Q02]: https://assets.ricoh-usa.com/j2jqn9lauv41/44t3CJUBaRAD2VYQY002f2/9b9a7083be467a59ce4ba31d691f2b9c/Pro_C7500_Brochure_CAEN.pdf
[Q03]: https://www.ricoh.co.jp/products/maker/ricoh/production-printer/information
[Q04]: https://www.news.xerox.com/news/xerox-introduces-new-xerox-primelink-c9200-series-production-color-press
