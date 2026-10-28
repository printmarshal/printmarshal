// Package store is the data-access layer (spec §5, §9). Store is an interface
// so the SQLite default and a PostgreSQL implementation can coexist behind it.
package store

import (
	"context"
	"errors"
	"time"

	"printmarshal/internal/model"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("not found")

// DeviceFilter selects devices for listing (spec §8.2 search/sort/filter).
type DeviceFilter struct {
	Q        string // free text over name, model, serial, address, location, asset tag
	State    string
	Make     string
	Model    string
	Severity string // ok | warning | critical | offline
	GroupIDs []string
	IDs      []string
	Sort     string // name | model | address | lastSeen | severity
	Limit    int
	Offset   int
	// IncludeDecommissioned includes decommissioned devices (default false).
	IncludeDecommissioned bool
}

// DevicePatch holds operator-editable fields (spec §8.2). Nil = unchanged.
type DevicePatch struct {
	DisplayName *string
	Location    *string
	AssetTag    *string
	Notes       *string
}

// PollUpdate is what a poll writes back onto a device (automatic fields only).
type PollUpdate struct {
	Status    *model.Status
	Supplies  []model.Supply
	Trays     []model.Tray
	Counters  *model.Counters
	Firmware  string
	Reachable bool
	At        time.Time
}

// AlertFilter selects alerts.
type AlertFilter struct {
	DeviceID   string
	ActiveOnly bool
	Severity   string
	Limit      int
}

// JobFilter selects jobs.
type JobFilter struct {
	Type   string
	Status string
	Limit  int
}

// TelemetryQuery selects samples.
type TelemetryQuery struct {
	DeviceID string
	Metric   string
	Label    string
	From     time.Time
	To       time.Time
	Limit    int
}

// ConsumableFilter selects consumables.
type ConsumableFilter struct {
	Q      string // free text over name, part number, location, notes, compatible models
	Type   string
	Color  string
	Status string // ok | low | critical | out (derived; filtered in Go)
	Model  string // compatible-model substring
	IDs    []string
}

// StockEventFilter selects stock events.
type StockEventFilter struct {
	ConsumableID string
	DeviceID     string
	Kind         string
	Status       string
	Limit        int
}

// SupplyLinkFilter selects supply links.
type SupplyLinkFilter struct {
	DeviceID     string
	ConsumableID string
	Source       string
}

// DashboardSummary feeds the dashboard.
type DashboardSummary struct {
	Devices        int            `json:"devices"`
	ByState        map[string]int `json:"byState"`
	BySeverity     map[string]int `json:"bySeverity"`
	Offline        int            `json:"offline"`
	ActiveAlerts   int            `json:"activeAlerts"`
	CriticalAlerts int            `json:"criticalAlerts"`
	LowSupplies    int            `json:"lowSupplies"`
	LowStock       int            `json:"lowStock"` // consumables at or below their warning threshold
	RunningJobs    int            `json:"runningJobs"`
}

// Store is the full persistence surface. Feature areas add their own
// sub-interface in a file of their own (store_<area>.go) and implement it on
// *SQLite in sqlite_<area>.go with its tables in schema_<area>.sql.
type Store interface {
	ConfigStore
	SecurityStore
	FirmwareStore
	OutboxStore

	Close() error

	// Devices
	UpsertDevice(ctx context.Context, id *model.Identity, caps map[model.Capability]string, discoveryMethods []string) (dev *model.Device, created bool, err error)
	GetDevice(ctx context.Context, id string) (*model.Device, error)
	ListDevices(ctx context.Context, f DeviceFilter) ([]*model.Device, error)
	CountDevices(ctx context.Context, f DeviceFilter) (int, error)
	PatchDevice(ctx context.Context, id string, p DevicePatch) (*model.Device, error)
	SetDeviceState(ctx context.Context, id string, state model.LifecycleState) (*model.Device, error)
	ApplyPoll(ctx context.Context, id string, u PollUpdate) (*model.Device, error)
	DeviceFailures(ctx context.Context, id string) (int, error)
	DeleteDevice(ctx context.Context, id string) error
	Dashboard(ctx context.Context) (*DashboardSummary, error)

	// Groups
	CreateGroup(ctx context.Context, g *model.Group) error
	UpdateGroup(ctx context.Context, g *model.Group) error
	DeleteGroup(ctx context.Context, id string) error
	GetGroup(ctx context.Context, id string) (*model.Group, error)
	ListGroups(ctx context.Context) ([]*model.Group, error)
	SetGroupMembers(ctx context.Context, groupID string, deviceIDs []string) error
	GroupsForDevice(ctx context.Context, deviceID string) ([]string, error)

	// Credentials (secrets are write-only through this interface except ResolveCreds)
	CreateCredential(ctx context.Context, c *model.Credential, secret *model.Secret) error
	ListCredentials(ctx context.Context) ([]*model.Credential, error)
	DeleteCredential(ctx context.Context, id string) error
	ResolveCreds(ctx context.Context, deviceID string, groupIDs []string) (model.Creds, error)

	// Jobs
	CreateJob(ctx context.Context, j *model.Job) error
	UpdateJob(ctx context.Context, j *model.Job) error
	GetJob(ctx context.Context, id string) (*model.Job, error)
	ListJobs(ctx context.Context, f JobFilter) ([]*model.Job, error)
	AddJobResult(ctx context.Context, r *model.JobResult) error
	ListJobResults(ctx context.Context, jobID string) ([]*model.JobResult, error)
	MarkInterruptedJobs(ctx context.Context) (int, error)

	// Telemetry
	AddSamples(ctx context.Context, samples []model.TelemetrySample) error
	QueryTelemetry(ctx context.Context, q TelemetryQuery) ([]model.TelemetrySample, error)
	PruneTelemetry(ctx context.Context, before time.Time) (int64, error)

	// Alerts
	ListAlerts(ctx context.Context, f AlertFilter) ([]*model.Alert, error)
	GetAlert(ctx context.Context, id string) (*model.Alert, error)
	RaiseAlert(ctx context.Context, deviceID, key, source string, sev model.Severity, desc string) (a *model.Alert, raised bool, err error)
	ClearAlert(ctx context.Context, id string) (*model.Alert, error)
	ClearAlertsExcept(ctx context.Context, deviceID string, source string, keepKeys []string) ([]*model.Alert, error)
	AckAlert(ctx context.Context, id, by string) (*model.Alert, error)

	// Notifications, audit
	AddNotificationLog(ctx context.Context, l *model.NotificationLog) error
	ListNotificationLog(ctx context.Context, limit int) ([]*model.NotificationLog, error)
	AddAudit(ctx context.Context, e *model.AuditEntry) error
	ListAudit(ctx context.Context, limit int) ([]*model.AuditEntry, error)

	// Users and sessions
	CreateUser(ctx context.Context, u *model.User, passwordHash string) error
	GetUserByName(ctx context.Context, username string) (*model.User, string, error)
	GetUser(ctx context.Context, id string) (*model.User, error)
	ListUsers(ctx context.Context) ([]*model.User, error)
	CountUsers(ctx context.Context) (int, error)
	DeleteUser(ctx context.Context, id string) error
	SetPassword(ctx context.Context, id, passwordHash string) error
	CreateSession(ctx context.Context, tokenHash, userID string, expires time.Time) error
	GetSession(ctx context.Context, tokenHash string) (*model.User, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	// DeleteUserSessions revokes every session of a user except keepHash ("" = all).
	DeleteUserSessions(ctx context.Context, userID, keepHash string) (int64, error)

	// Consumables (stock items are operator-owned; polls never write them)
	ListConsumables(ctx context.Context, f ConsumableFilter) ([]*model.Consumable, error)
	GetConsumable(ctx context.Context, id string) (*model.Consumable, error)
	CreateConsumable(ctx context.Context, c *model.Consumable) error
	UpdateConsumable(ctx context.Context, c *model.Consumable) error
	DeleteConsumable(ctx context.Context, id string) error
	// AdjustStock atomically applies ev (Kind set: Resulting is the new quantity;
	// otherwise Delta is added, never below zero), records the event and returns
	// the updated item. Replacement events with a non-applied status are recorded
	// without changing the quantity.
	AdjustStock(ctx context.Context, id string, ev *model.StockEvent) (*model.Consumable, error)
	ListStockEvents(ctx context.Context, f StockEventFilter) ([]*model.StockEvent, error)
	GetStockEvent(ctx context.Context, id int64) (*model.StockEvent, error)
	SetStockEventStatus(ctx context.Context, id int64, status string) error

	// Supply links: installed supply (device slot) -> consumable
	ListSupplyLinks(ctx context.Context, f SupplyLinkFilter) ([]*model.SupplyLink, error)
	SetSupplyLink(ctx context.Context, l *model.SupplyLink) error
	DeleteSupplyLink(ctx context.Context, deviceID string, supplyIndex int) error
	// ReplaceAutoLinks replaces every auto link of a device with links, keeping manual ones.
	ReplaceAutoLinks(ctx context.Context, deviceID string, links []*model.SupplyLink) error

	// Settings (small JSON values)
	GetSetting(ctx context.Context, key string, dst any) error
	SetSetting(ctx context.Context, key string, v any) error
}
