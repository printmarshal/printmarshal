package store

// ConfigStore is the persistence surface for the config area. It is defined here
// (own file), implemented on *SQLite in sqlite_config.go, with tables in
// schema_config.sql. Keep it additive.
type ConfigStore interface {
}
