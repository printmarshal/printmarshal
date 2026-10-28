package model

import "time"

// --- Firmware (spec §8.6) -----------------------------------------------------

// FirmwarePackage is an uploaded/registered firmware image.
type FirmwarePackage struct {
	ID          string    `json:"id"`
	Vendor      string    `json:"vendor"` // make this package applies to (free text matched case-insensitively)
	Models      []string  `json:"models"` // model substrings this package is compatible with
	Version     string    `json:"version"`
	MinVersion  string    `json:"minVersion,omitempty"` // required installed version (or newer) for upgrade
	FileName    string    `json:"fileName"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	StoragePath string    `json:"-"` // local path under the data dir
	Notes       string    `json:"notes,omitempty"`
	CreatedBy   string    `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CampaignStatus is the lifecycle of a firmware campaign.
type CampaignStatus string

const (
	CampaignDraft     CampaignStatus = "draft"
	CampaignScheduled CampaignStatus = "scheduled"
	CampaignRunning   CampaignStatus = "running"
	CampaignPaused    CampaignStatus = "paused" // waiting for canary confirmation or paused by failure threshold
	CampaignCompleted CampaignStatus = "completed"
	CampaignFailed    CampaignStatus = "failed"
	CampaignCancelled CampaignStatus = "cancelled"
)

// MaintenanceWindow limits when devices may be updated.
type MaintenanceWindow struct {
	Days  []string `json:"days,omitempty"`  // mon..sun; empty = any
	Start string   `json:"start,omitempty"` // "22:00"
	End   string   `json:"end,omitempty"`   // "06:00"
	TZ    string   `json:"tz,omitempty"`
}

// Campaign is a staged firmware rollout.
type Campaign struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	PackageID   string             `json:"packageId"`
	GroupID     string             `json:"groupId,omitempty"`
	DeviceIDs   []string           `json:"deviceIds,omitempty"` // explicit targets when no group
	CanaryCount int                `json:"canaryCount"`         // devices in wave 1; wait for confirmation after
	Concurrency int                `json:"concurrency"`
	Window      *MaintenanceWindow `json:"window,omitempty"`
	// FailureThresholdPercent pauses the campaign when exceeded (0 = never).
	FailureThresholdPercent int            `json:"failureThresholdPercent"`
	Status                  CampaignStatus `json:"status"`
	Wave                    int            `json:"wave"`      // 1 = canary, 2 = remainder
	Confirmed               bool           `json:"confirmed"` // operator confirmed after canary
	ScheduledAt             *time.Time     `json:"scheduledAt,omitempty"`
	StartedAt               *time.Time     `json:"startedAt,omitempty"`
	FinishedAt              *time.Time     `json:"finishedAt,omitempty"`
	CreatedBy               string         `json:"createdBy"`
	CreatedAt               time.Time      `json:"createdAt"`
	UpdatedAt               time.Time      `json:"updatedAt"`
	Error                   string         `json:"error,omitempty"`
}

// CampaignDeviceStatus is a device's progress inside a campaign.
type CampaignDeviceStatus string

const (
	CampaignDevicePending   CampaignDeviceStatus = "pending"
	CampaignDeviceSkipped   CampaignDeviceStatus = "skipped" // incompatible / already current / excluded
	CampaignDeviceUpdating  CampaignDeviceStatus = "updating"
	CampaignDeviceVerifying CampaignDeviceStatus = "verifying"
	CampaignDeviceSucceeded CampaignDeviceStatus = "succeeded"
	CampaignDeviceFailed    CampaignDeviceStatus = "failed"
)

// CampaignDevice is one target of a campaign.
type CampaignDevice struct {
	CampaignID  string               `json:"campaignId"`
	DeviceID    string               `json:"deviceId"`
	Wave        int                  `json:"wave"`
	Status      CampaignDeviceStatus `json:"status"`
	FromVersion string               `json:"fromVersion,omitempty"`
	ToVersion   string               `json:"toVersion,omitempty"`
	Message     string               `json:"message,omitempty"`
	StartedAt   *time.Time           `json:"startedAt,omitempty"`
	FinishedAt  *time.Time           `json:"finishedAt,omitempty"`
}

// FirmwareResult is a driver's outcome for one device update.
type FirmwareResult struct {
	DeviceID string    `json:"deviceId"`
	DryRun   bool      `json:"dryRun"`
	Accepted bool      `json:"accepted"` // device accepted the image
	Verified bool      `json:"verified"` // post-update version matched
	Version  string    `json:"version,omitempty"`
	Message  string    `json:"message,omitempty"`
	At       time.Time `json:"at"`
}
