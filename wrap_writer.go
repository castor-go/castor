package castor

import (
	"bufio"
	"io"
	"net"
	"net/http"
)

// WrapResponseWriter is an http.ResponseWriter proxy that records the status
// and body size and can tee or discard the body.
type WrapResponseWriter interface {
	http.ResponseWriter
	// Status returns the response status, or 0 before the header is written.
	Status() int
	// BytesWritten returns the number of bytes written to the response.
	BytesWritten() int
	// Tee copies the response body to tee in addition to the client.
	Tee(tee io.Writer)
	// Unwrap returns the original ResponseWriter.
	Unwrap() http.ResponseWriter
	// Discard stops writes from reaching the original ResponseWriter.
	// Tee'd writes, if any, are still recorded.
	Discard()
}

// NewWrapResponseWriter wraps w. protoMajor is accepted for API compatibility;
// the wrapper adapts to the underlying writer at call time.
func NewWrapResponseWriter(res http.ResponseWriter, protoMajor int) WrapResponseWriter {
	return &wrapWriter{ResponseWriter: res}
}

type wrapWriter struct {
	http.ResponseWriter
	tee         io.Writer
	code        int
	bytes       int
	wroteHeader bool
	discard     bool
}

func (w *wrapWriter) WriteHeader(code int) {
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		if !w.discard {
			w.ResponseWriter.WriteHeader(code)
		}
		return
	}
	if w.wroteHeader {
		return
	}
	w.code = code
	w.wroteHeader = true
	if !w.discard {
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *wrapWriter) Write(body []byte) (int, error) {
	w.maybeWriteHeader()

	if w.discard {
		if w.tee == nil {
			n, err := io.Discard.Write(body)
			w.bytes += n
			return n, err
		}
		n, err := w.tee.Write(body)
		w.bytes += n
		return n, err
	}

	n, err := w.ResponseWriter.Write(body)
	if w.tee != nil {
		if _, teeErr := w.tee.Write(body[:n]); err == nil {
			err = teeErr
		}
	}
	w.bytes += n
	return n, err
}

func (w *wrapWriter) maybeWriteHeader() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
}

func (w *wrapWriter) Flush() {
	w.maybeWriteHeader()
	if !w.discard {
		_ = http.NewResponseController(w.ResponseWriter).Flush()
	}
}

func (w *wrapWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *wrapWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *wrapWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.tee != nil || w.discard {
		return io.Copy(writerOnly{w}, reader)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := readerFrom.ReadFrom(reader)
		if n > 0 && !w.wroteHeader {
			w.code = http.StatusOK
			w.wroteHeader = true
		}
		w.bytes += int(n)
		return n, err
	}
	return io.Copy(writerOnly{w}, reader)
}

func (w *wrapWriter) Status() int {
	return w.code
}

func (w *wrapWriter) BytesWritten() int {
	return w.bytes
}

func (w *wrapWriter) Tee(tee io.Writer) {
	w.tee = tee
}

func (w *wrapWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *wrapWriter) Discard() {
	w.discard = true
}

type writerOnly struct {
	io.Writer
}

var (
	_ WrapResponseWriter = (*wrapWriter)(nil)
	_ http.Flusher       = (*wrapWriter)(nil)
	_ http.Hijacker      = (*wrapWriter)(nil)
	_ http.Pusher        = (*wrapWriter)(nil)
	_ io.ReaderFrom      = (*wrapWriter)(nil)
)
