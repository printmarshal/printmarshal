# PrintMarshal — Features & Capability Status (`site/features/index.html`)

## Status Labels
- **Implemented:** Ships end-to-end in the binary and works against supported protocols or standard endpoints today.
- **Device-dependent:** Implemented in PrintMarshal; depth or writability depends on what the target printer exposes via SNMP/IPP/eSCL.
- **Requires additional driver:** Workflow, UI, and job engine ship today, but device execution requires a vendor driver interface not yet shipped.
- **Simulator-only:** End-to-end execution is restricted to opt-in simulator targets (`pm-sim` / `fwsim`) for safe workflow evaluation.

## Functional Areas
1. **Security Posture Assessment & Certificates** (`Implemented` assessment, TCP/TLS probes, and SCEP/EST/ACME DNS-01 enrollment; `Requires additional driver` for `SecurityWriter` remediation/rotation writes and `CertManager` certificate installation; no auto-renewal).
2. **Fleet Discovery & Live Monitoring** (`Implemented` active/passive discovery, `serial → UUID → MAC → IP` reconciliation, SNMPv1/v2c traps, 14-day consumable burn-rate forecasting, and static/dynamic rule groups; telemetry depth is `Device-dependent`).
3. **Configuration Management & Dry-Run Writes** (`Implemented` 35-key canonical catalog across 7 areas, 50 snapshots per device, diff, drift, dry-run preview, read-back verification, and rollback history; writable keys over standards `snmp` and `ipp` are `Device-dependent`; no vendor-private `ConfigWriter` ships today).
4. **Device Model Catalogue & Drivers** (126 enterprise printer families / 495 base model IDs across 15 brands shipped as `research_candidate_unvalidated` with `0` physically validated in the shipped catalog; 3 standards drivers (`snmp`, `ipp`, `escl`) and 13 compiled-in vendor plugins).
5. **Firmware Campaigns, Print Release & Observability** (`Implemented` firmware artifact SHA-256 verification and canary/wave campaign engine; `Simulator-only` firmware delivery via `fwsim` on `PM_FIRMWARE_SIM_TARGETS`; `Device-dependent` IPP hold/release + PaperCut/uniFLOW CSV/JSON accounting import; `Implemented` Prometheus `/metrics` and RFC 5424 Syslog/CEF audit forwarding).
