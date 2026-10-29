package config

import (
	"os"
	"path/filepath"
	"testing"
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
