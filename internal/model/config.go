package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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

// --- Additive extensions used by the configuration service -------------------

// ConfigTarget selects the devices a profile is applied to or checked against:
// explicit device ids and/or every device of a group (spec §8.4 "a profile
// targets a device or a group").
type ConfigTarget struct {
	DeviceIDs []string `json:"deviceIds,omitempty"`
	GroupID   string   `json:"groupId,omitempty"`
}

// Empty reports whether the target selects nothing.
func (t ConfigTarget) Empty() bool { return len(t.DeviceIDs) == 0 && t.GroupID == "" }

// WriteRecord is a persisted WriteResult with its provenance, so a write can be
// listed, audited and reverted later.
type WriteRecord struct {
	ID        string          `json:"id"`
	JobID     string          `json:"jobId,omitempty"`
	ProfileID string          `json:"profileId,omitempty"` // "" for ad-hoc changes
	RevertOf  string          `json:"revertOf,omitempty"`  // write record this one reverted
	Actor     string          `json:"actor"`
	Changes   []SettingChange `json:"changes"` // what was requested
	Result    WriteResult     `json:"result"`
	Written   int             `json:"written"`
	Failed    int             `json:"failed"`
	Skipped   int             `json:"skipped"`
	Unchanged int             `json:"unchanged"`
}

// Reversible reports whether the record holds previous values that a revert can re-apply.
func (w *WriteRecord) Reversible() bool {
	return w != nil && !w.Result.DryRun && len(w.Result.Rollback) > 0
}

// Count fills the per-status counters from the result.
func (w *WriteRecord) Count() {
	w.Written, w.Failed, w.Skipped, w.Unchanged = 0, 0, 0, 0
	for _, r := range w.Result.Results {
		switch r.Status {
		case SettingWritten, SettingWouldWrite:
			w.Written++
		case SettingFailed:
			w.Failed++
		case SettingSkipped:
			w.Skipped++
		case SettingUnchanged:
			w.Unchanged++
		}
	}
}

// ConfigDiffItem is one key that differs between two configuration documents.
type ConfigDiffItem struct {
	Key   string `json:"key"`
	Name  string `json:"name,omitempty"`
	Area  string `json:"area,omitempty"`
	A     any    `json:"a"`
	B     any    `json:"b"`
	OnlyA bool   `json:"onlyA,omitempty"` // present only in A
	OnlyB bool   `json:"onlyB,omitempty"` // present only in B
}

// ConfigDiff compares two documents.
type ConfigDiff struct {
	A     string           `json:"a"` // device id
	B     string           `json:"b"`
	Items []ConfigDiffItem `json:"items"`
	Same  int              `json:"same"`
}

// IsSecretKey reports whether a catalogue key holds a secret; such values are
// never stored in snapshots, write records or audit entries.
func IsSecretKey(key string) bool {
	d, ok := CatalogByKey[key]
	return ok && d.Type == "secret"
}

// RedactedValue replaces secret values wherever they would otherwise be persisted.
const RedactedValue = "<redacted>"

// CoerceValue converts a desired value (typically decoded from JSON, where every
// number is a float64) to the catalogue type of key: int, bool, string, enum
// (validated against Options), list ([]string) or secret (string). Unknown keys
// are returned unchanged.
func CoerceValue(key string, v any) (any, error) {
	def, ok := CatalogByKey[key]
	if !ok {
		return v, nil
	}
	switch def.Type {
	case "int":
		switch x := v.(type) {
		case int:
			return x, nil
		case int32:
			return int(x), nil
		case int64:
			return int(x), nil
		case float64:
			if x != float64(int(x)) {
				return nil, fmt.Errorf("%s: expected an integer, got %v", key, v)
			}
			return int(x), nil
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(x))
			if err != nil {
				return nil, fmt.Errorf("%s: expected an integer, got %q", key, x)
			}
			return n, nil
		}
		return nil, fmt.Errorf("%s: expected an integer, got %T", key, v)
	case "bool":
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(x))
			if err != nil {
				return nil, fmt.Errorf("%s: expected a boolean, got %q", key, x)
			}
			return b, nil
		case float64:
			return x != 0, nil
		case int:
			return x != 0, nil
		}
		return nil, fmt.Errorf("%s: expected a boolean, got %T", key, v)
	case "list":
		switch x := v.(type) {
		case []string:
			return x, nil
		case []any:
			out := make([]string, 0, len(x))
			for _, e := range x {
				out = append(out, fmt.Sprint(e))
			}
			return out, nil
		case string:
			var out []string
			for _, p := range strings.Split(x, ",") {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out, nil
		}
		return nil, fmt.Errorf("%s: expected a list, got %T", key, v)
	case "enum":
		s := fmt.Sprint(v)
		if len(def.Options) > 0 {
			for _, o := range def.Options {
				if fmt.Sprint(o) == s {
					return s, nil
				}
			}
			return nil, fmt.Errorf("%s: %q is not one of %v", key, s, def.Options)
		}
		return s, nil
	default: // string, secret
		if v == nil {
			return "", nil
		}
		if s, ok := v.(string); ok {
			return s, nil
		}
		return fmt.Sprint(v), nil
	}
}

// ValuesEqual compares two setting values loosely: numbers regardless of their
// Go type, strings exactly, lists element-wise, everything else by JSON form.
func ValuesEqual(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return fa == fb
		}
		return false
	}
	switch x := a.(type) {
	case string:
		y, ok := b.(string)
		return ok && x == y
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case nil:
		return b == nil
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}
