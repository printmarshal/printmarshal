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
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (2, ?)`, now()); err != nil {
		return err
	}
	// Migration 3 (audit A19): purpose-bound credentials. The column is added
	// with ALTER TABLE because the schema files are only ever CREATE statements;
	// the guard makes it idempotent on fresh and existing databases alike, and
	// the default keeps every pre-existing credential read-only.
	if err := s.migrateCredentialPurpose(); err != nil {
		return fmt.Errorf("migrate credential purpose: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (3, ?)`, now()); err != nil {
		return err
	}
	// Migration 4: external identities are bound by (issuer, subject), never
	// by username (code review H-001).
	for _, col := range []string{"external_issuer", "external_subject"} {
		if err := s.addColumn("users", col, col+" TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate users.%s: %w", col, err)
		}
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS users_external ON users(external_issuer, external_subject) WHERE external_issuer <> ''`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (4, ?)`, now()); err != nil {
		return err
	}
	// Migration 5: fixed-width timestamps (see tsLayout). Runs once; it
	// touches every timestamp column, so it is guarded by its version row.
	if err := s.migrateFixedWidthTimestamps(); err != nil {
		return fmt.Errorf("migrate timestamps: %w", err)
	}
	// Migration 6: DNS name lookup and rename/sync metadata on devices.
	for _, col := range []struct{ name, ddl string }{
		{"dns_name", "dns_name TEXT NOT NULL DEFAULT ''"},
		{"dns_name_suspect", "dns_name_suspect INTEGER NOT NULL DEFAULT 0"},
		{"dns_checked_at", "dns_checked_at TEXT"},
		{"display_name_source", "display_name_source TEXT NOT NULL DEFAULT ''"},
		{"dns_sync", "dns_sync INTEGER NOT NULL DEFAULT 0"},
	} {
		if err := s.addColumn("devices", col.name, col.ddl); err != nil {
			return fmt.Errorf("migrate devices.%s: %w", col.name, err)
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (6, ?)`, now()); err != nil {
		return err
	}
	// Migration 7: the device web page (issue #7): what a probe found and an
	// operator's override.
	for _, col := range []string{"web_url_probed", "web_url_override"} {
		if err := s.addColumn("devices", col, col+" TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("migrate devices.%s: %w", col, err)
		}
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (7, ?)`, now()); err != nil {
		return err
	}
	// Migration 8: why each driver did not answer on the last identification
	// (issue #19).
	if err := s.addColumn("devices", "identify_errors", "identify_errors TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return fmt.Errorf("migrate devices.identify_errors: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (8, ?)`, now()); err != nil {
		return err
	}
	// Migration 9: where each operator field's value came from
	// (intent/csv-import, spec decision 4).
	if err := s.addColumn("devices", "field_sources", "field_sources TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return fmt.Errorf("migrate devices.field_sources: %w", err)
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (9, ?)`, now()); err != nil {
		return err
	}
	return nil
}

// timestampColumns lists every TEXT column holding a ts()/now() value.
var timestampColumns = map[string][]string{
	"schema_migrations":         {"applied_at"},
	"devices":                   {"first_seen", "last_seen", "dns_checked_at", "created_at", "updated_at"},
	"groups":                    {"created_at", "updated_at"},
	"credentials":               {"created_at", "updated_at"},
	"jobs":                      {"created_at", "started_at", "finished_at"},
	"job_results":               {"at"},
	"telemetry":                 {"at"},
	"alerts":                    {"raised_at", "updated_at", "cleared_at", "acked_at"},
	"notification_log":          {"at"},
	"audit":                     {"at"},
	"users":                     {"created_at"},
	"sessions":                  {"expires_at", "created_at"},
	"consumables":               {"created_at", "updated_at"},
	"consumable_stock_events":   {"at"},
	"supply_links":              {"updated_at"},
	"catalog_qualifications":    {"updated_at"},
	"config_profiles":           {"created_at", "updated_at"},
	"config_snapshots":          {"at"},
	"config_writes":             {"at"},
	"config_drift":              {"at"},
	"device_logs":               {"at"},
	"tombstones":                {"deleted_at"},
	"firmware_packages":         {"created_at"},
	"firmware_campaigns":        {"scheduled_at", "started_at", "finished_at", "created_at", "updated_at"},
	"firmware_campaign_devices": {"started_at", "finished_at", "updated_at"},
	"notification_outbox":       {"next_at", "created_at", "done_at"},
	"security_policies":         {"created_at", "updated_at"},
	"security_assessments":      {"at"},
	"ca_profiles":               {"created_at", "updated_at"},
	"issued_certificates":       {"not_before", "not_after", "installed_at", "created_at"},
	"device_certificates":       {"not_before", "not_after", "seen_at"},
	"api_tokens":                {"expires_at", "created_at", "last_used_at", "revoked_at"},
}

// migrateFixedWidthTimestamps rewrites every stored timestamp that is not
// already in tsLayout. SQLite's strftime keeps millisecond precision, so
// rows written before this migration lose sub-millisecond digits, which
// nothing depends on; rows already 30 characters long are in the new form.
func (s *SQLite) migrateFixedWidthTimestamps() error {
	var done int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=5`).Scan(&done); err != nil {
		return err
	}
	if done > 0 {
		return nil
	}
	tables := make([]string, 0, len(timestampColumns))
	for t := range timestampColumns {
		tables = append(tables, t)
	}
	sort.Strings(tables)
	// Only columns that exist at this schema version: later migrations add
	// timestamp columns of their own (devices.dns_checked_at, migration 6)
	// and those hold no legacy values to rewrite. Looked up before the
	// transaction opens, since SQLite serves one connection at a time.
	present := map[string]map[string]bool{}
	for _, t := range tables {
		cols, err := s.columns(t)
		if err != nil {
			return err
		}
		present[t] = cols
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range tables {
		for _, c := range timestampColumns[t] {
			if !present[t][c] {
				continue
			}
			q := fmt.Sprintf(`UPDATE %s SET %s = substr(strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', %s), 1, 23) || '000000Z'
				WHERE %s IS NOT NULL AND %s <> '' AND length(%s) <> 30 AND strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', %s) IS NOT NULL`, t, c, c, c, c, c, c)
			if _, err := tx.Exec(q); err != nil {
				return fmt.Errorf("%s.%s: %w", t, c, err)
			}
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (5, ?)`, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// columns returns the set of column names PRAGMA table_info lists for table.
func (s *SQLite) columns(table string) (map[string]bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

// addColumn adds a column with ALTER TABLE unless PRAGMA table_info already
// lists it, so migrations stay idempotent on fresh and existing databases.
func (s *SQLite) addColumn(table, column, ddl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + ddl)
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

// tsLayout is the fixed-width UTC form every SQLite timestamp is stored in:
// always nine fractional digits and a trailing Z, so the text sorts and
// compares (<, >=, ORDER BY) exactly like the instants it encodes. RFC 3339
// with trimmed fractions does not: "…:00Z" sorts after "…:00.5Z" because
// 'Z' > '.' (code review H-020). Migration 5 rewrites older rows.
const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

func now() string { return time.Now().UTC().Format(tsLayout) }

func ts(t time.Time) string { return t.UTC().Format(tsLayout) }

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

func (s *SQLite) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, key)
	return err
}

var _ Store = (*SQLite)(nil)
