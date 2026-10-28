// Package web embeds the built single-page application (web/dist is copied to
// internal/web/dist by the frontend build) and serves it with SPA fallback.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the SPA. Unknown paths fall back to index.html so client-side
// routing works; /api and /healthz are never handled here.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	files := http.FS(sub)
	fileServer := http.FileServer(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + r.URL.Path)
		if f, err := files.Open(p); err == nil {
			st, serr := f.Stat()
			f.Close()
			if serr == nil && !st.IsDir() {
				if strings.HasPrefix(p, "/assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
