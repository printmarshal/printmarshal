// Package hp is the HP vendor plugin (IANA enterprise 11, plus 236 for
// legacy Samsung badges that HP now owns).
//
// Match rule: sysObjectID enterprise 11 or 236; when an agent reports no
// enterprise, a sysDescr / hrDeviceDescr leading "HP" / "Hewlett-Packard" /
// "Samsung"; over IPP, printer-device-id MFG "HP" / "Hewlett-Packard".
//
// Capability / evidence table (see internal/plugins/README.md for sources):
//
//	identify         observed   PML product number, model, serial, firmware date code; Jetdirect sysDescr SN/PID
//	read_status      observed   HR/Printer MIB + PML panel line; LEDM ProductStatusDyn alerts (translated) when reachable
//	read_counters    observed   PML total / mono / colour / duplex; mono+colour must equal total
//	firmware_read    observed   PML date code + component table
//	read_config      observed   LEDM /DevMgmt pages -> canonical keys + vendor.hp.*; PML facts for devices without LEDM
//	read_security    observed   protocols.* / security.adminPassword.set from LEDM
//	write_config     unsupported  no public write contract (EWS/WJA are vendor tools)
//	write_security   unsupported
//	manage_certs     unsupported  EST connector is manager-side (WXP), not a device API
//	firmware_update  unsupported  .bdl is delivered through EWS / Web Jetadmin only
//
// Everything above marked observed was exercised read-only on 2026-10-28
// against an HP Color LaserJet MFP M283fdw (LaserJet Pro class). FutureSmart
// Enterprise / Managed devices share the PML identity and counter objects by
// HP's own MIB lineage but were not available for validation: treat them as
// documented-not-observed until a FutureSmart unit confirms them.
package hp
