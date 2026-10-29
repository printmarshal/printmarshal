package fleet

import (
	"context"
	"net"
	"strings"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/model"
)

// Resolver abstracts reverse (PTR) and forward host lookups so tests can
// inject a fake without touching the network. *net.Resolver implements it.
type Resolver interface {
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

// NewResolver returns a DNS resolver configured for the given server address.
// An empty server uses net.DefaultResolver; otherwise queries are sent to
// server (defaulting to port 53 when none is given).
func NewResolver(server string) *net.Resolver {
	server = strings.TrimSpace(server)
	if server == "" {
		return net.DefaultResolver
	}
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, server)
		},
	}
}

func (s *Service) rdnsConfig() config.ReverseDNS {
	if s.Cfg != nil {
		return s.Cfg.Discovery.ReverseDNS
	}
	return config.Default().Discovery.ReverseDNS
}

func (s *Service) resolver() Resolver {
	if s.Resolver != nil {
		return s.Resolver
	}
	return NewResolver(s.rdnsConfig().Server)
}

func hasMDNSHostname(dev *model.Device) bool {
	return dev != nil && strings.TrimSpace(dev.Hostname) != "" && dev.HostnameSource == "mdns"
}

// RefreshHostname performs an on-demand reverse DNS lookup for dev and stores
// the resulting hostname unless dev already has an mDNS hostname. Lookup
// errors or forward-confirmation mismatches are logged at debug level and
// leave the existing hostname untouched.
func (s *Service) RefreshHostname(ctx context.Context, dev *model.Device) (*model.Device, error) {
	if dev == nil || hasMDNSHostname(dev) {
		return dev, nil
	}
	name, ok := s.lookupHostname(ctx, dev.Address)
	if !ok {
		return dev, nil
	}
	s.dnsMu.Lock()
	if s.dnsRefreshed == nil {
		s.dnsRefreshed = map[string]time.Time{}
	}
	s.dnsRefreshed[dev.ID] = time.Now()
	s.dnsMu.Unlock()

	if dev.Hostname == name && dev.HostnameSource == "dns" {
		return dev, nil
	}
	updated, err := s.Store.SetDeviceHostname(ctx, dev.ID, name, "dns")
	if err != nil {
		return dev, err
	}
	return updated, nil
}

// refreshHostname wraps RefreshHostname for discovery/poll flows where store
// errors should be logged rather than aborting the caller.
func (s *Service) refreshHostname(ctx context.Context, dev *model.Device) *model.Device {
	if !s.rdnsConfig().Enabled {
		return dev
	}
	updated, err := s.RefreshHostname(ctx, dev)
	if err != nil {
		if s.Log != nil {
			s.Log.Debug("reverse dns store update failed", "device", dev.ID, "err", err)
		}
		return dev
	}
	return updated
}

// maybeRefreshHostname re-runs reverse DNS on a successful poll at most once
// per configured refreshInterval. Failed lookups are not recorded in
// dnsRefreshed so the next poll retries them.
func (s *Service) maybeRefreshHostname(ctx context.Context, dev *model.Device) *model.Device {
	cfg := s.rdnsConfig()
	if !cfg.Enabled || dev == nil || hasMDNSHostname(dev) {
		return dev
	}
	interval := cfg.RefreshInterval.D()
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	s.dnsMu.Lock()
	last := s.dnsRefreshed[dev.ID]
	s.dnsMu.Unlock()
	if !last.IsZero() && time.Since(last) < interval {
		return dev
	}
	return s.refreshHostname(ctx, dev)
}

// lookupHostname resolves the PTR record for addr within the configured
// timeout, optionally forward-confirms the returned name, and shortens it to
// the first label when ShortNames is enabled.
func (s *Service) lookupHostname(ctx context.Context, addr string) (string, bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", false
	}
	cfg := s.rdnsConfig()
	timeout := cfg.Timeout.D()
	if timeout <= 0 {
		timeout = time.Second
	}
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res := s.resolver()
	names, err := res.LookupAddr(lctx, addr)
	if err != nil || len(names) == 0 {
		if s.Log != nil {
			s.Log.Debug("reverse dns lookup failed", "address", addr, "err", err)
		}
		return "", false
	}
	var raw, fqdn string
	for _, n := range names {
		trimmed := strings.TrimSpace(n)
		clean := strings.TrimSpace(strings.TrimSuffix(trimmed, "."))
		if clean != "" {
			raw = trimmed
			fqdn = clean
			break
		}
	}
	if fqdn == "" {
		if s.Log != nil {
			s.Log.Debug("reverse dns returned empty name", "address", addr)
		}
		return "", false
	}

	if cfg.ForwardConfirm {
		addrs, err := res.LookupHost(lctx, fqdn)
		if err != nil && raw != fqdn {
			addrs, err = res.LookupHost(lctx, raw)
		}
		if err != nil || !containsAddress(addrs, addr) {
			if s.Log != nil {
				s.Log.Debug("reverse dns forward confirmation failed", "address", addr, "hostname", fqdn, "addrs", addrs, "err", err)
			}
			return "", false
		}
	}

	name := fqdn
	if cfg.ShortNames {
		if i := strings.IndexByte(name, '.'); i > 0 {
			name = name[:i]
		}
	}
	return name, true
}

func containsAddress(addrs []string, target string) bool {
	targetIP := net.ParseIP(target)
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if strings.EqualFold(a, target) {
			return true
		}
		if targetIP != nil {
			if ip := net.ParseIP(a); ip != nil && ip.Equal(targetIP) {
				return true
			}
		}
	}
	return false
}
