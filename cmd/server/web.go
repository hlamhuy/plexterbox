package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"
)

//go:embed all:dist
var embeddedUI embed.FS

// spaHandler serves the embedded Vite build.
// Any path that doesn't match a real file is served index.html so that
// client-side routing works after a hard refresh.
func spaHandler() http.Handler {
	sub, err := fs.Sub(embeddedUI, "dist")
	if err != nil {
		log.Fatalf("[startup] failed to sub embedded UI: %v", err)
	}
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested path in the embedded FS.
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		f, err := sub.Open(path)
		if err != nil {
			// Not found → serve index.html for SPA client-side routing.
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		f.Close()
		fileServer.ServeHTTP(w, r)
	})
}
