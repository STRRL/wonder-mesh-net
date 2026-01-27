package webui

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
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

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		if urlPath == "" || urlPath == "/" {
			urlPath = "/index.html"
		}

		cleanPath := strings.TrimPrefix(urlPath, "/")
		if _, err := fs.Stat(subFS, cleanPath); err != nil {
			cleanPath = "index.html"
		}

		f, err := subFS.Open(cleanPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if stat.IsDir() {
			cleanPath = path.Join(cleanPath, "index.html")
			f.Close()
			f, err = subFS.Open(cleanPath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			stat, _ = f.Stat()
		}

		w.Header().Set("Content-Type", contentType(cleanPath))
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, cleanPath, stat.ModTime(), rs)
		} else {
			w.WriteHeader(http.StatusOK)
			io.Copy(w, f)
		}
	}), nil
}

func contentType(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "application/javascript"
	case strings.HasSuffix(name, ".json"):
		return "application/json"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".ico"):
		return "image/x-icon"
	default:
		return "application/octet-stream"
	}
}
