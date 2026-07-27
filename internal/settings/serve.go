package settings

import (
	"context"
	"net"
	"net/http"
)

// Serve starts the settings server on 127.0.0.1 (a free port). It returns the
// bound address and a shutdown function.
func Serve(home string) (string, func(context.Context) error, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: Handler(home)}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), srv.Shutdown, nil
}
