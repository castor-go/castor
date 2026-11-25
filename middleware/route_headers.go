package middleware

import (
	"maps"
	"net/http"
	"slices"
	"strings"
)

// RouteHeaders directs requests through middleware based on header values.
type HeaderRouter map[string][]HeaderRoute

// RouteHeaders creates an empty header router.
func RouteHeaders() HeaderRouter {
	return HeaderRouter{}
}

// Route adds a middleware for an exact or "*"-wildcard header value.
func (hr HeaderRouter) Route(header, match string, middleware func(next http.Handler) http.Handler) HeaderRouter {
	return hr.add(strings.ToLower(header), HeaderRoute{
		MatchOne:   NewPattern(match),
		Middleware: middleware,
	})
}

// RouteAny adds a middleware matching any of the given header values.
func (hr HeaderRouter) RouteAny(header string, matches []string, middleware func(next http.Handler) http.Handler) HeaderRouter {
	patterns := make([]Pattern, len(matches))
	for i, match := range matches {
		patterns[i] = NewPattern(match)
	}
	return hr.add(strings.ToLower(header), HeaderRoute{
		MatchAny:   patterns,
		Middleware: middleware,
	})
}

// RouteDefault adds the middleware used when no header route matches.
func (hr HeaderRouter) RouteDefault(middleware func(next http.Handler) http.Handler) HeaderRouter {
	if hr == nil {
		hr = HeaderRouter{}
	}
	hr["*"] = []HeaderRoute{{Middleware: middleware}}
	return hr
}

func (hr HeaderRouter) add(header string, route HeaderRoute) HeaderRouter {
	if hr == nil {
		hr = HeaderRouter{}
	}
	hr[header] = append(hr[header], route)
	return hr
}

// Handler builds an http.Handler that dispatches to the first matching route.
func (hr HeaderRouter) Handler(next http.Handler) http.Handler {
	if len(hr) == 0 {
		return next
	}

	type compiledRoute struct {
		header  string
		matcher HeaderRoute
		handler http.Handler
	}

	compiled := make([]compiledRoute, 0, len(hr))
	for _, header := range slices.Sorted(maps.Keys(hr)) {
		if header == "*" {
			continue
		}
		for _, matcher := range hr[header] {
			handler := next
			if matcher.Middleware != nil {
				handler = matcher.Middleware(next)
			}
			compiled = append(compiled, compiledRoute{header, matcher, handler})
		}
	}

	var defaultHandler http.Handler
	if defaults := hr["*"]; len(defaults) > 0 && defaults[0].Middleware != nil {
		defaultHandler = defaults[0].Middleware(next)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, route := range compiled {
			value := r.Header.Get(route.header)
			if value == "" && route.header == "host" {
				value = r.Host
			}
			if value != "" && route.matcher.IsMatch(strings.ToLower(value)) {
				route.handler.ServeHTTP(w, r)
				return
			}
		}
		if defaultHandler != nil {
			defaultHandler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// HeaderRoute is a header matcher and its middleware.
type HeaderRoute struct {
	Middleware func(next http.Handler) http.Handler
	MatchOne   Pattern
	MatchAny   []Pattern
}

// IsMatch reports whether value satisfies the route's patterns.
func (r HeaderRoute) IsMatch(value string) bool {
	if len(r.MatchAny) > 0 {
		for _, matcher := range r.MatchAny {
			if matcher.Match(value) {
				return true
			}
		}
		return false
	}
	return r.MatchOne.Match(value)
}

// Pattern matches an exact header value or a value with one "*" wildcard.
type Pattern struct {
	prefix   string
	suffix   string
	minLen   int
	wildcard bool
}

// NewPattern parses an exact or wildcard header pattern.
func NewPattern(value string) Pattern {
	prefix, suffix, wildcard := strings.Cut(value, "*")
	return Pattern{
		prefix:   prefix,
		suffix:   suffix,
		minLen:   len(prefix) + len(suffix),
		wildcard: wildcard,
	}
}

// Match reports whether value matches the pattern.
func (p Pattern) Match(value string) bool {
	if !p.wildcard {
		return p.prefix == value
	}
	return len(value) >= p.minLen &&
		strings.HasPrefix(value, p.prefix) &&
		strings.HasSuffix(value, p.suffix)
}
