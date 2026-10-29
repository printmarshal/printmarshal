# PrintMarshal — Homepage Copy (`site/index.html`)

## Hero
- **Eyebrow:** Printer Fleet Security & Operations
- **Headline:** Know where your printer fleet is exposed.
- **Supporting copy:** Discover network printers, evaluate security policies against live SNMP, IPP, and TLS evidence, and preview supported configuration changes from one self-hosted console.
- **Primary actions:** `Explore the product` (`#walkthrough`), `Get started` (`/get-started/`)
- **Quiet link:** `View source on GitHub (Apache-2.0) →`

## The Printer Visibility Problem
Network printers sit on trusted VLANs with opaque configurations:
1. **Silent protocol exposure:** Legacy `SNMPv1/v2c` community strings, plain-HTTP admin consoles, self-signed or expiring TLS certificates, and raw port `9100`.
2. **The "unknown = pass" trap:** Standard Printer-MIB and IPP attributes do not expose every security control on every vendor model. PrintMarshal records `pass`, `fail`, and `unknown` separately and excludes `unknown` from the score denominator.
3. **Unverified configuration writes:** Printer firmware can accept an SNMP `SET` or IPP `Set-Printer-Attributes` request while ignoring or truncating the value. PrintMarshal previews changes in dry-run mode, captures rollback values, and re-reads every written setting (`verified`, `diverged`, `unverifiable`).

## Guided 5-Step Product Walkthrough
1. **Discover a printer fleet** (`fleet-discovery.png` / `mobile-fleet-cards.png`): Active CIDR/range scans (`SNMPv1/v2c/v3`, `IPP/IPPS`, `eSCL`, TCP probes) + passive `mDNS/WS-Discovery/SSDP`, deduplicated by `serial → UUID → MAC → IP`.
2. **Identify failed & unknown checks** (`security-compliance.png` / `mobile-security-compliance.png`): Built-in 14-item `Baseline` policy and custom policies with management-path lockout tags (`may cut SNMP path` / `may cut HTTP path`).
3. **Inspect a device & observations** (`device-security-detail.png` / `mobile-device-security.png`): Per-device compliance card (`4 / 5 items passing`, `score 81.8 · 9 unknown`) and TLS/certificate inventory.
4. **Understand capabilities & evidence** (`device-support-evidence.png` / `mobile-support-evidence.png`): Separates `Documented` (vendor source), `Observed` (live driver answer), and `Unknown` across the 126-family catalog.
5. **Preview & verify config changes** (`config-dryrun-preview.png` / `mobile-config-dryrun.png`): Dry-run preview (`would_write` vs `unchanged`) followed by read-back verification (`verified`, `diverged`, `unverifiable`) and one-click revert.

## Signature Visual Interaction: Evidence Trace
Traces `Device → Observation → Policy result → Evidence → Available next action` across:
- **Failing check (`protocols.snmp.v1v2cEnabled = true`)**: Fails `Baseline` rule `snmp-v1v2c-off`, flagged with `managementPath: "snmp"`, requires vendor `SecurityWriter` and alternative path to remediate safely.
- **Unknown evidence (`security.adminPassword.set = nil`)**: Recorded as `? Unknown`, excluded from score denominator, assess-only.
- **Passing check (`protocols.telnet.enabled = false`, `protocols.https.enabled = true`)**: Verified by live TCP/TLS port probe on `:23` and `:443`.
