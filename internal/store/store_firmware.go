package store

// FirmwareStore is the persistence surface for the firmware area. It is defined here
// (own file), implemented on *SQLite in sqlite_firmware.go, with tables in
// schema_firmware.sql. Keep it additive.
type FirmwareStore interface {
}
