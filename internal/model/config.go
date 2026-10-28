package model

import "time"

// --- Configuration management (spec §8.4) ------------------------------------

// Setting is one normalised device setting inside a ConfigDocument.
type Setting struct {
	// Key is the stable, vendor-neutral identifier, "<area>.<name>" in dotted
	// lower-camel form, e.g. "network.hostname", "energy.sleepDelayMinutes",
	// "protocols.snmpV1V2cEnabled", "security.adminPasswordSet".
	Key            string `json:"key"`
	Area           string `json:"area"` // network | protocols | security | trays | energy | panel | identity | print | scan | other
	Name           string `json:"name"` // human label
	Value          any    `json:"value"`
	Type           string `json:"type"`              // string | int | bool | enum | list | secret
	Options        []any  `json:"options,omitempty"` // allowed values for enum
	Writable       bool   `json:"writable"`
	Source         string `json:"source"` // driver that read it
	ReadOnlyReason string `json:"readOnlyReason,omitempty"`
}

// ConfigDocument is a device's current configuration, normalised by area.
type ConfigDocument struct {
	DeviceID string    `json:"deviceId"`
	Settings []Setting `json:"settings"`
	ReadAt   time.Time `json:"readAt"`
	Sources  []string  `json:"sources"` // drivers consulted
}

// Get returns a setting by key.
func (d *ConfigDocument) Get(key string) (*Setting, bool) {
	for i := range d.Settings {
		if d.Settings[i].Key == key {
			return &d.Settings[i], true
		}
	}
	return nil, false
}

// SettingChange is one desired value.
type SettingChange struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// ConfigSelector narrows a config read.
type ConfigSelector struct {
	Areas []string `json:"areas,omitempty"`
	Keys  []string `json:"keys,omitempty"`
}

// WriteOptions gate a write (spec §3 principle 4).
type WriteOptions struct {
	DryRun bool `json:"dryRun"`
}

// SettingResultStatus is the per-setting outcome of a write.
type SettingResultStatus string

const (
	SettingWritten    SettingResultStatus = "written"
	SettingUnchanged  SettingResultStatus = "unchanged"   // already had the desired value
	SettingWouldWrite SettingResultStatus = "would_write" // dry run
	SettingSkipped    SettingResultStatus = "skipped"     // unsupported / read-only
	SettingFailed     SettingResultStatus = "failed"
)

// SettingResult is one setting's outcome.
type SettingResult struct {
	Key    string              `json:"key"`
	Status SettingResultStatus `json:"status"`
	Before any                 `json:"before,omitempty"`
	After  any                 `json:"after,omitempty"`
	Error  string              `json:"error,omitempty"`
	Driver string              `json:"driver,omitempty"`
}

// WriteResult is a per-device write outcome.
type WriteResult struct {
	DeviceID string          `json:"deviceId"`
	DryRun   bool            `json:"dryRun"`
	Results  []SettingResult `json:"results"`
	At       time.Time       `json:"at"`
	// Rollback holds the previous values of everything written, for revert.
	Rollback []SettingChange `json:"rollback,omitempty"`
}

// Profile is a named, reusable set of desired settings (spec §8.4).
type Profile struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Settings    []SettingChange `json:"settings"`
	CreatedBy   string          `json:"createdBy"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

// DriftItem is one setting that differs from a profile.
type DriftItem struct {
	Key     string `json:"key"`
	Name    string `json:"name,omitempty"`
	Desired any    `json:"desired"`
	Current any    `json:"current"`
	Missing bool   `json:"missing"` // the device does not expose the setting
}

// DriftReport compares one device with a profile.
type DriftReport struct {
	DeviceID  string      `json:"deviceId"`
	ProfileID string      `json:"profileId"`
	Items     []DriftItem `json:"items"`
	Drifted   int         `json:"drifted"`
	Matching  int         `json:"matching"`
	At        time.Time   `json:"at"`
}

// ConfigSnapshot stores a read for history and device-vs-device diffs.
type ConfigSnapshot struct {
	ID       string          `json:"id"`
	DeviceID string          `json:"deviceId"`
	Document *ConfigDocument `json:"document"`
	At       time.Time       `json:"at"`
}
