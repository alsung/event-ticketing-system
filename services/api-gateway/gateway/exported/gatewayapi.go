package exported

import (
	"net/http"

	"github.com/alsung/event-ticketing-system/api-gateway/gateway/internal"
)

// NewGatewayHandler creates a handler that proxies requests internally
func NewGatewayHandler() http.Handler {
	router := internal.NewRouter()
	client := internal.NewClient(router)
	return &GatewayHandler{client: client}
}

type GatewayHandler struct {
	client internal.Client
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := g.client.ForwardRequest(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
