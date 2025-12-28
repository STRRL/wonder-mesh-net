package controller

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// HeadscaleProxyController forwards requests to the embedded Headscale instance.
type HeadscaleProxyController struct {
	proxy *httputil.ReverseProxy
}

// NewHeadscaleProxyController creates a new HeadscaleProxyController.
func NewHeadscaleProxyController(targetURL string) (*HeadscaleProxyController, error) {
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "error", err, "path", r.URL.Path)
		http.Error(w, "proxy error", http.StatusBadGateway)
	}

	return &HeadscaleProxyController{proxy: proxy}, nil
}

// ServeHTTP forwards requests to Headscale.
func (c *HeadscaleProxyController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.proxy.ServeHTTP(w, r)
}
