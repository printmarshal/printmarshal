package engine

import (
	"testing"

	"printmarshal/internal/model"
)

// Device text from any driver is cleaned as it is merged, so binary framing
// such as HP's FD E8 PML header never reaches the store or the UI.
func TestMergeIdentityCleansText(t *testing.T) {
	var dst model.Identity
	mergeIdentity(&dst, &model.Identity{
		Make:     "HP",
		Model:    "\xfd\xe8HP Color LaserJet Flow E87760",
		SysName:  "NPIABE5A1\x00",
		Hostname: "\x00",
	})
	if dst.Model != "HP Color LaserJet Flow E87760" {
		t.Errorf("Model = %q", dst.Model)
	}
	if dst.SysName != "NPIABE5A1" {
		t.Errorf("SysName = %q", dst.SysName)
	}
	// Text that is only junk must not fill the field, so a later driver can.
	if dst.Hostname != "" {
		t.Errorf("Hostname = %q, want empty", dst.Hostname)
	}
	mergeIdentity(&dst, &model.Identity{Hostname: "printer-1"})
	if dst.Hostname != "printer-1" {
		t.Errorf("Hostname after second driver = %q", dst.Hostname)
	}
}
