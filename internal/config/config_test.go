package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIPPWriteRequireTLS(t *testing.T) {
	if Default().Poll.IPPWriteRequireTLS {
		t.Fatal("ippWriteRequireTLS must default to false")
	}
	path := filepath.Join(t.TempDir(), "pm.json")
	if err := os.WriteFile(path, []byte(`{"poll":{"ippWriteRequireTLS":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Poll.IPPWriteRequireTLS {
		t.Error("JSON poll.ippWriteRequireTLS not applied")
	}
	t.Setenv("PM_IPP_WRITE_REQUIRE_TLS", "false")
	if c, err = Load(path); err != nil || c.Poll.IPPWriteRequireTLS {
		t.Errorf("env override should win over the file: %v %v", c.Poll.IPPWriteRequireTLS, err)
	}
	t.Setenv("PM_IPP_WRITE_REQUIRE_TLS", "yes")
	if c, err = Load(""); err != nil || !c.Poll.IPPWriteRequireTLS {
		t.Errorf("PM_IPP_WRITE_REQUIRE_TLS=yes: %v %v", c.Poll.IPPWriteRequireTLS, err)
	}
}

func TestReverseDNSConfig(t *testing.T) {
	def := Default().Discovery.ReverseDNS
	if def.Enabled || def.Server != "" || !def.ShortNames || def.Timeout.D() != time.Second || def.RefreshInterval.D() != 24*time.Hour || !def.ForwardConfirm {
		t.Fatalf("unexpected defaults: %+v", def)
	}
	path := filepath.Join(t.TempDir(), "pm.json")
	raw := `{"discovery":{"reverseDns":{"enabled":true,"server":"10.0.0.53","shortNames":false,"timeout":"2s","refreshInterval":"12h","forwardConfirm":false}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Discovery.ReverseDNS
	if !r.Enabled || r.Server != "10.0.0.53" || r.ShortNames || r.Timeout.D() != 2*time.Second || r.RefreshInterval.D() != 12*time.Hour || r.ForwardConfirm {
		t.Fatalf("JSON reverseDns not applied: %+v", r)
	}
	t.Setenv("PM_REVERSE_DNS_ENABLED", "false")
	t.Setenv("PM_REVERSE_DNS_SERVER", "192.0.2.53:5353")
	t.Setenv("PM_REVERSE_DNS_SHORT_NAMES", "1")
	t.Setenv("PM_REVERSE_DNS_TIMEOUT", "500ms")
	t.Setenv("PM_REVERSE_DNS_REFRESH_INTERVAL", "6h")
	t.Setenv("PM_REVERSE_DNS_FORWARD_CONFIRM", "yes")
	c, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r = c.Discovery.ReverseDNS
	if r.Enabled || r.Server != "192.0.2.53:5353" || !r.ShortNames || r.Timeout.D() != 500*time.Millisecond || r.RefreshInterval.D() != 6*time.Hour || !r.ForwardConfirm {
		t.Fatalf("env overrides not applied: %+v", r)
	}
}

func TestSNMPTuningEnv(t *testing.T) {
	def := Default().Poll
	if def.SNMPTimeout.D() != 2*time.Second || def.SNMPRetries != 1 || def.SNMPSessionsPerDevice != 2 {
		t.Fatalf("unexpected defaults: timeout=%s retries=%d sessions=%d", def.SNMPTimeout.D(), def.SNMPRetries, def.SNMPSessionsPerDevice)
	}
	path := filepath.Join(t.TempDir(), "pm.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PM_SNMP_TIMEOUT", "5s")
	t.Setenv("PM_SNMP_RETRIES", "3")
	t.Setenv("PM_SNMP_SESSIONS_PER_DEVICE", "1")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Poll.SNMPTimeout.D() != 5*time.Second || c.Poll.SNMPRetries != 3 || c.Poll.SNMPSessionsPerDevice != 1 {
		t.Fatalf("env not applied: %+v", c.Poll)
	}
}
