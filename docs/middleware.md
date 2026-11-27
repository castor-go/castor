# Middleware

The `middleware` package contains composable HTTP middleware for Castor and any `net/http` server. Every middleware is either a `func(http.Handler) http.Handler` or a function that returns one.

- Package: `github.com/castor-go/castor/middleware`
- Middleware signature: `func(http.Handler) http.Handler`
- Dependencies: standard library only

Core server documentation is in [castor.md](castor.md).

> These middlewares were heavily inspired/adapted from [go-chi](https://go-chi.io). Thank you so much.

## Contents

- [Applying middleware](#applying-middleware)
- [Composition helpers](#composition-helpers)
- [Authentication](#authentication)
- [CORS](#cors)
- [Request validation and limits](#request-validation-and-limits)
- [Request identity](#request-identity)
- [Path routing](#path-routing)
- [Response headers](#response-headers)
- [Compression and header routing](#compression-and-header-routing)
- [Resilience and throttling](#resilience-and-throttling)
- [Diagnostics](#diagnostics)

## Applying middleware

There are three places to apply middleware:

```go
server := castor.Server().
	// 1. Global: runs for every request, before routing.
	Middlewares(
		middleware.RequestID,
		middleware.Recoverer,
	).
	// 2. Group: runs for every route in the group.
	Groups(
		castor.Group("/admin", castor.Get("/", adminHandler)).
			Middlewares(middleware.BasicAuth("Admin", users)),
	).
	// 3. Route: runs only when the route matches.
	Routes(
		castor.Get("/reports", reportsHandler).
			Middlewares(middleware.Compress(5)),
	)
```

The first middleware in a list is the outermost and runs first. Global middleware wraps the router, so it also sees requests that do not match a route. Route middleware runs only after the router has selected that route.

Some middleware must run globally to work correctly: `CORS`, `Heartbeat`, `PageRoute`, `CleanPath`, `StripSlashes`, `RedirectSlashes`, `PathRewrite`, and `URLFormat`. Their sections call this out.

Snippets below omit imports. They assume:

```go
import (
	"net/http"

	"github.com/castor-go/castor"
	"github.com/castor-go/castor/middleware"
)
```

Snippets that use `compress/gzip`, `context`, `fmt`, `io`, `net/netip`, `strings`, or `time` assume the corresponding standard-library imports.

## Composition helpers

### New

```go
func New(h http.Handler) func(next http.Handler) http.Handler
```

`New` turns an existing handler into middleware that always serves `h` and ignores the next handler. It is useful for mounting a standalone handler in a middleware chain.

```go
health := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
	res.WriteHeader(http.StatusNoContent)
})

server := castor.Server().Middlewares(middleware.New(health))
```

### Maybe

```go
func Maybe(
	mw func(http.Handler) http.Handler,
	condition func(*http.Request) bool,
) func(http.Handler) http.Handler
```

`Maybe` applies `mw` only when `condition` returns true for the request. The middleware is built once, when `Maybe` is called.

```go
server := castor.Server().Middlewares(
	middleware.Maybe(
		middleware.BasicAuth("Admin", users),
		func(r *http.Request) bool {
			return strings.HasPrefix(r.URL.Path, "/admin/")
		},
	),
)
```

### SetHeader

```go
func SetHeader(key, value string) func(http.Handler) http.Handler
```

`SetHeader` sets a response header before calling the next handler.

```go
server := castor.Server().Middlewares(
	middleware.SetHeader("X-Content-Type-Options", "nosniff"),
)
```

For per-route headers, attach it to the route:

```go
server.Routes(
	castor.Get("/download", downloadHandler).
		Middlewares(middleware.SetHeader("Content-Disposition", "attachment")),
)
```

### WithValue

```go
func WithValue(key, value any) func(http.Handler) http.Handler
```

`WithValue` stores `value` in the request context under `key`. Retrieve it with [`castor.GetValue`](castor.md#getvalue).

```go
var environmentKey = &castor.ContextKey{Name: "environment"}

server := castor.Server().
	Middlewares(middleware.WithValue(environmentKey, "production")).
	Routes(castor.Get("/", func(res http.ResponseWriter, req *http.Request) {
		env := castor.GetValue[string](req.Context(), environmentKey)
		castor.Json(res, map[string]string{"environment": env})
	}))
```

## Authentication

### BasicAuth

```go
func BasicAuth(realm string, creds map[string]string) func(next http.Handler) http.Handler
```

`BasicAuth` protects the wrapped handler with HTTP Basic authentication. Credentials come from the map as plaintext username/password pairs. Password comparison uses `crypto/subtle` constant-time comparison; the username lookup itself is a normal map lookup.

Failed authentication returns `401 Unauthorized` with:

```text
WWW-Authenticate: Basic realm="<realm>"
```

```go
users := map[string]string{
	"alice": "correct-horse-battery-staple",
}

server.Routes(
	castor.Get("/admin", adminHandler).
		Middlewares(middleware.BasicAuth("Admin", users)),
)
```

Basic authentication sends credentials in an easily decoded header. Use it only over HTTPS.

## CORS

### CORS

```go
func CORS(opts CORSOpts) func(next http.Handler) http.Handler
```

`CORS` implements the Cross-Origin Resource Sharing protocol from [go-chi/cors](https://github.com/go-chi/cors), adapted to Castor.

Preflight requests are `OPTIONS` requests that include an `Origin` header and a non-empty `Access-Control-Request-Method`. Castor validates the origin, method, and requested headers, sets the appropriate `Access-Control-*` and `Vary` headers, and returns `200 OK` without calling the next handler. When `OptionsPassthrough` is enabled, the next handler is called after CORS headers are set.

Actual requests always continue to the next handler. When both the origin and the request method are allowed, Castor adds the allowed-origin, exposed-header, and credentials headers; otherwise it adds only `Vary: Origin`. A preflight that fails validation still returns `200 OK`, but it omits the `Access-Control-Allow-*` headers, so the browser rejects the cross-origin request.

```go
server := castor.Server().Middlewares(
	middleware.CORS(middleware.CORSOpts{
		AllowedOrigins: []string{
			"https://app.example.com",
			"https://*.example.com",
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}),
)
```

Register `CORS` globally, before the router, so `OPTIONS` requests do not need a matching route.

See [`CORSOpts`](#corsopts) for every option and [`CORSAllowAll`](#corsallowall) for a permissive preset.

### CORSOpts

```go
type CORSOpts struct {
	AllowedOrigins     []string
	AllowOriginFunc    func(r *http.Request, origin string) bool
	AllowedMethods     []string
	AllowedHeaders     []string
	ExposedHeaders     []string
	AllowCredentials   bool
	MaxAge             int
	OptionsPassthrough bool
}
```

| Field | Description |
| --- | --- |
| `AllowedOrigins` | Origins permitted to make cross-origin requests. Empty means all origins when `AllowOriginFunc` is unset. `"*"` allows all origins and makes responses use `"*"`. One `*` wildcard is supported per origin, for example `https://*.example.com`. Matching is case-insensitive. |
| `AllowOriginFunc` | Custom origin validator. When set, it takes precedence over `AllowedOrigins` for validating requests. It receives the request and the origin string. |
| `AllowedMethods` | Methods permitted for cross-origin requests. Defaults to `GET`, `POST`, and `HEAD`. `OPTIONS` is always allowed for preflight. |
| `AllowedHeaders` | Request headers permitted in preflight requests. `"*"` allows all headers. `Origin` is always allowed. Defaults to `Origin`, `Accept`, and `Content-Type`. |
| `ExposedHeaders` | Response headers exposed to browser JavaScript through `Access-Control-Expose-Headers`. |
| `AllowCredentials` | Adds `Access-Control-Allow-Credentials: true`. Browsers reject credentials combined with the literal `"*"` allowed origin, so configure explicit origins when this is enabled. |
| `MaxAge` | Seconds a preflight response may be cached through `Access-Control-Max-Age`. Values less than or equal to zero omit the header. |
| `OptionsPassthrough` | Calls the next handler for preflights instead of returning `200 OK`. Enable it when the application has its own `OPTIONS` routes. |

```go
opts := middleware.CORSOpts{
	AllowedOrigins: []string{"https://example.com"},
	AllowedMethods: []string{http.MethodGet, http.MethodPost},
	AllowedHeaders: []string{"Content-Type"},
	MaxAge:         600,
}
```

### CORSAllowAll

```go
func CORSAllowAll() func(next http.Handler) http.Handler
```

`CORSAllowAll` returns a permissive CORS middleware for all origins, all headers, and the common HTTP methods, without credentials.

```go
server := castor.Server().Middlewares(middleware.CORSAllowAll())
```

## Request validation and limits

### AllowContentType

```go
func AllowContentType(contentTypes ...string) func(http.Handler) http.Handler
```

`AllowContentType` rejects request bodies whose media type is not in the allowed list. Media type parameters are ignored and matching is case-insensitive. Requests with `Content-Length: 0` always pass. Rejected requests receive `415 Unsupported Media Type`.

```go
server.Routes(
	castor.Post("/api/items", createItem).
		Middlewares(middleware.AllowContentType("application/json")),
)
```

### AllowContentEncoding

```go
func AllowContentEncoding(contentEncodings ...string) func(next http.Handler) http.Handler
```

`AllowContentEncoding` rejects request bodies whose `Content-Encoding` is not in the allowed list. Every comma-separated encoding across all `Content-Encoding` headers must be allowed. Matching is case-insensitive, and requests with no body always pass. Rejected requests receive `415 Unsupported Media Type`.

This middleware validates the declared encoding; it does not decompress the body.

```go
server.Routes(
	castor.Post("/api/import", importData).
		Middlewares(middleware.AllowContentEncoding("gzip", "deflate")),
)
```

### ContentCharset

```go
func ContentCharset(charsets ...string) func(next http.Handler) http.Handler
```

`ContentCharset` rejects request bodies whose `Content-Type` charset is not in `charsets`. Matching is case-insensitive, requests without a body always pass, and a request without a charset passes only when `""` is included in the allowed list. Rejected requests receive `415 Unsupported Media Type`.

```go
server.Routes(
	castor.Post("/api/items", createItem).
		Middlewares(middleware.ContentCharset("utf-8", "utf-16")),
)
```

Include `""` to allow bodies that omit the charset:

```go
middleware.ContentCharset("", "utf-8")
```

### RequestSize

```go
func RequestSize(bytes int64) func(http.Handler) http.Handler
```

`RequestSize` replaces the request body with `http.MaxBytesReader`, limiting how many bytes handlers can read. The middleware itself does not write an error response; the handler must check read errors and respond, typically with `413 Request Entity Too Large`.

```go
server.Routes(
	castor.Post("/upload", func(res http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			http.Error(res, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		_ = body
		res.WriteHeader(http.StatusNoContent)
	}).Middlewares(middleware.RequestSize(1 << 20)),
)
```

## Request identity

### RequestID

```go
func RequestID(next http.Handler) http.Handler
```

`RequestID` stores a request ID in the context under `castor.RequestIDKey`. It reuses the incoming [`castor.RequestIDHeader`](castor.md#requestidheader) value when present; otherwise it generates an ID from the hostname, a random prefix, and a process-wide sequence number.

```go
server := castor.Server().Middlewares(middleware.RequestID)

server.Routes(castor.Get("/", func(res http.ResponseWriter, req *http.Request) {
	castor.Json(res, map[string]string{
		"request_id": middleware.GetReqID(req.Context()),
	})
}))
```

The generated ID looks like `hostname/randomprefix-000042`.

### GetReqID

```go
func GetReqID(ctx context.Context) string
```

`GetReqID` returns the ID stored by `RequestID`, or `""` when no request ID middleware is installed. It is equivalent to [`castor.GetReqID`](castor.md#getreqid).

```go
id := middleware.GetReqID(req.Context())
```

### NextRequestID

```go
func NextRequestID() uint64
```

`NextRequestID` increments and returns the process-wide request ID sequence. `RequestID` uses the same sequence when generating IDs.

```go
customID := fmt.Sprintf("job-%d", middleware.NextRequestID())
```

### ClientIPFromHeader

```go
func ClientIPFromHeader(
	trustedHeader string,
	trustedIPPrefixes ...string,
) func(http.Handler) http.Handler
```

`ClientIPFromHeader` stores the client IP from a proxy-managed header. The last value of the header wins. With no trusted prefixes, the header is always trusted. When trusted prefixes are supplied, the header is trusted only if `r.RemoteAddr` is inside one of them; otherwise a valid `RemoteAddr` is used. Invalid input leaves the client IP unset.

Read the result with [`GetClientIP`](#getclientip) or [`GetClientIPAddr`](#getclientipaddr).

```go
server := castor.Server().Middlewares(
	middleware.ClientIPFromHeader("X-Real-IP", "10.0.0.0/8"),
)

server.Routes(castor.Get("/whoami", func(res http.ResponseWriter, req *http.Request) {
	castor.Json(res, map[string]string{
		"ip": middleware.GetClientIP(req.Context()),
	})
}))
```

### ClientIPFromXFF

```go
func ClientIPFromXFF(trustedIPPrefixes ...string) func(http.Handler) http.Handler
```

`ClientIPFromXFF` stores the client IP from `X-Forwarded-For`, walking the chain from right to left and skipping addresses inside the trusted prefixes. A direct client whose `RemoteAddr` is outside the trusted prefixes is detected from `RemoteAddr`. If the whole chain is trusted, no client IP is stored.

At least one trusted prefix is required; the function panics when called without one. Prefixes are parsed with `netip.MustParsePrefix`, so invalid CIDR strings also panic.

```go
server := castor.Server().Middlewares(
	middleware.ClientIPFromXFF("10.0.0.0/8", "192.168.0.0/16"),
)
```

### ClientIPFromXFFTrustedProxies

```go
func ClientIPFromXFFTrustedProxies(numTrustedProxies int) func(http.Handler) http.Handler
```

`ClientIPFromXFFTrustedProxies` selects the client IP by counting trusted proxies from the right side of `X-Forwarded-For`. With `numTrustedProxies == 2`, the second address from the right is treated as the client. It reads only `X-Forwarded-For` and leaves the client IP unset when the chain is shorter than the configured proxy count.

The count must be at least 1; the function panics otherwise.

```go
server := castor.Server().Middlewares(
	middleware.ClientIPFromXFFTrustedProxies(2),
)
```

### ClientIPFromRemoteAddr

```go
func ClientIPFromRemoteAddr(next http.Handler) http.Handler
```

`ClientIPFromRemoteAddr` stores the IP from `r.RemoteAddr`. Use it when the server is directly exposed and there are no trusted proxies in front of it.

```go
server := castor.Server().Middlewares(middleware.ClientIPFromRemoteAddr)
```

### GetClientIP

```go
func GetClientIP(ctx context.Context) string
```

`GetClientIP` returns the client IP stored by any `ClientIPFrom*` middleware as a string, or `""` when none was stored.

```go
ip := middleware.GetClientIP(req.Context())
```

### GetClientIPAddr

```go
func GetClientIPAddr(ctx context.Context) netip.Addr
```

`GetClientIPAddr` returns the stored IP as a `netip.Addr`. The zero `netip.Addr` is returned when unset.

```go
addr := middleware.GetClientIPAddr(req.Context())
if addr.IsValid() {
	log.Printf("client: %s", addr)
}
```

### URLFormat

```go
func URLFormat(next http.Handler) http.Handler
```

`URLFormat` extracts a trailing extension from the request path, stores it in the context, and strips it before routing. For example, `/articles/42.json` is routed as `/articles/42` and the format is `"json"`.

It must run globally, before the router. Read the result with [`GetURLFormat`](#geturlformat).

```go
server := castor.Server().
	Middlewares(middleware.URLFormat).
	Routes(castor.Get("/articles/{id}", func(res http.ResponseWriter, req *http.Request) {
		castor.Json(res, map[string]string{
			"id":     castor.GetPathParam(r, "id"),
			"format": middleware.GetURLFormat(r),
		})
	}))
```

### GetURLFormat

```go
func GetURLFormat(r *http.Request) string
```

`GetURLFormat` returns the format extracted by `URLFormat`, or `""` when no extension was present or the middleware is not installed.

```go
format := middleware.GetURLFormat(req)
```

### URLFormatCtxKey

```go
var URLFormatCtxKey = &castor.ContextKey{Name: "URLFormat"}
```

`URLFormatCtxKey` is the context key used by `URLFormat`. `GetURLFormat` is the preferred way to read the value.

```go
format := castor.GetValue[string](req.Context(), middleware.URLFormatCtxKey)
```

## Path routing

### CleanPath

```go
func CleanPath(next http.Handler) http.Handler
```

`CleanPath` cleans the request path with `path.Clean`, removing duplicate slashes and dot segments. It updates `RawPath` when present. It mutates the request URL and must run globally, before the router.

```go
server := castor.Server().Middlewares(middleware.CleanPath)
```

### StripSlashes

```go
func StripSlashes(next http.Handler) http.Handler
```

`StripSlashes` removes one trailing slash from the request path before routing, except for the root path. It mutates the request URL and must run globally.

```go
server := castor.Server().Middlewares(middleware.StripSlashes)
```

A request for `/users/` is routed as `/users`.

### RedirectSlashes

```go
func RedirectSlashes(next http.Handler) http.Handler
```

`RedirectSlashes` sends a `301 Moved Permanently` redirect for paths that end in a slash, except for the root path. The target is cleaned by replacing backslashes, trimming slashes, and preserving the query string. It must run globally.

```go
server := castor.Server().Middlewares(middleware.RedirectSlashes)
```

A request for `/users/?page=2` redirects to `/users?page=2`.

### PathRewrite

```go
func PathRewrite(old, new string) func(http.Handler) http.Handler
```

`PathRewrite` replaces the first occurrence of `old` in the request path with `new`. It mutates the request URL and must run globally, before the router.

```go
server := castor.Server().Middlewares(
	middleware.PathRewrite("/api/v1", "/api/v2"),
)
```

### PageRoute

```go
func PageRoute(path string, handler http.Handler) func(http.Handler) http.Handler
```

`PageRoute` serves `handler` for an exact, case-insensitive `GET` request path and otherwise calls the next handler. It must run globally, before the mux.

```go
server := castor.Server().Middlewares(
	middleware.PageRoute("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})),
)
```

### Heartbeat

```go
func Heartbeat(endpoint string) func(http.Handler) http.Handler
```

`Heartbeat` answers `GET` and `HEAD` requests whose path equals `endpoint`, case-insensitively, with `200 OK`, `Content-Type: text/plain`, and a single `.` body. It does not call the next handler for those requests and must run globally.

```go
server := castor.Server().Middlewares(middleware.Heartbeat("/ping"))
```

### SupressNotFound

```go
func SupressNotFound(mux *http.ServeMux) func(http.Handler) http.Handler
```

`SupressNotFound` checks whether `mux` can match the request. If it cannot, it serves the mux's `404` or `405` response immediately and stops the remaining chain. If the mux matches, the next handler runs normally.

This is useful when later middleware should not process unmatched requests. Use it with the same mux that routes requests; `castor.Server` builds its router internally, so this helper is mostly useful in a directly composed `ServeMux` chain. The correctly spelled alias is [`SuppressNotFound`](#suppressnotfound). The misspelled name is preserved for compatibility. The function panics when `mux` is nil.

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

handler := middleware.SupressNotFound(mux)(
	http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		castor.Json(res, map[string]string{"matched": "yes"})
	}),
)
```

### SuppressNotFound

```go
func SuppressNotFound(mux *http.ServeMux) func(http.Handler) http.Handler
```

`SuppressNotFound` is the correctly spelled alias for [`SupressNotFound`](#supressnotfound).

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /ok", okHandler)

handler := middleware.SuppressNotFound(mux)(mux)
```

`SupressNotFound` needs the same `*http.ServeMux` used for routing. Because `castor.Server` builds its router internally, this helper is mostly useful when composing a `ServeMux` chain directly.


### GetHead

```go
func GetHead(next http.Handler) http.Handler
```

`GetHead` is a no-op for `net/http.ServeMux` servers. `ServeMux` already routes `HEAD` requests to handlers registered for `GET`. It is provided for migration parity with middleware stacks that required an explicit `GetHead` middleware.

```go
server := castor.Server().Middlewares(middleware.GetHead)
```

## Response headers

### NoCache

```go
func NoCache(next http.Handler) http.Handler
```

`NoCache` adds headers that discourage client and proxy caching:

- `Expires: Thu, 01 Jan 1970 00:00:00 GMT`
- `Cache-Control: no-cache, no-store, no-transform, must-revalidate, private, max-age=0`
- `Pragma: no-cache`
- `X-Accel-Expires: 0`

It also removes conditional request headers such as `If-None-Match` and `If-Modified-Since` before calling the next handler.

```go
server.Routes(
	castor.Get("/private", privateHandler).
		Middlewares(middleware.NoCache),
)
```

### Deprecation

```go
func Deprecation(deprecatedAt time.Time, links ...string) func(http.Handler) http.Handler
```

`Deprecation` marks a response as deprecated per RFC 9745. It sets `Deprecation` to `@<unix-seconds>` and appends any supplied `Link` values. It panics when `deprecatedAt` is the zero time.

```go
server.Routes(
	castor.Get("/api/v1/users", listUsers).
		Middlewares(middleware.Deprecation(
			time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			`</api/v2/users>; rel="deprecation"`,
		)),
)
```

### Sunset

```go
func Sunset(sunsetAt time.Time, links ...string) func(http.Handler) http.Handler
```

`Sunset` announces when an endpoint will stop working per RFC 8594. It sets `Sunset` to the UTC HTTP date and appends any supplied `Link` values. It panics when `sunsetAt` is the zero time.

```go
server.Routes(
	castor.Get("/api/v1/users", listUsers).
		Middlewares(middleware.Sunset(
			time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC),
			`</api/v2/users>; rel="successor-version"`,
		)),
)
```

## Compression and header routing

### Compress

```go
func Compress(level int, types ...string) func(next http.Handler) http.Handler
```

`Compress` compresses responses whose `Content-Type` is in `types`, based on `Accept-Encoding`. `level` is a `compress/flate` level from `flate.HuffmanOnly` through `flate.BestCompression`; invalid levels panic.

With no `types`, a built-in default set of text, JSON, XML, JavaScript, and SVG types is used. Custom types are lowercased; a `<type>/*` wildcard is allowed. Other wildcard shapes panic. The built-in encoders are gzip and deflate.

```go
server := castor.Server().Middlewares(middleware.Compress(5))
```

Restrict compression to specific types:

```go
server := castor.Server().Middlewares(
	middleware.Compress(5, "text/html", "application/json", "image/svg+xml"),
)
```

The middleware adds `Vary: Accept-Encoding`, skips `204 No Content` and `304 Not Modified` responses, and does not compress a response that already has a `Content-Encoding`.

### Compressor

```go
type Compressor struct {
	// unexported fields
}
```

`Compressor` holds compression encoders, allowed content types, and encoding precedence. Create one with [`NewCompressor`](#newcompressor), then use its [`Handler`](#compressorhandler) method as middleware.

```go
compressor := middleware.NewCompressor(5, "application/json")
server := castor.Server().Middlewares(compressor.Handler)
```

### NewCompressor

```go
func NewCompressor(level int, types ...string) *Compressor
```

`NewCompressor` builds a reusable compressor. It accepts the same level and content-type arguments as `Compress`.

```go
compressor := middleware.NewCompressor(5, "application/json")
server := castor.Server().Middlewares(compressor.Handler)
```

### Compressor.Handler

```go
func (c *Compressor) Handler(next http.Handler) http.Handler
```

`Handler` returns the compression middleware for a `Compressor`.

```go
server := castor.Server().Middlewares(
	middleware.NewCompressor(5),
)
```

### Compressor.SetEncoder

```go
func (c *Compressor) SetEncoder(encoding string, encoder EncoderFunc)
```

`SetEncoder` registers a compression algorithm for a `Compressor`. Custom encoders are useful for formats such as Brotli, but the encoder itself must be implemented without additional dependencies. Registering an encoding makes it take precedence over previously registered encodings.

```go
compressor := middleware.NewCompressor(5)
compressor.SetEncoder("gzip", func(w io.Writer, level int) io.Writer {
	return gzip.NewWriter(w)
})
```

### RouteHeaders

```go
func RouteHeaders() HeaderRouter
```

`RouteHeaders` starts a builder that chooses middleware based on request header values.

```go
router := middleware.RouteHeaders().
	Route("X-Role", "admin", adminMiddleware).
	Route("X-Role", "user-*", userMiddleware).
	RouteAny("X-Env", []string{"production", "staging"}, envMiddleware).
	RouteDefault(defaultMiddleware)

server := castor.Server().Middlewares(router.Handler)
```

Rules are evaluated in the order they were added, and the first match wins. Header names are case-insensitive because Go canonicalizes them. Header values are lowercased before matching, so write exact values and patterns in lowercase. A `Host` header rule falls back to `r.Host` when the `Host` header value is empty.

### HeaderRouter

```go
type HeaderRouter map[string][]HeaderRoute
```

`HeaderRouter` maps lowercase header names to ordered `HeaderRoute` entries. Use the builder methods rather than constructing it directly.

```go
router := middleware.HeaderRouter{}
router = router.Route("X-Role", "admin", adminMiddleware)
```

### HeaderRouter.Route

```go
func (hr HeaderRouter) Route(
	header, match string,
	middleware func(next http.Handler) http.Handler,
) HeaderRouter
```

`Route` adds middleware for a header value that exactly matches `match`, or matches a pattern containing one `*` wildcard.

```go
router := middleware.RouteHeaders().
	Route("X-Role", "admin", adminMiddleware)
```

### HeaderRouter.RouteAny

```go
func (hr HeaderRouter) RouteAny(
	header string,
	matches []string,
	middleware func(next http.Handler) http.Handler,
) HeaderRouter
```

`RouteAny` adds middleware that matches any of the supplied header values or patterns.

```go
router := middleware.RouteHeaders().
	RouteAny("X-Env", []string{"production", "staging"}, envMiddleware)
```

### HeaderRouter.RouteDefault

```go
func (hr HeaderRouter) RouteDefault(
	middleware func(next http.Handler) http.Handler,
) HeaderRouter
```

`RouteDefault` sets the middleware used when no header rule matches.

```go
router := middleware.RouteHeaders().
	Route("X-Role", "admin", adminMiddleware).
	RouteDefault(defaultMiddleware)
```

### HeaderRouter.Handler

```go
func (hr HeaderRouter) Handler(next http.Handler) http.Handler
```

`Handler` builds the dispatching middleware. It evaluates rules in insertion order and falls back to the default middleware or `next`.

```go
server := castor.Server().Middlewares(
	middleware.RouteHeaders().
		Route("X-Role", "admin", adminMiddleware).
		RouteDefault(defaultMiddleware).
		Handler,
)
```

### HeaderRoute

```go
type HeaderRoute struct {
	Middleware func(next http.Handler) http.Handler
	MatchOne   Pattern
	MatchAny   []Pattern
}
```

`HeaderRoute` combines a middleware with one pattern or a list of patterns. It is normally created by `HeaderRouter.Route` and `HeaderRouter.RouteAny`.

```go
route := middleware.HeaderRoute{
	Middleware: adminMiddleware,
	MatchOne:   middleware.NewPattern("admin"),
}
```

### HeaderRoute.IsMatch

```go
func (hr HeaderRoute) IsMatch(value string) bool
```

`IsMatch` reports whether `value` satisfies the route's pattern list. `MatchAny` takes precedence when present.

```go
matched := route.IsMatch("admin")
```

### Pattern

```go
type Pattern struct {
	// unexported fields
}
```

`Pattern` matches an exact string or a string containing one `*` wildcard. Use `NewPattern` to create one.

```go
pattern := middleware.NewPattern("user-*")
```

### NewPattern

```go
func NewPattern(value string) Pattern
```

`NewPattern` parses an exact value or a wildcard pattern. A pattern containing `*` matches any string with the given prefix and suffix; the wildcard may match zero or more characters.

```go
exact := middleware.NewPattern("admin")
wildcard := middleware.NewPattern("user-*")
```

### Pattern.Match

```go
func (p Pattern) Match(value string) bool
```

`Pattern.Match` reports whether `value` matches the pattern.

```go
if middleware.NewPattern("user-*").Match("user-42") {
	// matched
}
```

## Resilience and throttling

### Recoverer

```go
func Recoverer(next http.Handler) http.Handler
```

`Recoverer` recovers panics, logs them, and writes `500 Internal Server Error` when a response is still possible. It re-panics `http.ErrAbortHandler` so the connection is aborted. When a `castor.LogEntry` is in the request context, `Panic` is called on it; otherwise a formatted stack trace is printed to stderr.

```go
server := castor.Server().Middlewares(middleware.Recoverer)
```

### Timeout

```go
func Timeout(timeout time.Duration) func(next http.Handler) http.Handler
```

`Timeout` cancels the request context after `timeout`. If the handler finishes without writing a response and the deadline has expired, the middleware writes `504 Gateway Timeout`. Handlers should observe `r.Context().Done()` and stop work when the context is canceled.

```go
server.Routes(
	castor.Get("/slow", slowHandler).
		Middlewares(middleware.Timeout(5 * time.Second)),
)
```

### Throttle

```go
func Throttle(limit int) func(http.Handler) http.Handler
```

`Throttle` limits the number of in-flight requests to `limit`. When the limit is reached, the request receives `429 Too Many Requests`. It is shorthand for `ThrottleWithOpts` with the limit set and no backlog, so a saturated server rejects new requests immediately.

```go
server := castor.Server().Middlewares(middleware.Throttle(100))
```

The limit must be greater than zero; the function panics otherwise.

### ThrottleBacklog

```go
func ThrottleBacklog(
	limit, backlogLimit int,
	backlogTimeout time.Duration,
) func(http.Handler) http.Handler
```

`ThrottleBacklog` allows up to `backlogLimit` extra requests to wait for a slot. A waiting request is rejected with `429 Too Many Requests` if no slot becomes available within `backlogTimeout`, or if the request context is canceled.

```go
server := castor.Server().Middlewares(
	middleware.ThrottleBacklog(50, 25, 10*time.Second),
)
```

The limit must be greater than zero and the backlog limit must be zero or greater; invalid values panic.

### ThrottleWithOpts

```go
func ThrottleWithOpts(opts ThrottleOpts) func(http.Handler) http.Handler
```

`ThrottleWithOpts` is the configurable form of the throttle middleware.

```go
server := castor.Server().Middlewares(
	middleware.ThrottleWithOpts(middleware.ThrottleOpts{
		Limit:          50,
		BacklogLimit:   25,
		BacklogTimeout: 10 * time.Second,
		StatusCode:     http.StatusServiceUnavailable,
		RetryAfterFn: func(ctxDone bool) time.Duration {
			return 5 * time.Second
		},
	}),
)
```

`Limit` must be greater than zero and `BacklogLimit` must be zero or greater; invalid values panic.

### ThrottleOpts

```go
type ThrottleOpts struct {
	RetryAfterFn   func(ctxDone bool) time.Duration
	Limit          int
	BacklogLimit   int
	BacklogTimeout time.Duration
	StatusCode     int
}
```

| Field | Description |
| --- | --- |
| `Limit` | Maximum number of in-flight requests. Required; must be greater than zero. |
| `BacklogLimit` | Number of additional requests allowed to wait. Must be zero or greater. |
| `BacklogTimeout` | Maximum wait for a backlogged request. Set this when using a backlog; a zero duration makes the wait expire immediately. |
| `StatusCode` | Rejection status. Defaults to `429 Too Many Requests`. |
| `RetryAfterFn` | When set, enables the `Retry-After` header. It receives `true` when the request context was canceled and returns the delay. The value is rounded up to whole seconds; a negative result produces `Retry-After: 0`. |

```go
opts := middleware.ThrottleOpts{
	Limit:          10,
	BacklogLimit:   5,
	BacklogTimeout: time.Second,
	RetryAfterFn: func(ctxDone bool) time.Duration {
		return 2 * time.Second
	},
}
```

## Diagnostics

### Profiler

```go
func Profiler() http.Handler
```

`Profiler` returns a handler serving `net/http/pprof` and `expvar` endpoints. Mount it behind `http.StripPrefix`, normally under `/debug`. The handler also applies `NoCache`.

```go
mux := http.NewServeMux()
mux.Handle("/debug/", http.StripPrefix("/debug", middleware.Profiler()))
```

To mount it in a Castor server, use a custom route:

```go
server.Routes(castor.Route(func(mux *http.ServeMux, prefix string, mws []castor.Middleware) {
	mux.Handle(prefix+"/debug/", http.StripPrefix(prefix+"/debug", middleware.Profiler()))
}))
```
