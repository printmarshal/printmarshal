package store

import (
	"context"
	"testing"

	"printmarshal/internal/model"
)

func TestPickModel(t *testing.T) {
	const good = "HP Color LaserJet Flow E87760"
	const generic = "HP ETHERNET MULTI-ENVIRONMENT"
	cases := []struct{ name, newv, old, want string }{
		{"new specific replaces generic", good, generic, good},
		{"new specific replaces empty", good, "", good},
		{"new specific replaces other specific", "HP Color LaserJet MFP M283fdw", good, "HP Color LaserJet MFP M283fdw"},
		{"generic never replaces specific", generic, good, good},
		{"generic fills empty", generic, "", generic},
		{"generic keeps generic", generic, generic, generic},
		{"empty keeps old", "", good, good},
		{"blank keeps old", "  ", good, good},
	}
	for _, c := range cases {
		if got := pickModel(c.newv, c.old); got != c.want {
			t.Errorf("%s: pickModel(%q, %q) = %q, want %q", c.name, c.newv, c.old, got, c.want)
		}
	}
}

// A scan whose model read fails reports the print server's name. Identifying the
// same device again must not replace the model a good read stored earlier.
func TestUpsertDeviceKeepsSpecificModel(t *testing.T) {
	s, err := OpenSQLite(t.TempDir()+"/t.db", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	ident := func(m string) *model.Identity {
		return &model.Identity{Address: "172.20.78.22", Serial: "CZBBSB903L", Make: "HP", Model: m, Method: "hp,snmp"}
	}
	d, _, err := s.UpsertDevice(ctx, ident("HP Color LaserJet Flow E87760"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	d2, created, err := s.UpsertDevice(ctx, ident("HP ETHERNET MULTI-ENVIRONMENT"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created || d2.ID != d.ID {
		t.Fatalf("expected the same device, got created=%v id=%s want %s", created, d2.ID, d.ID)
	}
	if d2.Model != "HP Color LaserJet Flow E87760" {
		t.Errorf("model after a generic read = %q, want it kept", d2.Model)
	}
	// And the reverse still works: a device first seen with the generic name gets the real one.
	other := &model.Identity{Address: "172.20.78.53", Serial: "X1", Make: "HP", Model: "HP ETHERNET MULTI-ENVIRONMENT", Method: "hp"}
	if _, _, err := s.UpsertDevice(ctx, other, nil, nil); err != nil {
		t.Fatal(err)
	}
	other.Model = "HP Color LaserJet Flow E87760"
	d3, _, err := s.UpsertDevice(ctx, other, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d3.Model != "HP Color LaserJet Flow E87760" {
		t.Errorf("model after a specific read = %q", d3.Model)
	}
}
