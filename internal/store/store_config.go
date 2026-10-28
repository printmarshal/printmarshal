package store

import (
	"context"

	"printmarshal/internal/model"
)

// WriteRecordFilter selects config write records.
type WriteRecordFilter struct {
	DeviceID  string
	ProfileID string
	JobID     string
	Limit     int
}

// ConfigStore is the persistence surface for the config area (spec §8.4). It is
// defined here (own file), implemented on *SQLite in sqlite_config.go, with
// tables in schema_config.sql. Keep it additive.
type ConfigStore interface {
	// Profiles: named, reusable sets of desired settings.
	CreateProfile(ctx context.Context, p *model.Profile) error
	UpdateProfile(ctx context.Context, p *model.Profile) error
	DeleteProfile(ctx context.Context, id string) error
	GetProfile(ctx context.Context, id string) (*model.Profile, error)
	ListProfiles(ctx context.Context) ([]*model.Profile, error)

	// Snapshots: every config read is kept for history, drift and diffs.
	SaveConfigSnapshot(ctx context.Context, s *model.ConfigSnapshot) error
	GetConfigSnapshot(ctx context.Context, id string) (*model.ConfigSnapshot, error)
	LatestConfigSnapshot(ctx context.Context, deviceID string) (*model.ConfigSnapshot, error)
	ListConfigSnapshots(ctx context.Context, deviceID string, limit int) ([]*model.ConfigSnapshot, error)
	PruneConfigSnapshots(ctx context.Context, deviceID string, keep int) (int64, error)

	// Write records: per-device outcomes of every apply, with rollback data.
	SaveWriteRecord(ctx context.Context, w *model.WriteRecord) error
	GetWriteRecord(ctx context.Context, id string) (*model.WriteRecord, error)
	ListWriteRecords(ctx context.Context, f WriteRecordFilter) ([]*model.WriteRecord, error)

	// Drift reports: the last comparison of a device against a profile (cache).
	SaveDriftReport(ctx context.Context, r *model.DriftReport) error
	ListDriftReports(ctx context.Context, profileID string, limit int) ([]*model.DriftReport, error)
}
