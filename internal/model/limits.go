package model

// Lengths of operator fields, shared by the API's edits and the CSV import
// (intent/csv-import) so both refuse the same values.
const (
	// MaxLocation bounds a device's location.
	MaxLocation = 200
	// MaxNote bounds a device's notes, and a note a bulk edit appends.
	MaxNote = 2000
	// MaxAssetTag bounds an asset tag (applied by the import).
	MaxAssetTag = 200
	// MaxDisplayName bounds a display name (applied by the import).
	MaxDisplayName = 200
)
