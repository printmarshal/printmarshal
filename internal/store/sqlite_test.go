package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
)

func openSQLiteTest(t *testing.T) testStore {
	t.Helper()
	box, err := secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenSQLite(t.TempDir()+"/test.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// sqliteArgs converts time.Time arguments to the RFC 3339 text the SQLite
// schema stores, so the shared tests can pass times to raw statements.
func sqliteArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		if t, ok := a.(time.Time); ok {
			out[i] = ts(t)
			continue
		}
		out[i] = a
	}
	return out
}

func (s *SQLite) rawExec(q string, args ...any) error {
	_, err := s.db.Exec(q, sqliteArgs(args)...)
	return err
}

func (s *SQLite) rawRow(q string, args ...any) rowScanner {
	return s.db.QueryRow(q, sqliteArgs(args)...)
}

// TestSQLite runs the shared conformance suite (store_conformance_test.go)
// against the SQLite store.
func TestSQLite(t *testing.T) { runStoreTests(t, openSQLiteTest) }

// The phase-2 "stock" placeholder table is absorbed on open (SQLite only:
// a PostgreSQL database never had it).
func TestLegacyStockMigration(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	box, _ := secrets.New(make([]byte, 32))
	s, err := OpenSQLite(path, box)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the phase-2 placeholder table with a row and a stock alert.
	_, err = s.db.Exec(`CREATE TABLE stock (id TEXT PRIMARY KEY, name TEXT NOT NULL, match TEXT NOT NULL DEFAULT '', quantity INTEGER NOT NULL DEFAULT 0, warning INTEGER NOT NULL DEFAULT 0, critical INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL);
		INSERT INTO stock VALUES ('legacy-1','Spare black toner','black toner',1,2,1,'2026-01-01T00:00:00Z');
		DELETE FROM schema_migrations WHERE version=2;`)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = s.RaiseAlert(context.Background(), "", "stock:legacy-1", "stock", model.SeverityWarning, "Stock low")
	s.Close()

	s2, err := OpenSQLite(path, box)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	c, err := s2.GetConsumable(context.Background(), "legacy-1")
	if err != nil || c.Name != "Spare black toner" || c.OnHand != 1 || c.WarningAt != 2 || c.CriticalAt != 1 || !strings.Contains(c.Notes, "black toner") {
		t.Fatalf("migrated item = %+v (%v)", c, err)
	}
	var n int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='stock'`).Scan(&n); err != nil || n != 0 {
		t.Error("legacy table should be dropped")
	}
	active, _ := s2.ListAlerts(context.Background(), AlertFilter{ActiveOnly: true})
	if len(active) != 0 {
		t.Errorf("legacy stock alerts should be cleared: %+v", active)
	}
	events, _ := s2.ListStockEvents(context.Background(), StockEventFilter{ConsumableID: "legacy-1"})
	if len(events) != 1 || events[0].Resulting != 1 {
		t.Errorf("migration event = %+v", events)
	}
}
