package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"os"

	"github.com/gin-gonic/gin"
)

// ReverseProxy forwards requests to the specified microservice
func ReverseProxy(targetHost string) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, err := url.Parse(targetHost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid target URL"})
			return
		}

		// Rewrite URL path by removing the prefix
		c.Request.URL.Path = c.Param("path")

		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ServeHTTP(c.Writer, c.Request)
	}
}

// RegisterGatewayRoutes sets up routes for all microservices
func RegisterGatewayRoutes(router *gin.Engine) {
	userServiceURL := os.Getenv("USER_SERVICE_URL")
	eventServiceURL := os.Getenv("EVENT_SERVICE_URL")

	router.Any("/users/*path", ReverseProxy(userServiceURL))
	router.Any("/events/*path", ReverseProxy(eventServiceURL))
}
