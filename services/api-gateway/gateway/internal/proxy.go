package internal

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

type Proxy interface {
	ServeHTTP(targetHost, path string, w http.ResponseWriter, r *http.Request) error
}

type SingleHostProxy struct{}

func (p *SingleHostProxy) ServeHTTP(targetHost, path string, w http.ResponseWriter, r *http.Request) error {
	target, err := url.Parse(targetHost)
	if err != nil {
		return err
	}

	r.URL.Path = path
	r.Host = target.Host

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ServeHTTP(w, r)
	return nil
}
