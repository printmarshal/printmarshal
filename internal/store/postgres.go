package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"printmarshal/internal/secrets"
)

//go:embed pgschema.sql pgschema_*.sql
var pgSchemaFS embed.FS

// pgMigrationLock is the advisory lock key taken while the schema is applied,
// so several instances starting against one database do not race the
// CREATE ... IF NOT EXISTS statements.
const pgMigrationLock = 0x5052494e544d5348 // "PRINTMSH"

// Postgres implements Store on PostgreSQL (the scale-out datastore, spec §5).
//
// It is structured exactly like SQLite — one file per area, the same helpers
// and scan shapes — but with native column types: TIMESTAMPTZ instead of
// RFC 3339 text, BOOLEAN instead of 0/1, JSONB for documents, BYTEA for the
// sealed secrets. Queries use '?' placeholders like the SQLite code and are
// rewritten to $n on the way out (pgq), which keeps the two implementations
// diffable side by side.
type Postgres struct {
	db   *sql.DB
	pool *pgxpool.Pool
	box  *secrets.Box
}

// OpenPostgres connects to dsn (a postgres:// URL or key=value string, pool
// settings such as pool_max_conns included), applies the schema and returns
// the store. box encrypts credential secrets and issued private keys.
func OpenPostgres(ctx context.Context, dsn string, box *secrets.Box) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres dsn: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	s := &Postgres{db: stdlib.OpenDBFromPool(pool), pool: pool, box: box}
	if err := s.migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Postgres) migrate(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, int64(pgMigrationLock)); err != nil {
		return fmt.Errorf("postgres migration lock: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, int64(pgMigrationLock)) //nolint:errcheck
	entries, err := pgSchemaFS.ReadDir(".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names) // pgschema.sql first, then pgschema_<area>.sql alphabetically
	for _, name := range names {
		b, err := pgSchemaFS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("schema %s: %w", name, err)
		}
	}
	// Versions match the SQLite numbering: 1 = initial schema, 2 = the legacy
	// stock migration (nothing to do on a database that never had the phase-2
	// table), 3 = credentials.purpose (the ALTER ... IF NOT EXISTS above).
	// They are recorded so later migrations number the same on both backends.
	// 4 = users.external_issuer/external_subject (the ALTER ... IF NOT EXISTS above).
	// 5 = SQLite's fixed-width timestamp rewrite (nothing to do here: timestamptz).
	// 6 = devices.dns_name, dns_name_suspect, dns_checked_at, display_name_source, dns_sync (the ALTER ... IF NOT EXISTS above).
	// 7 = devices.web_url_probed/web_url_override (the ALTER ... IF NOT EXISTS above).
	// 8 = devices.identify_errors (the ALTER ... IF NOT EXISTS above).
	// 9 = devices.field_sources (the ALTER ... IF NOT EXISTS above).
	for _, v := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES ($1, $2) ON CONFLICT DO NOTHING`, v, nowUTC()); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the connection pool.
func (s *Postgres) Close() error {
	err := s.db.Close()
	s.pool.Close()
	return err
}

// --- helpers ---------------------------------------------------------------

// pgq rewrites '?' placeholders to PostgreSQL's $1..$n. The store never puts a
// question mark inside a string literal, so a plain scan is enough.
func pgq(q string) string {
	if !strings.Contains(q, "?") {
		return q
	}
	var b strings.Builder
	b.Grow(len(q) + 16)
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func itoa(n int) string { return fmt.Sprint(n) }

// pgErr normalises driver errors so callers can keep their string checks:
// the API layer maps "UNIQUE" in a store error to 409 Conflict (SQLite's
// wording), so unique violations carry that word here too.
func pgErr(err error) error {
	if err == nil {
		return nil
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return fmt.Errorf("UNIQUE constraint failed: %s: %w", pe.ConstraintName, err)
	}
	return err
}

// querier is satisfied by *sql.DB, *sql.Conn and *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func pgExec(ctx context.Context, q querier, query string, args ...any) (sql.Result, error) {
	res, err := q.ExecContext(ctx, pgq(query), args...)
	return res, pgErr(err)
}

func pgQuery(ctx context.Context, q querier, query string, args ...any) (*sql.Rows, error) {
	rows, err := q.QueryContext(ctx, pgq(query), args...)
	return rows, pgErr(err)
}

func pgRow(ctx context.Context, q querier, query string, args ...any) *sql.Row {
	return q.QueryRowContext(ctx, pgq(query), args...)
}

// affected reports ErrNotFound when a statement touched no row.
func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// nowUTC is the PostgreSQL counterpart of now(): a UTC time at the
// microsecond precision TIMESTAMPTZ stores, without a monotonic reading, so a
// value written and read back compares equal with ==.
func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }

// pgt normalises a time for storage (UTC, microseconds).
func pgt(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

// pgtp is pgt for optional times (nil = NULL).
func pgtp(t *time.Time) any {
	if t == nil {
		return nil
	}
	return pgt(*t)
}

// timeOf converts a scanned nullable time to the model's pointer form.
func timeOf(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time.UTC()
	return &v
}

// --- settings --------------------------------------------------------------

func (s *Postgres) GetSetting(ctx context.Context, key string, dst any) error {
	var v string
	err := pgRow(ctx, s.db, `SELECT value::text FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(v), dst)
}

func (s *Postgres) SetSetting(ctx context.Context, key string, v any) error {
	_, err := pgExec(ctx, s.db, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, toJSON(v))
	return err
}

func (s *Postgres) DeleteSetting(ctx context.Context, key string) error {
	_, err := pgExec(ctx, s.db, `DELETE FROM settings WHERE key=?`, key)
	return err
}

var _ Store = (*Postgres)(nil)
