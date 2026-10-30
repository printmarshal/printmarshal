package fleet

import (
	"context"
	"net"
	"strings"
	"time"

	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
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

// rdnsConfig returns the effective reverse-DNS options: the file / environment
// defaults overlaid with what an administrator saved in Settings (issue #18).
func (s *Service) rdnsConfig(ctx context.Context) config.ReverseDNS {
	if s.Cfg == nil {
		return config.Default().Discovery.ReverseDNS
	}
	if s.Store == nil {
		return s.Cfg.Discovery.ReverseDNS
	}
	return s.Runtime(ctx).ReverseDNS
}

// resolver returns the injected Resolver or one for the effective DNS server,
// rebuilt only when the server setting changes.
func (s *Service) resolver(server string) Resolver {
	if s.Resolver != nil {
		return s.Resolver
	}
	s.rdnsMu.Lock()
	defer s.rdnsMu.Unlock()
	if s.rdnsResolver == nil || s.rdnsServer != server {
		s.rdnsResolver, s.rdnsServer = NewResolver(server), server
	}
	return s.rdnsResolver
}

// RefreshHostname performs an on-demand reverse DNS lookup for dev and stores
// the resulting DNS name in dev.DNSName (never touching dev.Hostname). Lookup
// errors or forward-confirmation mismatches are logged at debug level and
// leave the existing DNSName untouched while updating DNSCheckedAt. When
// dev.DNSSync is true and the refreshed DNSName is non-empty, not suspect, and
// differs from dev.DisplayName, dev.DisplayName is updated and audited.
func (s *Service) RefreshHostname(ctx context.Context, dev *model.Device) (*model.Device, error) {
	if dev == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	name, fqdn, ok := s.lookupHostname(ctx, dev.Address)
	dnsName := dev.DNSName
	suspect := dev.DNSNameSuspect
	if ok {
		dnsName = name
		suspect = IsSuspectDNSName(name, dev.Address) || IsSuspectDNSName(fqdn, dev.Address)
		s.dnsMu.Lock()
		if s.dnsRefreshed == nil {
			s.dnsRefreshed = map[string]time.Time{}
		}
		s.dnsRefreshed[dev.ID] = now
		s.dnsMu.Unlock()
	}

	updated, err := s.Store.SetDeviceDNS(ctx, dev.ID, dnsName, suspect, now)
	if err != nil {
		return dev, err
	}

	if ok && updated.DNSSync && dnsName != "" && !suspect && dnsName != updated.DisplayName {
		beforeName := updated.DisplayName
		src := "dns"
		synced, err := s.Store.PatchDevice(ctx, updated.ID, store.DevicePatch{
			DisplayName:       &dnsName,
			DisplayNameSource: &src,
		})
		if err != nil {
			return updated, err
		}
		updated = synced
		_ = s.Store.AddAudit(ctx, &model.AuditEntry{
			Actor:  "system",
			Action: "device.patch",
			Target: updated.ID,
			Before: map[string]any{"displayName": beforeName},
			After:  map[string]any{"displayName": updated.DisplayName, "displayNameSource": "dns", "reason": "dns sync"},
			Result: "ok",
		})
		s.emit("audit.recorded", map[string]any{
			"actor":  "system",
			"action": "device.patch",
			"target": updated.ID,
			"result": "ok",
		})
	}
	return updated, nil
}

// refreshHostname wraps RefreshHostname for discovery/poll flows where store
// errors should be logged rather than aborting the caller.
func (s *Service) refreshHostname(ctx context.Context, dev *model.Device) *model.Device {
	if !s.rdnsConfig(ctx).Enabled {
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
	cfg := s.rdnsConfig(ctx)
	if !cfg.Enabled || dev == nil {
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
func (s *Service) lookupHostname(ctx context.Context, addr string) (name, fqdn string, ok bool) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", "", false
	}
	cfg := s.rdnsConfig(ctx)
	timeout := cfg.Timeout.D()
	if timeout <= 0 {
		timeout = time.Second
	}
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	res := s.resolver(cfg.Server)
	names, err := res.LookupAddr(lctx, addr)
	if err != nil || len(names) == 0 {
		if s.Log != nil {
			s.Log.Debug("reverse dns lookup failed", "address", addr, "err", err)
		}
		return "", "", false
	}
	var raw string
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
		return "", "", false
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
			return "", "", false
		}
	}

	name = fqdn
	if cfg.ShortNames {
		if i := strings.IndexByte(name, '.'); i > 0 {
			name = name[:i]
		}
	}
	return name, fqdn, true
}

// IsSuspectDNSName reports whether name has invalid hostname characters, is
// over 63 characters per label or 253 total, or looks auto-generated from
// addr (contains the address's octets joined by '-' or '.', e.g. 10-0-0-7,
// ip-10-0-0-7, host-10-0-0-7).
func IsSuspectDNSName(name, addr string) bool {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".")
	if name == "" || len(name) > 253 {
		return true
	}
	labels := strings.Split(name, ".")
	for _, l := range labels {
		if len(l) == 0 || len(l) > 63 {
			return true
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return true
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			valid := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-'
			if !valid {
				return true
			}
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(addr)); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			parts := strings.Split(v4.String(), ".")
			if len(parts) == 4 {
				rev := []string{parts[3], parts[2], parts[1], parts[0]}
				lower := strings.ToLower(name)
				for _, pat := range []string{
					strings.Join(parts, "-"),
					strings.Join(parts, "."),
					strings.Join(rev, "-"),
					strings.Join(rev, "."),
				} {
					if strings.Contains(lower, pat) {
						return true
					}
				}
			}
		}
	}
	return false
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
