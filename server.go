// Package webgo is a small, function-based DSL for HTTP services built only
// on the Go standard library.
package castor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// NotFoundHandler handles a 404 response.
//
// statusCode is the original status code and message is the original response
// body produced by the route or by ServeMux for an unmatched route.
type NotFoundHandler func(res http.ResponseWriter, statusCode int, message string)

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
		cfg:      ServerConfig{Host: "0.0.0.0", Port: 3000},
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

	for _, group := range s.groups {
		for _, subroute := range group.Routes {
			subroute(mux, group.Prefix, group.mws)
		}
	}

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

func defaultNotFound(res http.ResponseWriter, statusCode int, message string) {
	if message == "" {
		message = http.StatusText(statusCode)
	}
	http.Error(res, message, statusCode)
}

// withNotFoundHandler captures a 404 written by any route handler and lets the
// custom handler replace that response while retaining the original details.
func withNotFoundHandler(next http.Handler, handler NotFoundHandler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		writer := &notFoundWriter{
			ResponseWriter: res,
			headerSnapshot: res.Header().Clone(),
		}
		next.ServeHTTP(writer, req)
		if !writer.captured {
			return
		}
		writer.restoreHeaders()
		handler(res, writer.status, writer.body.String())
	})
}

type notFoundWriter struct {
	http.ResponseWriter
	headerSnapshot http.Header
	body           bytes.Buffer
	status         int
	wroteHeader    bool
	captured       bool
}

func (w *notFoundWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	if statusCode >= 100 && statusCode <= 199 && statusCode != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(statusCode)
		return
	}
	w.wroteHeader = true
	w.status = statusCode
	if statusCode == http.StatusNotFound {
		w.captured = true
		return
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *notFoundWriter) Write(body []byte) (int, error) {
	if w.captured {
		return w.body.Write(body)
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *notFoundWriter) Flush() {
	if w.captured {
		return
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *notFoundWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if w.captured {
		return nil, nil, http.ErrNotSupported
	}
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *notFoundWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *notFoundWriter) ReadFrom(src io.Reader) (int64, error) {
	if w.captured {
		return io.Copy(onlyWriter{w}, src)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(src)
		if n > 0 {
			w.wroteHeader = true
			w.status = http.StatusOK
		}
		return n, err
	}
	return io.Copy(onlyWriter{w}, src)
}

func (w *notFoundWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *notFoundWriter) restoreHeaders() {
	header := w.ResponseWriter.Header()
	clear(header)
	for key, values := range w.headerSnapshot {
		header[key] = values
	}
}

type onlyWriter struct {
	io.Writer
}

var (
	_ http.Flusher  = (*notFoundWriter)(nil)
	_ http.Hijacker = (*notFoundWriter)(nil)
	_ http.Pusher   = (*notFoundWriter)(nil)
	_ io.ReaderFrom = (*notFoundWriter)(nil)
)
