package api

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"printmarshal/internal/model"
	"printmarshal/internal/reports"
	"printmarshal/internal/store"
)

// Reports API (spec §8.2 CSV export, §8.3 trends, §8.5 reporting). Every
// report is read-only and open to viewers. Handlers live here (and
// reports_*.go) so server.go stays shared; server.go mounts them with one
// line: s.reportRoutes(mux).

// reportRoutes mounts the report endpoints. Each takes
// ?from=&to=&group=&device=&format=json|csv&table=<name>&top=N.
func (s *Server) reportRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/reports", s.require(model.RoleViewer, s.listReports))
	mux.Handle("GET /api/reports/utilisation", s.require(model.RoleViewer, s.report("utilisation", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.Utilisation(ctx, p)
	})))
	mux.Handle("GET /api/reports/fleet-health", s.require(model.RoleViewer, s.report("fleet-health", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.FleetHealth(ctx, p)
	})))
	mux.Handle("GET /api/reports/supplies", s.require(model.RoleViewer, s.report("supplies", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.Supplies(ctx, p)
	})))
	mux.Handle("GET /api/reports/alerts", s.require(model.RoleViewer, s.report("alerts", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.Alerts(ctx, p)
	})))
	mux.Handle("GET /api/reports/firmware", s.require(model.RoleViewer, s.report("firmware", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.Firmware(ctx, p)
	})))
	mux.Handle("GET /api/reports/compliance", s.require(model.RoleViewer, s.report("compliance", func(ctx context.Context, svc *reports.Service, p reports.Params) (reportResult, error) {
		return svc.Compliance(ctx, p)
	})))
}

// reportResult is any report: a JSON document that also exports flat tables.
type reportResult interface {
	Tables() []reports.Table
}

// reportNames lists the reports and their CSV tables (first = default).
var reportNames = []map[string]any{
	{"name": "utilisation", "title": "Utilisation", "tables": []string{"devices", "days", "groups"}},
	{"name": "fleet-health", "title": "Fleet health", "tables": []string{"devices", "days"}},
	{"name": "supplies", "title": "Supplies", "tables": []string{"supplies", "replacements", "consumption"}},
	{"name": "alerts", "title": "Alerts", "tables": []string{"devices", "days", "sources"}},
	{"name": "firmware", "title": "Firmware", "tables": []string{"devices", "models"}},
	{"name": "compliance", "title": "Compliance", "tables": []string{"assessments"}},
}

// reportService resolves the report service from the store. The ReportStore
// surface is reached through a type assertion until store.Store embeds it.
func (s *Server) reportService() (*reports.Service, bool) {
	rs, ok := s.Store.(reports.Store)
	if !ok {
		return nil, false
	}
	return reports.New(rs), true
}

func (s *Server) listReports(w http.ResponseWriter, r *http.Request) {
	_, ok := s.reportService()
	writeJSON(w, http.StatusOK, map[string]any{"available": ok, "reports": reportNames, "maxRangeDays": reports.MaxRangeDays})
}

// parseReportTime accepts RFC 3339 or a plain date. A plain date for `to`
// means the end of that day (exclusive next midnight UTC).
func parseReportTime(v string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is not an RFC 3339 time or YYYY-MM-DD date", v)
	}
	if endOfDay {
		t = t.Add(24 * time.Hour)
	}
	return t, nil
}

func reportParams(r *http.Request) (reports.Params, error) {
	q := r.URL.Query()
	p := reports.Params{Group: q.Get("group"), Device: q.Get("device"), TopN: intQuery(r, "top", 10)}
	var err error
	if v := q.Get("from"); v != "" {
		if p.From, err = parseReportTime(v, false); err != nil {
			return p, fmt.Errorf("from: %w", err)
		}
	}
	if v := q.Get("to"); v != "" {
		if p.To, err = parseReportTime(v, true); err != nil {
			return p, fmt.Errorf("to: %w", err)
		}
	}
	if v := q.Get("days"); v != "" && p.From.IsZero() {
		n := intQuery(r, "days", 30)
		if n <= 0 {
			return p, errors.New("days must be positive")
		}
		to := p.To
		if to.IsZero() {
			to = time.Now().UTC()
		}
		p.From = to.Add(-time.Duration(n) * 24 * time.Hour)
	}
	return p, nil
}

// report builds a handler for one report: JSON by default, CSV of the chosen
// table with ?format=csv (&table=<name>).
func (s *Server) report(name string, run func(context.Context, *reports.Service, reports.Params) (reportResult, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svc, ok := s.reportService()
		if !ok {
			writeErr(w, http.StatusServiceUnavailable, "reports need the SQLite store")
			return
		}
		p, err := reportParams(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		rep, err := run(ctx, svc, p)
		if err != nil {
			switch {
			case errors.Is(err, reports.ErrInvalid):
				writeErr(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), reports.ErrInvalid.Error()+": "))
			case errors.Is(err, store.ErrNotFound):
				writeErr(w, http.StatusNotFound, "group or device not found")
			default:
				s.fail(w, err)
			}
			return
		}
		if r.URL.Query().Get("format") != "csv" {
			writeJSON(w, http.StatusOK, rep)
			return
		}
		tables := rep.Tables()
		table := tables[0]
		if want := r.URL.Query().Get("table"); want != "" {
			found := false
			for _, t := range tables {
				if t.Name == want {
					table, found = t, true
				}
			}
			if !found {
				writeErr(w, http.StatusBadRequest, fmt.Sprintf("unknown table %q", want))
				return
			}
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, name, table.Name))
		cw := csv.NewWriter(w)
		_ = cw.Write(table.Columns)
		for _, row := range table.Rows {
			_ = cw.Write(row)
		}
		cw.Flush()
	}
}
