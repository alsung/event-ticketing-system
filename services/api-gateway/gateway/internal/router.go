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
			"/users/":  os.Getenv("USER_SERVICE_URL"),
			"/events/": os.Getenv("EVENT_SERVICE_URL"),
		},
	}
}

func (r *router) Match(path string) (string, string, error) {
	for prefix, host := range r.routes {
		log.Println("path: ", path)
		log.Println("prefix: ", prefix)
		log.Println(strings.HasPrefix(path, prefix))
		if path == prefix || strings.HasPrefix(path, prefix) {
			newPath := strings.TrimPrefix(path, prefix)
			if !strings.HasPrefix(newPath, "/") {
				newPath = "/" + newPath
			}
			log.Println("newPath: ", newPath)
			return host, newPath, nil
		}
	}
	return "", "", errors.New("service route not found")
}
