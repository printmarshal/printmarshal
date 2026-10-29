# Homepage Copy (`site/index.html`)

## 1. Hero
- **Eyebrow:** Printer Fleet Security & Configuration Console
- **H1:** Know where your printer fleet is exposed.
- **Lead:** Printers hold network credentials, TLS certificates, and open management ports, yet most teams only notice them when toner runs out. PrintMarshal discovers your fleet over standard SNMP and IPP, audits device security without treating unreadable settings as passing, and previews configuration changes before writing.
- **Reassurance Row:**
  - **Read-only by default:** Discovery and polling never write to devices
  - **Explicit unknown states:** Unreadable checks are never counted as pass
  - **Dry-run & read-back:** Preview diffs first, then verify after apply
  - **Built-in simulator:** Test workflows locally with `pm-sim`
- **Visual:** `security-compliance.png` (desktop) / `mobile-security-compliance.png` (mobile) · Caption: `Fleet security overview · Simulator data`

## 2. Guided Product Walkthrough (`#walkthrough`)
- **H2:** From fleet discovery to verified configuration changes.
- **5 Steps (Desktop Tabs + Compact Mobile Stepper):**
  1. **Discover fleet (`01 · Read-Only`):** Reconciles by `serial → UUID → MAC → IP` with identity-conflict detection while preserving operator-set fields.
  2. **Assess baseline (`02 · Audit`):** Evaluates the 14-item Baseline policy (`pass`, `fail`, `unknown`), excluding unreadable settings from the score denominator.
  3. **Inspect findings (`03 · Inspect`):** Shows per-device failed checks, unknown items, and TLS certificates. Enrolls via internal CA, EST, or SCEP; physical install requires a vendor driver or printer web UI.
  4. **Check support (`04 · Evidence`):** Separates live SNMP/IPP observations from the 15-vendor, 495-model research catalog (`0` physically validated out of the box).
  5. **Dry-run & verify (`05 · Change`):** Previews live diffs before writing, verifies changes on read-back (`verified` vs `accepted`), and saves `prevValues` for rollback.

## 3. Evidence Trace Pipeline
- **H3:** How PrintMarshal turns a printer poll into a verified action.
- **Connected Sequence:** `01 Device → 02 Observation → 03 Finding → 04 Next action`
- **Scenarios:**
  - `× Failing check: SNMPv1/v2c` (`protocols.snmp.v1v2cEnabled`)
  - `? Unknown: Admin password` (`security.adminPasswordSet`)
  - `✓ Passing: Telnet & HTTPS` (`protocols.telnet.enabled`, `protocols.https.enabled`)

## 4. Core Capabilities (`#capabilities`)
- **H2:** Discover printers. Investigate risk. Review changes.
- **Splits:**
  1. Discover printers and monitor fleet health (`crop-fleet-inventory.png` → `fleet-discovery.png`)
  2. Investigate security risk and TLS certificates (`crop-device-security.png` → `device-security-detail.png`)
  3. Preview configuration changes and verify writes (`crop-config-dryrun.png` → `config-dryrun-preview.png`)

## 5. Capability Status Summary (`#coverage`)
- **H2:** What works with your printers today.
- Summarizes `● Implemented`, `◐ Device-dependent`, `◈ Simulator-only`, and `◌ Requires vendor driver` boundaries, including the optional off-by-default Linux local CUPS queue add-on (`internal/printprovider/`).

## 6. Open Source & Evaluation (`#open-source`, `#evaluate`)
- **H2:** Open source. Self-hosted. Built to extend.
- **Closing CTA H2:** Evaluate locally with the built-in simulator.
