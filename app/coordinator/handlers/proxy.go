package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// HeadscaleProxyHandler proxies requests to the embedded Headscale instance.
type HeadscaleProxyHandler struct {
	proxy *httputil.ReverseProxy
}

// NewHeadscaleProxyHandler creates a new proxy handler that forwards requests
// to the Headscale instance at the given target URL.
func NewHeadscaleProxyHandler(targetURL string) (*HeadscaleProxyHandler, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		slog.Debug("headscale proxy", "method", req.Method, "path", req.URL.Path)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("headscale proxy error", "method", r.Method, "path", r.URL.Path, "error", err)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	return &HeadscaleProxyHandler{
		proxy: proxy,
	}, nil
}

func (h *HeadscaleProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}
