// Package discovery finds candidate printer addresses (spec §8.1): zero-config
// via mDNS/DNS-SD and WS-Discovery, and active scans of operator-defined
// targets. It is read-only; identification is the engine's job.
package discovery

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Candidate is an address worth identifying, with how it was found.
type Candidate struct {
	Address  string            `json:"address"`
	Hostname string            `json:"hostname,omitempty"`
	Methods  []string          `json:"methods"`
	Services []string          `json:"services,omitempty"`
	TXT      map[string]string `json:"txt,omitempty"`
}

// Merge folds b into a (same address).
func (a *Candidate) Merge(b Candidate) {
	if a.Hostname == "" {
		a.Hostname = b.Hostname
	}
	a.Methods = appendUnique(a.Methods, b.Methods...)
	a.Services = appendUnique(a.Services, b.Services...)
	if len(b.TXT) > 0 {
		if a.TXT == nil {
			a.TXT = map[string]string{}
		}
		for k, v := range b.TXT {
			if _, ok := a.TXT[k]; !ok {
				a.TXT[k] = v
			}
		}
	}
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		found := false
		for _, d := range dst {
			if d == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

// ExpandTargets turns operator target specs — "10.0.0.5", "10.0.0.1-10.0.0.50",
// "10.0.0.1-50", "10.0.0.0/24", or a hostname — into addresses. Expansion
// stops at max addresses and reports how many were skipped so nothing is
// silently truncated (spec §8.1).
func ExpandTargets(specs []string, max int) (addrs []string, skipped int, err error) {
	seen := map[string]bool{}
	add := func(a string) {
		if seen[a] {
			return
		}
		if max > 0 && len(addrs) >= max {
			skipped++
			return
		}
		seen[a] = true
		addrs = append(addrs, a)
	}
	for _, raw := range specs {
		spec := strings.TrimSpace(raw)
		if spec == "" {
			continue
		}
		switch {
		case strings.Contains(spec, "/"):
			pfx, perr := netip.ParsePrefix(spec)
			if perr != nil {
				return nil, 0, fmt.Errorf("bad CIDR %q: %w", spec, perr)
			}
			pfx = pfx.Masked()
			if !pfx.Addr().Is4() {
				return nil, 0, fmt.Errorf("only IPv4 CIDR scans are supported: %q", spec)
			}
			bits := pfx.Bits()
			first := pfx.Addr()
			count := 1 << (32 - bits)
			for i := 0; i < count; i++ {
				a := first
				for j := 0; j < i; j++ {
					a = a.Next()
				}
				if bits < 31 && (i == 0 || i == count-1) {
					continue // network and broadcast
				}
				add(a.String())
				if max > 0 && len(addrs) >= max {
					skipped += count - i - 1
					if bits < 31 {
						skipped--
					}
					break
				}
			}
		case strings.Contains(spec, "-"):
			lo, hi, ok := strings.Cut(spec, "-")
			start, perr := netip.ParseAddr(strings.TrimSpace(lo))
			if perr != nil || !start.Is4() {
				return nil, 0, fmt.Errorf("bad range start %q", spec)
			}
			hi = strings.TrimSpace(hi)
			var end netip.Addr
			if strings.Contains(hi, ".") {
				end, perr = netip.ParseAddr(hi)
				if perr != nil {
					return nil, 0, fmt.Errorf("bad range end %q", spec)
				}
			} else {
				var last int
				if _, perr := fmt.Sscanf(hi, "%d", &last); perr != nil || last < 0 || last > 255 {
					return nil, 0, fmt.Errorf("bad range end %q", spec)
				}
				b := start.As4()
				b[3] = byte(last)
				end = netip.AddrFrom4(b)
			}
			if !ok || end.Less(start) {
				return nil, 0, fmt.Errorf("bad range %q", spec)
			}
			for a := start; a.Compare(end) <= 0; a = a.Next() {
				add(a.String())
			}
		default:
			if _, perr := netip.ParseAddr(spec); perr == nil {
				add(spec)
				continue
			}
			ips, lerr := net.LookupIP(spec)
			if lerr != nil || len(ips) == 0 {
				return nil, 0, fmt.Errorf("cannot resolve %q", spec)
			}
			add(ips[0].String())
		}
	}
	return addrs, skipped, nil
}
