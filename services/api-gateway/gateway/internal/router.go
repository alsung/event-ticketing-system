package internal

import (
	"errors"
	"log"
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
			"/users":    os.Getenv("USER_SERVICE_URL"),
			"/users/":   os.Getenv("USER_SERVICE_URL"),
			"/events":   os.Getenv("EVENT_SERVICE_URL"),
			"/events/":  os.Getenv("EVENT_SERVICE_URL"),
			"/tickets":  os.Getenv("TICKET_SERVICE_URL"),
			"/tickets/": os.Getenv("TICKET_SERVICE_URL"),
		},
	}
}

func (r *router) Match(path string) (string, string, error) {
	for prefix, host := range r.routes {
		if strings.HasPrefix(path, prefix) {
			// Do NOT trim the prefix anymore - return path as-is
			log.Println("path", path)
			log.Println("host", host)
			return host, path, nil
		}
	}
	return "", "", errors.New("service route not found")
}
