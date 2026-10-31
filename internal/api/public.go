package api

import (
	"net/http"
	"time"
)

// Public status board (intent/public-status-dashboard). Step 2 of the plan:
// route stubs only, so the boundary tests compile and fail on behaviour.

const publicStatusKey = "public.status"

// publicShow holds the per-signal switches of the board.
type publicShow struct {
	Location bool `json:"location"`
	Online   bool `json:"online"`
	State    bool `json:"state"`
	Paper    bool `json:"paper"`
	Toner    bool `json:"toner"`
	Alerts   bool `json:"alerts"`
}

// publicStatusDevice is one listed device and its optional public label.
type publicStatusDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// publicStatusConfig is the stored configuration (settings key public.status).
type publicStatusConfig struct {
	Enabled      bool                 `json:"enabled"`
	Unlisted     bool                 `json:"unlisted"`
	Slug         string               `json:"slug"`
	Title        string               `json:"title"`
	FrameOrigins []string             `json:"frameOrigins"`
	Devices      []publicStatusDevice `json:"devices"`
	Show         publicShow           `json:"show"`
}

type publicState struct {
	cache *boardCache
}

type boardCache struct {
	now func() time.Time
}

func newPublicState() *publicState {
	return &publicState{cache: &boardCache{now: time.Now}}
}

func (s *Server) getPublicStatus(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "not implemented")
}

func (s *Server) getPublicStatusSettings(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "not implemented")
}

func (s *Server) putPublicStatusSettings(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotImplemented, "not implemented")
}

func alertCategory(source, key string) string { return "" }
