# PrintMarshal: free self-managed core and a managed cloud Pro service

**Harry · Research analyst · 30 October 2026**

## Recommendation and evidence standard

Keep PrintMarshal's self-hosted product and on-site relay under Apache-2.0. Sell **PrintMarshal Pro as managed fleet operations**: hosting, recovery, maintained connections to external systems, service-backed intelligence and support. Do not introduce a paid self-hosted feature edition. The commercial distinction should be who operates the system and what continuing service PrintMarshal supplies.

Start with regional customer isolation, an outbound relay and reliable monitoring. Introduce directory lifecycle automation, executive reporting and maintained ticketing integrations next. Defer cross-customer AI training, a native mobile app and 24-hour support until demand and operating capacity justify them. Existing authentication, forecasting, security assessment, campaigns, integrations and exports remain free.

This report distinguishes **repository observations**, **external evidence**, and **proposals/estimates**. All external citations link to their URLs; **read 2026-10-30** means the date consulted, not a vendor effective date. Live pages and search-index extracts can disagree. Current primary evidence takes precedence; unresolved or inaccessible prices are identified. Monetary amounts are vendor list prices only where explicitly attributed; PrintMarshal prices and infrastructure budgets are proposals, not discovered prices or quotations. No currency conversions are assumed.

## 1. The product we must preserve

The review covered [README](../README.md), the 17 Markdown files in [docs](../docs/), the 18 Markdown files in [wiki](../wiki/), and package names/documentation comments in `internal/`. The build specification and UI briefs describe aspirations as well as completed work. The newer wiki's implementation qualifications matter more than broad README statements.

The free baseline already includes discovery over mDNS/WS-Discovery and active scans; SNMP/IPP/eSCL identification; inventory, groups, lifecycle and naming; polling, telemetry, traps, alerts; stock tracking, replacement detection and supply forecasts; configuration snapshots, diffs, drift and guarded writes; security policies, assessment, remediation orchestration, enrollment and rotation; firmware packages and staged campaigns; reports and CSV/JSON; REST/WebSocket APIs; notifications, durable notification retries, SIEM export and incremental sync; SQLite/PostgreSQL; packaging, backup helpers, catalogue/vendor plugins and the simulator. These must not become Pro entitlements. See the comparison below and [wiki feature map](../wiki/Home.md).

Four qualifications materially affect the business plan:

1. **Identity is already enterprise-capable.** OIDC, SAML, LDAP/Active Directory and RADIUS sign-in, claim/group-to-role mapping, API tokens and audit events exist. Provider branding is not a new paid feature. See [identity guide](../wiki/Identity-and-access.md), `internal/auth/{oidc,saml,ldap,radius}` and `internal/api/directory.go`.
2. **Group scopes are not customer isolation.** Streams, jobs, settings and several aggregates/sync surfaces are fleet-wide or incompletely scope-filtered. Do not sell separate MSP customer accounts implemented merely as groups. See the identity guide's “Exact scope limitations” and `internal/api/scope.go`.
3. **PostgreSQL is implemented; distributed execution is not.** There is one active scheduler/runner. Ordinary interrupted jobs fail rather than automatically resume; firmware has its own recovery state. The live bus is not a durable replay log; SIEM queues can drop. See [architecture](../wiki/Architecture.md), [events](../wiki/Events-notifications-and-integrations.md), `internal/jobs` and `internal/events`.
4. **Orchestration does not prove hardware transport support.** Firmware campaigns exist, but shipped physical-vendor plugins do not deliver firmware or install certificates; the implemented delivery driver is the explicitly enabled simulator transport. Limited standards configuration writes exist. Security assessment/history APIs work, while `/api/reports/compliance` remains an unavailable placeholder. Cloud hosting cannot remove these limitations. See [firmware](../wiki/Firmware.md), [security](../wiki/Security.md), [monitoring/reports](../wiki/Monitoring-and-alerts.md), `internal/plugins/README.md` and `internal/reports/compliance.go`.

## 2. Reference models: what is actually free and paid

Prices below are starting points and exclude excess usage/taxes unless stated. “No incident established” means this review found no specific controversy to substantiate; it does not certify community satisfaction. The lesson column is Harry's analysis.

| Reference | Free / self-managed boundary | Paid offer and pricing evidence | Observed change, friction and PrintMarshal lesson |
| --- | --- | --- | --- |
| **Canvas / Instructure** | Canvas LMS repository is AGPLv3 and provides installation instructions. A free hosted educator offering is a separate product from downloading the server. | Institutional Canvas now offers Core, Plus and Next, with services/support and personalized quotations; there is no universal public per-student list price in the cited tier page. | Its FAQ documents commercial dual licensing and proprietary extensions: this is not a promise of absolute hosted/source parity. Copy the managed-subscription proposition, not its copyleft/CLA structure. [B01], [B02], [B03] (read 2026-10-30). |
| **GitLab** | Community source control/CI and a Free service/self-managed tier; hosted allowances differ from operating your own infrastructure. | Premium **$29/user/month, billed annually**; Ultimate currently custom-priced. Paid tiers add advanced CI/CD, project management, support, and security/compliance capabilities. | Premium increased from $19 to $29 in April 2023, with a transition rate. That illustrates budget shock even with notice. Publish renewal rules; do not copy paid security gates into PrintMarshal's existing core. [B05], [B06] (read 2026-10-30). |
| **Grafana** | Self-managed visualization, dashboards and alerting; Cloud Free has usage allowances and 14-day telemetry retention. | Cloud Pro **$19/month platform fee plus usage**; Enterprise starts at **$25,000/year commitment**. Pro includes longer retention and business-hours email support; enterprise plugins have separate economics. | Core projects moved from Apache-2.0 to AGPLv3 in 2021. AGPL remains open source, but this license change is outside PrintMarshal's intent. Copy paid operation/retention, not relicensing or withholding already-free integrations. [B07], [B08], [B09] (read 2026-10-30). |
| **Sentry** | Free self-hosting of most monitoring capabilities; source uses FSL, which later converts to Apache-2.0, rather than an unrestricted current Apache release. Seer is unavailable in ordinary self-hosted deployments. | Free Developer service; Team and Business combine base subscriptions with data-volume purchases. A live secondary pricing report lists **$26/$80 monthly equivalents with annual billing**. The primary pricing URL could not be read by this tool, so these amounts are secondary evidence, not independently confirmed list prices. | Sentry records BSD → BSL → FSL and debate about calling restricted code open source. Separate “free to run,” “source available” and “open source.” Hosted AI is a service opportunity, not a reason to relabel a restricted core. [B10], [B11], [B12], [B13] (read 2026-10-30). |
| **Mattermost** | Team Edition remains an open-source path; today's standard download instead installs **Entry**, a limited-use Enterprise edition for evaluation. Do not confuse that free binary with the OSS edition. | Professional, Enterprise and Enterprise Advanced are currently sales-led; annual prepaid seat licensing, not a verified public $10/user offer. They charge for collaboration controls, administration, resiliency, enterprise operations and support. | v11 notes remove GitLab SSO and Playbooks functionality from Team Edition. This is a concrete warning about narrowing the free experience. Keep free downloads obvious and preserve PrintMarshal's existing SSO. [B14], [B15], [B16], [B17] (read 2026-10-30). |
| **n8n** | Community self-hosting under the **Sustainable Use License**: free internal use, with commercial requirements for specified hosting/embedding/external-user scenarios. This is source available, not an Apache-like OSS comparator. | Cloud Starter **€20/month annually**, 2,500 executions; Pro **€50**, 10,000 executions. Self-hosted Business **€667/month annually** adds SSO/LDAP and environments; Enterprise is quoted. Execution counts, not seats or workflow steps, drive the main meter. | Licensing restrictions make deployment/business-use boundaries consequential; its paid SSO illustrates a boundary PrintMarshal cannot copy. Copy comprehensible service metering, not commercial-use restrictions. No distinct backlash episode is asserted here. [B18], [B19] (read 2026-10-30). |
| **Plausible** | Free AGPL Community Edition; infrastructure is operator-managed and releases are twice yearly. Cloud has features absent from CE, including funnels, user journeys, ecommerce goals, SSO and the Sites API. | Traffic-volume subscriptions; current homepage starts Starter at **$9/month for 10,000 pageviews**, with higher Growth/Business tiers and enterprise quotations. **30-day no-card trial**. | MIT → AGPL in 2020, followed by the CE/cloud split. Its own comparison makes feature divergence explicit. Publish PrintMarshal's boundary just as clearly, but charge for delivery/maintenance rather than moving existing functionality. [B20], [B21], [B22], [B23] (read 2026-10-30). |
| **Netdata** | Agent is GPLv3+ and includes collection, storage, ML and alerting; the bundled dashboard is separately licensed under NCUL1 and is not open source. Cloud Community currently permits five active connected nodes for personal/noncommercial use. | Business **$4.50/node/month annually**, **$6 monthly**; paid cloud adds organizational controls, SCIM, integrations and AI service capabilities. Enterprise On-Prem is quoted. Metrics remain on premises in its distributed model. | Official 2024 announcement retired Early Bird/Unlimited Community plans into a trial then limited Community. Avoid unlimited/lifetime promises that must later shrink; keep PrintMarshal's local UI independent of a cloud account. [B24], [B25], [B26] (read 2026-10-30). |
| **Zabbix** | Full monitoring software without a paid feature edition. Releases from 7.0 use AGPLv3; earlier versions GPLv2. | Paid support, consulting, implementation and training; cloud capacity priced by new values/second, publicly starting at **$50/month for 50 NVPS**. Services beyond published cloud capacity require scope/quotation. | License changed, but remains OSS. This is the strongest services-first reference alongside hosted Canvas. Keep capabilities free and monetize operational responsibility. No particular backlash episode established here. [B27], [B28], [B29] (read 2026-10-30). |
| **Metabase** | Free AGPL self-hosted analytics, query builder and dashboards; current comparison also includes AI with a customer's own model. | Cloud Starter **$100/month including five users**; Pro **$575 including ten**, monthly billing, with $12/additional user cited by its current article; yearly discounts shown separately. Paid governance includes SAML/SCIM and finer data controls; Enterprise custom. | Not every paid analytics capability needs cloud infrastructure. Do not sell basic forecasts or generic identity again under an AI/enterprise label. No specific controversy established in these sources. [B30], [B31], [B32] (read 2026-10-30). |
| **Cal.com** | **Current correction:** April 2026 announcement split out **MIT Cal.diy**, a community self-hosted edition with enterprise features removed. The commercial codebase is now closed. Older AGPL descriptions are stale for the current offer. | Hosted Individuals free; Teams **$12/user/month annually**, Organizations **$28 annually**; Enterprise custom. Team collaboration, organization controls, SAML/SCIM, support and dedicated deployment distinguish paid plans. | The community repo explicitly excludes Teams, Organizations, Workflows and SSO/SAML. This major split demonstrates why a license promise alone is insufficient without a useful maintained free product. Do not follow that retreat. [B33], [B34], [B35] (read 2026-10-30). |
| **PostHog** | Freely self-hostable MIT deployment, explicitly unsupported; no tagged self-hosted releases. Cloud Free currently includes one million analytics events/month and unlimited team members. | Per-product usage beyond allowances, billing limits, longer retention and support; advanced add-ons. No single per-seat subscription captures its model. | Current docs say no self-hosted CVE publication/versioned release process. This is evidence of reduced self-managed operational commitment, even without relicensing. Preserve PrintMarshal release artifacts and security advisories. [B36], [B37] (read 2026-10-30). |
| **Supabase** | Self-hosted Postgres-based backend stack; repository license Apache-2.0, with component licenses to check separately. Operating the stack, secrets, backups and updates is the user's job. | Cloud Free with quotas; Pro **from $25/month**, one Micro covered by compute credits; extra projects/compute/usage cost more. Team **from $599/month** adds dashboard SSO, priority support and compliance-related access; Enterprise quoted. | Public self-hosting feedback records operational/documentation gaps, not proof that every install fails. Its platform-dashboard SSO is a useful control-plane distinction, but PrintMarshal should include safe hosted sign-in in every paid plan. [B38], [B39], [B40], [B41] (read 2026-10-30). |

Canvas deserves an additional current qualification. On 14 October 2026, a community member complained that public source updates had stalled after an incident. An Instructure employee replied that an open-source update was still forthcoming. That supports **an unresolved source-publication concern**, not a finding that Canvas's AGPL rights have disappeared or a verified account of the incident's resolution. A managed-service business must fund dependable source releases too. [B04] (read 2026-10-30).

The transferable pattern is a useful autonomous product plus a convenient operated service. The nontransferable pattern is a license restriction preventing competing hosting: Apache-2.0 grants broad use, modification and distribution rights, including derivatives under additional terms subject to its conditions. Competing managed services are a business risk PrintMarshal accepts; it should compete through trust, expertise and delivery. [L01] (read 2026-10-30).

Proposed community covenant: keep released and future core work Apache-2.0; publish security fixes and ordinary release artifacts for self-managers; require no registration, cloud connection or device cap locally; keep protocols, drivers, exports, safety controls and the local UI useful offline. Publish service limits and renewal notice periods. Apply security/isolation fixes to the shared OSS implementation wherever relevant. Do not require copyright reassignment merely to enable a future relicense.

## 3. Proposed feature boundary

**Legend:** **Both** = existing/free capability also operated in Pro; **Free** = local/community capability; **Pro** = proposed new managed service. A dash means no included managed service, not a license prohibition on implementing one. All Pro additions below are recommendations, not claims of current availability. Evidence for the existing column is the repository review above.

| Feature area | Free · open source · self-hosted | Pro · cloud subscription |
| --- | --- | --- |
| Discovery, identification, capabilities, model catalogue and vendor drivers | **Both:** all current protocols/evidence; future ordinary driver improvements stay free | Same engine through local collectors; managed collector enrollment/health |
| Inventory, search, lifecycle, naming, static/rule groups | **Both:** complete existing functions | Same plus centrally operated site inventory; no artificial local device cap |
| Polling, trays, supplies, counters and firmware inventory | **Both:** scheduled/on-demand monitoring | Managed ingestion, collector health and service availability |
| SNMP traps and printer syslog reception | **Both:** local listeners and attribution | Collector receives locally and forwards selected normalized events; no internet-exposed printer listener |
| Alerts and notifications | **Both:** raise/ack/clear, rules, SMTP, Slack/Teams/Discord/webhooks, notification outbox | Managed delivery infrastructure, destination setup, incident routing and delivery monitoring |
| Consumables and stock | **Both:** stock history, links, replacements, CSV, days remaining/reorder forecasts | Same functionality, operated; cross-site supply planning can be a new service |
| Configuration | **Both:** snapshots, diff, profiles, drift, apply/revert, dry runs and audit | Relay execution and central orchestration; same supported device capabilities |
| Security policy and assessment | **Both:** probes, policies, compliance scores/history | Managed scheduling/evidence service; no paid gate on baseline checks |
| Remediation, purpose-bound credentials, enrollment and rotation | **Both:** current orchestration and actual driver limits | Local secret custody, centrally approved work; no claim of new hardware support |
| Firmware packages/campaigns | **Both:** canaries, windows, thresholds, verification and recovery state | Managed coordination/storage; not a firmware entitlement or universal update transport |
| Reports and exports | **Both:** current utilisation, health, supplies, alerts, firmware, CSV/JSON; compliance report placeholder stays a core completion item | Managed report execution; new scheduling/delivery service below |
| Enterprise authentication | **Both:** local, OIDC, SAML, LDAP/AD, RADIUS; existing role mappings remain free | Hosted use of the same standards; legacy directory access through an explicitly secured local path if needed |
| RBAC, scoped API tokens, sessions and encrypted secrets | **Both:** existing controls, with documented scope limitations | Strong customer boundary and organization/MSP authorization added; do not substitute groups for isolation |
| REST, WebSocket, incremental sync, SIEM export, Prometheus | **Both:** current APIs/export formats remain free | Service endpoints, managed ingestion/connectors and monitored delivery; new replay protocol required |
| Simulator, fixtures, development tools | **Free:** all existing simulator capabilities | Hosted sandbox/demo may use the simulator; no paid simulator license |
| Optional CUPS/IPP print-provider bridge | **Free:** local host convenience, off by default | May stay on the relay host; print jobs do not traverse Pro |
| SQLite/PostgreSQL, packaging, backup/restore and upgrades | **Both:** tools/instructions; operator runs them | PrintMarshal performs upgrades, restore drills and backups against defined targets |
| Multiple locations within one organization's routed network | **Both:** existing targets/groups and independent instances remain usable | Regional aggregation, outbound relays and remote operating convenience |
| MSP/dealer customer accounts and delegated dashboards | No current isolated multi-customer service; separate instances possible | **Pro:** operated tenant registry, isolation, delegated access, consolidated views and billing |
| Entra/Google directory lifecycle automation | Existing claim/LDAP group mapping stays free; proposed shared SCIM protocol components should be OSS | **Pro:** maintained consent/onboarding, tenant directory sync, group lookup/mapping reconciliation, SCIM provisioning/deprovisioning, token refresh and failure queues |
| Conditional access and hosted control-plane SSO | Generic OIDC/SAML already permits IdP policy use | **Both, hosted twist:** verified tenant routing, policy setup/testing and session controls; hosted console SSO included, not a premium login tax |
| AI supplies and anomaly analysis | Existing statistical forecasting stays free; local/BYO-model implementations permitted | **Pro:** managed training/inference, confidence monitoring and compute budgets; earn value beyond the existing baseline |
| Cross-fleet benchmarking | Local reports remain free | **Pro:** optional aggregation across consenting fleets, minimum cohort size and suppression of identifying data |
| Natural-language reports | APIs/exports remain usable by local tools | **Pro:** governed hosted inference, maintained report schema, citations to fleet data and explicit query limits |
| Long-term telemetry and audit retention | Operator controls their storage/retention; no artificial license ceiling | **Pro:** purchased storage/durability, rollups, search, archive lifecycle and deletion guarantees |
| Firmware vulnerability intelligence | Existing version/package comparisons and security checks remain free | **Pro:** continuously maintained advisory matching, reviewed applicability and notification; public feeds remain public |
| Scheduled executive reports | Existing report APIs can be scheduled by self-managers | **Pro:** managed time zones, PDF/email delivery, templates, delivery logs and retries |
| Mobile experience | Keep usable responsive browser access; publish a client if one is built | **Both, hosted twist:** paid value is push infrastructure, managed enrollment and cross-site access, not phone screen size |
| Marketplace integrations | Existing webhooks, SMTP, SIEM and CMDB sync stay free; community adapters welcome | **Pro:** maintained OAuth apps, bidirectional ServiceNow/Jira/PSA workers and supported installation/configuration |
| Support and service commitments | **Free:** community support, no response commitment | **Pro:** business-hours support; contracted uptime/response targets; dedicated/24×7 arrangements only when staffed |
| Residency, customer offboarding and recovery | Operator chooses location; retains local data/export freedom | **Pro:** selected EU/US region, regional backups, documented export/import and account deletion service |

### Why these new services earn their place

**Directory automation:** generic sign-in and sign-in-time role mapping are already free. The new value is keeping the directory and cloud organization synchronized between logins: removal, suspension, renamed groups, nested/group-overage handling, reconciliation and session/token revocation. Microsoft documents SCIM as provisioning alongside SAML/OIDC, rather than a replacement for federation. Google exposes user lifecycle operations through its Directory API. [I01], [I02] (read 2026-10-30). Run maintained connector workers as a service; publish reusable protocol/agent changes. Do not claim standard SCIM requires cloud hosting.

**Conditional access:** policies belong to the customer's IdP. Microsoft currently requires Entra P1 for Conditional Access and P2 for risk-based policies. A PrintMarshal subscription does not include those licenses. [I03] (read 2026-10-30). Pro can validate setup, maintain enterprise-app onboarding and enforce its own session/re-authentication rules. Existing OSS federation must still benefit from customer IdP policies. Google onboarding must use the actual supported Workspace/Cloud Identity APIs and customer editions, not assume every identity product supplies the same provisioning or policy facilities.

**AI and benchmarks:** larger histories and opt-in cohorts can support better calibration; centralized inference removes model operations from customer sites. This is a hypothesis to test against the existing rate-based forecast, using backtests and uncertainty, not a promised accuracy gain. Start with explainable anomaly summaries; natural-language queries should operate on authorized aggregates, show their evidence and never initiate writes. Default cross-customer training off. One customer's data must not appear in another customer's response.

**Retention, intelligence, integrations and reports:** the service pays for stored bytes, dependable workers, feed review, OAuth registrations and compatibility work. ServiceNow should mean maintained CMDB reconciliation and ticket lifecycle; Jira, supported issue automation; Teams/Slack, managed app installation and authorized actions beyond existing webhooks; ConnectWise/Autotask, maintained customer/asset/ticket mappings. These named integrations are proposed development, not verified shipped connectors. Support user-provided adapters and credentials locally.

**Mobile and support:** a basic mobile client has little inherent need for a paid cloud. Push delivery and managed multi-site access do. An SLA purchases a staffed operating obligation; it must specify response versus resolution, business hours and exclusions for the customer LAN. Cloud convenience must never be described as permission to use existing code.

## 4. Architecture: a collector remains on the customer LAN

The repository's [deployment guide](../wiki/Installation-and-deployment.md) explains the boundary: multicast discovery needs local network presence; active scans and SNMP/IPP need routed reachability; traps need a reachable local address and preserved source attribution. An internet cloud endpoint cannot simply replace that address.

| Option | Design and trade-off | Recommendation |
| --- | --- | --- |
| **Existing server as relay** | Keep the complete local server/database/UI; add an OSS outbound replication/command adapter. Preserves offline operation and minimizes engine duplication, but needs explicit conflict/ownership rules. | First implementation; do not describe today's sync API as a complete relay protocol. |
| **Small OSS edge collector** | Reuse engine/drivers, local encrypted spool, credential vault and execution gates without the full local UI. Smaller deployment, but another release/update surface and offline diagnostics to maintain. | Extract only after relay behavior is proven. Keep Apache-2.0 and usable with alternative endpoints. |
| **VPN/routed cloud access** | Cloud binary reaches printers through customer tunnels; no multicast without extra design, traps still need routing and source preservation, and blast radius grows. | Exception for approved private connectivity; poor default onboarding for a small team. |

Proposed data flow:

```text
Printers -- SNMP/IPP/eSCL reads; local traps/syslog --> OSS relay + local vault/spool
                                                           |
                                        outbound HTTPS 443 / mTLS, regional endpoint
                                                           v
Browser -- HTTPS + WebSocket --> regional gateway --> isolated customer runtime
                                                      |        |
                                            durable ingest     PostgreSQL + archive
                                                      |
                                   managed reports / directory / integration workers

Approved command <-- same outbound relay session <-- authorized customer runtime
      |
local expiry, policy, capability and credential checks --> dry run / gated execution
      |
read-back result + audit receipt ------------------------> regional cloud
```

Enrollment uses a short-lived one-use token to bind the relay's key to one tenant, site and region. Rotate/revoke relay certificates, enforce egress allow-lists, and support proxies. The relay should continue monitoring and local alerting during internet outages. Give its spool a documented time/size bound, distinguish last-seen from offline-device state, and deduplicate replay by `(relay, sequence/event ID)`. Persist acknowledgements only after commit. Mark missing history honestly after overflow.

Do not tunnel arbitrary shell commands, raw SNMP or the printer's administration website through the first release. A command envelope should identify tenant/site/device, actor, allowed operation, expiry, nonce, desired-state version and package hash. Recheck authorization at execution; preserve local write-purpose credentials, dry-run defaults, canary gates and maintenance windows. An expired command must not run after a long outage. An interrupted firmware write requires manual verification, not automatic redelivery.

### What leaves the site

| Data category | Proposed default and customer control |
| --- | --- |
| Inventory | Opaque device/site IDs, manufacturer/model, firmware, capabilities and timestamps. Serial numbers, hostnames, IP/MAC, location and operator notes are configurable fields; transmit only what the selected service needs. |
| Operational telemetry | Normalized status, counters, supply/tray readings, alert changes and rollups; include freshness and quality. Sampling/rollups should bound cloud cost without limiting local capture. |
| Security/audit | Assessment findings, public-certificate metadata/fingerprints and necessary actor/action/result records. Raw syslog/configuration text excluded by default because it may contain names or secrets. |
| Secrets | SNMP/admin passwords, private keys and local CA credentials remain local. Cloud integration OAuth tokens are separate tenant secrets, encrypted and regionally stored. A future escrow option needs separate threat modeling. |
| Documents/jobs | No print content, scan images, spool files, job titles or per-user print history. The existing CUPS bridge remains local. |
| Firmware | Operator-authorized package download to relay via expiring access, hash verification and local compatibility checks. Upload/storage optional; vendor distribution rights must be checked. |
| AI/benchmark input | Authorized, minimized telemetry only; additional explicit opt-in before cohort use. Exclude names, addresses, notes and document content; set cohort suppression rules. |

Cloud SSO/email/billing adds account data independently of printer telemetry. Publish this complete inventory, retention and subprocessors, including operational logs and support tools.

### Isolation and execution plan

At launch use **one active customer runtime, one database and restricted database role per customer**, on shared regional PostgreSQL clusters; separate encryption keys, object prefixes/access policies and relay identities. Tenant resolution must come from authenticated routing, never an untrusted request header. A regional gateway can serve many customers without one load balancer per customer. An MSP gets explicitly delegated access to distinct customer runtimes; its consolidated view is new authorized aggregation.

This requires development: a collector ingestion/command adapter, separation of cloud scheduling from local polling, and trustworthy routing. It is not “deploy the current binary and enable tenancy.” Keep one active runner per customer; deployment handover must stop/fence the old runner before starting the replacement. High availability can initially mean bounded restart, not two simultaneous workers. Later pooled execution needs tenant-aware stores, transactionally durable queues, leases/fencing, replayable streams and isolation tests across every route, background job, cache and export. Share generally useful fixes with OSS.

## 5. Hosting platforms and managed PostgreSQL

The table evaluates a long-lived Go service, PostgreSQL, customer isolation and WebSockets. Regions are a starting point: contract scope must cover **processing, backups, logs, support and TLS termination**, not just the database pin. The suitability judgments are analysis; cited capabilities/prices are external evidence.

| Platform | Fit for this workload and cost signal | Residency, assurance and trade-off |
| --- | --- | --- |
| **AWS** | ECS/Fargate runs the container; ALB supports WebSockets; RDS provides PostgreSQL with Multi-AZ/backups. Fargate's US-East example publishes $0.000011244/vCPU-second and $0.000001235/GiB-second, before networking/storage extras. | Recommended compute platform: regional cells, private networks and mature control/evidence options. AWS SOC reports do not cover PrintMarshal's application controls. More IAM/network/billing work than a simple PaaS. [C01], [C02], [C03], [C04], [C05] (read 2026-10-30). |
| **Google Cloud** | Cloud Run + Cloud SQL fits Go/Postgres. WebSockets supported, with request timeouts up to 60 minutes and reconnect required; open streams keep instances active. Cloud SQL prices by resources/HA, not tenant count. | Strong alternative if the team knows GCP. Use always-available capacity for connections/workers; do not budget scale-to-zero savings for persistent streams. Regional endpoint support varies by service; Google publishes SOC offerings. [C06], [C07], [C08], [C09], [C10] (read 2026-10-30). |
| **Azure** | Container Apps supports HTTP/TCP ingress and WebSockets; managed PostgreSQL supports same-/cross-zone HA choices. Container Apps meters resource usage and requests under its consumption offering. | Good for an Azure-skilled team and Microsoft-heavy procurement, but Entra sign-in does not require hosting on Azure. Check product/region and ingress behavior rather than treating an ordinary HTTP timeout as a WebSocket lifetime guarantee. SOC evidence available through Microsoft. [C11], [C12], [C13], [C14] (read 2026-10-30). |
| **Fly.io** | Straightforward Machines deployment and managed PostgreSQL. Published performance-1x/2 GiB machine **$33/month**; managed PG Basic **$38/month + $0.28/GB**, including HA/backups/pooling. Treat the tiny Basic plan as a sizing starting point, not tested fleet capacity. | Attractive developer experience; important EU caveat: Fly documents US control-plane data, platform logs and ordinary volume snapshots regardless of app region; managed-PG backups use the same geographic area as cluster. SOC 2 report access requires its Compliance Package. Not an unconditional EU-only solution. [C15], [C16], [C17] (read 2026-10-30). |
| **Hetzner** | Ordinary cloud VMs can run Go, a WebSocket proxy and PostgreSQL; VM/database administration would largely be PrintMarshal's responsibility in this proposed stack. No verified current numeric VM price is quoted because the fetched selector did not expose the price table. | Locations include Germany/Finland and US sites. ISO 27001 certificate cited covers the three named European datacenter parks; do not infer US scope or a SOC 2 report. Attractive if the team wants to operate infrastructure; weaker fit for a deliberately managed-first small team. [C18], [C19] (read 2026-10-30). |
| **DigitalOcean** | App Platform accepts container services/workers; published shared 1 GiB **$12/month**, dedicated 1 vCPU/2 GiB **$39**. PostgreSQL starts **$15 single-node**; current Advanced starts **$130/month primary + $130 matching standby** for HA, plus applicable storage. | Best simpler-PaaS alternative. EU/US application regions and SOC controls are documented. From 15 October 2026 for new accounts, new Standard clusters cannot add standby nodes; do not use older $60 HA estimates for a new production order. Verify edition and region. [C20], [C21], [C22], [C23] (read 2026-10-30). |
| **Cloudflare** | Containers can run native Go; Workers/Durable Objects can front sessions. Containers meter CPU, memory, disk and egress. D1 meters reads/writes/storage and has a **10 GB per-database maximum**; it is not a drop-in PostgreSQL store. | Current Containers docs support regional/jurisdiction placement constraints, including EU; avoid the outdated claim that none exist. D1 also has location/jurisdiction controls. Validate end-to-end placement separately. Best considered for edge services first; Workers/D1 would require a larger architecture/storage rewrite. Review product scope in Cloudflare assurance material. [C24], [C25], [C26], [C27], [C28], [C29] (read 2026-10-30). |

**Recommendation:** AWS Fargate with **Crunchy Bridge PostgreSQL on AWS**, colocated per regional cell, is a defensible small-team starting point when customer isolation and a SOC 2 path outweigh the lowest VM bill. Crunchy supplies database expertise and published production pricing; RDS is the alternative when a single cloud-provider contract/private-service arrangement is more important. DigitalOcean deserves a short operational bake-off if the team lacks AWS experience. Avoid Kubernetes initially; one primary provider and portable Go/Postgres are enough.

Candidate regional pairs for validation are AWS Ireland/Virginia (both in Crunchy's region list), Cloud Run services Belgium/Iowa, or DigitalOcean Frankfurt/New York. These are available deployment locations, not blanket promises that every linked service, backup or support workflow stays there. [D02], [C31], [C22] (read 2026-10-30).

| Managed PostgreSQL choice | Verified pricing/feature boundary | Decision |
| --- | --- | --- |
| **Neon** | Launch **$0.106/CU-hour**, Scale **$0.222**, storage **$0.35/GB-month**; autosuspend and separately metered history. | Excellent for intermittent databases/branches. A continuously ingesting fleet may not suspend, so price an always-on workload. Check plan/report/private-network requirements and region availability. [D01] (read 2026-10-30). |
| **Supabase** | Pro from **$25/month**, additional project compute from $10; 7-day daily backups. PITR separately listed **$100/month per seven days retention**. | Useful if Auth/Storage/Realtime are needed. Existing PrintMarshal already implements auth and streams, so avoid taking on extra platform coupling only for PostgreSQL. Do not budget one free Micro per customer. [B39] (read 2026-10-30). |
| **Crunchy Bridge** | AWS-East production Standard-4 **$70**, Standard-8 **$140**, Standard-16 **$280**, Standard-32 **$560/month**; HA doubles cluster price; storage **$0.10/GB-month**. Hobby lacks HA/SLA. | Recommended managed database for this model. Private peering and TLS, SOC 2 Type 2, and automated backup/PITR documented. Confirm regional prices, report scope and backup geography contractually. [D02], [D03], [D04] (read 2026-10-30). |
| **RDS PostgreSQL** | Instance, storage/I/O, backup/network and HA choices determine bill; on-demand/reserved options. Standard Postgres interfaces, Multi-AZ and PITR available. | Strong native AWS choice. Use a dated regional calculator export before purchase; this report does not fabricate a universal RDS monthly price. [C03], [D05] (read 2026-10-30). |
| **Cloud SQL PostgreSQL** | Resource/edition/HA-dependent pricing; the pricing page excludes small shared-core machines from its SLA. | Natural GCP pairing. Avoid cross-cloud database traffic just to pick the cheapest entry offer. [C08] (read 2026-10-30). |

### Reproducible planning budget: 10, 100 and 1,000 tenants

These are **Harry's USD monthly engineering estimates**, not provider offers or measured capacity. Model a paying tenant as one isolated customer organization with **50 printers and one relay**: 500 / 5,000 / 50,000 printers. Split customers equally between an EU cell and a US cell; neither cell replicates customer telemetry into the other. This is dual-region availability of the product, not cross-region disaster failover for each tenant.

Assume 730 hours/month; local polling every five minutes; cloud history receives one compact 1 KiB snapshot/device/15 minutes plus immediate material changes; 30-day hot snapshots, 13-month hourly rollups and 12-month audit archive. At 1,000 tenants that is **4.8 million snapshots/day**, about **4.58 GiB/day payload before compression**, excluding events. Budget 2.5× hot payload for rows/indexes, roughly 230 GB for indexed hourly rollups, and round total primary storage to 1 TB including headroom. These packed records/rollups are **new cloud storage design**, not the current scalar telemetry schema. Retaining every scalar locally is still allowed.

Keep one 0.25-vCPU/0.5-GiB Fargate customer runtime active per tenant, plus 4 / 8 / 32 shared 1-vCPU/2-GiB task equivalents for gateways, ingest, reporting and connectors. This allocation is unbenchmarked; heavy tenants need more. Calculating from AWS's published per-second example gives approximately **$9.01/customer task-month** and **$36.04/shared task-month**. ALB examples publish $0.0225/hour plus capacity-unit charges; a gateway avoids per-customer ALBs. [C01], [C30] (read 2026-10-30).

| Monthly component / sizing assumption | 10 tenants | 100 tenants | 1,000 tenants |
| --- | ---: | ---: | ---: |
| Customer runtimes, $9.01 each | $90 | $901 | $9,010 |
| Shared service task equivalents | $144 | $288 | $1,153 |
| DB compute: 2× Standard-4 HA / 2× Standard-8 HA / 4× Standard-32 HA | $280 | $560 | $4,480 |
| DB storage allowance, including standby copy: 200 / 400 / 2,000 GB at $0.10 | $20 | $40 | $200 |
| EU/regional price-difference allowance | $60 | $150 | $1,000 |
| Load balancing, private networking/endpoints/NAT and egress allowance | $80 | $180 | $700 |
| Logs, monitoring, secrets, operational tooling and provider-support allowance | $200 | $250 | $1,000 |
| Archives, restore copies, email, registry and miscellaneous allowance | $75 | $100 | $400 |
| Subtotal | **$949** | **$2,469** | **$17,943** |
| 25% contingency, rounded | $237 | $617 | $4,486 |
| **Planning total** | **$1,200/month** | **$3,100/month** | **$22,400/month** |
| Approximate infrastructure/tenant | $120 | $31 | $22.40 |

Database compute/storage arithmetic uses Crunchy's **AWS-East sample prices**, not a claimed EU quote. The regional allowance and all networking/tooling rows are explicit budget assumptions; actual networking, backup charging and support purchases can change them. [D02] (read 2026-10-30). The four large clusters are two shards per cell, sharing infrastructure while retaining separate customer databases/roles. Pool database connections carefully; do not multiply the current default pool by 1,000 without a connection budget. Confirm the assumed instance/process memory before adopting the 0.5-GiB allocation.

Excluded: salaries, customer-support labor, payment processing, tax, audits/legal work, customer relay hardware/bandwidth, large firmware transfer and unbounded AI usage. Budget AI as capped purchased inference with margin; no GPU fleet is assumed. A single-region design-partner beta reduces duplicated shared costs but cannot claim EU and US service availability. A rough half/twice budget range is reasonable until load and failover measurements exist.

Sensitivity is material: full five-minute snapshots triple historical ingest relative to this model; unbatched scalar writes can multiply row counts further. Doubling the customer container allocation adds roughly $9/tenant/month before other effects. Thousands of tiny MSP customers still incur runtime costs even if their aggregate printer count is small. Benchmark ingest/replay, 13-month queries, concurrent streams, a database restore and collector reconnection storms before committing to retention or availability promises.

## 6. Pricing grounded in fleet-market evidence

| Market observation | Verified public evidence and limitation |
| --- | --- |
| **Print Tracker** | Landing page lists **$795 lifetime support/upgrades plus $50/month up to 100 devices**; Enterprise device bands **$0.50 (100–499), $0.45 (500–999), $0.35 (1,000–2,499)**. The Starter period is explicitly monthly; the Enterprise cards omit an equally clear period, so confirm their billing period before calculating annual comparisons. Page advertises remote technician, reports and predictive analytics; those are vendor claims, not independently tested features. [M01] (read 2026-10-30). |
| **MeterBeam** | **$25/month up to ten printers**; regular Single Seat **$150/month including 50**, then $1/printer; regular Dealer **$400/month including 500 across unlimited sites**, then $0.50/printer. Founding $100/$250 offers are conditional on availability. 30-day trial currently requires payment details. Do not repeat its unsourced competitor/dealer quote estimates as ECI list prices. [M02] (read 2026-10-30). |
| **Domotz, adjacent network management** | Current retrieved page displays **€35/collector/month**, unlimited devices/users, lower annual/volume MSP rates and a 14-day no-card trial. Other indexed pages show dollars or old device pricing; retain the retrieved currency and do not convert it silently. This is a broader network product, not a printer-feature-equivalence test. [M03] (read 2026-10-30). |

There is evidence for both a device meter and a site minimum, but no evidence here that customers will pay a particular PrintMarshal price. Printer-specific competitors put pressure on a premium monitoring-only offer. Validate the incremental value of managed security operations, trustworthy reporting and onboarding with design partners; do not assume “AI” commands a premium.

Three **alternative proposed models**, not cumulative fees:

| Model | Proposed public price | Strength / weakness |
| --- | --- | --- |
| **Pure device** | $1.50/managed printer/month, $49 organization minimum; users and ordinary sites included | Easy fleet comparison; weak economics for many tiny isolated customer accounts. |
| **Per site** | $49/site/month including 50 printers, then $0.75/additional printer at that site | Pays for relay/site overhead; penalizes distributed organizations and needs a device allowance to bound storage. |
| **Tiered subscription — recommended** | **Standard $99/month including 50 printers**, $0.75/additional; **Operations $199 including 100**, $0.75/additional; **MSP $499 including 500 and 20 isolated customer accounts**, $0.50/additional printer and $9/additional active customer account beyond 20 | Makes managed-service scope visible while covering isolation overhead. Publish exact worked examples and avoid double-counting a printer across sites/relays. |

For the tiered proposal, Standard provides all existing core features as a managed service, regional hosting, upgrades/backups, hosted SSO, business-hours email support, 30-day hot history and 13-month rollups. Operations adds the proposed maintained directory lifecycle service, scheduled executive delivery, managed integration workers, longer searchable audit retention and a capped AI allowance. MSP adds delegation/customer organization operations and consolidated reports. A native phone client is not a tier gate. Features are sellable only after implementation and validation.

Standard examples: 50 printers **$99/month**; 100 **$136.50**; 500 **$436.50**. MSP: 500 **$499**; 1,000 **$749**, assuming no extra customer-account fees. Those are PrintMarshal calculations, not competitor prices. The $99 minimum is intentionally above bare meter collection: if customers only value monitoring, use the simpler alternative or reduce operating cost before selling it.

Include a **30-day full Pro trial without a card**, with guided collector setup and a simulator demo before installation. Thirty days gives supply histories a chance to become useful; it is not a guarantee of forecast accuracy. After trial, offer an optional free Cloud Starter: five printers, one site, seven-day cloud history, community support, standard sign-in and export, no AI/service SLA. Implement this on an independently verified pooled free-service architecture; a permanent dedicated runtime per free account would undermine the cost model. Until that architecture exists, offer the trial and free self-hosting only, not an uneconomic permanent-cloud promise.

Define billable devices as enrolled, actively managed physical devices, averaged daily; count serial/UUID once, exclude simulators/decommissioned records and explain offline-device treatment. Show usage, upcoming invoice and an approved spending cap. Trial expiry must stop paid cloud ingestion gracefully while leaving the local product running and allowing export. Offer 10% annual prepayment discount after retention is proven; no “lifetime” guarantee.

At 50 printers/customer, Standard MRR is $990 / $9,900 / $99,000 at the three scales. Against the budget above, infrastructure alone consumes about **121% / 31% / 23%**. Ten dual-region customers are a subsidized launch, not a profitable steady state. At 100 customers, approximately 69% remains before support, AI, payments and company overhead. Support and connector work must therefore be scoped and measured; 24×7 response cannot be included casually. Price a contracted response/availability package separately after staffing and customer dependencies are understood.

## 7. Risks and operating commitments

**Community trust:** the strongest failure mode is a free project that becomes hard to obtain, unsafe to maintain or dependent on cloud login. Mattermost's removed free features, Netdata's retired plans, Cal.com's commercial/community split and Canvas's unresolved publication concern make the risk concrete. [B17], [B26], [B33], [B04] (read 2026-10-30). Publish the boundary and release cadence, keep an explicit free download, disclose cloud-only service costs and welcome competing endpoints/adapters. Service investment should improve core reliability too.

**Licensing:** Apache-2.0 permits additional terms for modifications/derivative works subject to its conditions; it does not grant trademark rights. Preserve notices, attribution and change notices where required. [L01] (read 2026-10-30). Recommended Pro code treatment: privately owned proprietary service orchestration/billing/managed-connector code in a separately identified repository, with clear copyright/license headers and hosted service terms. Core/relay remain Apache-2.0; no license key in their essential execution path. Do not relabel inherited Apache code proprietary. Keep SBOM/license review for dependencies, firmware redistribution and model/data licenses; do not introduce copyleft/restricted components into the core without evaluating obligations. This separation is an implementation recommendation, not a claim that a separate process automatically eliminates license obligations.

**Lock-in:** database portability does not by itself make offboarding possible. Add documented complete export and restore/import pathways, including telemetry, settings, group membership and audit. The current SQLite-to-PostgreSQL transition is not automated; do not promise a one-click migration today. Keep SQL portable, archive formats documented and the relay transport versioned. Build a cancellation drill and retain enough local information for continuing free operation without cloud credentials.

**Data protection:** fleet telemetry can contain personal data when names, notes, addresses or audit identities identify people. Treat those fields conservatively and minimize collection. A customer-controller/PrintMarshal-processor arrangement needs a processing contract and subprocessor governance; EEA transfers require the applicable Chapter V mechanism, such as adequacy or appropriate safeguards with necessary assessments. [L02], [L03] (read 2026-10-30). Proposed deliverables before general availability: field-level data inventory, DPA, regional/subprocessor map, retention/deletion implementation, documented support access and tested exports. Do not call hashing a serial number anonymization.

**Security and incidents:** a cloud-command compromise could become a LAN-management compromise. Keep credentials local, remote writes disabled by default, narrow command types, signed builds, revocation, canaries and local execution policy. Obtain an independent tenant-isolation/relay assessment. Alert on collector gaps, dropped audits and replay failures. For personal-data breaches, a processor must notify the controller without undue delay; a controller's regulatory notification can have a 72-hour deadline where applicable, rather than a universal 72-hour processor allowance. [L04] (read 2026-10-30).

**SOC 2 path:** provider reports support supplier due diligence; they do not attest to PrintMarshal's software or organization. Shared responsibility remains with the service operator for its application/configuration. [C05], [L05] (read 2026-10-30). Define scope, owners and evidence from launch: staff MFA/access reviews, change control, secret management, vulnerability response, supplier review, monitoring, restore exercises and incident response. Pursue Type I/readiness and then operating-period Type II with an auditor; claim only the assurance actually achieved. Audit/legal costs are deliberately outside the hosting estimate.

**Intelligence quality:** propose NVD CVE data, CISA Known Exploited Vulnerabilities (KEV) and vendor security advisories as upstreams. CISA's SBOM-consumption guidance discusses NVD/CVE and KEV vulnerability information; the paid value would be maintaining and reviewing printer-specific applicability. [V01] (read 2026-10-30). Advisory matching is not proof a specific printer is vulnerable. Preserve exact model/firmware/platform context, source links, match confidence, “unknown” and vendor remediation notes. Raw public information must not be misrepresented as proprietary knowledge. Do not turn a CVE match or AI recommendation into automatic firmware delivery. The existing unverified hardware-write boundaries remain in force.

**Support load and availability:** first diagnose relay connectivity, firewall/VLAN/proxy, device credentials and supported capabilities separately from cloud health. Create a supported deployment matrix, diagnostic bundle with secret scrubbing and a hardware qualification process. Agree response targets and cloud availability separately from printer uptime or firmware outcomes. Cap custom onboarding and connector development; escalate unsupported vendor transports rather than promising a fix. Track tickets/tenant and minutes/onboarding alongside infrastructure margin.

Launch gates should be concrete: isolated tenants, loss/replay behavior tested, regional backup restore demonstrated, directory disablement revoking access, complete export rehearsed, and enough paying design partners to validate service value. AI benchmarking and a large integration marketplace can follow; they should not delay a dependable hosted core.

## 8. Sources

All sources below were consulted **2026-10-30**. Search-index-only or failed access is identified where it affects confidence. Historical announcements are evidence of the dated change, not present pricing. Vendor feature statements have not been independently product-tested.

### Business models and changes

- [B01] Canvas LMS repository and AGPL statement. Read 2026-10-30.
- [B02] Canvas FAQ: commercial dual licensing, contribution agreement, extensions. Read 2026-10-30.
- [B03] Current Canvas tiers and quotation/support model. Read 2026-10-30.
- [B04] Instructure Community, 14 October 2026 source-update discussion and employee reply. Read 2026-10-30.
- [B05] Current GitLab pricing/features. Read 2026-10-30.
- [B06] GitLab's March 2023 Premium price-change announcement. Read 2026-10-30.
- [B07] Grafana pricing. Read 2026-10-30.
- [B08] Grafana OSS/Cloud/Enterprise deployment comparison. Read 2026-10-30.
- [B09] Grafana licensing/2021 AGPL change. Read 2026-10-30.
- [B10] Sentry FSL introduction and prior license history. Read 2026-10-30.
- [B11] Sentry's self-hosted developer guide source; rendered documentation endpoint failed. Read 2026-10-30.
- [B12] Sentry blog: hosted Seer and self-hosting limitation. Read 2026-10-30.
- [B13] Gleap's dated October 2026 Sentry pricing report; secondary evidence because `https://sentry.io/pricing/` returned an unsupported content-type error. Read 2026-10-30.
- [B14] Mattermost pricing/seat terms. Read 2026-10-30.
- [B15] Mattermost download: Entry default. Read 2026-10-30.
- [B16] Mattermost v11 free-offerings announcement, including Team Edition boundary; search-index extract. Read 2026-10-30.
- [B17] Mattermost important upgrade notes: GitLab SSO/Playbooks changes. Read 2026-10-30.
- [B18] n8n current plans/pricing. Read 2026-10-30.
- [B19] n8n licensing help, updated 6 October 2026. Read 2026-10-30.
- [B20] Plausible self-hosted/cloud comparison. Read 2026-10-30.
- [B21] Plausible current homepage pricing. Read 2026-10-30.
- [B22] Plausible subscription-plan definitions. Read 2026-10-30.
- [B23] Plausible Community Edition announcement and license history. Read 2026-10-30.
- [B24] Netdata Agent repository/license/component boundary. Read 2026-10-30.
- [B25] Netdata product description, plan/pricing/license details. Read 2026-10-30.
- [B26] Netdata March 2024 plan retirement announcement. Read 2026-10-30.
- [B27] Zabbix license/version boundary. Read 2026-10-30.
- [B28] Zabbix Cloud capacity/pricing. Read 2026-10-30.
- [B29] Zabbix professional-services brochure; indexed official PDF. Read 2026-10-30.
- [B30] Metabase pricing. Read 2026-10-30.
- [B31] Metabase plan comparison and licenses. Read 2026-10-30.
- [B32] Metabase's current dbt article, including plan/user prices. Read 2026-10-30.
- [B33] Cal.com 15 April 2026 commercial-code/Cal.diy announcement. Read 2026-10-30.
- [B34] Cal.diy repository, MIT license and removed enterprise functionality. Read 2026-10-30.
- [B35] Cal.com current hosted pricing. Read 2026-10-30.
- [B36] PostHog self-hosting status, releases and CVE posture. Read 2026-10-30.
- [B37] PostHog pricing, allowances and billing limits. Read 2026-10-30.
- [B38] Supabase Docker self-hosting guide. Read 2026-10-30.
- [B39] Supabase pricing/compute/PITR. Read 2026-10-30.
- [B40] Supabase repository license. Read 2026-10-30.
- [B41] Supabase maintainer/community self-hosting discussion; individual experiences, not universal product findings. Read 2026-10-30.

### Identity, platforms and database services

- [I01] Microsoft SCIM provisioning with SAML/OIDC. Read 2026-10-30.
- [I02] Google Directory API user lifecycle operations. Read 2026-10-30.
- [I03] Entra Conditional Access overview/license requirements. Read 2026-10-30.
- [C01] AWS Fargate pricing, especially US-East Example 1. Read 2026-10-30.
- [C02] AWS ALB listeners/WebSocket support. Read 2026-10-30.
- [C03] RDS PostgreSQL capabilities. Read 2026-10-30.
- [C04] AWS SOC information. Read 2026-10-30.
- [C05] AWS shared-responsibility model. Read 2026-10-30.
- [C06] Cloud Run WebSockets/reconnection/billing behavior. Read 2026-10-30.
- [C07] Cloud Run pricing. Read 2026-10-30.
- [C08] Cloud SQL pricing and shared-core SLA limitation. Read 2026-10-30.
- [C09] Google regional/multi-regional endpoint boundaries. Read 2026-10-30.
- [C10] Google Cloud Trust Center. Read 2026-10-30.
- [C11] Azure Container Apps ingress/protocols. Read 2026-10-30.
- [C12] Azure PostgreSQL overview. Read 2026-10-30.
- [C13] Container Apps billing dimensions. Read 2026-10-30.
- [C14] Microsoft SOC 2 scope/responsibilities. Read 2026-10-30.
- [C15] Fly current main pricing page; some older resource-doc machine prices differ. Read 2026-10-30.
- [C16] Fly managed PostgreSQL capabilities/pricing. Read 2026-10-30.
- [C17] Fly security/compliance questionnaire, updated 26 October 2026; residency exceptions and report access. Read 2026-10-30.
- [C18] Hetzner cloud locations; numeric price selector not available in fetched text. Read 2026-10-30.
- [C19] Hetzner ISO certificate scope. Read 2026-10-30.
- [C20] DigitalOcean App Platform pricing. Read 2026-10-30.
- [C21] DigitalOcean PostgreSQL pricing, including October/November 2026 edition-transition note. Read 2026-10-30.
- [C22] DigitalOcean App Platform region availability. Read 2026-10-30.
- [C23] DigitalOcean trust controls. Read 2026-10-30.
- [C24] Cloudflare Containers overview. Read 2026-10-30.
- [C25] Cloudflare Containers pricing. Read 2026-10-30.
- [C26] Cloudflare Containers placement, updated October 2026. Read 2026-10-30.
- [C27] D1 limits; PostgreSQL incompatibility is an architectural inference from its distinct API/storage model. Read 2026-10-30.
- [C28] D1 location/jurisdiction controls. Read 2026-10-30.
- [C29] Cloudflare compliance resources. Read 2026-10-30.
- [C30] ALB pricing examples. Read 2026-10-30.
- [C31] Cloud Run service locations. Read 2026-10-30.
- [D01] Neon pricing: compute, storage, restore history. Read 2026-10-30.
- [D02] Crunchy Bridge regional sample prices, storage and HA multiplier. Read 2026-10-30.
- [D03] Crunchy Bridge encryption, peering, firewall defaults and SOC 2. Read 2026-10-30.
- [D04] Crunchy Bridge backup/PITR retention. Read 2026-10-30.
- [D05] RDS PostgreSQL pricing dimensions. Read 2026-10-30.

### Market, license and obligations

- [M01] Print Tracker's own landing-page feature/pricing cards; confirm Enterprise billing period and whether this offer remains orderable. Read 2026-10-30.
- [M02] MeterBeam's own pricing; founding availability is conditional. Competitor-price assertions on that page were not adopted as evidence. Read 2026-10-30.
- [M03] Domotz's current retrieved pricing, displayed in EUR. Read 2026-10-30.
- [L01] Apache License 2.0 text, especially sections 2–4 and 6. Read 2026-10-30.
- [L02] EDPB controller/processor roles and contracts. Read 2026-10-30.
- [L03] EDPB international-transfer conditions and safeguards. Read 2026-10-30.
- [L04] EDPB breach-notification guidance, including processor/controller timing distinction. Read 2026-10-30.
- [L05] Microsoft explanation of SOC 2 Type 2 and customer responsibility. Read 2026-10-30.
- [V01] CISA recommended practices for SBOM consumption; official PDF indexed extracts. NVD's API page returned no readable text and CISA's catalog returned 403, so neither was treated as a verified live API contract. Read 2026-10-30.

[B01]: https://github.com/instructure/canvas-lms/blob/master/README.md
[B02]: https://github.com/instructure/canvas-lms/wiki/FAQ
[B03]: https://www.instructure.com/products/canvas-tiers
[B04]: https://community.instructure.com/en/discussion/667478/canvas-no-longer-open-source-software
[B05]: https://about.gitlab.com/pricing/
[B06]: https://about.gitlab.com/blog/gitlab-premium-update/
[B07]: https://grafana.com/pricing/
[B08]: https://grafana.com/grafana/deployment-options/
[B09]: https://grafana.com/licensing/
[B10]: https://blog.sentry.io/introducing-the-functional-source-license-freedom-without-free-riding/
[B11]: https://github.com/getsentry/develop/blob/master/src/docs/self-hosted/index.mdx
[B12]: https://blog.sentry.io/seer-mcp-cli-or-coding-agent/
[B13]: https://www.gleap.ai/blog/sentry-pricing
[B14]: https://mattermost.com/pricing/
[B15]: https://mattermost.com/download/
[B16]: https://forum.mattermost.com/t/mattermost-v11-changes-in-free-offerings/25126
[B17]: https://docs.mattermost.com/administration-guide/upgrade/important-upgrade-notes
[B18]: https://n8n.io/pricing/
[B19]: https://support.n8n.io/article/can-i-use-your-license-for-my-use-case
[B20]: https://plausible.io/self-hosted-web-analytics
[B21]: https://plausible.io/
[B22]: https://plausible.io/docs/subscription-plans
[B23]: https://plausible.io/blog/community-edition
[B24]: https://github.com/netdata/netdata
[B25]: https://learn.netdata.cloud/docs/welcome-to-netdata/complete-product-description
[B26]: https://www.netdata.cloud/blog/netdata-unified-plans/
[B27]: https://www.zabbix.com/license
[B28]: https://www.zabbix.com/cloud
[B29]: https://assets.zabbix.com/files/Brochures/Zabbix_Professional_Services_brochure.pdf
[B30]: https://www.metabase.com/pricing
[B31]: https://www.metabase.com/pricing/compare-plans
[B32]: https://www.metabase.com/articles/metabase-on-dbt
[B33]: https://cal.com/blog/calcom-v6-4
[B34]: https://github.com/calcom/cal.diy
[B35]: https://cal.com/pricing
[B36]: https://posthog.com/docs/self-host
[B37]: https://posthog.com/pricing
[B38]: https://supabase.com/docs/guides/self-hosting/docker
[B39]: https://supabase.com/pricing
[B40]: https://github.com/supabase/supabase/blob/master/LICENSE
[B41]: https://github.com/orgs/supabase/discussions/39820
[I01]: https://learn.microsoft.com/en-us/entra/identity/app-provisioning/use-scim-to-provision-users-and-groups
[I02]: https://developers.google.com/workspace/admin/directory/v1/guides/manage-users
[I03]: https://learn.microsoft.com/en-us/entra/identity/conditional-access/overview
[C01]: https://aws.amazon.com/fargate/pricing/
[C02]: https://docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-listeners.html
[C03]: https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/CHAP_PostgreSQL.html
[C04]: https://aws.amazon.com/compliance/soc-faqs/
[C05]: https://aws.amazon.com/compliance/shared-responsibility-model/
[C06]: https://docs.cloud.google.com/run/docs/triggering/websockets
[C07]: https://cloud.google.com/run/pricing
[C08]: https://cloud.google.com/sql/pricing
[C09]: https://docs.cloud.google.com/docs/security/compliance/about-regional-endpoints
[C10]: https://cloud.google.com/trust-center
[C11]: https://learn.microsoft.com/azure/container-apps/ingress?tabs=bash
[C12]: https://learn.microsoft.com/en-us/azure/postgresql/overview
[C13]: https://azure.microsoft.com/en-us/pricing/details/container-apps/
[C14]: https://learn.microsoft.com/en-us/compliance/regulatory/offering-soc-2
[C15]: https://fly.io/pricing/
[C16]: https://fly.io/docs/mpg/
[C17]: https://fly.io/legal/security-questionnaire/
[C18]: https://www.hetzner.com/cloud/
[C19]: https://www.hetzner.com/unternehmen/zertifizierung/
[C20]: https://www.digitalocean.com/pricing/app-platform
[C21]: https://docs.digitalocean.com/products/databases/postgresql/details/pricing/
[C22]: https://docs.digitalocean.com/products/app-platform/details/availability/
[C23]: https://www.digitalocean.com/trust/controls
[C24]: https://developers.cloudflare.com/containers/
[C25]: https://developers.cloudflare.com/containers/platform/pricing/
[C26]: https://developers.cloudflare.com/containers/concepts/placement/
[C27]: https://developers.cloudflare.com/d1/platform/limits/
[C28]: https://developers.cloudflare.com/d1/configuration/data-location/
[C29]: https://www.cloudflare.com/trust-hub/compliance-resources/
[C30]: https://aws.amazon.com/elasticloadbalancing/pricing/
[C31]: https://docs.cloud.google.com/run/docs/locations
[D01]: https://neon.com/pricing
[D02]: https://docs.crunchybridge.com/concepts/plans-pricing
[D03]: https://docs.crunchybridge.com/concepts/security
[D04]: https://docs.crunchybridge.com/concepts/backups
[D05]: https://aws.amazon.com/rds/postgresql/pricing/
[M01]: https://landing.printtrackerpro.com/
[M02]: https://meterbeam.com/pricing/
[M03]: https://www.domotz.com/pricing.php
[L01]: https://www.apache.org/licenses/LICENSE-2.0
[L02]: https://www.edpb.europa.eu/sme/learn-the-basics/data-controller-or-data-processor_en
[L03]: https://www.edpb.europa.eu/sme/be-compliant/international-data-transfers_en
[L04]: https://www.edpb.europa.eu/system/files/2022-10/edpb_guidelines_202209_personal_data_breach_notification_targetedupdate_en.pdf
[L05]: https://learn.microsoft.com/en-us/compliance/regulatory/offering-soc-2
[V01]: https://www.cisa.gov/sites/default/files/2024-08/SECURING_THE_SOFTWARE_SUPPLY_CHAIN_RECOMMENDED_PRACTICES_FOR_SOFTWARE_BILL_OF_MATERIALS_CONSUMPTION-508.pdf
