package internal

import "net/http"

type Client interface {
	ForwardRequest(w http.ResponseWriter, r *http.Request) error
}

type GatewayClient struct {
	router Router
	proxy  Proxy
}

func NewClient(router Router) Client {
	return &GatewayClient{
		router: router,
		proxy:  &SingleHostProxy{},
	}
}

func (c *GatewayClient) ForwardRequest(w http.ResponseWriter, r *http.Request) error {
	targetHost, path, err := c.router.Match(r.URL.Path)
	if err != nil {
		return err
	}
	return c.proxy.ServeHTTP(targetHost, path, w, r)
}
