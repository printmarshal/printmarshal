package discovery

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

// ServiceTypes are the DNS-SD service types printers and scanners advertise (spec §7).
var ServiceTypes = []string{
	"_ipp._tcp", "_ipps._tcp", "_printer._tcp", "_pdl-datastream._tcp",
	"_scanner._tcp", "_uscan._tcp", "_uscans._tcp",
}

// BrowseMDNS listens for DNS-SD advertisements for the given duration and
// returns one candidate per IPv4 address seen.
func BrowseMDNS(ctx context.Context, wait time.Duration) ([]Candidate, error) {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		byIP  = map[string]*Candidate{}
		order []string
		wg    sync.WaitGroup
	)
	for _, svc := range ServiceTypes {
		entries := make(chan *zeroconf.ServiceEntry, 32)
		wg.Add(1)
		go func(svc string, entries chan *zeroconf.ServiceEntry) {
			defer wg.Done()
			for e := range entries {
				if e == nil {
					continue
				}
				txt := map[string]string{}
				for _, kv := range e.Text {
					k, v, _ := strings.Cut(kv, "=")
					txt[strings.ToLower(k)] = v
				}
				for _, ip := range e.AddrIPv4 {
					mu.Lock()
					c, ok := byIP[ip.String()]
					if !ok {
						c = &Candidate{Address: ip.String()}
						byIP[ip.String()] = c
						order = append(order, ip.String())
					}
					c.Merge(Candidate{
						Hostname: strings.TrimSuffix(e.HostName, "."),
						Methods:  []string{"mdns"},
						Services: []string{svc},
						TXT:      txt,
					})
					mu.Unlock()
				}
			}
		}(svc, entries)
		if err := resolver.Browse(ctx, svc, "local.", entries); err != nil {
			close(entries)
			continue
		}
	}
	<-ctx.Done()
	// zeroconf closes the entries channels when ctx is done.
	wg.Wait()
	out := make([]Candidate, 0, len(order))
	for _, ip := range order {
		out = append(out, *byIP[ip])
	}
	return out, nil
}
