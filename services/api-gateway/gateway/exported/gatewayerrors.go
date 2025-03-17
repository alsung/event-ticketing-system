package exported

import "errors"

var (
	ErrServiceNotFound = errors.New("service not found")
	ErrBadGateway      = errors.New("bad gateway")
)
