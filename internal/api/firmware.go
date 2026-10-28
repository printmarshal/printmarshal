package api

import "net/http"

// firmwareRoutes mounts the firmware endpoints. Implemented by the firmware feature; keep
// handlers in this file (and firmware_*.go) so server.go stays shared.
func (s *Server) firmwareRoutes(mux *http.ServeMux) {
	_ = mux
}
