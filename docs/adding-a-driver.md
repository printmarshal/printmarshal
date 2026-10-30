# Adding a driver or vendor plugin

A driver serves one or more *capabilities* for devices (spec §6). The two standards
drivers (`internal/driver/snmp`, `internal/driver/ipp`) are the baseline; a vendor
plugin is just another driver that talks to a manufacturer's device web service.

1. Implement `engine.Driver`:

   ```go
   type Driver interface {
       Name() string
       Capabilities() []model.Capability
       Supports(d *model.Device, cap model.Capability) bool   // no network I/O
       Priority(cap model.Capability) int                     // higher wins
   }
   ```

   `engine.ProfileSupports(d, name, cap)` is the standard `Supports` rule: serve the
   capability when the device's profile assigned it to you, or when nobody serves it
   yet and you identified the device.

2. Implement the typed interface for each capability you declare: `engine.Identifier`,
   `StatusReader`, `SupplyReader`, `TrayReader`, `CounterReader`, `FirmwareReader`,
   and on the write side `ConfigReader` / `ConfigWriter` (see
   [`configuration.md`](configuration.md): map your fields onto the catalogue keys,
   honour `WriteOptions.DryRun` with zero write I/O, report every setting, fill
   `Rollback`), `SecurityReader` / `SecurityWriter`, `CertManager`, `FirmwareUpdater`.

3. In `Identify`, return only the capabilities you can actually serve for *that*
   device (probe once, cheaply). The engine merges identities by priority and records
   your name against each capability you win.
   Before the drivers fan out, the engine reads the system group once (`engine.Preflight`:
   whether SNMP answered, the sysObjectID enterprise, sysDescr). A driver that can tell
   from that alone it does not apply implements `engine.Gated` (`Applies(Preflight) bool`)
   and is then skipped without opening a session; `vendorkit.Base` does this for its
   enterprise numbers. When a read that decides "is this a printer" times out, return a
   read failure (the SNMP driver's `ErrReadFailed`), never "not a printer" (issue #19).

4. Register the driver at startup in `cmd/printmarshal/main.go`:

   ```go
   reg.Register(myvendor.New(opts))
   ```

   Nothing else in the core changes. Vendor enterprise OIDs, private REST/XML endpoints
   and model quirks all live inside the plugin.

5. Record real device responses as fixtures under `testdata/fixtures` and unit-test the
   parser against them; add the driver to the contract test so it remains
   interchangeable with the standards drivers.

## Vendor plugins

Built-in vendor plugins live under `internal/plugins/<vendor>/` and share
`internal/plugins/vendorkit` (enterprise-number and badge matching, SNMP
helpers, canonical-key mapping, evidence descriptors). Start a new vendor from
an existing skeleton such as `internal/plugins/oki`; the capability/evidence
matrix and research citations are in `internal/plugins/README.md`.
