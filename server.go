// Package castor is a small, function-based DSL for HTTP services built only
// on the Go standard library.
package castor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	shutdownTimeout = 10 * time.Second
)

// ServerConfig holds the runtime configuration for a WebServer.
type ServerConfig struct {
	Host         string
	Port         int
	Logger       LoggerInterface
	IdleTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// ServerOption configures a WebServer.
type ServerOption func(*WebServer)

// Config applies cfg to the server.
func Config(cfg ServerConfig) ServerOption {
	return func(s *WebServer) {
		s.cfg = cfg
	}
}

func WithHost(addr string) ServerOption {
	return func(s *WebServer) {
		s.cfg.Host = addr
	}
}

func WithPort(port int) ServerOption {
	return func(s *WebServer) {
		s.cfg.Port = port
	}
}

func WithLogger(logger LoggerInterface) ServerOption {
	return func(s *WebServer) {
		s.cfg.Logger = logger
	}
}

// WebServer is the DSL entry point.
type WebServer struct {
	cfg            ServerConfig
	routes         []Route
	groups         []RouteGroup
	mws            []Middleware
	notFound       NotFoundHandler
	customNotFound bool
}

// Server creates a server listening on 0.0.0.0:3000 by default.
func Server(opts ...ServerOption) *WebServer {
	s := &WebServer{
		cfg:      ServerConfig{Host: "0.0.0.0", Port: 3000, IdleTimeout: 10 * time.Second},
		notFound: defaultNotFound,
	}

	s.mws = append(s.mws, Logger)

	for _, opt := range opts {
		opt(s)
	}
	return s
}

// NotFound sets the handler invoked for any 404 response, including 404s
// written by matched route handlers. The handler is responsible for writing
// the response. Passing nil restores the default behavior.
func (s *WebServer) NotFound(handler NotFoundHandler) *WebServer {
	if handler == nil {
		s.notFound = defaultNotFound
		s.customNotFound = false
		return s
	}
	s.notFound = handler
	s.customNotFound = true
	return s
}

// Middlewares appends global middleware in execution order.
func (s *WebServer) Middlewares(middlewares ...Middleware) *WebServer {
	s.mws = append(s.mws, middlewares...)
	return s
}

// Routes adds routes and groups to the server.
func (s *WebServer) Routes(routes ...Route) *WebServer {
	s.routes = append(s.routes, routes...)
	return s
}

func (s *WebServer) Groups(groups ...RouteGroup) *WebServer {
	s.groups = append(s.groups, groups...)
	return s
}

// StaticAssets serves dir under pattern, for example "/assets/**".
func (s *WebServer) StaticAssets(pattern, dir string, cfgs ...StaticConfig) *WebServer {
	s.routes = append(s.routes, staticNode(pattern, dir, cfgs...))
	return s
}

// Start builds the router, listens, and blocks until SIGINT or SIGTERM before
// shutting down gracefully.
func (s *WebServer) Start() error {
	srv := &http.Server{
		Addr:         net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port)),
		Handler:      s.handler(),
		IdleTimeout:  s.cfg.IdleTimeout,
		ReadTimeout:  s.cfg.ReadTimeout,
		WriteTimeout: s.cfg.WriteTimeout,
	}

	// SIGKILL cannot be caught; these cover the graceful shutdown signals.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill, syscall.SIGTERM)
	defer stop()

	slog.Info(fmt.Sprintf("Server up and running at http://%s", srv.Addr))
	listener, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (s *WebServer) handler() http.Handler {
	mux := http.NewServeMux()

	// group routes
	for _, group := range s.groups {
		for _, subroute := range group.Routes {
			subroute(mux, group.Prefix, group.mws)
		}
	}

	// top-level routes
	for _, route := range s.routes {
		route(mux, "", nil)
	}

	if s.customNotFound {
		notFound := s.notFound
		if notFound == nil {
			notFound = defaultNotFound
		}
		return chain(withNotFoundHandler(mux, notFound), s.mws)
	}
	return chain(mux, s.mws)
}
