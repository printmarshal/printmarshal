package discovery

import (
	"context"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/ipv4"
)

// ServiceTypes are the DNS-SD service types printers and scanners advertise (spec §7).
var ServiceTypes = []string{
	"_ipp._tcp", "_ipps._tcp", "_printer._tcp", "_pdl-datastream._tcp",
	"_scanner._tcp", "_uscan._tcp", "_uscans._tcp",
}

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// BrowseMDNS sends DNS-SD PTR queries for each printer service type on every
// multicast-capable IPv4 interface (RFC 6762/6763) and collects the answering
// devices for `wait`. Responses carry PTR → SRV/TXT/A records in one packet on
// most printers; where an A record is missing, the source address is used.
func BrowseMDNS(ctx context.Context, wait time.Duration) ([]Candidate, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var (
		mu    sync.Mutex
		byIP  = map[string]*Candidate{}
		order []string
		wg    sync.WaitGroup
	)
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	collect := func(resp *dns.Msg, from net.IP) {
		for _, c := range parseMDNS(resp, from) {
			mu.Lock()
			cur, ok := byIP[c.Address]
			if !ok {
				cc := c
				byIP[c.Address] = &cc
				order = append(order, c.Address)
			} else {
				cur.Merge(c)
			}
			mu.Unlock()
		}
	}

	// Group listener: responders usually answer to the multicast group itself
	// (RFC 6762 §6), so join 224.0.0.251:5353 with port reuse alongside any
	// system resolver.
	if gc, err := listenGroup(ifaces); err == nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer gc.Close()
			_ = gc.SetReadDeadline(deadline)
			buf := make([]byte, 65535)
			for {
				n, from, err := gc.ReadFromUDP(buf)
				if err != nil {
					return
				}
				var resp dns.Msg
				if err := resp.Unpack(buf[:n]); err == nil && resp.Response {
					collect(&resp, from.IP)
				}
			}
		}()
	}

	listened := 0
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Unicast-reply socket bound to a random port on this interface's IPv4 address.
		ip := ifaceIPv4(ifi)
		if ip == nil {
			continue
		}
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: 0})
		if err != nil {
			continue
		}
		listened++
		wg.Add(1)
		go func(conn *net.UDPConn) {
			defer wg.Done()
			defer conn.Close()
			for _, svc := range ServiceTypes {
				m := new(dns.Msg)
				m.SetQuestion(dns.Fqdn(svc+".local"), dns.TypePTR)
				m.RecursionDesired = false
				m.Question[0].Qclass |= 1 << 15 // QU: please answer by unicast (RFC 6762 §5.4)
				if b, err := m.Pack(); err == nil {
					_, _ = conn.WriteToUDP(b, mdnsGroup)
				}
			}
			_ = conn.SetReadDeadline(deadline)
			buf := make([]byte, 65535)
			for {
				n, from, err := conn.ReadFromUDP(buf)
				if err != nil {
					return
				}
				var resp dns.Msg
				if err := resp.Unpack(buf[:n]); err != nil {
					continue
				}
				collect(&resp, from.IP)
			}
		}(conn)
	}
	if listened == 0 {
		return nil, nil
	}
	wg.Wait()
	out := make([]Candidate, 0, len(order))
	for _, ip := range order {
		out = append(out, *byIP[ip])
	}
	return out, nil
}

// listenGroup opens a reusable socket on 0.0.0.0:5353 joined to the mDNS group
// on every usable interface.
func listenGroup(ifaces []net.Interface) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) { serr = setReuse(fd) })
		if err != nil {
			return err
		}
		return serr
	}}
	pc, err := lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:5353")
	if err != nil {
		return nil, err
	}
	conn := pc.(*net.UDPConn)
	p := ipv4.NewPacketConn(conn)
	joined := 0
	for i := range ifaces {
		ifi := ifaces[i]
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 || ifaceIPv4(ifi) == nil {
			continue
		}
		if err := p.JoinGroup(&ifi, mdnsGroup); err == nil {
			joined++
		}
	}
	if joined == 0 {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func ifaceIPv4(ifi net.Interface) net.IP {
	addrs, err := ifi.Addrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok {
			if v4 := ipn.IP.To4(); v4 != nil {
				return v4
			}
		}
	}
	return nil
}

// parseMDNS turns one response into candidates. A response may describe
// several service instances; each becomes a Candidate keyed by its A record
// (or the sender's address when no A record is present).
func parseMDNS(resp *dns.Msg, from net.IP) []Candidate {
	all := append(append(append([]dns.RR{}, resp.Answer...), resp.Ns...), resp.Extra...)
	type inst struct {
		svc   string
		host  string
		txt   map[string]string
		port  int
		found bool
	}
	insts := map[string]*inst{} // instance FQDN -> info
	hostIP := map[string]string{}
	for _, rr := range all {
		switch r := rr.(type) {
		case *dns.PTR:
			svc := serviceOf(r.Hdr.Name)
			if svc == "" {
				continue
			}
			i := insts[r.Ptr]
			if i == nil {
				i = &inst{txt: map[string]string{}}
				insts[r.Ptr] = i
			}
			i.svc, i.found = svc, true
		case *dns.A:
			hostIP[strings.ToLower(r.Hdr.Name)] = r.A.String()
		}
	}
	for _, rr := range all {
		switch r := rr.(type) {
		case *dns.SRV:
			i := insts[r.Hdr.Name]
			if i == nil {
				if svc := serviceOf(r.Hdr.Name); svc != "" {
					i = &inst{txt: map[string]string{}, svc: svc, found: true}
					insts[r.Hdr.Name] = i
				} else {
					continue
				}
			}
			i.host, i.port = strings.TrimSuffix(r.Target, "."), int(r.Port)
		case *dns.TXT:
			i := insts[r.Hdr.Name]
			if i == nil {
				continue
			}
			for _, kv := range r.Txt {
				k, v, _ := strings.Cut(kv, "=")
				if k != "" {
					i.txt[strings.ToLower(k)] = v
				}
			}
		}
	}
	var out []Candidate
	for name, i := range insts {
		if !i.found {
			continue
		}
		addr := hostIP[strings.ToLower(i.host)+"."]
		if addr == "" {
			addr = hostIP[strings.ToLower(i.host)]
		}
		if addr == "" {
			addr = from.String()
		}
		instance := strings.TrimSuffix(strings.TrimSuffix(name, "."), "."+i.svc+".local")
		txt := i.txt
		if _, ok := txt["instance"]; !ok {
			txt["instance"] = unescapeLabel(instance)
		}
		out = append(out, Candidate{Address: addr, Hostname: i.host, Methods: []string{"mdns"}, Services: []string{i.svc}, TXT: txt})
	}
	return out
}

// serviceOf extracts "_ipp._tcp" from "_ipp._tcp.local." or an instance name.
func serviceOf(fqdn string) string {
	n := strings.ToLower(strings.TrimSuffix(fqdn, "."))
	for _, svc := range ServiceTypes {
		if n == svc+".local" || strings.HasSuffix(n, "."+svc+".local") {
			return svc
		}
	}
	return ""
}

// unescapeLabel undoes DNS presentation-format escaping: "\ " / "\(" style
// and "\DDD" decimal escapes (RFC 1035 §5.1).
func unescapeLabel(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			continue
		}
		if i+3 < len(s) && isDigit(s[i+1]) && isDigit(s[i+2]) && isDigit(s[i+3]) {
			n := int(s[i+1]-'0')*100 + int(s[i+2]-'0')*10 + int(s[i+3]-'0')
			b.WriteByte(byte(n))
			i += 3
			continue
		}
		b.WriteByte(s[i+1])
		i++
	}
	return b.String()
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
