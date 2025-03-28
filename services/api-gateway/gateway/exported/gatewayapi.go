package exported

import (
	"net/http"
	"strings"

	"github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported/middleware"
	"github.com/alsung/event-ticketing-system/services/api-gateway/gateway/internal"
)

type GatewayHandler struct {
	client internal.Client
}

// Define the list of routes that require JWT auth
var protectedPaths = []string{
	"/events/create",
	"/tickets/create",
	"/tickets/purchase",
	"/tickets/mine",
}

// NewGatewayHandler creates a handler that proxies requests internally
func NewGatewayHandler() http.Handler {
	router := internal.NewRouter()
	client := internal.NewClient(router)
	return &GatewayHandler{client: client}
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Check if the path is protected and needs JWT auth
	for _, protected := range protectedPaths {
		if strings.HasPrefix(path, protected) {
			// Wrap with JWT middleware
			middleware.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := g.client.ForwardRequest(w, r); err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
				}
			})).ServeHTTP(w, r)
			return
		}
	}

	// Public route - forward directly
	if err := g.client.ForwardRequest(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}
