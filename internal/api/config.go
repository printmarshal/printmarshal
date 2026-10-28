package api

import "net/http"

// configRoutes mounts the config endpoints. Implemented by the config feature; keep
// handlers in this file (and config_*.go) so server.go stays shared.
func (s *Server) configRoutes(mux *http.ServeMux) {
	_ = mux
}
