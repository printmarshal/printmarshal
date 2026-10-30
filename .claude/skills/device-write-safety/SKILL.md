---
name: device-write-safety
description: Apply PrintMarshal's device-write policy. Use whenever creating or changing code, tests, drivers or jobs that send anything but a read to a printer (SNMP SET, IPP Set-Printer-Attributes, firmware delivery, certificate install, credential rotation), or when running such code locally.
---

# Device write safety

Every path that writes to a device must hold these invariants:

1. **Dry run first.** The operation accepts `WriteOptions.DryRun` and performs zero device I/O
   when it is set; the API defaults to dry run and requires an explicit `dryRun:false`.
2. **Write credentials only.** Writes use a credential bound to the device with purpose `write`
   or `both`; a device with none is refused, never silently read-only.
3. **Read back and report.** After a real write the driver reads the value back and reports
   `verified`, `diverged` or `unverifiable`; "accepted" is never reported as success.
4. **Audit every write.** Actor, action, target, before/after and result go to the audit log
   and the event bus; a database failure is an operational failure to surface, not a reason
   to skip the record.
5. **Never remove the last management path** (remediation and configuration apply).
6. **Local runs target the simulator only.** `cmd/pm-sim` on loopback receives all writes in
   development and tests; the lab printer (10.113.1.205) is read-only, and the Bash hook
   blocks writes elsewhere.

When you change a write path, add or extend a test under the package that proves the dry run
performs no I/O and that a failed read-back is reported as diverged.
