package handlers

import (
	"log"
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
		log.Printf("[Headscale] %s %s", req.Method, req.URL.Path)
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("[Headscale] Error: %s %s: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Proxy error", http.StatusBadGateway)
	}

	return &HeadscaleProxyHandler{
		proxy: proxy,
	}, nil
}

func (h *HeadscaleProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.proxy.ServeHTTP(w, r)
}
