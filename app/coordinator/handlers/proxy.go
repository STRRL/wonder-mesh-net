package handlers

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// HeadscaleProxyHandler proxies requests to the embedded Headscale instance.
// It strips the configured prefix from incoming requests before forwarding.
type HeadscaleProxyHandler struct {
	proxy  *httputil.ReverseProxy
	prefix string
}

// NewHeadscaleProxyHandler creates a new proxy handler that forwards requests
// to the Headscale instance at the given target URL.
// The prefix (e.g., "/hs") is stripped from incoming request paths.
func NewHeadscaleProxyHandler(targetURL string, prefix string) (*HeadscaleProxyHandler, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalPath := req.URL.Path
		originalDirector(req)
		// Strip the prefix from the path
		if len(req.URL.Path) >= len(prefix) {
			req.URL.Path = req.URL.Path[len(prefix):]
		}
		if req.URL.Path == "" {
			req.URL.Path = "/"
		}
		// Also update RawPath if set
		if req.URL.RawPath != "" && len(req.URL.RawPath) >= len(prefix) {
			req.URL.RawPath = req.URL.RawPath[len(prefix):]
		}
		// Preserve original host for WebSocket connections
		req.Host = target.Host
		log.Printf("[Proxy] Forwarding: %s -> %s (Host: %s)", originalPath, req.URL.Path, req.Host)
	}

	// ModifyResponse can be used to handle errors, but we leave it nil for now
	proxy.ModifyResponse = nil

	// ErrorHandler to log errors
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Proxy] Error forwarding %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	return &HeadscaleProxyHandler{
		proxy:  proxy,
		prefix: prefix,
	}, nil
}

func (h *HeadscaleProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Proxy] Incoming: %s %s | Connection: %s | Upgrade: %s",
		r.Method, r.URL.Path, r.Header.Get("Connection"), r.Header.Get("Upgrade"))
	h.proxy.ServeHTTP(w, r)
}
