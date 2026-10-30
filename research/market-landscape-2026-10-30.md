# Printer fleet management: the market as of 30 October 2026

This is the public summary of the market research behind PrintMarshal's positioning:
what exists today for managing fleets of network printers, who makes it, how it is
deployed and licensed, and where a self-hosted, vendor-neutral tool fits. Every
external claim carries its URL and the date it was read. "Not published" means the
cited material does not establish the fact, not that the feature cannot exist. No
product was installed, scanned or trialled for this study; the tables record what
vendors and projects publish about themselves.

## Scope

PrintMarshal manages the devices: discovery, inventory, telemetry, supplies, configuration,
security assessment and firmware orchestration over SNMP, IPP and eSCL, self-hosted under
Apache-2.0. It is not a print-management product. PaperCut, Pharos, PrinterLogic/Vasion
Print, uniFLOW, Printix and Equitrac track, charge for and release print jobs; they are an
adjacent market that coexists with a device console and are out of scope here.
[ADJ; read 2026-10-30](https://www.papercut.com/products/)

The same qualification applies to PrintMarshal itself: the wiki states that no shipped
vendor plugin yet implements security writes, certificate installation or firmware delivery
on real hardware, the standards drivers permit a limited configuration write set, and the
model catalogue is research evidence rather than hardware certification. The comparison
below therefore measures documented scope on both sides, not marketing.
[PMD; read 2026-10-30](../wiki/Vendor-plugins-and-drivers.md) [PMW; read 2026-10-30](../wiki/Home.md)

**Reading the tables.** Capability shorthand: **D** discovery/inventory; **U** supplies;
**A** alerts; **S** printer security-policy assessment; **C** device configuration
deployment; **F** firmware deployment; **K** device certificate management; **R**
reporting/export; **X** API/integrations. "Multi/basic" means third-party devices get
narrower treatment than the maker's own. A free download does not establish free hosting,
support or source access. Published prices are quoted in the currency and unit the vendor
shows and are not converted.

## 1. Manufacturer fleet tools

Every major manufacturer ships a fleet tool for its own devices. Most are free to download
and run on premises; the cloud successors are commercial or bundled with service contracts.
Nearly all monitor other makers' printers at a basic level, and nearly all reserve
configuration, firmware and security control for their own hardware.

| Product; maker / buyer | Deployment / collector | Vendor coverage | Business model, published price and free portion | Published printer capabilities / evidence |
|---|---|---|---|---|
| **Web Jetadmin; HP / enterprise IT** | On-premises Windows service/server; network polling | Multi/basic; richer HP/plugins | Free download; no recurring software price; host/support costs separate | D,U,A,C,F,R; security settings; K documented in guide; X not published here. [HP1; read 2026-10-30](https://support.hp.com/us-en/document/ish_3007439-2783223-16) [HP2; read 2026-10-30](https://h20195.www2.hp.com/v2/getpdf.aspx/4AA5-2718ENW.pdf) [HP3; read 2026-10-30](https://kaas.hpcloud.hp.com/pdf-public/pdf_3017438_en-US-1.pdf) |
| **Smart Device Services (SDS); HP / MPS dealers** | Cloud services through compatible MPS DCA or supported cloud-connected devices | HP-specific advanced service | Current standalone price/unit not published. Historical announcement: no additional cost for qualified partners’ contractual devices using Original supplies | D,U,A,C,F and diagnostic/predictive service functions depend on model matrix; rest not published. [HP4; read 2026-10-30](https://jamanagement.hp.com/API/DeviceCapabilitiesUI/) [HP5; read 2026-10-30](https://www.hp.com/us-en/newsroom/press-releases/2016/hp-inc--unleashes-next-generation-a3-printing-portfolio-to-disru.html) [M2; read 2026-10-30](https://www.mpsmonitor.com/sds/) |
| **Command Center; HP / channel partners** | Cloud portal; device/app connection | HP ecosystem | Subscription/app marketplace described; fleet-console price and free entitlement not published | App discovery, subscription, deployment/configuration; printer U,S,F,K,R,X not established by this description. [HP6; read 2026-10-30](https://developers.hp.com/sites/default/files/CommandCenterDescription_0.pdf) |
| **WXP Print Standard / Pro; HP / enterprise IT, partners** | Cloud; direct HP connection or Print Fleet Proxy plus Web Jetadmin | Multi/basic monitoring; deep control select FutureSmart 5 HP devices | Tiered commercial plans; numerical prices and charging unit not published; WXP Starter access included | D,C,F,R; Standard essential security; Pro advanced S,K; U/A/X print-specific entitlement not published in comparison. [HP7; read 2026-10-30](https://workforceexperience.hp.com/pricing/) [HP8; read 2026-10-30](https://learn.workforceexperience.hp.com/docs/overview-of-print) |
| **CentreWare Web; Xerox / enterprise IT** | On-premises web application/server | Multi-vendor monitoring; model-dependent writes | Free download | D,U,A,C,R and maintenance; S,F,K,X detail not published in cited brochure. [X1; read 2026-10-30](https://www.office.xerox.com/latest/SFTBR-05U.PDF) |
| **Device Manager (XDM); Xerox / MPS service teams** | Local monitoring/configuration component plus hosted Xerox Services Manager | Xerox and non-Xerox MPS fleet; depth varies | Services-contract context; standalone price/unit/free entitlement not published | D,U,A,C,R through service architecture; independent S,F,K,X entitlement not published. [X2; read 2026-10-30](https://www.xerox.com/downloads/usa/en/services/brochure/Federal_Government_Capability_Brief_EN_US.pdf) |
| **Workplace Cloud Fleet Management; Xerox / SMB, enterprise IT, MPS** | Cloud; Workplace Cloud Agent or embedded Cloud Direct on compatible Xerox models | Multi/basic agent architecture; direct/deep management Xerox-focused | Paid device licence bundles; numeric price not published; no permanent free plan established | D,U,A,S,C,F,R; passwords and MPS integration; K not established by capability brief. [X3; read 2026-10-30](https://www.xerox.com/downloads/usa/en/services/brief/Xerox-Workplace-Cloud-Fleet-Management-Solution-Capability-Brief-ENUS.pdf) [X4; read 2026-10-30](https://xwc.services.xerox.com/TermsOfUse) [X5; read 2026-10-30](https://security.business.xerox.com/wp-content/uploads/2020/10/Xer_WorkplaceCloud-5_6_1_Security_Guide.pdf) |
| **Device Manager NX Lite; Ricoh / IT, service providers** | Local client utility | Multi/basic; cloning/configuration Ricoh | Download offered; licence price/free entitlement not explicitly established on reviewed page | D,U,C,R; up to 250 devices; rest not published. [R1; read 2026-10-30](https://www.ricoh-usa.com/en/products/pd/software/device-set-up-and-management/device-usage-and-monitoring/ricoh-device-manager-nx-lite) |
| **Device Manager NX Pro / Enterprise; Ricoh / enterprise IT** | On-premises servers/database; optional USB agents and @Remote connector | Multi/basic; Ricoh configuration | Commercial licence/maintenance structure. 2026 GSA schedule: NX Pro M&S $226; Enterprise Core M&S $566; these are maintenance line items, not full licence prices | D,U,A,C,R,X help-desk API; F via @Remote architecture; S,K not published here. Legacy brochure. [R2; read 2026-10-30](https://assets.ctfassets.net/j2jqn9lauv41/5Ckxovn2HWJJPmodJda1K8/2fd5fa9300a730de7ffa6045a5456e86/DMNX-Enterprise-Brochure.pdf) [R6; read 2026-10-30](https://www.gsaadvantage.gov/ref_text/GS25F0051S/110BBQ.3WQO6E_GS-25F-0051S_GSACONTRACTGS25F0051SMOD233.PDF) |
| **Streamline NX device-management module; Ricoh / enterprise IT** | Server suite; delegation/connector components | Mixed-fleet management; advanced Ricoh scope model-dependent | Modular commercial suite; price/unit not published; @Remote and certificate service optional | D,A,C,R,K; firmware-check tasks; F deployment and U detail not established on product page; X not published. [R3; read 2026-10-30](https://www.ricoh-usa.com/en/products/pd/software/device-set-up-and-management/ricoh-streamline-nx) [R5; read 2026-10-30](https://docs.ricoh-sdc.com/24r1-en/Content/AdministrationTopics/DeviceMonitor/MonitorwithAtRemote.htm) |
| **@Remote; Ricoh / dealer service operations** | Hosted service; embedded communication, RC Gate or licensed connector | Multi/basic monitoring; fuller Ricoh services | Contract/service-led; connector licence required in documented path; price not published | U,A,R, meters, automated toner/service calls; Ricoh F via connected NX architecture; S,C,K,X not independently established. [R4; read 2026-10-30](https://www.ricoh.com/products/at-remote) [R5; read 2026-10-30](https://docs.ricoh-sdc.com/24r1-en/Content/AdministrationTopics/DeviceMonitor/MonitorwithAtRemote.htm) [R2; read 2026-10-30](https://assets.ctfassets.net/j2jqn9lauv41/5Ckxovn2HWJJPmodJda1K8/2fd5fa9300a730de7ffa6045a5456e86/DMNX-Enterprise-Brochure.pdf) |
| **Markvision Enterprise (MVE); Lexmark / enterprise IT** | On-premises web utility/local database | Deep management documented for Lexmark; current third-party management scope not published in reviewed pages | Available at no cost | D,U,A,S,C,F,K,R; scheduled audits/conformance and maintenance windows; X not published here. [L1; read 2026-10-30](https://www.lexmark.com/en_us/solutions/print/lexmark-markvision-enterprise.html) [L2; read 2026-10-30](https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-ace31bbf-90f9-41b6-9c50-5dfab3f44062-en.html) |
| **Cloud Fleet Management; Lexmark / partners, enterprise IT** | Cloud; Fleet Agent on server, Native/Printer Agent on supported devices, Local Agent for USB | Multi/basic data collection; Lexmark configuration/firmware | Commercial availability; standalone price, unit and permanent free entitlement not published | D,U,A,C,F,R,X; model restrictions apply; S,K entitlement not established here. [L3; read 2026-10-30](https://publications.lexmark.com/publications/lexmark_solutions/Cloud/AG/pdf/Lexmark_CloudServices_AdminGuide_en.pdf) |
| **Net Admin / Net Viewer; Kyocera / IT** | Legacy on-premises administration; agents for remote segments in Net Admin | Kyocera management; precise third-party write scope not published | Net Admin price/free terms not published in reviewed guide; current KDM offered as free successor-generation utility | D,U,C,R; model-specific multi-set templates; rest not published here. [K3; read 2026-10-30](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/us/documents/utility-guides/KNAENUG__SupplementR38_1_2019_03__pdf.download.pdf) [K1; read 2026-10-30](https://www.kyoceradocumentsolutions.us/en/products/software/KYOCERADEVICEMANAGER.html) [K2; read 2026-10-30](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/master/documents/brochures-and-specs/Kyocera_Software_Portfolio_Brochure_pdf.download.pdf) |
| **Device Manager (KDM); Kyocera / mid-large enterprise IT** | On-premises server/web UI | Kyocera-focused; cross-vendor write coverage not published | Explicitly “Free utility” in portfolio brochure | D,C and application deployment; password/security settings; U,A,F,K,R,X detail not established in short product page. [K1; read 2026-10-30](https://www.kyoceradocumentsolutions.us/en/products/software/KYOCERADEVICEMANAGER.html) [K2; read 2026-10-30](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/master/documents/brochures-and-specs/Kyocera_Software_Portfolio_Brochure_pdf.download.pdf) |
| **Fleet Services (KFS); Kyocera / dealers, fleet IT** | Cloud; embedded KFS or Windows Gateway/NetGateway for legacy/other makers | Multi/basic; advanced Kyocera control | Commercial/channel service; numeric price, charging unit and free entitlement not published | D,U,A,C,F,R,X; audit logs; S,K not established as full lifecycle services. [K4; read 2026-10-30](https://kyoceradocumentsolutions.com.au/kyocera-fleet-services) [K5; read 2026-10-30](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/us/documents/user-guides/KFSUserGuideENOG_HELP_SYSTEM_pdf.download.pdf) |
| **PageScope Net Care Device Manager; Konica Minolta / IT** | Legacy on-premises web system | Multi/basic; full Konica Minolta settings | Current price/free terms not published; APAC downloads/support ended 2022-06-30 | D,A,C,R; cloning/templates/scheduling; rest not published. [KM1; read 2026-10-30](https://www.konicaminolta.co.uk/en-gb/software/pagescope-netcare-device-manager) [KM2; read 2026-10-30](https://www.btapac.konicaminolta.com/) |
| **Fleet RMM; Konica Minolta / IT** | On-premises web tool | Konica Minolta-focused; third-party scope not published | Utility download; explicit licence price/free terms not published | D,C, status/counters; U,A,S,F,K,R,X not established by utility listing. [KM3; read 2026-10-30](https://www.btapac.konicaminolta.com/utilities/index.html) |
| **Shield Guard; Konica Minolta / IT, service providers** | Cloud; embedded agent on MFP | Supported bizhub devices | Licensed cloud service; portal trial and agent free; numerical subscription price/unit not published in reviewed product description | S,C,A,R; password management; D,U,F,K,X not established. [KM4; read 2026-10-30](https://www.konicaminolta.asia/asia-en/solutions/bizhub-productivity-tools/shield-guard) |
| **CS Remote Care / regional bizhub remote-care services; Konica Minolta / dealers** | Remote service infrastructure; most communication methods need no customer installation | Konica Minolta | Service-led; contract price/unit and free entitlement not published | U,A,C,F,R, automatic counters and service calls; D,S,K,X not published. [KM5; read 2026-10-30](https://www.konicaminolta.eu/eu-en/digital-workplace/software-services/cs-remote-care-device-management?aliaspath=%2FSoftware%2FDevice-management%2FCS-Remote-Care-device-management) |
| **imageWARE / iW Enterprise Management Console; Canon / enterprise IT** | On-premises web utility | Multi/basic Printer-MIB monitoring; Canon write functions | Canon 2023 guide says v4.1.2+ no charge; US store now says discontinued. Current regional entitlement must be checked | D,U,A,C,F,R; credentials/profiles; S,K,X not established. [C1; read 2026-10-30](https://downloads.canon.com/bicg2023/guides/svc-imagerunner-advance-dx-8700-series-sg07.pdf) [C2; read 2026-10-30](https://www.usa.canon.com/shop/p/imageware-enterprise-management-console) [C5; read 2026-10-30](https://info.omniapartners.com/hubfs/Public%20-%20Supplier%20Contract%20Information/Canon%20Solutions%20America/Canon_Multi-Function%20Devices.pdf) |
| **imageWARE Remote / remote monitoring, CDS; Canon / authorized dealers** | Device-embedded remote service/cloud; local CDS option for updates | Canon | Remote component standard in Canon devices, activation required; service/CDS prices not published | U,A,F,R, automated meter/service/supply workflow; D,S,C,K,X not published here. [C4; read 2026-10-30](https://community.usa.canon.com/html/assets/2023_CUSA_BICG_SS_Remote_Services.pdf) |
| **BRAdmin Professional 4; Brother / IT, SMB** | Windows utility; Azure option with local clients/compatible firmware | Multi/basic SNMP monitoring; Brother writes | Free software; Azure storage/service charges not included in that claim | D,U,A,C,F,R; model-dependent settings; S,K,X not published here. [B1; read 2026-10-30](https://www.brother.com.sg/en/solutions/bradmin-professional4) [B2; read 2026-10-30](https://www.brother.co.za/-/media/ap2/south-africa/landing-page/multifunction-printers/solutions/brother-bradmin-professional-4.pdf) [B4; read 2026-10-30](https://support.brother.com/g/b/manualtop.aspx?c=us&lang=en&prod=group2) |
| **Brother Fleet Management tools; Brother / SMB, dealers** | Tool family: BRAdmin desktop, embedded EWS and deployment tools | Brother-focused; BRAdmin limited multivendor view | BRAdmin free; no separately priced universal “Brother Fleet Management” SKU established | Monitoring, supplies/email alerts and fleet installation; assess named component rather than double-counting a platform. [B3; read 2026-10-30](https://www.brother.com.my/en/solutions/fleet-management-detail) |
| **Device Admin; Epson / IT** | Windows workstation/server; USB-through-PC supported | Epson management; some third-party scope requires separate compatibility check | Free download | D,C,F,R, grouping, MPS billing reports; U,A commonly part of device management but detailed scope not established in selected page; S,K,X not published. [E1; read 2026-10-30](https://www.epson.com.sg/software-solutions-details) |
| **Remote Services (ERS); Epson / dealers** | Cloud plus collection agent; network/USB models | Epson | UK page explicitly free; regional/partner eligibility applies; paid support terms not published | U,A,R, remote diagnostics, reboot/printhead maintenance; D,S,C,F,K,X not established here. [E2; read 2026-10-30](https://www.epson.co.uk/en_GB/epson-remote-services) [E3; read 2026-10-30](https://mediaserver.goepson.com/ImConvServlet/imconv/f6a782c79d7138aad1307feda3697502cc762456/original?assetDescr=Epson_Remote_Services_Software_Specifications&use=banner) |
| **Remote Device Manager (SRDM); Sharp / IT, service providers** | On-premises SRDM server/web console | Multi/basic; third-party information capture only | Download offered; explicit price/free licence terms not published on reviewed page | D,U,A,S,C,F,R, SIEM integration and cloning; K,X details not published. [SH1; read 2026-10-30](https://business.sharpusa.com/software/srdm) |
| **Synappx Manage; Sharp / IT** | Cloud; collector/embedded prerequisites not published in overview | Sharp MFPs/displays | Price/unit and free entitlement not published | Remote monitoring/automated remediation; detailed D,U,A,S,C,F,K,R,X entitlements not published in overview. [SH2; read 2026-10-30](https://business.sharpusa.com/synappx-support) |
| **e-BRIDGE CloudConnect / SKY Suite management; Toshiba / dealer, IT** | Device-to-cloud; SKY portal | Toshiba | Service/channel model; price/unit/free entitlement not published | U,A,C,F,R, backup and policy-based settings; D,S,K,X not published here. [T1; read 2026-10-30](https://www.toshibatec.co.th/en/solution/e-bridge-cloud-connect/) [T3; read 2026-10-30](https://www.toshibatec.com/products_overseas/ebridge_cs/) |
| **Elevate Sky Manage / View; Toshiba / dealers, end customers** | Cloud services/portal; collection architecture varies by service | View explicitly Toshiba and third-party; deep Manage functions Toshiba-focused | Licence/subscription portal; prices and charging units not published | Fleet monitoring/reporting and deployment/configuration; broader S,F,K,X entitlement not established in brochure. [T2; read 2026-10-30](https://business.toshiba.com/app/uploads/2023/11/Toshiba-Elevate-Cloud-Services-Brochure.pdf) |
| **e-BRIDGE Fleet Management System; Toshiba / IT** | Legacy fleet software; current architecture not published on licence page | Connected Toshiba MFPs | Licence terms offered; numeric price/unit/free entitlement not published | Device-data registration; current U,A,S,C,F,K,R,X not published in reviewed legacy listing. [T4; read 2026-10-30](https://www.toshibatec.nl/service-en-support/licentievoorwaarden/) |
| **PrintSuperVision; OKI / SMB, enterprise IT** | On-premises web application/network collector | Multi-vendor compatible devices | Vendor page offers download without explicit price; reseller says legacy Professional/Enterprise free. Current vendor licence confirmation not published | D,U,A,R, supply forecasts/maps/maintenance logs; S,C,F,K,X not established. [O1; read 2026-10-30](https://www.oki.com/sg/printing/services-and-solutions/smart-solutions/printsupervision/index.html) [O2; read 2026-10-30](https://www.m5digital.ca/services/printsupervision/) |
| **Printer Profile Manager Enterprise; Zebra / enterprise/industrial IT** | Customer-installed server; printers connect through Link-OS/WebLink | Zebra supported Link-OS models; Basic excluded | Paid licence; 60-day full trial; price/unit not published in support page | D,C,F,K, resource/profile deployment and operation logs; U,A,S,R,X precise entitlement not established here. [Z1; read 2026-10-30](https://www.zebra.com/us/en/support-downloads/software/printer-software/profile-manager-enterprise.html) [Z2; read 2026-10-30](https://www.zebra.com/content/dam/support-dam/en/documentation/unrestricted/guide/software/ppme-ug-en.pdf) |
| **Device Setup / ApeosWare device-management module; Fujifilm Business Innovation / IT** | Local Device Setup; ApeosWare server suite | Fujifilm-supported model matrix | Device Setup free; ApeosWare price/unit not published in selected material | D,C; suite model matrix includes status/security monitoring; U,A,F,K,R,X detail not established here. [FU1; read 2026-10-30](https://support-fb.fujifilm.com/driver_downloads/DLS-userguide_en_for_FF.pdf) [FU2; read 2026-10-30](https://asset-fb.fujifilm.com/www/fbkr/files/2026-01/3c035779b3d1371901c9979117b4f21b/support_models.pdf) |

## 2. Multivendor MPS and fleet platforms

These platforms serve managed-print-service dealers and multi-site operators across brands.
They are commercial, mostly cloud-hosted with a local collector, and strongest at meters,
supplies and service workflow; deep configuration, firmware and security operations are
usually delivered through a manufacturer's own service layer (for example HP SDS).

| Product; maker / buyer | Deployment / collector | Vendor coverage | Business model, published price and free portion | Published printer capabilities / evidence |
|---|---|---|---|---|
| **FMAudit; ECI / MPS dealers** | Legacy Central: vendor-hosted or on-premises; Onsite/DCA | Multi-vendor | Commercial; current rate, charging unit and permanent free entitlement not published | D,U,A,R; deeper S,C,F,K,X not established by selected legacy paper. [EC3; read 2026-10-30](https://www.marconet.com/hubfs/FMAudit%20Technical%20Whitepaper%20NEW%204.5.pdf) [EC1; read 2026-10-30](https://www.ecisolutions.com/news/eci-launches-printanista-new-cloud-based-managed-print-solution/) |
| **PrintFleet; ECI / MPS dealers, distributors** | Legacy DCA/Pulse to web platform | Multi-vendor | Commercial; current price/unit/free entitlement not published | Device meters/status/supplies via SNMP; general S,C,F,K,X entitlement not published. [EC4; read 2026-10-30](https://www.nwpsi.com/wp-content/uploads/2025/02/Printfleet-pdf-for-web-link-Final.pdf) [EC1; read 2026-10-30](https://www.ecisolutions.com/news/eci-launches-printanista-new-cloud-based-managed-print-solution/) |
| **Printanista / Hub; ECI / dealers, MPS/MSP** | Cloud; local remotely maintained DCA | Multi-vendor, most major manufacturers | Commercial demo/quote; numerical price, billing unit and permanent free tier not published | D,U,A,R,X ERP integrations; S,C,F,K not established on reviewed product page. [EC2; read 2026-10-30](https://www.ecisolutions.com/en-gb/products/printanista-hub/) |
| **MPS Monitor; MPS Monitor / dealers, MPS providers** | SaaS; local eXplorer/embedded DCAs; eligible HP direct-cloud alternative | Multi-vendor; deep SDS operations HP-specific | Enterprise monthly in arrears per managed device; rate not published; HP SDS Cloud DCA included for subscribers; 2021 report described free basic unmanaged-device volumes, current terms not published [M5; read 2026-10-30](https://www.mpsmonitor.com/wp-content/uploads/2021/05/Quocirca_Cloud_Print_Services_2021_MPS_Monitor_Excerpt.pdf) | D,U,A,R,X; HP SDS adds S,C,F and remote EWS; K not published. [M1; read 2026-10-30](https://www.mpsmonitor.com/enterprise-prices/) [M2; read 2026-10-30](https://www.mpsmonitor.com/sds/) [M3; read 2026-10-30](https://www.mpsmonitor.com/wp-content/uploads/2024/02/MPSMonitor_Security-Technical-Whitepaper.pdf) |
| **Nexera; Nexera, Valsoft group / dealers, service managers** | Hosted analytics/customer-data website fed by service ERP; no separate printer collector established | Vendor-neutral service-data analysis | Monthly by active field technician; numeric fee not published; unlimited user logins included | R/benchmarking/consulting; direct D,U,A,S,C,F,K,X printer capability not published. [N1; read 2026-10-30](https://www.nexera.net/getting-started/) [M4; read 2026-10-30](https://www.mpsmonitor.com/blog/valsoft-kevin-tetu-president-of-managed-print-division/) |
| **3manager; 3manager / dealers, distributors, enterprise IT** | Cloud; Windows/macOS/Linux/Raspberry Pi collectors | Multi-brand | Essentials/Professional/Elite modules in regional plan page; rates/unit not published; free trial | D,U,A,R,X, forecasts, contracts, service/sales workflow; HP SDS integration; native S,C,F,K scope not published. [TH1; read 2026-10-30](https://www.3manager.com/) [TH2; read 2026-10-30](https://www.3manager.com.au/plans) [TH3; read 2026-10-30](https://www.3manager.com.au/) |
| **Princity; Princity / dealers, MPS, end customers** | Cloud or on-premises; collection agents/embedded options | Multi-brand | Published selector: **€110/month for 100**, **€1.10/printer/month**; all four modules; trial and API included | D,U,A,R,X; stock/orders/faults/billing; S,C,F,K not published. [PR1; read 2026-10-30](https://princity.com/pricing) [PR2; read 2026-10-30](https://princity.com/solutions) |
| **Print Tracker; Print Tracker, Inc. / MPS dealers** | Hosted application plus local Windows/macOS/Linux DCA [PT3; read 2026-10-30](https://docs.printtrackerpro.com/docs/guides/installation) | Multi-vendor supported printers | Starter **$795 upfront + $50/month**, up to 100; Enterprise $0.50 (100–499), $0.45 (500–999), $0.35 (1,000–2,499) per device with monthly billing; unmanaged quarterly reads free [PT4; read 2026-10-30](https://docs.printtrackerpro.com/docs/guides/billing); API from **$50/month/50 GB/entity** [PT2; read 2026-10-30](https://docs.printtrackerpro.com/docs/guides/integrations); trial | D,U,A,R,X paid integration, meters/predictive supplies, Remote Technician; S,C,F,K campaigns not established. [PT1; read 2026-10-30](https://landing.printtrackerpro.com/pricing) [PT3; read 2026-10-30](https://docs.printtrackerpro.com/docs/guides/installation) |
| **KPAX; KPAX / dealers, distributors, enterprise IT** | Cloud plus software collector or Liberty hardware; on-premises described as planned/“soon” | Multi-brand | Monthly per-device feature credits: 1 Koin each for meters, supplies, technical management; currency price not published; trial | D,U,A,R, connectors; S,C,F,K not published. [KP1; read 2026-10-30](https://www.kpax-manage.com/kpax/) [KP2; read 2026-10-30](https://www.kpax-manage.com/pricing/) |
| **EKM Insight; EKM Global / OEMs, resellers, enterprises** | Cloud platform; local software, cloud collectors or dedicated DCA appliance | Hardware/vendor-agnostic | Modular commercial offer; numeric rate/unit/free entitlement not published | D,U,A,R,X ERP/CRM/ITSM, supplies/service/billing automation; S,C,F,K depth not published on reviewed page. [EK1; read 2026-10-30](https://www.ekmglobal.com/) |
| **Fiery IQ / Manage, formerly EFI IQ; Fiery / production print providers** | Cloud plus Fiery Cloud Connector | Multiple OEMs within supported Fiery ecosystem, not arbitrary office printers | Dashboard/Insight/Notify/Go free; Manage and ColorGuard subscription; prices/unit not published | Status/A/R, production analytics, resource sync/configuration compliance via Manage; security-policy S,F,K,X not established. [FI1; read 2026-10-30](https://www.fiery.com/products/automation/fiery-iq/) [FI2; read 2026-10-30](https://help.fiery.com/Security/45218169_Rev_B_Fiery_IQ_SecurityWhitePaper.pdf) |
| **CompuData / Prismatic: unresolved fleet-product suggestions** | No qualified fleet architecture established | Not published | Fleet price/model/free portion not published | CompuData redirects to CBIZ IT services; Prismatic.io is embedded integration software. No verified independent printer-fleet SKU established under these names. [CD1; read 2026-10-30](https://www.compudata.com/) [PX1; read 2026-10-30](https://prismatic.io/) |

## 3. IT-operations monitoring with printer coverage

General monitoring tools already in many estates cover printers through SNMP: status,
consumables, alerts and inventory. None of the reviewed material establishes printer
security-policy assessment, configuration deployment, firmware or certificate management.

| Product; maker / buyer | Deployment / collector | Vendor coverage | Business model, published price and free portion | Published printer capabilities / evidence |
|---|---|---|---|---|
| **Lansweeper; Lansweeper / IT, MSP** | Local network scanning; cloud-connected inventory platform | Multi-vendor SNMP | Annual asset tiers; 100 assets free; Starter includes 2,000. Numeric currency-specific price not reliably established from extraction | D,U,A,R; toner/ink, pages, status and email alerts. Printer S,C,F,K not published; full X plan-dependent. [I1; read 2026-10-30](https://www.lansweeper.com/assets/hardware/printer/) [I2; read 2026-10-30](https://www.lansweeper.com/pricing/) |
| **NinjaOne NMS; NinjaOne / MSP, enterprise IT** | Cloud RMM; local Windows NMS probe/delegate | Multi-vendor SNMP | Endpoint/functionality quote; numerical NMS rate/free permanent plan not published; trial | D,A,R,X platform; U with custom SNMP OIDs. Printer S,C,F,K not published. [I3; read 2026-10-30](https://www.ninjaone.com/docs/network-management-system/nms/) [I4; read 2026-10-30](https://www.ninjaone.com/pricing/) [I5; read 2026-10-30](https://www.ninjaone.com/rmm/snmp-monitoring/) |
| **PRTG; Paessler / IT, MSP** | On-premises Network Monitor or hosted alternative; local/remote probes | Multi-vendor SNMP | Sensor subscription; 500 sensors **$200/month annually**, 1,000 **$358**; 100 sensors freeware; 30-day trial | D,U,A,R,X monitoring; printer sensor status/consumables; S,C,F,K printer lifecycle not published. [I6; read 2026-10-30](https://www.paessler.com/pricing) [I7; read 2026-10-30](https://www.paessler.com/manuals/prtg/snmp_printer_sensor) |
| **Zabbix; Zabbix / IT, service providers** | Self-hosted server; optional proxies; printer requires no installed agent | Multivendor community/model templates | Free AGPLv3 from v7; commercial support/services separately sold, price not established here | D,U,A,R; SNMP toner/counters/trays; extensible integrations. Printer S,C,F,K not published in reviewed templates. [I8; read 2026-10-30](https://www.zabbix.com/integrations/printers) [I9; read 2026-10-30](https://www.zabbix.com/license) |
| **LibreNMS; community / IT, SMB** | Self-hosted SNMP poller; distributed collection possible | Multi-vendor supported MIBs | Free GPLv3+ software; optional partner commercial support, price not published here | D,U,A,R; printer-supplies collection; X/platform extensions; printer S,C,F,K not established. [I10; read 2026-10-30](https://github.com/librenms/librenms) [I11; read 2026-10-30](https://github.com/librenms/librenms/issues/20664) |
| **Checkmk; Checkmk / IT, MSP** | Self-hosted Community/Pro/Ultimate or Cloud; SNMP reachable poller | Multi-vendor Printer-MIB | Community free; Pro from **€190/month**, Ultimate **€275/month**, annually; actual entitlement sized by services/workload, not unlimited printers | U,A; discover supply services for toner/fuser/waste; platform reporting/API; printer S,C,F,K not published. [I12; read 2026-10-30](https://checkmk.com/pricing) [I13; read 2026-10-30](https://checkmk.com/integrations/printer_supply) |
| **Nagios Core / XI; Nagios Enterprises / IT, MSP** | Self-hosted monitoring server/plugins | Multi-vendor through plugins; stock HPJD narrower | Core free GPLv2; XI perpetual by nodes: Standard 100 **$2,595**; Enterprise 100 **$4,690**; XI free 7 nodes or 100 services, first limit | D/A/R/X via XI/platform; toner-low/error status via HPJD, richer U via community plugin. Printer S,C,F,K not published. [I14; read 2026-10-30](https://www.nagios.com/pricing-plans/) [I15; read 2026-10-30](https://www.nagios.com/products/nagios-xi/) [I16; read 2026-10-30](https://assets.nagios.com/downloads/nagioscore/docs/nagios-3.pdf) [I23; read 2026-10-30](https://www.nagios.org/projects/nagios-core/) |
| **Network Device Monitoring; Datadog / enterprise operations** | SaaS; Datadog Agent SNMP poller onsite | Multi-vendor/custom profiles | From **$7/device/month annually**, **$10.20 on demand**, plus one or more paid Infrastructure poller hosts; trial | D,A,R,X; printer U requires validating/applying appropriate profile/OIDs, not a verified universal built-in promise; printer S,C,F,K not published. [I17; read 2026-10-30](https://www.datadoghq.com/pricing/?product=network-monitoring) [I18; read 2026-10-30](https://docs.datadoghq.com/network_monitoring/devices/snmp_metrics/) [I24; read 2026-10-30](https://docs.datadoghq.com/network_monitoring/devices/profiles/device_profiles/) |
| **Auvik Network Management; Auvik / MSP, IT** | Cloud; local collector | Multi-vendor network discovery/SNMP | Quote by billable infrastructure devices; printers monitored at no additional charge within service; total minimum/rate not published | D,A/R network visibility; exact consumable depth not established; printer S,C,F,K not published. [I19; read 2026-10-30](https://support.auvik.com/hc/en-us/articles/201515670-How-much-does-Auvik-cost) [I20; read 2026-10-30](https://www.auvik.com/network-management-software/) |
| **Domotz; Domotz / MSP, distributed IT** | Cloud; local collector on existing hardware/appliance | Multi-vendor/custom SNMP | **€35/collector/month**, unlimited devices; advertised annual/volume rates down to €17.50; 14-day trial | D,A,R,X; U via matching SNMP templates/OIDs; printer S,C,F,K not established. [I21; read 2026-10-30](https://www.domotz.com/pricing.php) [I22; read 2026-10-30](https://help.domotz.com/monitoring-management/snmp-monitoring/) |

## 4. Open-source and community alternatives

Zabbix, LibreNMS, Nagios Core and Checkmk Community in the previous table are open source
and are not repeated here. The projects below cover network inventory, collection building
blocks and small printer-specific tools; maintenance status is noted where it could be
observed, and dormancy is not asserted from an old version header alone.

| Product; maker / buyer | Deployment / collector | Vendor coverage | Business model, published price and free portion | Published printer capabilities / evidence |
|---|---|---|---|---|
| **Netdisco; Netdisco community / network IT** | Self-hosted Linux/Unix web app, PostgreSQL and backend collector | Multi-vendor | Free BSD-3-Clause source; commercial service pricing not published | D,R/network port-location inventory; printer U,A,S,C,F,K not established; network switch actions are separate. [OS1; read 2026-10-30](https://github.com/netdisco/netdisco) |
| **GLPI Agent network inventory; GLPI/Teclib community / IT asset teams** | Local agent with NetInventory module; GLPI server or stand-alone CLI | SNMP-compatible printers/network equipment | Free GPLv2+ agent; server hosting/support costs separate, amounts not published here | D and SNMP inventory; GLPI integration; device-dependent supplies/counters need model validation; S,C,F,K not published. [OS2; read 2026-10-30](https://glpi-agent.readthedocs.io/en/1.21/tasks/network-inventory.html) [OS10; read 2026-10-30](https://github.com/glpi-project/glpi-agent) |
| **FusionInventory Agent; FusionInventory community / IT asset teams** | Local discovery/inventory agent; optional GLPI orchestration | Multivendor SNMP | Free GPLv2+; no per-printer software fee | D and network inventory; printer lifecycle S,C,F,K not published; legacy agent maintenance status requires version-specific checking. [OS3; read 2026-10-30](https://documentation.fusioninventory.org/FusionInventory_tasks/networkinventory/) [OS4; read 2026-10-30](https://github.com/fusioninventory/fusioninventory-agent) [OS5; read 2026-10-30](https://fusioninventory.github.io/news.html) |
| **snmp_exporter; Prometheus community / monitoring engineers** | Self-hosted exporter plus Prometheus; monitoring/alerts dashboards assembled separately | Arbitrary supported/generated SNMP modules | Free Apache-2.0 source; engineering/hosting separate | SNMP metric collection; U possible with generated Printer-MIB module, not turnkey fleet UI; D,A,S,C,F,K,R not delivered by exporter alone. [OS6; read 2026-10-30](https://github.com/prometheus/snmp_exporter) |
| **check_snmp_printer; Tylan/contributors / Nagios, Icinga admins** | Local Perl/Net-SNMP plugin; existing monitor required | Lists Canon, HP, Xerox, Ricoh, Brother, Lexmark, Kyocera, Olivetti | Free GPL plugin; host platform separate | U/trays/pagecounts and performance data; host handles A/R; S,C,F,K,X not published. Code version 2.0.1 dated 2023-04-05. [OS7; read 2026-10-30](https://github.com/Tylan/check_snmp_printer) [OS8; read 2026-10-30](https://github.com/Tylan/check_snmp_printer/blob/master/check_snmp_printer) |
| **PrinterMonitor Pro proxy; printmonitor / community, small IT** | Local Python collector, SQLite or remote API | SNMP printer scope; validated model matrix not published | Public source/local mode; exact reuse licence and commercial cloud price not verified | README advertises D,U/status/counters; discovery also appears as unfinished scaffolding/manual setup; S,C,F,K,R not established. [OS9; read 2026-10-30](https://github.com/printmonitor/printermonitor-proxy) |

## 5. Printer security assessment and managed security

| Product; maker / buyer | Deployment / collector | Vendor coverage | Business model, published price and free portion | Published printer capabilities / evidence |
|---|---|---|---|---|
| **Security Manager; HP / enterprise security/IT** | On-premises service | Supported HP devices; other-vendor risk-assessment depth not established here | Paid device licences, node-locked to server MAC; price not published. Demonstration assesses up to 50 with restricted policy/editor | S,C,R, recurring assessment/remediation and re-assessment; K exact scope not established by selected extract; general D,U,A,F,X not published here. [SEC1; read 2026-10-30](https://kaas.hpcloud.hp.com/pdf-public/pdf_1980161_en-US-1.pdf) [SEC2; read 2026-10-30](https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=4AA3-9275ENW) |
| **Print Fleet Cyber Security as a Service; Symphion / enterprise IT/security** | Managed service; exact hosting/collector prerequisites not published in reviewed pages | Advertised vendor-agnostic | Contracted device-count fee; rate not published. Base/Advanced; certificate/additional firmware add-ons; implementation fee below 1,000 devices, amount not published | D,S,C,F,R; Base daily checks/remediation and two firmware updates/year; K service extra; U,A,X not fully specified. [SEC5; read 2026-10-30](https://symphion.com/pricing/) [SEC6; read 2026-10-30](https://symphion.com/) |
| **Printer Fleet Cybersecurity as a Service; Canon USA / enterprise security** | Managed service; hosting/collector not published | Mixed-make fleet context; explicit per-model support matrix not published | Service quote; numeric price/unit/free entitlement not published | S,A,R-style monitoring, security baseline, asset management/firmware security; C,F,K delivery details not published in page. [SEC7; read 2026-10-30](https://www.usa.canon.com/business/managed-services/managed-security) |
| **PRET; RUB-NDS/Jens Müller and contributors / authorized researchers** | Local CLI; direct network/USB connection | Multiple printers supporting PJL/PostScript/PCL; commands vary | Free GPL-2.0; no paid service price published; apparently dormant since visible 2023 commit | D/security research tests and language-level device interactions; no production S-policy/C/F/K campaign/R/API platform established. [SEC3; read 2026-10-30](https://github.com/RUB-NDS/PRET) [SEC4; read 2026-10-30](https://github.com/RUB-NDS/PRET/commits/master/) |
| **Nmap + printer NSE; Nmap Project / security teams** | Local scanner; no printer agent | Protocol-based multi-vendor | Downloadable/source tool; Nmap-specific licence; integration/redistribution terms separate; no per-printer scan fee established | D, protocol/exposure checks; PJL script can read/change ready message; U,S,C,F,K managed lifecycle not published. [SEC8; read 2026-10-30](https://nmap.org/nsedoc/scripts/pjl-ready-message.html) |
| **Nessus / Tenable scanner ecosystem; Tenable / security teams** | Customer-run Nessus; hosted/enterprise products differ | Multi-vendor plugins; exact coverage plugin/model-dependent | Nessus Professional **$4,790 for one year**; Expert **$6,790/year**; trials; no permanent free entitlement assumed | Vulnerability checks/reporting; printer identification and explicit scan-exclusion behavior; no U,C,F,K fleet lifecycle. [SEC9; read 2026-10-30](https://www.tenable.com/plugins/nessus/44920) [SEC10; read 2026-10-30](https://www.tenable.com/plugins/nnm/6639) [SEC16; read 2026-10-30](https://www.tenable.com/buy) |
| **Greenbone/OpenVAS community stack; Greenbone / security teams** | Self-hosted scanner/manager/web UI; feeds; enterprise alternatives | Multi-vendor vulnerability tests | Community software; commercial feeds/appliances/services separate, price not published in selected sources | Security scans/R/X via GMP; no printer U,C,F,K management evidence. Printer exclusion is best effort. [SEC12; read 2026-10-30](https://greenbone.github.io/docs/latest/background) [SEC13; read 2026-10-30](https://forum.greenbone.net/t/printer-isnt-ignored-in-full-and-fast-scan-config/13212) |
| **Nipper / InfraSight; Titania / network-security auditors — unverified printer coverage** | Offline assessment of exported configurations on local machine | Published routers/switches/firewalls/SD-WAN/APs; printer support **not published** | Commercial trial/purchase; relevant printer pricing/unit/free coverage not published | Configuration S/R for documented network-device classes; no verified printer D,U,A,S,C,F,K,X award. [SEC14; read 2026-10-30](https://www.titania.com/support) [SEC15; read 2026-10-30](https://www.titania.com/nipper-infrasight) |

Shield Guard, Markvision Enterprise, Sharp SRDM and HP WXP Print in the earlier tables are
security competitors too.

## 6. What the landscape shows

The pattern across the five groups is consistent:

- **Each manufacturer has its own tool, usually free, for its own devices.** HP Web
  Jetadmin, Lexmark Markvision Enterprise, Xerox CentreWare Web, Kyocera Device Manager,
  Brother BRAdmin Professional, Epson Device Admin and their peers are free downloads. Their
  depth stops at the vendor boundary: other makers' printers are discovered and monitored at
  a basic level, while configuration, firmware, certificates and security policy apply to
  the maker's hardware.
- **A mixed fleet therefore has two conventional choices.** Run several of these free
  single-vendor tools side by side, each with its own server, data and workflow, or pay for
  a multivendor managed platform or managed security service, which is cloud-hosted,
  priced per device or per site, and whose deepest operations still route through the
  manufacturers' own service layers.
- **General IT monitoring fills the observation gap, not the management gap.** Zabbix,
  LibreNMS, Checkmk, PRTG and the others answer "is it up and does it have toner"; none of
  the reviewed material covers security assessment, configuration or firmware for printers.
- **Vendor-neutral security assessment of printers exists mainly as a managed service**
  (Symphion, Canon's managed security) or as a single-vendor product (HP Security Manager,
  Konica Minolta Shield Guard, Lexmark MVE). Research and scanning tools such as PRET, Nmap
  scripts and vulnerability scanners find exposure but do not manage a fleet.

### Where PrintMarshal sits

The map below uses the operating model as one axis and printer operation depth as the
other. Placement reflects documented product emphasis, not a benchmark.

| Operating model | Discovery / telemetry / supplies | Device configuration / firmware | Security-policy assessment / remediation |
|---|---|---|---|
| **Self-hosted, open/community source** | LibreNMS, Zabbix, Checkmk Community; PrintMarshal [I10; read 2026-10-30](https://github.com/librenms/librenms) [I8; read 2026-10-30](https://www.zabbix.com/integrations/printers) [I12; read 2026-10-30](https://checkmk.com/pricing) [PM; read 2026-10-30](../README.md) | PrintMarshal limited standards writes; campaign orchestration ahead of real-driver delivery [PMD; read 2026-10-30](../wiki/Vendor-plugins-and-drivers.md) [PMF; read 2026-10-30](../wiki/Firmware.md) | PrintMarshal policy/probe assessment; actual remediation driver-limited [PMS; read 2026-10-30](../wiki/Security.md) |
| **Self-hosted manufacturer utilities** | Web Jetadmin, CentreWare Web, BRAdmin [HP1; read 2026-10-30](https://support.hp.com/us-en/document/ish_3007439-2783223-16) [X1; read 2026-10-30](https://www.office.xerox.com/latest/SFTBR-05U.PDF) [B1; read 2026-10-30](https://www.brother.com.sg/en/solutions/bradmin-professional4) | Deep supported-vendor control: Web Jetadmin, MVE, SRDM [HP3; read 2026-10-30](https://kaas.hpcloud.hp.com/pdf-public/pdf_3017438_en-US-1.pdf) [L2; read 2026-10-30](https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-ace31bbf-90f9-41b6-9c50-5dfab3f44062-en.html) [SH1; read 2026-10-30](https://business.sharpusa.com/software/srdm) | MVE, HP Security Manager, SRDM; vendor/model boundaries [L1; read 2026-10-30](https://www.lexmark.com/en_us/solutions/print/lexmark-markvision-enterprise.html) [SEC2; read 2026-10-30](https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=4AA3-9275ENW) [SH1; read 2026-10-30](https://business.sharpusa.com/software/srdm) |
| **Commercial cloud / hybrid platforms** | Printanista, MPS Monitor, Princity, KPAX; Princity also on-premises [EC2; read 2026-10-30](https://www.ecisolutions.com/en-gb/products/printanista-hub/) [M3; read 2026-10-30](https://www.mpsmonitor.com/wp-content/uploads/2024/02/MPSMonitor_Security-Technical-Whitepaper.pdf) [PR2; read 2026-10-30](https://princity.com/solutions) [KP1; read 2026-10-30](https://www.kpax-manage.com/kpax/) | MPS Monitor via HP SDS; WXP; Lexmark CFM [M2; read 2026-10-30](https://www.mpsmonitor.com/sds/) [HP7; read 2026-10-30](https://workforceexperience.hp.com/pricing/) [L3; read 2026-10-30](https://publications.lexmark.com/publications/lexmark_solutions/Cloud/AG/pdf/Lexmark_CloudServices_AdminGuide_en.pdf) | WXP, Shield Guard, Xerox cloud fleet; SDS-dependent MPS Monitor [HP7; read 2026-10-30](https://workforceexperience.hp.com/pricing/) [KM4; read 2026-10-30](https://www.konicaminolta.asia/asia-en/solutions/bizhub-productivity-tools/shield-guard) [X3; read 2026-10-30](https://www.xerox.com/downloads/usa/en/services/brief/Xerox-Workplace-Cloud-Fleet-Management-Solution-Capability-Brief-ENUS.pdf) [M2; read 2026-10-30](https://www.mpsmonitor.com/sds/) |
| **Managed security service** | Evergreen asset inventory rather than MPS supplies emphasis [SEC5; read 2026-10-30](https://symphion.com/pricing/) | Symphion firmware service [SEC5; read 2026-10-30](https://symphion.com/pricing/) | Symphion and Canon PFCaaS [SEC6; read 2026-10-30](https://symphion.com/) [SEC7; read 2026-10-30](https://www.usa.canon.com/business/managed-services/managed-security) |

PrintMarshal is the only entry in the self-hosted, open-source row that goes beyond
monitoring into configuration, firmware orchestration and security assessment, and it is
vendor-neutral by design: one console, one data store and one audit trail for a mixed
fleet, running on the operator's own infrastructure with readable source. That is the gap
the conventional choices leave open.

The boundaries are equally documented. The manufacturers' tools have deeper real-device
control of their own hardware than PrintMarshal's shipped drivers; Lexmark MVE documents
automatic certificate installation and scheduled firmware, and HP Web Jetadmin documents
device-specific HP coverage, where PrintMarshal has the orchestration, dry runs, canaries
and audit but not yet a vendor writer proven on physical printers. Multivendor MPS
platforms have dealer, ERP and ticketing integrations and remote-site collectors that
PrintMarshal does not. The honest comparison is a neutral, inspectable core with explicit
"documented, observed, unknown" evidence per device, against deeper but vendor-bound or
service-bound alternatives. [PMD; read 2026-10-30](../wiki/Vendor-plugins-and-drivers.md)
[L1; read 2026-10-30](https://www.lexmark.com/en_us/solutions/print/lexmark-markvision-enterprise.html)
[HP1; read 2026-10-30](https://support.hp.com/us-en/document/ish_3007439-2783223-16)
[M2; read 2026-10-30](https://www.mpsmonitor.com/sds/)

## 7. Sources

All read **2026-10-30**. Manufacturer and project documentation is preferred; dealer-hosted
vendor documents and procurement listings are identified as such in the entries above.

- **PM** — [PrintMarshal README](../README.md). Read **2026-10-30**.
- **PMW** — [PrintMarshal wiki: current boundaries](../wiki/Home.md). Read **2026-10-30**.
- **PMD** — [PrintMarshal driver capability limits](../wiki/Vendor-plugins-and-drivers.md). Read **2026-10-30**.
- **PMA** — [PrintMarshal identity and scope limits](../wiki/Identity-and-access.md). Read **2026-10-30**.
- **ADJ** — [PaperCut product scope](https://www.papercut.com/products/). Read **2026-10-30**.
- **HP1** — [HP Web Jetadmin system requirements and third-party scope](https://support.hp.com/us-en/document/ish_3007439-2783223-16). Read **2026-10-30**.
- **HP2** — [HP Web Jetadmin solution brochure](https://h20195.www2.hp.com/v2/getpdf.aspx/4AA5-2718ENW.pdf). Read **2026-10-30**.
- **HP3** — [HP Web Jetadmin user guide](https://kaas.hpcloud.hp.com/pdf-public/pdf_3017438_en-US-1.pdf). Read **2026-10-30**.
- **HP4** — [HP Smart Device Services capability matrix (indexed extract; live page requires JavaScript)](https://jamanagement.hp.com/API/DeviceCapabilitiesUI/). Read **2026-10-30**.
- **HP5** — [HP SDS original eligibility announcement (2016; historical terms)](https://www.hp.com/us-en/newsroom/press-releases/2016/hp-inc--unleashes-next-generation-a3-printing-portfolio-to-disru.html). Read **2026-10-30**.
- **M2** — [MPS Monitor HP SDS functions and included cloud DCA](https://www.mpsmonitor.com/sds/). Read **2026-10-30**.
- **HP6** — [HP Command Center description](https://developers.hp.com/sites/default/files/CommandCenterDescription_0.pdf). Read **2026-10-30**.
- **HP7** — [HP WXP pricing and print plan comparison](https://workforceexperience.hp.com/pricing/). Read **2026-10-30**.
- **HP8** — [HP WXP printer fleet architecture, updated June 2026](https://learn.workforceexperience.hp.com/docs/overview-of-print). Read **2026-10-30**.
- **X1** — [Xerox CentreWare Web brochure](https://www.office.xerox.com/latest/SFTBR-05U.PDF). Read **2026-10-30**.
- **X2** — [Xerox MPS architecture and Device Manager](https://www.xerox.com/downloads/usa/en/services/brochure/Federal_Government_Capability_Brief_EN_US.pdf). Read **2026-10-30**.
- **X3** — [Xerox Workplace Cloud Fleet Management capability brief](https://www.xerox.com/downloads/usa/en/services/brief/Xerox-Workplace-Cloud-Fleet-Management-Solution-Capability-Brief-ENUS.pdf). Read **2026-10-30**.
- **X4** — [Xerox Workplace Cloud fleet licensing terms](https://xwc.services.xerox.com/TermsOfUse). Read **2026-10-30**.
- **X5** — [Xerox Workplace Cloud security guide](https://security.business.xerox.com/wp-content/uploads/2020/10/Xer_WorkplaceCloud-5_6_1_Security_Guide.pdf). Read **2026-10-30**.
- **R1** — [Ricoh Device Manager NX Lite](https://www.ricoh-usa.com/en/products/pd/software/device-set-up-and-management/device-usage-and-monitoring/ricoh-device-manager-nx-lite). Read **2026-10-30**.
- **R2** — [Ricoh NX Pro/Enterprise brochure (legacy)](https://assets.ctfassets.net/j2jqn9lauv41/5Ckxovn2HWJJPmodJda1K8/2fd5fa9300a730de7ffa6045a5456e86/DMNX-Enterprise-Brochure.pdf). Read **2026-10-30**.
- **R6** — [Ricoh 2026 GSA reseller contract schedule; procurement-specific, not retail](https://www.gsaadvantage.gov/ref_text/GS25F0051S/110BBQ.3WQO6E_GS-25F-0051S_GSACONTRACTGS25F0051SMOD233.PDF). Read **2026-10-30**.
- **R3** — [Ricoh Streamline NX](https://www.ricoh-usa.com/en/products/pd/software/device-set-up-and-management/ricoh-streamline-nx). Read **2026-10-30**.
- **R5** — [Ricoh @Remote licensed connector and model limits](https://docs.ricoh-sdc.com/24r1-en/Content/AdministrationTopics/DeviceMonitor/MonitorwithAtRemote.htm). Read **2026-10-30**.
- **R4** — [Ricoh @Remote](https://www.ricoh.com/products/at-remote). Read **2026-10-30**.
- **L1** — [Lexmark Markvision Enterprise product and free licence](https://www.lexmark.com/en_us/solutions/print/lexmark-markvision-enterprise.html). Read **2026-10-30**.
- **L2** — [Lexmark MVE 4.7 administrator overview](https://publications.lexmark.com/publications/lexmark_solutions/MarkVision/AG/v4.7/html/en/GUID-ace31bbf-90f9-41b6-9c50-5dfab3f44062-en.html). Read **2026-10-30**.
- **L3** — [Lexmark Cloud Services administrator guide](https://publications.lexmark.com/publications/lexmark_solutions/Cloud/AG/pdf/Lexmark_CloudServices_AdminGuide_en.pdf). Read **2026-10-30**.
- **K3** — [Kyocera Net Admin 3.8 supplement](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/us/documents/utility-guides/KNAENUG__SupplementR38_1_2019_03__pdf.download.pdf). Read **2026-10-30**.
- **K1** — [Kyocera Device Manager](https://www.kyoceradocumentsolutions.us/en/products/software/KYOCERADEVICEMANAGER.html). Read **2026-10-30**.
- **K2** — [Kyocera software portfolio: KDM free utility](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/master/documents/brochures-and-specs/Kyocera_Software_Portfolio_Brochure_pdf.download.pdf). Read **2026-10-30**.
- **K4** — [Kyocera Fleet Services](https://kyoceradocumentsolutions.com.au/kyocera-fleet-services). Read **2026-10-30**.
- **K5** — [Kyocera Fleet Services gateway architecture](https://www.kyoceradocumentsolutions.us/content/dam/download-center-americas-cf/us/documents/user-guides/KFSUserGuideENOG_HELP_SYSTEM_pdf.download.pdf). Read **2026-10-30**.
- **KM1** — [Konica Minolta Net Care scope](https://www.konicaminolta.co.uk/en-gb/software/pagescope-netcare-device-manager). Read **2026-10-30**.
- **KM2** — [Konica Minolta Net Care end-of-support notice](https://www.btapac.konicaminolta.com/). Read **2026-10-30**.
- **KM3** — [Konica Minolta Fleet RMM utility description](https://www.btapac.konicaminolta.com/utilities/index.html). Read **2026-10-30**.
- **KM4** — [Konica Minolta Shield Guard](https://www.konicaminolta.asia/asia-en/solutions/bizhub-productivity-tools/shield-guard). Read **2026-10-30**.
- **KM5** — [Konica Minolta CS Remote Care](https://www.konicaminolta.eu/eu-en/digital-workplace/software-services/cs-remote-care-device-management?aliaspath=%2FSoftware%2FDevice-management%2FCS-Remote-Care-device-management). Read **2026-10-30**.
- **C1** — [Canon iW EMC 4.1.2 free distribution and features; March 2023 service guide](https://downloads.canon.com/bicg2023/guides/svc-imagerunner-advance-dx-8700-series-sg07.pdf). Read **2026-10-30**.
- **C2** — [Canon US iW EMC discontinued storefront](https://www.usa.canon.com/shop/p/imageware-enterprise-management-console). Read **2026-10-30**.
- **C5** — [Canon-origin device overview including third-party MIB monitoring; procurement copy](https://info.omniapartners.com/hubfs/Public%20-%20Supplier%20Contract%20Information/Canon%20Solutions%20America/Canon_Multi-Function%20Devices.pdf). Read **2026-10-30**.
- **C4** — [Canon imageWARE Remote and CDS](https://community.usa.canon.com/html/assets/2023_CUSA_BICG_SS_Remote_Services.pdf). Read **2026-10-30**.
- **B1** — [Brother BRAdmin Pro 4 feature and coverage](https://www.brother.com.sg/en/solutions/bradmin-professional4). Read **2026-10-30**.
- **B2** — [Brother BRAdmin Pro 4 independent test distributed by Brother](https://www.brother.co.za/-/media/ap2/south-africa/landing-page/multifunction-printers/solutions/brother-bradmin-professional-4.pdf). Read **2026-10-30**.
- **B4** — [Brother BRAdmin/Azure manuals](https://support.brother.com/g/b/manualtop.aspx?c=us&lang=en&prod=group2). Read **2026-10-30**.
- **B3** — [Brother fleet tools overview](https://www.brother.com.my/en/solutions/fleet-management-detail). Read **2026-10-30**.
- **E1** — [Epson Device Admin free software and batch firmware features](https://www.epson.com.sg/software-solutions-details). Read **2026-10-30**.
- **E2** — [Epson Remote Services: UK free service](https://www.epson.co.uk/en_GB/epson-remote-services). Read **2026-10-30**.
- **E3** — [Epson Remote Services architecture and maintenance](https://mediaserver.goepson.com/ImConvServlet/imconv/f6a782c79d7138aad1307feda3697502cc762456/original?assetDescr=Epson_Remote_Services_Software_Specifications&use=banner). Read **2026-10-30**.
- **SH1** — [Sharp SRDM features and third-party restrictions](https://business.sharpusa.com/software/srdm). Read **2026-10-30**.
- **SH2** — [Sharp Synappx Manage cloud platform](https://business.sharpusa.com/synappx-support). Read **2026-10-30**.
- **T1** — [Toshiba e-BRIDGE CloudConnect functions](https://www.toshibatec.co.th/en/solution/e-bridge-cloud-connect/). Read **2026-10-30**.
- **T3** — [Toshiba e-BRIDGE SKY Suite](https://www.toshibatec.com/products_overseas/ebridge_cs/). Read **2026-10-30**.
- **T2** — [Toshiba Elevate Sky services brochure](https://business.toshiba.com/app/uploads/2023/11/Toshiba-Elevate-Cloud-Services-Brochure.pdf). Read **2026-10-30**.
- **T4** — [Toshiba e-BRIDGE Fleet Management System licence page](https://www.toshibatec.nl/service-en-support/licentievoorwaarden/). Read **2026-10-30**.
- **O1** — [OKI PrintSuperVision](https://www.oki.com/sg/printing/services-and-solutions/smart-solutions/printsupervision/index.html). Read **2026-10-30**.
- **O2** — [OKI reseller: legacy PSV Professional/Enterprise free downloads](https://www.m5digital.ca/services/printsupervision/). Read **2026-10-30**.
- **Z1** — [Zebra PPME compatibility and trial](https://www.zebra.com/us/en/support-downloads/software/printer-software/profile-manager-enterprise.html). Read **2026-10-30**.
- **Z2** — [Zebra PPME administrator guide](https://www.zebra.com/content/dam/support-dam/en/documentation/unrestricted/guide/software/ppme-ug-en.pdf). Read **2026-10-30**.
- **FU1** — [Fujifilm Device Setup free tool and deployment](https://support-fb.fujifilm.com/driver_downloads/DLS-userguide_en_for_FF.pdf). Read **2026-10-30**.
- **FU2** — [Fujifilm ApeosWare model/function matrix](https://asset-fb.fujifilm.com/www/fbkr/files/2026-01/3c035779b3d1371901c9979117b4f21b/support_models.pdf). Read **2026-10-30**.
- **C3** — [Canon iW EMC 4.2.1 manual; JavaScript shell](https://oip.manual.canon/USRMB-1146-zz-DS-enUV/). Read **2026-10-30**.
- **EC3** — [FMAudit vendor technical whitepaper, dealer-hosted copy](https://www.marconet.com/hubfs/FMAudit%20Technical%20Whitepaper%20NEW%204.5.pdf). Read **2026-10-30**.
- **EC1** — [ECI Printanista consolidation announcement](https://www.ecisolutions.com/news/eci-launches-printanista-new-cloud-based-managed-print-solution/). Read **2026-10-30**.
- **EC4** — [ECI PrintFleet security overview, dealer-hosted copy](https://www.nwpsi.com/wp-content/uploads/2025/02/Printfleet-pdf-for-web-link-Final.pdf). Read **2026-10-30**.
- **EC2** — [ECI Printanista Hub](https://www.ecisolutions.com/en-gb/products/printanista-hub/). Read **2026-10-30**.
- **M5** — [MPS Monitor-hosted 2021 Quocirca report; historical unmanaged free monitoring terms](https://www.mpsmonitor.com/wp-content/uploads/2021/05/Quocirca_Cloud_Print_Services_2021_MPS_Monitor_Excerpt.pdf). Read **2026-10-30**.
- **M1** — [MPS Monitor enterprise per-device monthly billing](https://www.mpsmonitor.com/enterprise-prices/). Read **2026-10-30**.
- **M3** — [MPS Monitor technical security and collector whitepaper](https://www.mpsmonitor.com/wp-content/uploads/2024/02/MPSMonitor_Security-Technical-Whitepaper.pdf). Read **2026-10-30**.
- **N1** — [Nexera technician-based monthly investment](https://www.nexera.net/getting-started/). Read **2026-10-30**.
- **M4** — [MPS Monitor/Valsoft and Nexera organizational relationship](https://www.mpsmonitor.com/blog/valsoft-kevin-tetu-president-of-managed-print-division/). Read **2026-10-30**.
- **TH1** — [3manager multibrand platform and API](https://www.3manager.com/). Read **2026-10-30**.
- **TH2** — [3manager plan tiers](https://www.3manager.com.au/plans). Read **2026-10-30**.
- **TH3** — [3manager local collectors](https://www.3manager.com.au/). Read **2026-10-30**.
- **PR1** — [Princity pricing](https://princity.com/pricing). Read **2026-10-30**.
- **PR2** — [Princity features, deployment and API](https://princity.com/solutions). Read **2026-10-30**.
- **PT3** — [Print Tracker local collectors](https://docs.printtrackerpro.com/docs/guides/installation). Read **2026-10-30**.
- **PT4** — [Print Tracker monthly billing and free unmanaged quarterly readings; guide marked under construction](https://docs.printtrackerpro.com/docs/guides/billing). Read **2026-10-30**.
- **PT2** — [Print Tracker integration pricing and per-entity transfer blocks](https://docs.printtrackerpro.com/docs/guides/integrations). Read **2026-10-30**.
- **PT1** — [Print Tracker published pricing](https://landing.printtrackerpro.com/pricing). Read **2026-10-30**.
- **KP1** — [KPAX deployment; on-premises described as planned](https://www.kpax-manage.com/kpax/). Read **2026-10-30**.
- **KP2** — [KPAX Koins charging model](https://www.kpax-manage.com/pricing/). Read **2026-10-30**.
- **EK1** — [EKM Insight product and collector architecture](https://www.ekmglobal.com/). Read **2026-10-30**.
- **FI1** — [Fiery IQ, formerly EFI IQ, free and paid apps](https://www.fiery.com/products/automation/fiery-iq/). Read **2026-10-30**.
- **FI2** — [Fiery IQ security whitepaper and paid Manage subscription](https://help.fiery.com/Security/45218169_Rev_B_Fiery_IQ_SecurityWhitePaper.pdf). Read **2026-10-30**.
- **CD1** — [CompuData URL redirect to CBIZ Technology](https://www.compudata.com/). Read **2026-10-30**.
- **PX1** — [Prismatic official embedded integration platform](https://prismatic.io/). Read **2026-10-30**.
- **I1** — [Lansweeper printer features](https://www.lansweeper.com/assets/hardware/printer/). Read **2026-10-30**.
- **I2** — [Lansweeper plans; currency labels missing in text extraction](https://www.lansweeper.com/pricing/). Read **2026-10-30**.
- **I3** — [NinjaOne NMS custom SNMP FAQ](https://www.ninjaone.com/docs/network-management-system/nms/). Read **2026-10-30**.
- **I4** — [NinjaOne pricing](https://www.ninjaone.com/pricing/). Read **2026-10-30**.
- **I5** — [NinjaOne Windows SNMP probe architecture](https://www.ninjaone.com/rmm/snmp-monitoring/). Read **2026-10-30**.
- **I6** — [PRTG pricing](https://www.paessler.com/pricing). Read **2026-10-30**.
- **I7** — [PRTG SNMP Printer sensor](https://www.paessler.com/manuals/prtg/snmp_printer_sensor). Read **2026-10-30**.
- **I8** — [Zabbix printer integrations; community quality warning](https://www.zabbix.com/integrations/printers). Read **2026-10-30**.
- **I9** — [Zabbix AGPL licence](https://www.zabbix.com/license). Read **2026-10-30**.
- **I10** — [LibreNMS repository and licence](https://github.com/librenms/librenms). Read **2026-10-30**.
- **I11** — [LibreNMS printer supplies issue, September 2026](https://github.com/librenms/librenms/issues/20664). Read **2026-10-30**.
- **I12** — [Checkmk pricing](https://checkmk.com/pricing). Read **2026-10-30**.
- **I13** — [Checkmk generic printer supplies check](https://checkmk.com/integrations/printer_supply). Read **2026-10-30**.
- **I14** — [Nagios XI pricing](https://www.nagios.com/pricing-plans/). Read **2026-10-30**.
- **I15** — [Nagios XI perpetual licensing](https://www.nagios.com/products/nagios-xi/). Read **2026-10-30**.
- **I16** — [Nagios printer status and low-toner checks](https://assets.nagios.com/downloads/nagioscore/docs/nagios-3.pdf). Read **2026-10-30**.
- **I23** — [Nagios Core free GPLv2 edition](https://www.nagios.org/projects/nagios-core/). Read **2026-10-30**.
- **I17** — [Datadog NDM prices and required infrastructure poller](https://www.datadoghq.com/pricing/?product=network-monitoring). Read **2026-10-30**.
- **I18** — [Datadog SNMP metrics architecture](https://docs.datadoghq.com/network_monitoring/devices/snmp_metrics/). Read **2026-10-30**.
- **I24** — [Datadog profile-based/custom metric collection](https://docs.datadoghq.com/network_monitoring/devices/profiles/device_profiles/). Read **2026-10-30**.
- **I19** — [Auvik billable and free monitored devices](https://support.auvik.com/hc/en-us/articles/201515670-How-much-does-Auvik-cost). Read **2026-10-30**.
- **I20** — [Auvik discovery coverage](https://www.auvik.com/network-management-software/). Read **2026-10-30**.
- **I21** — [Domotz September 2026 pricing](https://www.domotz.com/pricing.php). Read **2026-10-30**.
- **I22** — [Domotz SNMP sensor coverage](https://help.domotz.com/monitoring-management/snmp-monitoring/). Read **2026-10-30**.
- **OS7** — [Tylan SNMP printer monitoring plugin](https://github.com/Tylan/check_snmp_printer). Read **2026-10-30**.
- **SEC3** — [PRET research toolkit](https://github.com/RUB-NDS/PRET). Read **2026-10-30**.
- **SEC12** — [Greenbone architecture and management API](https://greenbone.github.io/docs/latest/background). Read **2026-10-30**.
- **OS1** — [Netdisco repository and BSD licence](https://github.com/netdisco/netdisco). Read **2026-10-30**.
- **OS2** — [GLPI Agent network inventory](https://glpi-agent.readthedocs.io/en/1.21/tasks/network-inventory.html). Read **2026-10-30**.
- **OS10** — [GLPI Agent repository and licence](https://github.com/glpi-project/glpi-agent). Read **2026-10-30**.
- **OS3** — [FusionInventory network inventory](https://documentation.fusioninventory.org/FusionInventory_tasks/networkinventory/). Read **2026-10-30**.
- **OS4** — [FusionInventory Agent repository and GPL licence](https://github.com/fusioninventory/fusioninventory-agent). Read **2026-10-30**.
- **OS5** — [FusionInventory project news](https://fusioninventory.github.io/news.html). Read **2026-10-30**.
- **OS6** — [Prometheus SNMP exporter repository and Apache licence](https://github.com/prometheus/snmp_exporter). Read **2026-10-30**.
- **OS8** — [Tylan plugin version 2.0.1 dated April 2023](https://github.com/Tylan/check_snmp_printer/blob/master/check_snmp_printer). Read **2026-10-30**.
- **OS9** — [PrinterMonitor proxy repository; preliminary scope](https://github.com/printmonitor/printermonitor-proxy). Read **2026-10-30**.
- **SEC4** — [PRET commit history; latest visible July 13, 2023](https://github.com/RUB-NDS/PRET/commits/master/). Read **2026-10-30**.
- **SEC1** — [HP Security Manager licence and limited demonstration](https://kaas.hpcloud.hp.com/pdf-public/pdf_1980161_en-US-1.pdf). Read **2026-10-30**.
- **SEC2** — [HP Security Manager risk assessment and remediation datasheet (indexed extract)](https://h20195.www2.hp.com/v2/GetDocument.aspx?docname=4AA3-9275ENW). Read **2026-10-30**.
- **SEC5** — [Symphion per-device service pricing and inclusions](https://symphion.com/pricing/). Read **2026-10-30**.
- **SEC6** — [Symphion vendor-neutral security service (indexed extract)](https://symphion.com/). Read **2026-10-30**.
- **SEC7** — [Canon Printer Fleet Cybersecurity as a Service](https://www.usa.canon.com/business/managed-services/managed-security). Read **2026-10-30**.
- **SEC8** — [Nmap PJL script behavior and licence reference](https://nmap.org/nsedoc/scripts/pjl-ready-message.html). Read **2026-10-30**.
- **SEC9** — [Tenable printer detection and scan exclusion](https://www.tenable.com/plugins/nessus/44920). Read **2026-10-30**.
- **SEC10** — [Tenable Samsung/Dell printer SNMP backdoor test](https://www.tenable.com/plugins/nnm/6639). Read **2026-10-30**.
- **SEC16** — [Tenable published annual Nessus purchase prices](https://www.tenable.com/buy). Read **2026-10-30**.
- **SEC13** — [Greenbone printer exclusion limitations](https://forum.greenbone.net/t/printer-isnt-ignored-in-full-and-fast-scan-config/13212). Read **2026-10-30**.
- **SEC14** — [Titania Nipper documented supported device types](https://www.titania.com/support). Read **2026-10-30**.
- **SEC15** — [Titania Nipper InfraSight configuration-assessment scope](https://www.titania.com/nipper-infrasight). Read **2026-10-30**.
- **PMF** — [PrintMarshal firmware](../wiki/Firmware.md). Read **2026-10-30**.
- **PMS** — [PrintMarshal security and PKI limits](../wiki/Security.md). Read **2026-10-30**.
