// Package trap receives SNMP traps and informs from devices (spec §8.3
// asynchronous alerts) and hands them to the fleet as events.
package trap

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/model"
)

// Handler consumes decoded traps.
type Handler interface {
	HandleTrap(ctx context.Context, sourceIP string, description string, severity model.Severity, varbinds map[string]string)
}

// Listener wraps gosnmp's trap listener.
type Listener struct {
	addr string
	h    Handler
	log  *slog.Logger
	tl   *gosnmp.TrapListener

	// Allowed restricts accepted sources (CIDRs or addresses); nil = any.
	Allowed []*net.IPNet
	// RatePerMinute caps handled traps per source address (0 = 60).
	RatePerMinute int

	mu     sync.Mutex
	seen   map[string][]time.Time
	denied map[string]time.Time // last time a drop was logged per source
}

// New returns a listener bound to addr (e.g. "0.0.0.0:162"; ports below 1024 need privileges).
func New(addr string, h Handler, log *slog.Logger) *Listener {
	return &Listener{addr: addr, h: h, log: log, seen: map[string][]time.Time{}, denied: map[string]time.Time{}}
}

// ParseSources turns CIDRs or bare addresses into networks.
func ParseSources(list []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			if ip := net.ParseIP(s); ip != nil {
				if ip.To4() != nil {
					s += "/32"
				} else {
					s += "/128"
				}
			}
		}
		_, n, err := net.ParseCIDR(s)
		if err != nil {
			return nil, fmt.Errorf("trap allowed source %q: %w", s, err)
		}
		out = append(out, n)
	}
	return out, nil
}

// accept applies the source allow-list and the per-source rate limit (audit A13).
func (l *Listener) accept(ip net.IP) bool {
	if len(l.Allowed) > 0 {
		ok := false
		for _, n := range l.Allowed {
			if n.Contains(ip) {
				ok = true
				break
			}
		}
		if !ok {
			l.drop(ip.String(), "source not in trap.allowedSources")
			return false
		}
	}
	limit := l.RatePerMinute
	if limit <= 0 {
		limit = 60
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	key := ip.String()
	keep := l.seen[key][:0]
	for _, t := range l.seen[key] {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) >= limit {
		l.seen[key] = keep
		l.dropLocked(key, "rate limit exceeded")
		return false
	}
	l.seen[key] = append(keep, now)
	return true
}

func (l *Listener) drop(src, why string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.dropLocked(src, why)
}

// dropLocked logs at most one drop per source per minute so a flood cannot
// flood the log too.
func (l *Listener) dropLocked(src, why string) {
	if t, ok := l.denied[src]; ok && time.Since(t) < time.Minute {
		return
	}
	l.denied[src] = time.Now()
	l.log.Warn("trap dropped", "source", src, "reason", why)
}

// Well-known trap OIDs.
const (
	oidSnmpTrapOID  = ".1.3.6.1.6.3.1.1.4.1.0"
	oidColdStart    = ".1.3.6.1.6.3.1.1.5.1"
	oidWarmStart    = ".1.3.6.1.6.3.1.1.5.2"
	oidLinkDown     = ".1.3.6.1.6.3.1.1.5.3"
	oidLinkUp       = ".1.3.6.1.6.3.1.1.5.4"
	oidAuthFailure  = ".1.3.6.1.6.3.1.1.5.5"
	oidPrinterAlert = ".1.3.6.1.2.1.43.18.2.0.1" // printerV2Alert (RFC 3805)
)

// Run listens until ctx ends.
func (l *Listener) Run(ctx context.Context) error {
	l.tl = gosnmp.NewTrapListener()
	l.tl.Params = gosnmp.Default
	l.tl.Params.Community = "" // accept any community on receive; devices are matched by source address
	l.tl.OnNewTrap = l.onTrap
	errCh := make(chan error, 1)
	go func() { errCh <- l.tl.Listen(l.addr) }()
	select {
	case <-l.tl.Listening():
		l.log.Info("trap listener ready", "addr", l.addr)
	case err := <-errCh:
		return err
	}
	select {
	case <-ctx.Done():
		l.tl.Close()
		return nil
	case err := <-errCh:
		return err
	}
}

func (l *Listener) onTrap(p *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	if !l.accept(addr.IP) {
		return
	}
	vars := map[string]string{}
	trapOID := ""
	desc := ""
	sev := model.SeverityWarning
	var alertRows []snmp.PDU
	for _, v := range p.Variables {
		pdu := snmp.PDU{OID: v.Name, Type: v.Type, Value: v.Value}
		vars[v.Name] = snmp.Str(pdu)
		switch {
		case v.Name == oidSnmpTrapOID:
			trapOID = snmp.Str(pdu)
		case strings.HasPrefix(v.Name, snmp.OIDPrtAlertTable+"."):
			alertRows = append(alertRows, pdu)
		}
	}
	if p.Version == gosnmp.Version1 && trapOID == "" {
		trapOID = fmt.Sprintf("%s generic=%d specific=%d", p.Enterprise, p.GenericTrap, p.SpecificTrap)
	}
	if len(alertRows) > 0 {
		if alerts := snmp.ParseDeviceAlerts(alertRows); len(alerts) > 0 {
			a := alerts[0]
			desc = a.Description
			if a.Severity != model.SeverityOK {
				sev = a.Severity
			}
		}
	}
	if desc == "" {
		switch trapOID {
		case oidColdStart:
			desc, sev = "Device cold start", model.SeverityWarning
		case oidWarmStart:
			desc, sev = "Device warm start", model.SeverityWarning
		case oidLinkDown:
			desc, sev = "Link down", model.SeverityCritical
		case oidLinkUp:
			desc, sev = "Link up", model.SeverityOK
		case oidAuthFailure:
			desc, sev = "SNMP authentication failure", model.SeverityWarning
		case oidPrinterAlert:
			desc = "Printer alert"
		default:
			desc = "Trap " + trapOID
		}
	}
	if sev == model.SeverityOK {
		l.log.Info("informational trap", "source", addr.IP.String(), "description", desc)
		return
	}
	l.h.HandleTrap(context.Background(), addr.IP.String(), desc, sev, vars)
}
