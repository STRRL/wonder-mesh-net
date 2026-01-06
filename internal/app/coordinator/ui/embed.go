package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

// Handler returns an http.Handler that serves the embedded UI static files.
// It handles SPA routing by serving index.html for unknown paths.
func Handler() http.Handler {
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("embed static fs: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "" || path == "/" {
			path = "/index.html"
		}

		cleanPath := strings.TrimPrefix(path, "/")
		if _, err := fs.Stat(subFS, cleanPath); err != nil {
			path = "/index.html"
		}

		r.URL.Path = path
		fileServer.ServeHTTP(w, r)
	})
}
