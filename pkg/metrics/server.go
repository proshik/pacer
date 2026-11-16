package metrics

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Path is where the scrape is served on the metrics listener.
const Path = "/metrics"

// Server is the metrics listener. It is separate from the public HTTP port, so
// the scrape endpoint is not reachable through the ingress at all.
type Server struct {
	server   *http.Server
	listener net.Listener
	done     chan struct{}
}

// Listen binds addr right away, so a busy or malformed address fails at
// startup, and serves the scrape on Path in the background.
func (m *Metrics) Listen(addr, token string) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen for metrics on %q: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.Handle("GET "+Path, m.Handler(token))

	s := &Server{
		server: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
		listener: listener,
		done:     make(chan struct{}),
	}

	go func() {
		defer close(s.done)
		// A failure here leaves the app serving without metrics; it is
		// logged rather than bringing the bot down.
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server stopped unexpectedly", "err", err)
		}
	}()

	return s, nil
}

// Addr is the address the listener is bound to, with the real port when addr
// asked for port 0.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// Shutdown stops the listener and waits for in-flight scrapes. It is safe on a
// nil *Server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}

	err := s.server.Shutdown(ctx)
	select {
	case <-s.done:
	case <-ctx.Done():
	}

	return err
}
