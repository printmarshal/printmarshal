package fleet

import (
	"context"
	"testing"
)

// An isolated instance that lists only its own targets must be able to keep
// scheduled runs off the wider network: with zeroconf on, a run browses mDNS and
// WS-Discovery and then probes whatever answers.
func TestScheduledDiscoveryHonoursZeroConfSetting(t *testing.T) {
	svc, _ := newTestService(t, "generic-laser.snmp")
	r := svc.Runtime(context.Background())
	r.DiscoveryTargets = []string{"127.0.0.1"}

	if got := svc.scheduledDiscoveryParams(r); !got.ZeroConf {
		t.Error("zeroconf must stay on by default")
	}
	svc.Cfg.Discovery.ZeroConf = false
	got := svc.scheduledDiscoveryParams(r)
	if got.ZeroConf {
		t.Error("discovery.zeroconf=false: a scheduled run still browses mDNS and WS-Discovery")
	}
	if len(got.Targets) != 1 || got.Targets[0] != "127.0.0.1" {
		t.Errorf("targets = %v", got.Targets)
	}
}
