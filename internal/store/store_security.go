package store

// SecurityStore is the persistence surface for the security area. It is defined here
// (own file), implemented on *SQLite in sqlite_security.go, with tables in
// schema_security.sql. Keep it additive.
type SecurityStore interface {
}
