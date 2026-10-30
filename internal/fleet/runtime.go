package fleet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/store"
)

// NameSettings configures which name source takes precedence in lists and headers.
type NameSettings struct {
	Source string `json:"source"` // advertised | dns | display
}

// RuntimeSettings are the operator-adjustable scheduler and alert settings.
// The config file/environment provide defaults; overrides saved through the
// API live in the settings table and take effect on the next scheduler tick.
type RuntimeSettings struct {
	PollInterval          config.Duration `json:"pollInterval"`
	PollConcurrency       int             `json:"pollConcurrency"`
	DiscoveryEnabled      bool            `json:"discoveryEnabled"`
	DiscoveryInterval     config.Duration `json:"discoveryInterval"`
	DiscoveryTargets      []string        `json:"discoveryTargets"`
	SupplyWarningPercent  int             `json:"supplyWarningPercent"`
	SupplyCriticalPercent int             `json:"supplyCriticalPercent"`
	OfflineAfterFailures  int             `json:"offlineAfterFailures"`
	TelemetryRetention    config.Duration `json:"telemetryRetention"`
	Names                 NameSettings    `json:"names"`
	// ReverseDNS is the lookup switch and its options (issue #18): the file
	// and PM_REVERSE_DNS_* variables are the defaults, Settings overrides
	// them without a restart.
	ReverseDNS config.ReverseDNS `json:"reverseDns"`
}

const runtimeKey = "runtime"

// Defaults returns the settings from the config file / environment.
func (s *Service) Defaults() RuntimeSettings {
	c := s.Cfg
	return RuntimeSettings{
		PollInterval: c.Poll.Interval, PollConcurrency: c.Poll.Concurrency,
		DiscoveryEnabled: c.Discovery.Enabled, DiscoveryInterval: c.Discovery.Interval, DiscoveryTargets: append([]string{}, c.Discovery.Targets...),
		SupplyWarningPercent: c.Alerts.SupplyWarningPercent, SupplyCriticalPercent: c.Alerts.SupplyCriticalPercent, OfflineAfterFailures: c.Alerts.OfflineAfterFailures,
		TelemetryRetention: c.Telemetry.Retention,
		Names:              NameSettings{Source: "advertised"},
		ReverseDNS:         c.Discovery.ReverseDNS,
	}
}

// Runtime returns the effective settings (defaults overlaid with saved overrides).
func (s *Service) Runtime(ctx context.Context) RuntimeSettings {
	eff := s.Defaults()
	var saved RuntimeSettings
	if err := s.Store.GetSetting(ctx, runtimeKey, &saved); err != nil {
		return eff
	}
	if saved.PollInterval == 0 {
		// ResetRuntime stores an empty record; SaveRuntime never stores a
		// poll interval under 30s, so this means "no overrides" and every
		// field, booleans included, comes from the file / environment.
		return eff
	}
	if saved.PollInterval > 0 {
		eff.PollInterval = saved.PollInterval
	}
	if saved.PollConcurrency > 0 {
		eff.PollConcurrency = saved.PollConcurrency
	}
	eff.DiscoveryEnabled = saved.DiscoveryEnabled
	if saved.DiscoveryInterval > 0 {
		eff.DiscoveryInterval = saved.DiscoveryInterval
	}
	if saved.DiscoveryTargets != nil {
		eff.DiscoveryTargets = saved.DiscoveryTargets
	}
	if saved.SupplyWarningPercent > 0 {
		eff.SupplyWarningPercent = saved.SupplyWarningPercent
	}
	if saved.SupplyCriticalPercent > 0 {
		eff.SupplyCriticalPercent = saved.SupplyCriticalPercent
	}
	if saved.OfflineAfterFailures > 0 {
		eff.OfflineAfterFailures = saved.OfflineAfterFailures
	}
	if saved.TelemetryRetention > 0 {
		eff.TelemetryRetention = saved.TelemetryRetention
	}
	if saved.Names.Source != "" {
		eff.Names.Source = saved.Names.Source
	}
	eff.ReverseDNS = saved.ReverseDNS
	if eff.ReverseDNS.Timeout <= 0 {
		eff.ReverseDNS.Timeout = s.Defaults().ReverseDNS.Timeout
	}
	if eff.ReverseDNS.RefreshInterval <= 0 {
		eff.ReverseDNS.RefreshInterval = s.Defaults().ReverseDNS.RefreshInterval
	}
	return eff
}

// SaveRuntime validates and persists overrides.
func (s *Service) SaveRuntime(ctx context.Context, r RuntimeSettings) error {
	if r.PollInterval.D() < 30*time.Second {
		return errors.New("poll interval must be at least 30s")
	}
	if r.PollConcurrency < 1 || r.PollConcurrency > 512 {
		return errors.New("poll concurrency must be between 1 and 512")
	}
	if r.DiscoveryInterval.D() < 5*time.Minute {
		return errors.New("discovery interval must be at least 5m")
	}
	if r.SupplyCriticalPercent < 0 || r.SupplyWarningPercent > 100 || r.SupplyCriticalPercent >= r.SupplyWarningPercent {
		return errors.New("supply thresholds must satisfy 0 ≤ critical < warning ≤ 100")
	}
	if r.OfflineAfterFailures < 1 {
		return errors.New("offline after failures must be at least 1")
	}
	if r.TelemetryRetention.D() < 24*time.Hour {
		return errors.New("telemetry retention must be at least 24h")
	}
	if r.Names.Source == "" {
		r.Names.Source = "advertised"
	}
	switch r.Names.Source {
	case "advertised", "dns", "display":
	default:
		return errors.New(`names.source must be "advertised", "dns" or "display"`)
	}
	for i, t := range r.DiscoveryTargets {
		r.DiscoveryTargets[i] = strings.TrimSpace(t)
		if r.DiscoveryTargets[i] == "" {
			return fmt.Errorf("discovery target %d is empty", i+1)
		}
	}
	if err := validateReverseDNS(&r.ReverseDNS); err != nil {
		return err
	}
	return s.Store.SetSetting(ctx, runtimeKey, r)
}

// validateReverseDNS normalises and checks the reverse-DNS options.
func validateReverseDNS(r *config.ReverseDNS) error {
	r.Server = strings.TrimSpace(r.Server)
	if r.Server != "" {
		host := r.Server
		if h, p, err := net.SplitHostPort(r.Server); err == nil {
			host = h
			if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("reverse DNS server port %q is not valid", p)
			}
		}
		host = strings.Trim(host, "[]")
		if host == "" || strings.ContainsAny(host, " \t/\\") {
			return fmt.Errorf("reverse DNS server %q must be a host or host:port", r.Server)
		}
	}
	if d := r.Timeout.D(); d < 100*time.Millisecond || d > 30*time.Second {
		return errors.New("reverse DNS lookup timeout must be between 100ms and 30s")
	}
	if d := r.RefreshInterval.D(); d < time.Minute {
		return errors.New("reverse DNS refresh interval must be at least 1m")
	}
	return nil
}

// ResetRuntime removes the overrides.
func (s *Service) ResetRuntime(ctx context.Context) error {
	return s.Store.SetSetting(ctx, runtimeKey, RuntimeSettings{})
}

// GroupDeviceIDs resolves a group (static members or rule matches) to device ids.
func (s *Service) GroupDeviceIDs(ctx context.Context, groupID string) ([]string, error) {
	g, err := s.Store.GetGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if g.Kind != "rule" {
		return g.MemberIDs, nil
	}
	all, err := s.Store.ListDevices(ctx, store.DeviceFilter{})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, d := range all {
		if store.MatchRule(g.Rule, d) {
			ids = append(ids, d.ID)
		}
	}
	return ids, nil
}
