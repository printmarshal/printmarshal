package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Operational endpoints (audit G21): /readyz for orchestrators and /metrics in
// the Prometheus text exposition format, with no client library dependency.

type httpStats struct {
	requests [6]atomic.Int64 // by status class 0..5 (index = status/100)
	inflight atomic.Int64
	latency  atomic.Int64 // cumulative milliseconds
}

func (s *Server) stats() *httpStats {
	if s.http == nil {
		s.http = &httpStats{}
	}
	return s.http
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if _, err := s.Store.CountUsers(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "database": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": s.Version})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var b strings.Builder
	gauge := func(name, help string, labels map[string]string, v float64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		b.WriteString(name)
		if len(labels) > 0 {
			keys := make([]string, 0, len(labels))
			for k := range labels {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
			}
			b.WriteString("{" + strings.Join(parts, ",") + "}")
		}
		fmt.Fprintf(&b, " %g\n", v)
	}
	series := func(name, help, typ string, rows map[string]float64, label string) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
		keys := make([]string, 0, len(rows))
		for k := range rows {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s{%s=%q} %g\n", name, label, k, rows[k])
		}
	}
	gauge("printmarshal_info", "Build information.", map[string]string{"version": s.Version}, 1)
	gauge("printmarshal_up", "1 when the API is serving.", nil, 1)
	if ds, err := s.Store.Dashboard(ctx); err == nil {
		gauge("printmarshal_devices_total", "Devices in the inventory.", nil, float64(ds.Devices))
		gauge("printmarshal_devices_offline", "Devices currently offline.", nil, float64(ds.Offline))
		bySev := map[string]float64{}
		for k, v := range ds.BySeverity {
			bySev[k] = float64(v)
		}
		series("printmarshal_devices_by_severity", "Devices by worst current severity.", "gauge", bySev, "severity")
		byState := map[string]float64{}
		for k, v := range ds.ByState {
			byState[k] = float64(v)
		}
		series("printmarshal_devices_by_state", "Devices by lifecycle state.", "gauge", byState, "state")
		gauge("printmarshal_alerts_active", "Active alerts.", nil, float64(ds.ActiveAlerts))
		gauge("printmarshal_alerts_critical", "Active critical alerts.", nil, float64(ds.CriticalAlerts))
		gauge("printmarshal_supplies_low", "Supplies at or below the warning threshold.", nil, float64(ds.LowSupplies))
		gauge("printmarshal_stock_low", "Stock items at or below their warning level.", nil, float64(ds.LowStock))
		gauge("printmarshal_jobs_running", "Jobs currently running.", nil, float64(ds.RunningJobs))
	} else {
		gauge("printmarshal_store_error", "1 when the last store read for metrics failed.", nil, 1)
	}
	for _, state := range []string{"pending", "dead"} {
		rows, err := s.Store.ListOutbox(ctx, state, 10000)
		if err == nil {
			gauge("printmarshal_notifications_outbox", "Notification outbox rows by state.", map[string]string{"state": state}, float64(len(rows)))
		}
	}
	st := s.stats()
	fmt.Fprintf(&b, "# HELP printmarshal_http_requests_total API requests by status class.\n# TYPE printmarshal_http_requests_total counter\n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, "printmarshal_http_requests_total{class=\"%dxx\"} %d\n", i, st.requests[i].Load())
	}
	gauge("printmarshal_http_inflight", "API requests in flight.", nil, float64(st.inflight.Load()))
	fmt.Fprintf(&b, "# HELP printmarshal_http_latency_ms_total Cumulative API request time in milliseconds.\n# TYPE printmarshal_http_latency_ms_total counter\nprintmarshal_http_latency_ms_total %d\n", st.latency.Load())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}
