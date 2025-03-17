package internal

import (
	"errors"
	"os"
	"strings"
)

type Router interface {
	Match(path string) (targetHost, newPath string, err error)
}

type router struct {
	routes map[string]string
}

func NewRouter() Router {
	return &router{
		routes: map[string]string{
			"/users/":  os.Getenv("USER_SERVICE_URL"),
			"/events/": os.Getenv("EVENT_SERVICE_URL"),
		},
	}
}

func (r *router) Match(path string) (string, string, error) {
	for prefix, host := range r.routes {
		if strings.HasPrefix(path, prefix) {
			newPath := strings.TrimPrefix(path, strings.TrimSuffix(prefix, "/"))
			if newPath == "" {
				newPath = "/"
			}
			return host, newPath, nil
		}
	}
	return "", "", errors.New("service route not found")
}
