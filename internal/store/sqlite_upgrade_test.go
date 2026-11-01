package store

import (
	"context"
	"testing"

	"printmarshal/internal/model"
)

// A database left at schema version 4 (before the DNS columns of migration 6
// existed) must still open on the current build. Migration 5 rewrites every
// timestamp column, including devices.dns_checked_at, which only migration 6
// adds, so an upgrade that has to run both fails on "no such column" (found
// by the 2026-10-31 design audit's first server start against an old data dir).
func TestUpgradeFromSchemaVersion4(t *testing.T) {
	path := t.TempDir() + "/t.db"
	s, err := OpenSQLite(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	d, _, err := s.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.1", Serial: "S1", Method: "snmp"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Roll the devices table back to what migration 4 left behind.
	for _, c := range []string{"dns_name", "dns_name_suspect", "dns_checked_at", "display_name_source", "dns_sync", "web_url_probed", "web_url_override", "identify_errors", "field_sources"} {
		if _, err := s.db.Exec(`ALTER TABLE devices DROP COLUMN ` + c); err != nil {
			t.Fatalf("drop %s: %v", c, err)
		}
	}
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE version >= 5`); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = OpenSQLite(path, nil)
	if err != nil {
		t.Fatalf("reopen from schema version 4: %v", err)
	}
	defer s.Close()
	if _, err := s.GetDevice(ctx, d.ID); err != nil {
		t.Fatalf("device after upgrade: %v", err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version IN (5,6,7,8,9)`).Scan(&n); err != nil || n != 5 {
		t.Fatalf("migrations recorded after upgrade: %d (%v)", n, err)
	}
}
