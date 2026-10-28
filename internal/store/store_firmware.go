package store

import (
	"context"

	"printmarshal/internal/model"
)

// CampaignFilter selects firmware campaigns.
type CampaignFilter struct {
	Status    string
	PackageID string
	// Active selects campaigns that are not finished (scheduled, running, paused, draft).
	Active bool
	Limit  int
}

// FirmwareStore is the persistence surface for the firmware area. It is defined here
// (own file), implemented on *SQLite in sqlite_firmware.go, with tables in
// schema_firmware.sql. Keep it additive.
type FirmwareStore interface {
	// Packages
	CreateFirmwarePackage(ctx context.Context, p *model.FirmwarePackage) error
	GetFirmwarePackage(ctx context.Context, id string) (*model.FirmwarePackage, error)
	ListFirmwarePackages(ctx context.Context) ([]*model.FirmwarePackage, error)
	DeleteFirmwarePackage(ctx context.Context, id string) error

	// Campaigns
	CreateCampaign(ctx context.Context, c *model.Campaign) error
	UpdateCampaign(ctx context.Context, c *model.Campaign) error
	GetCampaign(ctx context.Context, id string) (*model.Campaign, error)
	ListCampaigns(ctx context.Context, f CampaignFilter) ([]*model.Campaign, error)
	DeleteCampaign(ctx context.Context, id string) error

	// Campaign devices (the detailed per-device state)
	UpsertCampaignDevice(ctx context.Context, d *model.CampaignDevice) error
	ListCampaignDevices(ctx context.Context, campaignID string) ([]*model.CampaignDevice, error)
	DeleteCampaignDevices(ctx context.Context, campaignID string) error
	// FirmwareHistory lists every campaign a device took part in, newest first.
	FirmwareHistory(ctx context.Context, deviceID string, limit int) ([]*model.FirmwareHistoryEntry, error)
}
