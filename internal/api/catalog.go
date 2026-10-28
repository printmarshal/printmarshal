package api

import "net/http"

// catalogRoutes mounts the device model catalogue endpoints (implemented by the
// catalogue feature in this file; server.go stays shared).
func (s *Server) catalogRoutes(mux *http.ServeMux) {
	_ = mux
}
