package discovery

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WS-Discovery (OASIS, 2009) multicast probe. Devices answer with a ProbeMatch
// whose XAddrs carry their transport URLs; we also record the reply's source IP.
const wsdMulticast = "239.255.255.250:3702"

var probeTemplate = `<?xml version="1.0" encoding="utf-8"?>
<soap:Envelope xmlns:soap="http://www.w3.org/2003/05/soap-envelope" xmlns:wsa="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:wsd="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:wsdp="http://schemas.xmlsoap.org/ws/2006/02/devprof">
<soap:Header><wsa:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</wsa:To><wsa:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</wsa:Action><wsa:MessageID>urn:uuid:%s</wsa:MessageID></soap:Header>
<soap:Body><wsd:Probe><wsd:Types>wsdp:Device</wsd:Types></wsd:Probe></soap:Body></soap:Envelope>`

var xaddrsRe = regexp.MustCompile(`(?s)<[^>]*XAddrs[^>]*>([^<]+)<`)

// ProbeWSD sends a WS-Discovery Probe and collects replies for wait.
func ProbeWSD(ctx context.Context, wait time.Duration) ([]Candidate, error) {
	raddr, err := net.ResolveUDPAddr("udp4", wsdMulticast)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	msg := fmt.Sprintf(probeTemplate, uuidString())
	if _, err := conn.WriteToUDP([]byte(msg), raddr); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)

	byIP := map[string]*Candidate{}
	var order []string
	buf := make([]byte, 64<<10)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		body := string(buf[:n])
		ip := from.IP.String()
		c, ok := byIP[ip]
		if !ok {
			c = &Candidate{Address: ip, Methods: []string{"wsd"}}
			byIP[ip] = c
			order = append(order, ip)
		}
		if m := xaddrsRe.FindStringSubmatch(body); len(m) == 2 {
			for _, x := range strings.Fields(m[1]) {
				if u, err := url.Parse(x); err == nil && u.Hostname() != "" {
					c.Services = appendUnique(c.Services, "wsd:"+u.Scheme)
					if c.TXT == nil {
						c.TXT = map[string]string{}
					}
					c.TXT["xaddr"] = x
				}
			}
		}
	}
	out := make([]Candidate, 0, len(order))
	for _, ip := range order {
		out = append(out, *byIP[ip])
	}
	return out, nil
}

func uuidString() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
