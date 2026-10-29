package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/stdlib"

	"printmarshal/internal/secrets"
)

var pgSchemaSeq atomic.Int64

// openPostgresTest opens the store on a fresh schema of the PM_TEST_PG_DSN
// database (search_path is a connection parameter, so every test gets its own
// empty namespace and the schema is dropped on cleanup). Run the suite with:
//
//	docker run -d --name pm-pg-test -e POSTGRES_PASSWORD=pm -e POSTGRES_DB=pm -p 55432:5432 postgres:16-alpine
//	PM_TEST_PG_DSN='postgres://postgres:pm@127.0.0.1:55432/pm?sslmode=disable' go test ./internal/store/
func openPostgresTest(t *testing.T) testStore {
	t.Helper()
	dsn := os.Getenv("PM_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("PM_TEST_PG_DSN not set")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("pm_test_%d_%d", time.Now().UnixNano()%1_000_000_000, pgSchemaSeq.Add(1))
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	box, err := secrets.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenPostgres(ctx, withSearchPath(dsn, schema), box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// withSearchPath adds search_path=schema to a URL or key=value DSN.
func withSearchPath(dsn, schema string) string {
	if strings.Contains(dsn, "://") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		return dsn + sep + "search_path=" + schema
	}
	return dsn + " search_path=" + schema
}

func (s *Postgres) rawExec(q string, args ...any) error {
	_, err := pgExec(context.Background(), s.db, q, args...)
	return err
}

func (s *Postgres) rawRow(q string, args ...any) rowScanner {
	return pgRow(context.Background(), s.db, q, args...)
}

func TestPostgres(t *testing.T) {
	if os.Getenv("PM_TEST_PG_DSN") == "" {
		t.Skip("PM_TEST_PG_DSN not set; start postgres:16-alpine and export the DSN to run the conformance suite against PostgreSQL")
	}
	runStoreTests(t, openPostgresTest)
	t.Run("MigrateIsIdempotent", func(t *testing.T) {
		s := openPostgresTest(t).(*Postgres)
		if err := s.migrate(context.Background()); err != nil {
			t.Fatalf("second migrate: %v", err)
		}
		var n int
		if err := s.rawRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 5 {
			t.Errorf("migrations = %d (%v)", n, err)
		}
	})
}

var _ = stdlib.OpenDB // the "pgx" database/sql driver is registered by importing stdlib
