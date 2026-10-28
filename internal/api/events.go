package api

import (
	"net/http"
	"strconv"

	"printmarshal/internal/events"
	"printmarshal/internal/model"
)

// eventRoutes exposes the event catalogue and the durable notification outbox.
func (s *Server) eventRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/events/catalog", s.require(model.RoleViewer, s.eventCatalog))
	mux.Handle("GET /api/notifications/outbox", s.require(model.RoleOperator, s.listOutbox))
	mux.Handle("POST /api/notifications/outbox/{id}/requeue", s.require(model.RoleAdmin, s.requeueOutbox))
	mux.Handle("POST /api/notifications/drain", s.require(model.RoleAdmin, s.drainOutbox))
	mux.Handle("POST /api/notifiers/{name}/test", s.require(model.RoleAdmin, s.testNotifier))
}

func (s *Server) eventCatalog(w http.ResponseWriter, r *http.Request) {
	type def struct {
		Type        string `json:"type"`
		Description string `json:"description"`
		Notifiable  bool   `json:"notifiable"`
		Security    bool   `json:"security"`
	}
	out := []def{}
	for _, d := range events.Catalog {
		out = append(out, def{d.Type, d.Description, d.Notifiable, d.Security})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) listOutbox(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Store.ListOutbox(r.Context(), r.URL.Query().Get("state"), intQuery(r, "limit", 200))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Server) requeueOutbox(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.Store.RequeueDead(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r.Context(), r, "notification.requeue", r.PathValue("id"), nil, nil, "ok")
	if s.Notifier != nil {
		go s.Notifier.Drain(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) drainOutbox(w http.ResponseWriter, r *http.Request) {
	if s.Notifier != nil {
		s.Notifier.Drain(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// testNotifier sends a test message through the outbox to one destination.
func (s *Server) testNotifier(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	found := false
	for _, n := range s.Cfg.Notifiers {
		if n.Name == name {
			found = true
		}
	}
	if !found || s.Notifier == nil {
		writeErr(w, http.StatusNotFound, "no such notifier")
		return
	}
	if s.Bus != nil {
		s.Bus.PublishFrom(model.Event{Type: "notification.test", Data: map[string]any{"subject": "[TEST] PrintMarshal notification", "body": "This is a test from PrintMarshal (" + s.Version + ") requested by " + actor(r) + ".", "notifier": name}},
			events.Origin{Source: "operator", Actor: actor(r), IP: r.RemoteAddr})
	}
	s.audit(r.Context(), r, "notifier.test", name, nil, nil, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// publish sends an API-originated event through the bus (so notifiers and
// exporters see it too), falling back to the hub alone in tests without a bus.
func (s *Server) publish(r *http.Request, e model.Event) {
	if s.Bus != nil {
		origin := events.System
		if r != nil {
			origin = events.Origin{Source: "operator", Actor: actor(r), IP: clientIP(r)}
		}
		s.Bus.PublishFrom(e, origin)
		return
	}
	if s.Hub != nil {
		s.Hub.Publish(e)
	}
}
