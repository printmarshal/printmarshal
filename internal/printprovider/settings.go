// Package printprovider is the optional print-provider add-on (spec §8.7): a
// CUPS / IPP Everywhere bridge that creates and serves local print queues for
// discovered printers. It is off by default, lives entirely outside the core,
// and only ever configures the local CUPS instance — it never writes to a
// printer. It is a convenience for the "run it as a print server too" case,
// not fleet management.
package printprovider

import (
	"fmt"
	"regexp"
	"strings"
)

// SettingsKey is the settings-table key the module persists under.
const SettingsKey = "printprovider"

// JobType is the job type of a queue synchronisation run.
const JobType = "printprovider.sync"

// Modes.
const (
	// ModeCUPS creates and maintains queues on the local CUPS via lpadmin.
	ModeCUPS = "cups"
	// ModeList only publishes the IPP Everywhere URIs clients can add
	// themselves; nothing on this host is changed.
	ModeList = "ipp-everywhere-list"
)

// Settings is the module configuration, persisted via store.SetSetting.
type Settings struct {
	Enabled bool   `json:"enabled"`
	Mode    string `json:"mode"` // cups | ipp-everywhere-list
	// QueuePrefix is prepended to every queue name; it is also how the module
	// recognises queues it owns, so it must never be empty.
	QueuePrefix string `json:"queuePrefix"`
	// DefaultOptions are lpadmin -o key=value pairs applied to every queue,
	// whitespace separated (e.g. "media=iso_a4_210x297mm sides=two-sided-long-edge").
	DefaultOptions string `json:"defaultOptions"`
	// AutoCreateForGroupID restricts queue creation to one group; empty means
	// every eligible device.
	AutoCreateForGroupID string `json:"autoCreateForGroupId"`
	// Driverless uses the CUPS "everywhere" model (IPP Everywhere); when false
	// a raw queue is created and clients supply the driver.
	Driverless bool `json:"driverless"`
}

// DefaultSettings is the off-by-default configuration.
func DefaultSettings() Settings {
	return Settings{Enabled: false, Mode: ModeCUPS, QueuePrefix: "pm-", Driverless: true}
}

var (
	prefixRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	optKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Validate checks the settings and normalises whitespace.
func (s *Settings) Validate() error {
	s.Mode = strings.TrimSpace(s.Mode)
	if s.Mode == "" {
		s.Mode = ModeCUPS
	}
	if s.Mode != ModeCUPS && s.Mode != ModeList {
		return fmt.Errorf("mode must be %q or %q", ModeCUPS, ModeList)
	}
	s.QueuePrefix = strings.TrimSpace(s.QueuePrefix)
	if !prefixRe.MatchString(s.QueuePrefix) {
		return fmt.Errorf("queuePrefix must be 1-32 characters of letters, digits, '_' or '-' (it marks the queues this module owns)")
	}
	s.DefaultOptions = strings.Join(strings.Fields(s.DefaultOptions), " ")
	if _, err := ParseOptions(s.DefaultOptions); err != nil {
		return err
	}
	s.AutoCreateForGroupID = strings.TrimSpace(s.AutoCreateForGroupID)
	return nil
}

// ParseOptions splits "k=v k2=v2" into ordered pairs.
func ParseOptions(s string) ([][2]string, error) {
	var out [][2]string
	for _, f := range strings.Fields(s) {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" || !optKeyRe.MatchString(k) {
			return nil, fmt.Errorf("defaultOptions: %q is not key=value", f)
		}
		out = append(out, [2]string{k, v})
	}
	return out, nil
}
