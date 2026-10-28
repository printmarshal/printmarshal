package api

import "net/http"

// printProviderRoutes mounts the optional print-provider (CUPS / IPP Everywhere
// bridge) endpoints; implemented by that feature in this file.
func (s *Server) printProviderRoutes(mux *http.ServeMux) {
	_ = mux
}
