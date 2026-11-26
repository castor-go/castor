package middleware

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

const poweredByHeader = "X-Powered-By"

var defaultSecureHeaders = [...]struct {
	name  string
	value string
}{
	{"Cross-Origin-Opener-Policy", "same-origin"},
	{"Cross-Origin-Resource-Policy", "same-origin"},
	{"Origin-Agent-Cluster", "?1"},
	{"Referrer-Policy", "no-referrer"},
	{"Strict-Transport-Security", "max-age=15552000; includeSubDomains"},
	{"X-Content-Type-Options", "nosniff"},
	{"X-DNS-Prefetch-Control", "off"},
	{"X-Download-Options", "noopen"},
	{"X-Frame-Options", "SAMEORIGIN"},
	{"X-Permitted-Cross-Domain-Policies", "none"},
	{"X-XSS-Protection", "0"},
}

// SecureHeaders adds the default set of security headers.
func SecureHeaders(next http.Handler) http.Handler {
	return SecureHeadersWithOpts(SecureHeadersOpts{})(next)
}

// SecureHeadersOpts configures SecureHeadersWithOpts. The zero value uses the defaults.
type SecureHeadersOpts struct {
	// Headers adds or overrides response headers.
	Headers http.Header
	// DisableHeaders omits default headers by name.
	DisableHeaders []string
	// ContentSecurityPolicy sets the Content-Security-Policy header.
	ContentSecurityPolicy CSPDirectives
	// ContentSecurityPolicyReportOnly sets the report-only CSP header.
	ContentSecurityPolicyReportOnly CSPDirectives
	// PermissionsPolicy sets the Permissions-Policy header.
	PermissionsPolicy PermissionsDirectives
	// ReportingEndpoints sets the Reporting-Endpoints header.
	ReportingEndpoints []ReportingEndpoint
	// ReportTo sets the Report-To header.
	ReportTo []ReportTo
	// KeepHandlerHeaders preserves headers already set by the handler.
	KeepHandlerHeaders bool
	// KeepPoweredBy leaves the X-Powered-By header untouched.
	KeepPoweredBy bool
}

// CSPDirectives maps Content-Security-Policy directive names to their sources.
// An empty value list emits a directive without a value. Directive names may
// be camelCase or kebab-case.
type CSPDirectives map[string][]string

// PermissionsDirectives maps Permissions-Policy directives to their allowlists.
// An empty list emits "()", "*" allows all, and "none" denies all. Directive
// names may be camelCase or kebab-case.
type PermissionsDirectives map[string][]string

// ReportingEndpoint is one named endpoint in a Reporting-Endpoints header.
type ReportingEndpoint struct {
	Name string
	URL  string
}

// ReportTo is one reporting group in a Report-To header.
type ReportTo struct {
	Group     string             `json:"group"`
	MaxAge    int                `json:"max_age"`
	Endpoints []ReportToEndpoint `json:"endpoints"`
}

// ReportToEndpoint is a URL inside a ReportTo group.
type ReportToEndpoint struct {
	URL string `json:"url"`
}

// SecureHeadersWithOpts returns middleware configured by opts.
func SecureHeadersWithOpts(opts SecureHeadersOpts) func(next http.Handler) http.Handler {
	headers := newSecureHeaders(opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			writer := &secureHeadersWriter{
				ResponseWriter:  res,
				headers:         headers,
				overwrite:       !opts.KeepHandlerHeaders,
				removePoweredBy: !opts.KeepPoweredBy,
			}

			defer func() {
				if !writer.wroteHeader {
					writer.writeHeaders()
				}
			}()

			next.ServeHTTP(writer, req)
		})
	}
}

func newSecureHeaders(opts SecureHeadersOpts) http.Header {
	headers := make(http.Header, len(defaultSecureHeaders))
	for _, header := range defaultSecureHeaders {
		headers.Set(header.name, header.value)
	}

	for _, name := range opts.DisableHeaders {
		headers.Del(name)
	}

	if value := buildCSP(opts.ContentSecurityPolicy); value != "" {
		headers.Set("Content-Security-Policy", value)
	}
	if value := buildCSP(opts.ContentSecurityPolicyReportOnly); value != "" {
		headers.Set("Content-Security-Policy-Report-Only", value)
	}
	if value := buildPermissionsPolicy(opts.PermissionsPolicy); value != "" {
		headers.Set("Permissions-Policy", value)
	}
	if value := buildReportingEndpoints(opts.ReportingEndpoints); value != "" {
		headers.Set("Reporting-Endpoints", value)
	}
	if value := buildReportTo(opts.ReportTo); value != "" {
		headers.Set("Report-To", value)
	}

	for name, values := range opts.Headers {
		name = http.CanonicalHeaderKey(name)
		if len(values) == 0 {
			headers.Del(name)
			continue
		}
		headers[name] = slices.Clone(values)
	}

	return headers
}

func buildCSP(policy CSPDirectives) string {
	if len(policy) == 0 {
		return ""
	}

	byName := normalizedDirectives(policy)
	names := sortedNames(byName)

	directives := make([]string, 0, len(names))
	for _, name := range names {
		values := nonEmpty(byName[name])
		if len(values) == 0 {
			directives = append(directives, name)
			continue
		}
		directives = append(directives, name+" "+strings.Join(values, " "))
	}
	return strings.Join(directives, "; ")
}

func buildPermissionsPolicy(policy PermissionsDirectives) string {
	if len(policy) == 0 {
		return ""
	}

	byName := normalizedDirectives(policy)
	names := sortedNames(byName)

	directives := make([]string, 0, len(names))
	for _, name := range names {
		values := nonEmpty(byName[name])
		switch {
		case len(values) == 0:
			directives = append(directives, name+"=()")
		case len(values) == 1 && values[0] == "*":
			directives = append(directives, name+"=*")
		case len(values) == 1 && values[0] == "none":
			directives = append(directives, name+"=()")
		default:
			allowlist := make([]string, len(values))
			for i, value := range values {
				if value == "self" || value == "src" {
					allowlist[i] = value
					continue
				}
				allowlist[i] = strconv.Quote(value)
			}
			directives = append(directives, name+"=("+strings.Join(allowlist, " ")+")")
		}
	}
	return strings.Join(directives, ", ")
}

func buildReportingEndpoints(endpoints []ReportingEndpoint) string {
	values := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Name == "" || endpoint.URL == "" {
			continue
		}
		values = append(values, endpoint.Name+"="+strconv.Quote(endpoint.URL))
	}
	return strings.Join(values, ", ")
}

func buildReportTo(groups []ReportTo) string {
	values := make([]string, 0, len(groups))
	for _, group := range groups {
		data, err := json.Marshal(group)
		if err != nil {
			continue
		}
		values = append(values, string(data))
	}
	return strings.Join(values, ", ")
}

func normalizedDirectives(directives map[string][]string) map[string][]string {
	normalized := make(map[string][]string, len(directives))
	for name, values := range directives {
		if name = kebabCase(name); name != "" {
			normalized[name] = values
		}
	}
	return normalized
}

func sortedNames(directives map[string][]string) []string {
	names := make([]string, 0, len(directives))
	for name := range directives {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func kebabCase(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) + 2)
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			if i > 0 && value[i-1] != '-' {
				builder.WriteByte('-')
			}
			builder.WriteByte(byte(r + ('a' - 'A')))
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

type secureHeadersWriter struct {
	http.ResponseWriter
	headers         http.Header
	overwrite       bool
	removePoweredBy bool
	wroteHeader     bool
}

func (w *secureHeadersWriter) WriteHeader(code int) {
	if code >= 100 && code <= 199 && code != http.StatusSwitchingProtocols {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.writeHeaders()
	w.ResponseWriter.WriteHeader(code)
}

func (w *secureHeadersWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *secureHeadersWriter) writeHeaders() {
	header := w.Header()
	if w.removePoweredBy {
		header.Del(poweredByHeader)
	}
	for name, values := range w.headers {
		if !w.overwrite {
			if _, exists := header[name]; exists {
				continue
			}
		}
		header[name] = slices.Clone(values)
	}
}

func (w *secureHeadersWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *secureHeadersWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return http.NewResponseController(w.ResponseWriter).Hijack()
}

func (w *secureHeadersWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *secureHeadersWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(src)
	}
	return io.Copy(w.ResponseWriter, src)
}

func (w *secureHeadersWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
