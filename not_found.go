package castor

import (
	"bufio"
	"bytes"
	"io"
	"maps"
	"net"
	"net/http"
)

// NotFoundHandler handles a 404 response. message is the original response
// body produced by the route or by ServeMux for an unmatched route. The handler
// is responsible for writing the response status and body.
type NotFoundHandler func(res http.ResponseWriter, message string)

type onlyWriter struct {
	io.Writer
}

var (
	_ http.Flusher  = (*notFoundWriter)(nil)
	_ http.Hijacker = (*notFoundWriter)(nil)
	_ http.Pusher   = (*notFoundWriter)(nil)
	_ io.ReaderFrom = (*notFoundWriter)(nil)
)

func defaultNotFound(res http.ResponseWriter, message string) {
	if message == "" {
		message = http.StatusText(http.StatusNotFound)
	}
	http.Error(res, message, http.StatusNotFound)
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
		handler(res, writer.body.String())
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
	maps.Copy(header, w.headerSnapshot)
}
