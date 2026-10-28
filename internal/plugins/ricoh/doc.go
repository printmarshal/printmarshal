// Package ricoh is the Ricoh vendor plugin (IANA enterprise 367), covering
// the Savin, Lanier, Gestetner, Nashuatec, Rex-Rotary, NRG and IKON badges
// that sell Ricoh engines.
//
// Match rule: sysObjectID enterprise 367. When an agent reports no
// enterprise, a leading "RICOH" / badge word in sysDescr or hrDeviceDescr;
// over IPP, printer-device-id MFG RICOH / SAVIN / LANIER / GESTETNER /
// NASHUATEC / REX-ROTARY / NRG. Identity.Make is always "Ricoh"; the badge the
// device presents is kept in the model text and in vendor.ricoh.badge.
//
// Capability / evidence table (see internal/plugins/README.md for sources):
//
//	identify         documented   enterprise 367; JISEC-enumerated badge set [R23]
//	read_config      documented   identity.* (SNMP system group) + vendor.ricoh.badge / family / sysObjectId
//	read_counters    unverified   private-MIB engine counter table (community OID, probe-gated on a "Total" row)
//	read_status      unsupported  served by the standards drivers
//	read_supplies    unsupported  served by the standards drivers
//	read_trays       unsupported  served by the standards drivers
//	read_security    unsupported  Web Image Monitor only; no documented API [R10] [R11]
//	write_config     unsupported  PMAPI / Device Interfaces need RiDP access [R08] [R09]
//	write_security   unsupported
//	manage_certs     unsupported  native SCEP/EST/ACME not established [R11]
//	firmware_update  unsupported  no public update endpoint may be inferred from the Firmware Update Tool / ACT [R21] [R27]
//
// What a real-hardware test should verify next (research backlog, priority 1):
// confirm the counter table OID/columns and names on an IM C3010, obtain the
// RiDP Standard/Private MIB specifications, and record the IM C3000 (HDD,
// older controller) and IM 3500 (mono) variants [R01] [R02] [R07] [R08].
package ricoh
