package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed static/*
var staticFS embed.FS

// Handler returns an http.Handler that serves the embedded UI static files.
// It handles SPA routing by serving index.html for unknown paths.
// Returns an error if the embedded filesystem cannot be accessed.
func Handler() (http.Handler, error) {
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, fmt.Errorf("embed static fs: %w", err)
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

		req := r.Clone(r.Context())
		req.URL.Path = path
		fileServer.ServeHTTP(w, req)
	}), nil
}
