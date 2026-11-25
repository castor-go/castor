package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"unicode"
)

// CORSOpts configures the CORS middleware.
type CORSOpts struct {
	// AllowedOrigins lists the origins allowed to make cross-origin requests.
	// An empty list allows every origin when AllowOriginFunc is not set. The
	// special value "*" allows every origin and makes responses use "*" as the
	// allowed origin. An origin may contain one "*" wildcard, for example
	// "https://*.example.com". Matching is case-insensitive.
	AllowedOrigins []string

	// AllowOriginFunc validates an origin. When set, it takes precedence over
	// AllowedOrigins for validating requests.
	AllowOriginFunc func(r *http.Request, origin string) bool

	// AllowedMethods lists the methods allowed for cross-origin requests.
	// Defaults to GET, POST and HEAD.
	AllowedMethods []string

	// AllowedHeaders lists the non-simple request headers allowed for
	// cross-origin requests. The special value "*" allows every header.
	// Origin is always allowed. Defaults to Origin, Accept and Content-Type.
	AllowedHeaders []string

	// ExposedHeaders lists the response headers exposed to the browser.
	ExposedHeaders []string

	// AllowCredentials allows cookies and HTTP authentication credentials.
	// Browsers reject credentials combined with the "*" allowed origin, so
	// configure explicit origins when this option is enabled.
	AllowCredentials bool

	// MaxAge is how long, in seconds, a preflight response may be cached.
	MaxAge int

	// OptionsPassthrough calls the next handler for preflight requests instead
	// of responding directly. Enable it when the application handles OPTIONS.
	OptionsPassthrough bool
}

// CORS returns middleware that handles cross-origin resource sharing.
// Preflight requests are answered without invoking the next handler unless
// OptionsPassthrough is set. Register CORS globally, before the router, so
// preflights do not need a matching OPTIONS route.
func CORS(opts CORSOpts) func(next http.Handler) http.Handler {
	c := newCORS(opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			_, hasOrigin := req.Header["Origin"]
			preflight := req.Method == http.MethodOptions &&
				hasOrigin &&
				req.Header.Get("Access-Control-Request-Method") != ""

			if preflight {
				c.handlePreflight(res, req)
				if !c.passthrough {
					res.WriteHeader(http.StatusOK)
					return
				}
			} else {
				c.handleActual(res, req)
			}
			next.ServeHTTP(res, req)
		})
	}
}

// CORSAllowAll returns permissive CORS middleware for all origins, methods and
// headers without credentials.
func CORSAllowAll() func(next http.Handler) http.Handler {
	return CORS(CORSOpts{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders: []string{"*"},
	})
}

type corsConfig struct {
	allowOriginFunc  func(req *http.Request, origin string) bool
	origins          set[string]
	wildcardOrigins  []originWildcard
	methods          set[string]
	headers          set[string]
	exposedHeaders   []string
	allOrigins       bool
	allHeaders       bool
	allowCredentials bool
	maxAge           int
	passthrough      bool
}

func newCORS(opts CORSOpts) *corsConfig {
	c := &corsConfig{
		allowOriginFunc:  opts.AllowOriginFunc,
		allowCredentials: opts.AllowCredentials,
		maxAge:           opts.MaxAge,
		passthrough:      opts.OptionsPassthrough,
	}

	if len(opts.AllowedOrigins) == 0 {
		c.allOrigins = opts.AllowOriginFunc == nil
	} else {
		c.origins = make(set[string], len(opts.AllowedOrigins))
		for _, origin := range opts.AllowedOrigins {
			origin = strings.ToLower(origin)
			if origin == "*" {
				c.allOrigins = true
				c.origins = nil
				c.wildcardOrigins = nil
				break
			}
			if prefix, suffix, wildcard := strings.Cut(origin, "*"); wildcard {
				c.wildcardOrigins = append(c.wildcardOrigins, originWildcard{prefix: prefix, suffix: suffix})
			} else {
				c.origins.add(origin)
			}
		}
	}

	if len(opts.AllowedMethods) == 0 {
		c.methods = newSet(http.MethodGet, http.MethodPost, http.MethodHead)
	} else {
		c.methods = make(set[string], len(opts.AllowedMethods))
		for _, method := range opts.AllowedMethods {
			if method = strings.ToUpper(method); method != "" {
				c.methods.add(method)
			}
		}
	}

	if len(opts.AllowedHeaders) == 0 {
		c.headers = newSet("Origin", "Accept", "Content-Type")
	} else {
		c.headers = make(set[string], len(opts.AllowedHeaders))
		for _, header := range opts.AllowedHeaders {
			if header == "*" {
				c.allHeaders = true
				c.headers = nil
				break
			}
			c.headers.add(http.CanonicalHeaderKey(header))
		}
		if !c.allHeaders {
			c.headers.add("Origin")
		}
	}

	c.exposedHeaders = make([]string, len(opts.ExposedHeaders))
	for i, header := range opts.ExposedHeaders {
		c.exposedHeaders[i] = http.CanonicalHeaderKey(header)
	}

	return c
}

func (c *corsConfig) handlePreflight(res http.ResponseWriter, req *http.Request) {
	header := res.Header()
	header.Add("Vary", "Origin")
	header.Add("Vary", "Access-Control-Request-Method")
	header.Add("Vary", "Access-Control-Request-Headers")

	origin := req.Header.Get("Origin")
	if !c.originAllowed(req, origin) {
		return
	}

	requestedMethod := strings.ToUpper(strings.TrimSpace(req.Header.Get("Access-Control-Request-Method")))
	if !c.methodAllowed(requestedMethod) {
		return
	}

	requestedHeaders := parseHeaderList(req.Header.Values("Access-Control-Request-Headers"))
	if !c.headersAllowed(requestedHeaders) {
		return
	}

	if c.allOrigins {
		header.Set("Access-Control-Allow-Origin", "*")
	} else {
		header.Set("Access-Control-Allow-Origin", origin)
	}
	header.Set("Access-Control-Allow-Methods", requestedMethod)
	if len(requestedHeaders) > 0 {
		header.Set("Access-Control-Allow-Headers", strings.Join(requestedHeaders, ", "))
	}
	if c.allowCredentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
	if c.maxAge > 0 {
		header.Set("Access-Control-Max-Age", strconv.Itoa(c.maxAge))
	}
}

func (c *corsConfig) handleActual(res http.ResponseWriter, req *http.Request) {
	header := res.Header()
	header.Add("Vary", "Origin")

	origin := req.Header.Get("Origin")
	if _, hasOrigin := req.Header["Origin"]; !hasOrigin {
		return
	}
	if !c.originAllowed(req, origin) || !c.methodAllowed(req.Method) {
		return
	}

	if c.allOrigins {
		header.Set("Access-Control-Allow-Origin", "*")
	} else {
		header.Set("Access-Control-Allow-Origin", origin)
	}
	if len(c.exposedHeaders) > 0 {
		header.Set("Access-Control-Expose-Headers", strings.Join(c.exposedHeaders, ", "))
	}
	if c.allowCredentials {
		header.Set("Access-Control-Allow-Credentials", "true")
	}
}

func (c *corsConfig) originAllowed(req *http.Request, origin string) bool {
	if c.allowOriginFunc != nil {
		return c.allowOriginFunc(req, origin)
	}
	if c.allOrigins {
		return true
	}

	origin = strings.ToLower(origin)
	if c.origins.has(origin) {
		return true
	}
	for _, wildcard := range c.wildcardOrigins {
		if wildcard.match(origin) {
			return true
		}
	}
	return false
}

func (c *corsConfig) methodAllowed(method string) bool {
	method = strings.ToUpper(method)
	return method == http.MethodOptions || c.methods.has(method)
}

func (c *corsConfig) headersAllowed(headers []string) bool {
	if c.allHeaders {
		return true
	}
	for _, header := range headers {
		if !c.headers.has(header) {
			return false
		}
	}
	return true
}

// parseHeaderList parses one or more comma- or whitespace-separated header
// lists and canonicalizes each header name.
func parseHeaderList(values []string) []string {
	var headers []string
	for _, value := range values {
		for header := range strings.FieldsFuncSeq(value, isHeaderListSeparator) {
			headers = append(headers, http.CanonicalHeaderKey(header))
		}
	}
	return headers
}

func isHeaderListSeparator(r rune) bool {
	return r == ',' || unicode.IsSpace(r)
}

// originWildcard matches an origin with a single "*" wildcard.
type originWildcard struct {
	prefix string
	suffix string
}

func (w originWildcard) match(origin string) bool {
	return len(origin) >= len(w.prefix)+len(w.suffix) &&
		strings.HasPrefix(origin, w.prefix) &&
		strings.HasSuffix(origin, w.suffix)
}

// set is a lookup set used for allowed origins, methods and headers.
type set[T comparable] map[T]struct{}

func newSet[T comparable](values ...T) set[T] {
	s := make(set[T], len(values))
	s.add(values...)
	return s
}

func (s set[T]) add(values ...T) {
	for _, value := range values {
		s[value] = struct{}{}
	}
}

func (s set[T]) has(value T) bool {
	_, ok := s[value]
	return ok
}
