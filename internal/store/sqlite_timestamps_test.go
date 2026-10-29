package store

import (
	"context"
	"testing"
	"time"

	"printmarshal/internal/model"
)

// TestFixedWidthTimestampMigration (code review H-020): rows written with
// trimmed RFC 3339 fractions are rewritten to the fixed-width layout on the
// next open, after which text comparison matches chronological order.
func TestFixedWidthTimestampMigration(t *testing.T) {
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
	// Pretend the database predates migration 5 and holds legacy values.
	if _, err := s.db.Exec(`DELETE FROM schema_migrations WHERE version=5`); err != nil {
		t.Fatal(err)
	}
	for _, at := range []string{"2026-10-29T00:00:00Z", "2026-10-29T00:00:00.5Z", "2026-10-29T00:00:01Z"} {
		if _, err := s.db.Exec(`INSERT INTO telemetry(device_id,metric,label,value,at) VALUES(?,?,?,?,?)`, d.ID, "counter.total", "", 1, at); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`UPDATE devices SET last_seen='2026-10-29T00:00:00Z' WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = OpenSQLite(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.db.Query(`SELECT at FROM telemetry ORDER BY at`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var at string
		_ = rows.Scan(&at)
		got = append(got, at)
	}
	rows.Close()
	want := []string{"2026-10-29T00:00:00.000000000Z", "2026-10-29T00:00:00.500000000Z", "2026-10-29T00:00:01.000000000Z"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("migrated = %v, want %v", got, want)
	}
	var seen string
	_ = s.db.QueryRow(`SELECT last_seen FROM devices WHERE id=?`, d.ID).Scan(&seen)
	if seen != want[0] {
		t.Errorf("last_seen = %q", seen)
	}
	// A half-open range starting at midnight includes the sample in the first
	// fractional second and excludes the one at the next whole second.
	from := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM telemetry WHERE at >= ? AND at < ?`, ts(from), ts(from.Add(time.Second))).Scan(&n)
	if n != 2 {
		t.Errorf("range count = %d, want 2", n)
	}
	// New writes use the layout and round-trip with full precision.
	at := time.Date(2026, 10, 29, 1, 2, 3, 123456789, time.UTC)
	if err := s.AddSamples(ctx, []model.TelemetrySample{{DeviceID: d.ID, Metric: "counter.total", Value: 2, At: at}}); err != nil {
		t.Fatal(err)
	}
	var stored string
	_ = s.db.QueryRow(`SELECT at FROM telemetry WHERE value=2`).Scan(&stored)
	if stored != "2026-10-29T01:02:03.123456789Z" || !parseTime(stored).Equal(at) {
		t.Errorf("stored = %q", stored)
	}
	// Running the migration again is a no-op.
	if err := s.migrateFixedWidthTimestamps(); err != nil {
		t.Fatal(err)
	}
}
