package store

import (
	"context"
	"time"

	"printmarshal/internal/model"
)

// SyncCursor is the position of an incremental sync walk (audit A15): the raw
// stored timestamp of the last row seen and its id. Rows are ordered by
// (updatedAt, id) ascending with a strict comparison, so a client that
// resumes from the cursor of the last row it received neither misses nor
// repeats a row. The timestamp is kept as the stored string so that the
// comparison in SQL and the cursor agree byte for byte.
type SyncCursor struct {
	At string
	ID string
}

// Zero reports whether the cursor is the beginning of the collection.
func (c SyncCursor) Zero() bool { return c.At == "" && c.ID == "" }

// SyncRow is one row of a sync page: the cursor it advances to and the item.
// Item is the collection's model type (*model.Device, *model.Alert,
// *model.AuditEntry, *model.Job, *model.WriteRecord) or a *Tombstone.
type SyncRow struct {
	Cursor SyncCursor
	Item   any
}

// Tombstone records that a row of a collection was deleted so sync clients
// can retire it.
type Tombstone struct {
	ID        string    `json:"id"`
	Deleted   bool      `json:"deleted"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// LogStore persists printer syslog lines, deletion tombstones and the
// cursor-ordered sync queries (docs/export.md). It is defined here (own
// file), implemented on *SQLite in sqlite_logs.go, with tables in
// schema_export.sql. Keep it additive.
type LogStore interface {
	// Device logs (printer syslog receiver)
	AddDeviceLog(ctx context.Context, l *model.DeviceLog) error
	// ListDeviceLogs returns a device's lines, newest first.
	ListDeviceLogs(ctx context.Context, deviceID string, limit int) ([]*model.DeviceLog, error)
	PruneDeviceLogs(ctx context.Context, before time.Time) (int64, error)

	// Tombstones
	AddTombstone(ctx context.Context, collection, id string) error

	// Incremental sync: rows strictly after cursor, ordered by (updatedAt, id),
	// at most limit of them, and whether more remain.
	SyncDevices(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error)
	SyncAlerts(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error)
	SyncAudit(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error)
	SyncJobs(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error)
	SyncConfigWrites(ctx context.Context, after SyncCursor, limit int) ([]SyncRow, bool, error)
}
