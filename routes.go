package castor

import (
	"net/http"
	"slices"
	"strings"
)

// Middleware wraps an http.Handler.
type Middleware = func(http.Handler) http.Handler

// Route is a composable piece of the route tree.
type Route func(mux *http.ServeMux, prefix string, middlewares []Middleware)

type RouteGroup struct {
	Prefix string
	Routes []Route
	mws    []Middleware
}

// Middlewares adds middleware to this node and everything below it.
func (n Route) Middlewares(middlewares ...Middleware) Route {
	return func(mux *http.ServeMux, prefix string, mws []Middleware) {
		n(mux, prefix, appendAll(mws, middlewares))
	}
}

func (rg RouteGroup) Middlewares(middlewares ...Middleware) RouteGroup {
	rg.mws = append(rg.mws, middlewares...)
	return rg
}

// Group mounts nodes under a shared prefix with an optional middleware chain.
func Group(prefix string, routes ...Route) RouteGroup {
	return RouteGroup{
		Prefix: prefix,
		Routes: routes,
	}
}

// Get registers a GET route.
func Get(path string, handler http.HandlerFunc) Route {
	return route(http.MethodGet, path, handler)
}

// Query is an alias for Get.
func Query(pattern string, handler http.HandlerFunc) Route {
	return Get(pattern, handler)
}

// Post registers a POST route.
func Post(pattern string, handler http.HandlerFunc) Route {
	return route(http.MethodPost, pattern, handler)
}

// Put registers a PUT route.
func Put(pattern string, handler http.HandlerFunc) Route {
	return route(http.MethodPut, pattern, handler)
}

// Patch registers a PATCH route.
func Patch(pattern string, handler http.HandlerFunc) Route {
	return route(http.MethodPatch, pattern, handler)
}

// Delete registers a DELETE route.
func Delete(pattern string, handler http.HandlerFunc) Route {
	return route(http.MethodDelete, pattern, handler)
}

// Options registers an OPTIONS route.
func Options(pattern string, handler http.HandlerFunc) Route {
	return route(http.MethodOptions, pattern, handler)
}

func route(method, pattern string, handler http.Handler) Route {
	return func(mux *http.ServeMux, prefix string, middlewares []Middleware) {
		mux.Handle(method+" "+endpointPattern(joinPath(prefix, pattern)), chain(handler, middlewares))
	}
}

func chain(handler http.Handler, middlewares []Middleware) http.Handler {
	for _, middlew := range slices.Backward(middlewares) {
		handler = middlew(handler)
	}
	return handler
}

func joinPath(prefix, path string) string {
	var joined string
	switch {
	case path == "" || path == "/":
		joined = prefix
		if joined == "" {
			return "/"
		}
	case prefix == "":
		joined = path
	default:
		joined = strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
	}
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	return joined
}

// endpointPattern keeps trailing-slash routes exact instead of treating them
// as ServeMux subtree patterns.
func endpointPattern(path string) string {
	if strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/") + "/{$}"
	}
	return path
}

func appendAll(base, extra []Middleware) []Middleware {
	all := make([]Middleware, 0, len(base)+len(extra))
	all = append(all, base...)
	return append(all, extra...)
}
