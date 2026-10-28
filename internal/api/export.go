package api

import (
	"context"
	"net/http"
	"sync"

	"printmarshal/internal/model"
	"printmarshal/internal/siem"
	"printmarshal/internal/store"
)

// SIEM / log export and the incremental sync contract (audit A12, A15;
// docs/export.md). The exporter is wired from main through SetExport rather
// than a Server field, so the shared server file stays untouched.

var (
	exportMu sync.RWMutex
	exporter *siem.Exporter
)

// SetExport registers the exporter the routes report on. main calls it once:
// api.SetExport(exporter).
func SetExport(e *siem.Exporter) {
	exportMu.Lock()
	defer exportMu.Unlock()
	exporter = e
}

func getExporter() *siem.Exporter {
	exportMu.RLock()
	defer exportMu.RUnlock()
	return exporter
}

// exportRoutes registers the export status, device log and sync endpoints.
func (s *Server) exportRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/export/status", s.require(model.RoleViewer, s.exportStatus))
	mux.Handle("GET /api/devices/{id}/logs", s.require(model.RoleViewer, s.deviceLogs))
	mux.Handle("GET /api/sync/{collection}", s.require(model.RoleViewer, s.sync))
}

// logStore returns the LogStore facet of the store, or nil when the store
// does not provide it.
func (s *Server) logStore() store.LogStore {
	ls, _ := s.Store.(store.LogStore)
	return ls
}

// tombstone records a deletion for sync clients (called from deleteDevice).
func (s *Server) tombstone(ctx context.Context, collection, id string) {
	ls := s.logStore()
	if ls == nil {
		return
	}
	if err := ls.AddTombstone(ctx, collection, id); err != nil {
		s.Log.Error("tombstone write failed", "collection", collection, "id", id, "err", err)
	}
}

// exportStatus reports sink states, queue and drop counters and the ingest
// listener. Without an exporter (tests, or main not wired) it reports disabled.
func (s *Server) exportStatus(w http.ResponseWriter, r *http.Request) {
	e := getExporter()
	if e == nil {
		writeJSON(w, http.StatusOK, siem.Status{Enabled: false, Sinks: []siem.SinkStatus{}, Events: siem.ExportedTypes(), Collection: siem.SyncCollections,
			Ingest: siem.IngestStatus{Retention: s.Cfg.Export.IngestRetention.D().String()}})
		return
	}
	writeJSON(w, http.StatusOK, e.Status())
}

// deviceLogs lists the syslog lines a device sent, newest first.
func (s *Server) deviceLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetDevice(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	ls := s.logStore()
	if ls == nil {
		writeErr(w, http.StatusNotImplemented, "device logs are not available on this datastore")
		return
	}
	limit := intQuery(r, "limit", 200)
	if limit > 2000 {
		limit = 2000
	}
	rows, err := ls.ListDeviceLogs(r.Context(), id, limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	type line struct {
		*model.DeviceLog
		SeverityName string `json:"severityName"`
		FacilityName string `json:"facilityName"`
	}
	out := make([]line, 0, len(rows))
	for _, l := range rows {
		out = append(out, line{l, model.SyslogSeverityName(l.Severity), model.SyslogFacilityName(l.Facility)})
	}
	writeJSON(w, http.StatusOK, out)
}
