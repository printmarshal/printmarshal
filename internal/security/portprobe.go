package security

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

// PortReport is what the port/protocol probe observed.
type PortReport struct {
	Host string `json:"host"`
	// Open maps a TCP port to whether a connect succeeded.
	Open map[int]bool `json:"open"`
	// SNMPv2c is true when the device answered an SNMP v2c GET of sysDescr
	// with one of the configured read communities; false means no answer,
	// which cannot distinguish "disabled" from "wrong community" over UDP.
	SNMPv2c bool `json:"snmpV2c"`
	// SNMPTried is true when an SNMP probe was attempted.
	SNMPTried bool `json:"snmpTried"`
}

// PortProber probes a host's service ports.
type PortProber func(ctx context.Context, host string, creds model.Creds) PortReport

// ProbedPorts are the well-known printer services the probe checks, with the
// protocol setting each one observes.
var ProbedPorts = []struct {
	Port int
	Key  string
	Name string
}{
	{21, "protocols.ftp.enabled", "ftp"},
	{23, "protocols.telnet.enabled", "telnet"},
	{80, "protocols.http.enabled", "http"},
	{443, "protocols.https.enabled", "https"},
	{515, "protocols.lpd.enabled", "lpd"},
	{631, "protocols.ipp.enabled", "ipp"},
	{9100, "protocols.raw9100.enabled", "raw9100"},
}

// SNMPPort is the UDP port the SNMP probe uses (overridable for simulators).
var SNMPPort uint16 = 161

// ProbePorts performs TCP connect checks on the well-known printer service
// ports and one SNMP v2c GET (RFC 3416) of sysDescr.0 per configured read
// community (falling back to "public"). It sends no protocol payload on the
// TCP ports and never writes anything.
func ProbePorts(ctx context.Context, host string, creds model.Creds) PortReport {
	rep := PortReport{Host: host, Open: map[int]bool{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	d := &net.Dialer{Timeout: 3 * time.Second}
	for _, p := range ProbedPorts {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
			if err == nil {
				c.Close()
			}
			mu.Lock()
			rep.Open[port] = err == nil
			mu.Unlock()
		}(p.Port)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return rep
	}
	communities := creds.Communities
	if len(communities) == 0 {
		communities = []string{"public"}
	}
	rep.SNMPTried = true
	for _, comm := range communities {
		if snmpAnswers(ctx, host, comm) {
			rep.SNMPv2c = true
			break
		}
	}
	return rep
}

func snmpAnswers(ctx context.Context, host, community string) bool {
	g := &gosnmp.GoSNMP{Target: host, Port: SNMPPort, Community: community, Version: gosnmp.Version2c,
		Timeout: 2 * time.Second, Retries: 1, Context: ctx}
	if err := g.Connect(); err != nil {
		return false
	}
	defer g.Conn.Close()
	pkt, err := g.Get([]string{"1.3.6.1.2.1.1.1.0"})
	return err == nil && pkt != nil && pkt.Error == gosnmp.NoError && len(pkt.Variables) > 0
}

// PortSettings turns a port report into observed protocol settings. A closed
// TCP port is reported as disabled; the SNMP v1/v2c setting is only reported
// when the device answered (a silent device is not evidence either way).
func PortSettings(rep PortReport) []model.Setting {
	var out []model.Setting
	for _, p := range ProbedPorts {
		open, ok := rep.Open[p.Port]
		if !ok {
			continue
		}
		out = append(out, probeSetting(p.Key, open))
	}
	if rep.SNMPTried && rep.SNMPv2c {
		out = append(out, probeSetting("protocols.snmp.v1v2cEnabled", true))
	}
	return out
}
