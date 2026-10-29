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
	if !def.Enabled || def.Server != "" || !def.ShortNames || def.Timeout.D() != time.Second || def.RefreshInterval.D() != 24*time.Hour || !def.ForwardConfirm {
		t.Fatalf("unexpected defaults: %+v", def)
	}
	path := filepath.Join(t.TempDir(), "pm.json")
	raw := `{"discovery":{"reverseDns":{"enabled":false,"server":"10.0.0.53","shortNames":false,"timeout":"2s","refreshInterval":"12h","forwardConfirm":false}}}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Discovery.ReverseDNS
	if r.Enabled || r.Server != "10.0.0.53" || r.ShortNames || r.Timeout.D() != 2*time.Second || r.RefreshInterval.D() != 12*time.Hour || r.ForwardConfirm {
		t.Fatalf("JSON reverseDns not applied: %+v", r)
	}
	t.Setenv("PM_REVERSE_DNS_ENABLED", "true")
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
	if !r.Enabled || r.Server != "192.0.2.53:5353" || !r.ShortNames || r.Timeout.D() != 500*time.Millisecond || r.RefreshInterval.D() != 6*time.Hour || !r.ForwardConfirm {
		t.Fatalf("env overrides not applied: %+v", r)
	}
}
