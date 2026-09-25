package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type Server struct {
	srv  *http.Server
	addr string
}

func New(addr string, h http.Handler) *Server {
	return &Server{
		addr: addr,
		srv: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}
}

// Start binds the port synchronously, so a busy port fails fast at boot,
// and then serves in the background. Anything that stops the server other
// than Shutdown is reported on errs.
func (s *Server) Start(errs chan<- error) error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.srv.Addr, err)
	}
	s.addr = ln.Addr().String()

	go func() {
		if err := s.srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			errs <- fmt.Errorf("serve %s: %w", s.addr, err)
		}
	}()
	return nil
}

// Addr is the address actually bound, which matters when the port is 0.
func (s *Server) Addr() string {
	return s.addr
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
