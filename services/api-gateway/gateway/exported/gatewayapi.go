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

// protectedPrefixes lists the routes that require a valid JWT. Everything else
// is public: POST /users/register, POST /users/login and GET /events.
//
// This is the single place auth is enforced at the edge. It used to be applied
// here *and* globally in cmd/main.go, where the global pass exempted only
// /users/*, so GET /events demanded a token despite being a public browse
// endpoint.
//
// Prefixes cannot distinguish methods, so PUT /events/{id} passes through here
// unauthenticated -- GET on the same path is public. The service authenticates
// it regardless, which is why services verify tokens themselves rather than
// trusting the edge.
var protectedPrefixes = []string{
	"/events/create",
	"/organizer/",
	"/tickets/",
}

// NewGatewayHandler creates a handler that proxies requests internally
func NewGatewayHandler() http.Handler {
	router := internal.NewRouter()
	client := internal.NewClient(router)
	return &GatewayHandler{client: client}
}

func (g *GatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	forward := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := g.client.ForwardRequest(w, r); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	})

	if isProtected(r.URL.Path) {
		middleware.JWTMiddleware(forward).ServeHTTP(w, r)
		return
	}

	forward.ServeHTTP(w, r)
}

func isProtected(path string) bool {
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
