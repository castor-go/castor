package castor

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type LogFormat int

const (
	JsonFormat = iota + 1
	TextFormat
)

type (
	// LoggerInterface is satisfied by the standard library logger.
	LoggerInterface interface {
		Info(msg string, args ...any)
		Warn(msg string, args ...any)
		Error(msg string, args ...any)
	}

	// DefaultLogFormatter is the default text log formatter.
	DefaultLogFormatter struct {
		// Logger receives formatted entries. log.Default is used when nil.
		Logger LoggerInterface
		Format LogFormat
	}

	// LogFormatter creates a LogEntry for each request.
	LogFormatter interface {
		NewLogEntry(req *http.Request) LogEntry
	}

	// LogEntry records the result of a request.
	LogEntry interface {
		LoggerInterface
		// Write is called after the request handler completes.
		Write(status, bytes int, header http.Header, elapsed time.Duration, extra any)
		// Panic is called by Recoverer when the handler panics.
		Panic(v any, stack []byte)
	}

	RequestLogEntry struct {
		Scheme    string
		Method    string
		Host      string
		URI       string
		Protocol  string
		RequestID string
	}

	ResponseLogEntry struct {
		Status      int
		ContentType string
		Timer       time.Duration
	}
)

// LogEntryCtxKey is the context key that holds the request LogEntry.
var LogEntryCtxKey = &ContextKey{"LogEntry"}

// DefaultLogger is used by Logger and can be replaced for custom logging.
var DefaultLogger func(next http.Handler) http.Handler

// Logger logs the start and end of each request.
//
// Place Logger before Recoverer so recovered panics are logged with the
// request entry.
func Logger(next http.Handler) http.Handler {
	return DefaultLogger(next)
}

// RequestLogger returns a logger middleware using formatter.
func RequestLogger(formatter LogFormatter) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			entry := formatter.NewLogEntry(req)
			reqEntry := makeRequestLog(req)
			wrapped := NewWrapResponseWriter(res, req.ProtoMajor)
			start := time.Now()
			defer func() {
				respEntry := ResponseLogEntry{
					Status:      wrapped.Status(),
					ContentType: wrapped.Header().Get("Content-Type"),
					Timer:       time.Since(start),
				}
				logRequestLog(entry, reqEntry, respEntry)
			}()
			next.ServeHTTP(wrapped, WithLogEntry(req, entry))
		})
	}
}

func makeRequestLog(req *http.Request) RequestLogEntry {
	// clientIP := GetClientIP(r.Context())
	return RequestLogEntry{
		Scheme:    req.URL.Scheme,
		Method:    req.Method,
		Host:      req.Host,
		Protocol:  req.Proto,
		URI:       req.RequestURI,
		RequestID: GetReqID(req.Context()),
	}
}

func logRequestLog(entry LogEntry, reqEntry RequestLogEntry, resEntry ResponseLogEntry) {
	entries := make([]any, 0, 7)
	if reqEntry.RequestID != "" {
		entries = append(entries, slog.String("requestId", reqEntry.RequestID))
	}
	entries = append(entries, slog.String("host", reqEntry.Host))
	entries = append(entries, slog.String("method", reqEntry.Method))
	entries = append(entries, slog.String("path", reqEntry.URI))
	entries = append(entries, slog.Int("status", resEntry.Status))
	if resEntry.ContentType != "" {
		entries = append(entries, slog.String("mime-type", resEntry.ContentType))
	}
	entries = append(entries, slog.Duration("time", resEntry.Timer))

	entry.Info("Request log", entries...)
}

// WithLogEntry stores entry in the request context.
func WithLogEntry(r *http.Request, entry LogEntry) *http.Request {
	ctx := context.WithValue(r.Context(), LogEntryCtxKey, entry)
	return r.WithContext(ctx)
}

// NewLogEntry starts formatting a request log line.
func (f *DefaultLogFormatter) NewLogEntry(req *http.Request) LogEntry {
	entry := &defaultLogEntry{
		DefaultLogFormatter: f,
		buf:                 &bytes.Buffer{},
	}

	return entry
}

type defaultLogEntry struct {
	*DefaultLogFormatter
	buf *bytes.Buffer
}

func (e *defaultLogEntry) Info(msg string, params ...any) {
	e.Logger.Info(msg, params...)
}

func (e *defaultLogEntry) Warn(msg string, params ...any) {
	e.Logger.Warn(msg, params...)
}

func (e *defaultLogEntry) Error(msg string, params ...any) {
	e.Logger.Error(msg, params...)
}

func (entry *defaultLogEntry) Write(status, bytes int, _ http.Header, elapsed time.Duration, _ any) {
	if entry.Logger != nil {
		slog.Info(entry.buf.String(), "status", status, "time", elapsed)
		return
	}
	Write(entry.buf, "status=%d", status)
	Write(entry.buf, " %v ", bytes)
	Write(entry.buf, "time=%s", elapsed)
	entry.Logger.Info(entry.buf.String())
}

func (entry *defaultLogEntry) Panic(v any, stack []byte) {
	// printPrettyStackBytes(v, stack)
}

func init() {
	logFormatter := &DefaultLogFormatter{
		Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
	}
	if !IsTTY {
		logFormatter = &DefaultLogFormatter{
			Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
		}
	}
	DefaultLogger = RequestLogger(logFormatter)
}

// GetReqID returns the request ID stored in ctx.
func GetReqID(ctx context.Context) string {
	return GetValue[string](ctx, RequestIDKey)
}
