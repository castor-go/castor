package middleware

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var defaultCompressibleContentTypes = [...]string{
	"text/html",
	"text/css",
	"text/plain",
	"text/javascript",
	"text/markdown",
	"text/csv",
	"text/vtt",
	"application/javascript",
	"application/x-javascript",
	"application/json",
	"application/atom+xml",
	"application/rss+xml",
	"application/xml",
	"text/xml",
	"image/svg+xml",
}

// Compress compresses responses of the given content types based on
// Accept-Encoding. Level is a compress/flate level.
func Compress(level int, types ...string) func(next http.Handler) http.Handler {
	return NewCompressor(level, types...).Handler
}

// Compressor holds compression encoders and allowed content types.
type Compressor struct {
	encoders           map[string]EncoderFunc
	pooledEncoders     map[string]*sync.Pool
	allowedTypes       map[string]struct{}
	allowedWildcards   map[string]struct{}
	encodingPrecedence []string
	level              int
}

// NewCompressor creates a Compressor. If no content types are given, a
// sensible text/json default set is used.
func NewCompressor(level int, types ...string) *Compressor {
	if level < flate.HuffmanOnly || level > flate.BestCompression {
		panic("middleware: invalid compression level")
	}

	compressor := &Compressor{
		level:            level,
		encoders:         make(map[string]EncoderFunc),
		pooledEncoders:   make(map[string]*sync.Pool),
		allowedTypes:     make(map[string]struct{}),
		allowedWildcards: make(map[string]struct{}),
	}

	if len(types) == 0 {
		for _, contentType := range defaultCompressibleContentTypes {
			compressor.allowedTypes[contentType] = struct{}{}
		}
	} else {
		for _, contentType := range types {
			contentType = strings.ToLower(strings.TrimSpace(contentType))
			if !strings.Contains(strings.TrimSuffix(contentType, "/*"), "*") {
				if prefix, ok := strings.CutSuffix(contentType, "/*"); ok {
					if prefix == "" {
						panic(fmt.Sprintf("middleware/compress: Unsupported content-type wildcard pattern '%s'. Only '<type>/*' supported", contentType))
					}
					compressor.allowedWildcards[prefix] = struct{}{}
					continue
				}
				compressor.allowedTypes[contentType] = struct{}{}
				continue
			}
			panic(fmt.Sprintf("middleware/compress: Unsupported content-type wildcard pattern '%s'. Only '<type>/*' supported", contentType))
		}
	}

	compressor.SetEncoder("deflate", encoderDeflate)
	compressor.SetEncoder("gzip", encoderGzip)
	return compressor
}

// SetEncoder registers a compression algorithm. Custom encoders can be used
// for formats such as Brotli, but must be implemented without extra
// dependencies.
func (c *Compressor) SetEncoder(encoding string, encoder EncoderFunc) {
	encoding = strings.ToLower(strings.TrimSpace(encoding))
	if encoding == "" {
		panic("middleware: encoding can not be empty")
	}
	if encoder == nil {
		panic("middleware: encoder must not be nil")
	}

	delete(c.pooledEncoders, encoding)
	delete(c.encoders, encoding)

	sample := encoder(io.Discard, c.level)
	if sample == nil {
		panic("middleware: encoder returned a nil writer")
	}
	if _, ok := sample.(ioResetterWriter); ok {
		c.pooledEncoders[encoding] = &sync.Pool{
			New: func() any { return encoder(io.Discard, c.level) },
		}
	} else {
		c.encoders[encoding] = encoder
	}

	for i, registered := range c.encodingPrecedence {
		if registered == encoding {
			c.encodingPrecedence = append(c.encodingPrecedence[:i], c.encodingPrecedence[i+1:]...)
			break
		}
	}
	c.encodingPrecedence = append([]string{encoding}, c.encodingPrecedence...)
}

// Handler compresses responses according to the current Compressor.
func (c *Compressor) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoder, encoding, cleanup := c.selectEncoder(r.Header, w)
		if encoding == "" {
			w.Header().Add("Vary", "Accept-Encoding")
			next.ServeHTTP(w, r)
			return
		}

		wrapped := &compressResponseWriter{
			ResponseWriter:   w,
			w:                w,
			contentTypes:     c.allowedTypes,
			contentWildcards: c.allowedWildcards,
			encoding:         encoding,
		}
		if encoder != nil {
			wrapped.w = encoder
		}
		defer cleanup()
		defer wrapped.Close()
		next.ServeHTTP(wrapped, r)
	})
}

func (c *Compressor) selectEncoder(header http.Header, w io.Writer) (io.Writer, string, func()) {
	accepted := strings.Split(strings.Join(header.Values("Accept-Encoding"), ","), ",")

	for _, name := range c.encodingPrecedence {
		if !matchAcceptEncoding(accepted, name) {
			continue
		}
		if pool, ok := c.pooledEncoders[name]; ok {
			encoder := pool.Get().(ioResetterWriter)
			encoder.Reset(w)
			return encoder, name, func() { pool.Put(encoder) }
		}
		if encoder, ok := c.encoders[name]; ok {
			if writer := encoder(w, c.level); writer != nil {
				return writer, name, func() {}
			}
		}
	}
	return nil, "", func() {}
}

func matchAcceptEncoding(accepted []string, encoding string) bool {
	wildcard := false
	for _, value := range accepted {
		name, params, _ := strings.Cut(strings.TrimSpace(value), ";")
		quality := encodingQuality(params)
		switch name {
		case encoding:
			return quality > 0
		case "*":
			wildcard = wildcard || quality > 0
		}
	}
	return wildcard
}

func encodingQuality(params string) float64 {
	quality := 1.0
	for param := range strings.SplitSeq(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(key, "q") {
			continue
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			quality = parsed
		}
	}
	return quality
}

// EncoderFunc wraps w with a streaming compressor.
type EncoderFunc func(w io.Writer, level int) io.Writer

type ioResetterWriter interface {
	io.Writer
	Reset(w io.Writer)
}

type compressResponseWriter struct {
	http.ResponseWriter

	w                io.Writer
	contentTypes     map[string]struct{}
	contentWildcards map[string]struct{}
	encoding         string
	wroteHeader      bool
	compressible     bool
}

func (cw *compressResponseWriter) isCompressible() bool {
	contentType := strings.ToLower(cw.Header().Get("Content-Type"))
	contentType, _, _ = strings.Cut(contentType, ";")
	contentType = strings.TrimSpace(contentType)

	if _, ok := cw.contentTypes[contentType]; ok {
		return true
	}
	if prefix, _, ok := strings.Cut(contentType, "/"); ok {
		_, exists := cw.contentWildcards[prefix]
		return exists
	}
	return false
}

func (cw *compressResponseWriter) WriteHeader(code int) {
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		cw.ResponseWriter.WriteHeader(code)
		return
	}
	if cw.wroteHeader {
		return
	}
	cw.wroteHeader = true
	defer cw.ResponseWriter.WriteHeader(code)

	if code == http.StatusNoContent || code == http.StatusNotModified {
		return
	}
	if cw.Header().Get("Content-Encoding") != "" || !cw.isCompressible() || cw.encoding == "" {
		return
	}

	cw.compressible = true
	cw.Header().Set("Content-Encoding", cw.encoding)
	cw.Header().Add("Vary", "Accept-Encoding")
	cw.Header().Del("Content-Length")
}

func (cw *compressResponseWriter) Write(body []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	return cw.writer().Write(body)
}

func (cw *compressResponseWriter) writer() io.Writer {
	if cw.compressible {
		return cw.w
	}
	return cw.ResponseWriter
}

type compressFlusher interface {
	Flush() error
}

func (cw *compressResponseWriter) Flush() {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}

	if flusher, ok := cw.writer().(http.Flusher); ok {
		flusher.Flush()
	}
	if flusher, ok := cw.writer().(compressFlusher); ok {
		_ = flusher.Flush()
		if underlying, ok := cw.ResponseWriter.(http.Flusher); ok {
			underlying.Flush()
		}
	}
}

func (cw *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := cw.writer().(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("middleware: http.Hijacker is unavailable")
}

func (cw *compressResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := cw.writer().(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return errors.New("middleware: http.Pusher is unavailable")
}

func (cw *compressResponseWriter) Close() error {
	if closer, ok := cw.writer().(io.WriteCloser); ok {
		return closer.Close()
	}
	return nil
}

func (cw *compressResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}

func encoderGzip(w io.Writer, level int) io.Writer {
	writer, err := gzip.NewWriterLevel(w, level)
	if err != nil {
		return nil
	}
	return writer
}

func encoderDeflate(w io.Writer, level int) io.Writer {
	writer, err := flate.NewWriter(w, level)
	if err != nil {
		return nil
	}
	return writer
}
