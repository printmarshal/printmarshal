package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"printmarshal/internal/secrets"
)

//go:embed schema.sql schema_*.sql
var schemaFS embed.FS

// SQLite implements Store on a single SQLite file (the single-container default, spec §5).
type SQLite struct {
	db  *sql.DB
	box *secrets.Box
}

// OpenSQLite opens (creating if needed) the database at path. box encrypts credential secrets.
func OpenSQLite(path string, box *secrets.Box) (*SQLite, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, err
		}
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer; one connection avoids SQLITE_BUSY
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite %s: %w", pragma, err)
		}
	}
	s := &SQLite{db: db, box: box}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLite) migrate() error {
	entries, err := schemaFS.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names) // schema.sql first, then schema_<area>.sql alphabetically
	for _, name := range names {
		b, err := schemaFS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.db.Exec(string(b)); err != nil {
			return fmt.Errorf("schema %s: %w", name, err)
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, ?)`, now()); err != nil {
		return err
	}
	if err := s.migrateLegacyStock(); err != nil {
		return fmt.Errorf("migrate stock: %w", err)
	}
	_, err = s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, ?)`, now())
	return err
}

// migrateLegacyStock absorbs the phase-2 placeholder "stock" table into
// consumables (migration 2). Each row becomes a consumable of type "other";
// the old free-text match rule is kept in the notes so nothing is lost.
func (s *SQLite) migrateLegacyStock() error {
	var name string
	err := s.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='stock'`).Scan(&name)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,name,match,quantity,warning,critical,updated_at FROM stock`)
	if err != nil {
		return err
	}
	type legacy struct {
		id, name, match, upd string
		qty, warn, crit      int
	}
	var items []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.id, &l.name, &l.match, &l.qty, &l.warn, &l.crit, &l.upd); err != nil {
			rows.Close()
			return err
		}
		items = append(items, l)
	}
	rows.Close()
	n := now()
	for _, l := range items {
		notes := ""
		if l.match != "" {
			notes = "Migrated from stock item; matched supplies containing: " + l.match
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO consumables(id,name,type,color,part_number,compatible_models,unit,on_hand,warning_at,critical_at,location,notes,reorder_url,unit_cost,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, l.id, l.name, "other", "", "", "[]", "each", l.qty, l.warn, l.crit, "", notes, "", nil, l.upd, n); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO consumable_stock_events(consumable_id,actor,kind,delta,resulting,reason,status,at) VALUES(?,?,?,?,?,?,?,?)`,
			l.id, "migration", "set", 0, l.qty, "migrated from legacy stock table", "applied", n); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DROP TABLE stock`); err != nil {
		return err
	}
	// Legacy stock alerts used source "stock"; the consumable rules re-raise what still applies.
	if _, err := tx.Exec(`UPDATE alerts SET cleared_at=?, updated_at=? WHERE source='stock' AND cleared_at IS NULL`, n, n); err != nil {
		return err
	}
	return tx.Commit()
}

// Close closes the database.
func (s *SQLite) Close() error { return s.db.Close() }

// --- helpers ---------------------------------------------------------------

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func tsp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func parseTimeP(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

func toJSON(v any) string {
	if v == nil {
		return "null"
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func fromJSON(s sql.NullString, dst any) {
	if !s.Valid || s.String == "" || s.String == "null" {
		return
	}
	_ = json.Unmarshal([]byte(s.String), dst)
}

func newID() string { return uuid.NewString() }

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// --- settings --------------------------------------------------------------

func (s *SQLite) GetSetting(ctx context.Context, key string, dst any) error {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(v), dst)
}

func (s *SQLite) SetSetting(ctx context.Context, key string, v any) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, toJSON(v))
	return err
}

var _ Store = (*SQLite)(nil)
