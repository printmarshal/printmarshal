package api

import "net/http"

// securityRoutes mounts the security endpoints. Implemented by the security feature; keep
// handlers in this file (and security_*.go) so server.go stays shared.
func (s *Server) securityRoutes(mux *http.ServeMux) {
	_ = mux
}
