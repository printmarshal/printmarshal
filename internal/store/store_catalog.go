package store

import (
	"context"

	"printmarshal/internal/catalog"
)

// CatalogStore is the persistence surface for the device model catalogue
// feature: operator-set qualification overrides per catalogue family. It is
// defined here (own file), implemented on *SQLite in sqlite_catalog.go, with
// its table in schema_catalog.sql. Keep it additive.
//
// It is not yet embedded in Store (store.go is shared); the API reaches it
// through a type assertion until the embed line lands.
type CatalogStore interface {
	// SetFamilyQualification records (or replaces) the operator's qualification of a family.
	SetFamilyQualification(ctx context.Context, q *catalog.Qualification) error
	// GetFamilyQualification returns the override for a family, or ErrNotFound.
	GetFamilyQualification(ctx context.Context, familyID string) (*catalog.Qualification, error)
	// ListFamilyQualifications returns every override, keyed by family id.
	ListFamilyQualifications(ctx context.Context) (map[string]*catalog.Qualification, error)
}
