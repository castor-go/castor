# Castor

Castor is a small, function-based DSL for building HTTP services using only the Go standard library. Servers, routes, route groups, middleware, and helpers are plain Go functions and values, so there is no runtime container or reflection layer to learn.

- Module: `github.com/castor-go/castor`
- Package name: `castor`
- Go: 1.27+
- Dependencies: standard library only

The companion middleware reference is in [middleware.md](middleware.md).

## Contents

- [Installation](#installation)
- [Quick start](#quick-start)
- [Server](#server)
- [Routing](#routing)
- [Requests](#requests)
- [Responses](#responses)
- [Cookies](#cookies)
- [Flash messages](#flash-messages)
- [Context values](#context-values)
- [Logging](#logging)
- [Response writer](#response-writer)
- [Types and helpers](#types-and-helpers)

## Installation

```bash
go get github.com/castor-go/castor
```

Snippets in this document omit imports for brevity. Unless stated otherwise, they assume these imports:

```go
import (
	"net/http"

	"github.com/castor-go/castor"
	"github.com/castor-go/castor/middleware"
)
```

Snippets that use `bytes`, `context`, `fmt`, `io`, `log`, `log/slog`, `mime/multipart`, `os`, `strings`, or `time` assume the corresponding standard-library imports.

## Quick start

```go
package main

import (
	"log"
	"net/http"

	"github.com/castor-go/castor"
	"github.com/castor-go/castor/middleware"
)

func main() {
	server := castor.Server().
		Middlewares(middleware.Recoverer).
		Routes(
			castor.Get("/", func(res http.ResponseWriter, req *http.Request) {
				castor.Json(res, map[string]string{"status": "ok"})
			}),
			castor.Get("/hello/{name}", func(res http.ResponseWriter, req *http.Request) {
				castor.Json(res, map[string]string{
					"message": "Hello " + castor.GetPathParam(req, "name"),
				})
			}),
		)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
```

`Start` blocks until the process receives `SIGINT` or `SIGTERM`, then shuts the server down with a 10-second grace period.

## Server

A `WebServer` collects global middleware, routes, groups, and static-asset handlers. `Server` installs the built-in request logger first, then any global middleware added with `Middlewares`.

### Middleware

```go
type Middleware = func(http.Handler) http.Handler
```

`Middleware` is an alias for the standard `func(http.Handler) http.Handler` signature. Any function with that shape can be passed to `Middlewares`, `Route.Middlewares`, or `RouteGroup.Middlewares`.

```go
func timing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s took %s", r.URL.Path, time.Since(start))
	})
}

server := castor.Server().Middlewares(timing)
```

### Server

```go
func Server(opts ...ServerOption) *WebServer
```

`Server` creates a `*WebServer`. The default host is `0.0.0.0` and the default port is `3000`. Every server starts with `Logger` in its global middleware chain.

```go
server := castor.Server()
server.Routes(castor.Get("/", func(res http.ResponseWriter, req *http.Request) {
	res.WriteHeader(http.StatusNoContent)
}))

if err := server.Start(); err != nil {
	log.Fatal(err)
}
```

See the [Quick start](#quick-start) for a complete program.

### Config

```go
func Config(cfg ServerConfig) ServerOption
```

`Config` returns a `ServerOption` that replaces the server configuration. Values not supplied in the struct become their Go zero values.

```go
// full server configuration
server := castor.Server(castor.Config(castor.ServerConfig{
	Host:         "127.0.0.1",
	Port:         8080,
	IdleTimeout:  2 * time.Minute,
	ReadTimeout:  10 * time.Second,
	WriteTimeout: 30 * time.Second,
}))
```

Or you can specify a few configurations by using:
```go
server := castor.Server(
	castor.WithHost("127.0.0.1"),
	castor.WithPort(4000), 
	castor.WithLogger(customLogger),
)
```
### ServerConfig

```go
type ServerConfig struct {
	Host         string
	Port         int
	Logger       LoggerInterface
	IdleTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}
```

| Field | Description |
| --- | --- |
| `Host` | Interface to listen on. Default `0.0.0.0`. |
| `Port` | TCP port. Default `3000`. |
| `Logger` | Logger carried by the configuration. Request logging is controlled by [`DefaultLogger`](#defaultlogger) and [`RequestLogger`](#requestlogger); the bundled server does not automatically wire this field into the middleware chain. |
| `IdleTimeout` | Maximum idle keep-alive connection time. |
| `ReadTimeout` | Maximum time allowed to read the request, including the body. |
| `WriteTimeout` | Maximum time allowed to write the response. |

See [Config](#config) for a usage example.

### ServerOption

```go
type ServerOption func(*WebServer)
```

`ServerOption` customizes a `WebServer` while it is being created. A custom option can call any `WebServer` method.

```go
func withRecoverer(s *castor.WebServer) {
	s.Middlewares(middleware.Recoverer)
}

server := castor.Server(withRecoverer)
```

### WebServer

`WebServer` is the DSL entry point. Configure it with `Middlewares`, `Routes`, `Groups`, `StaticAssets`, and `NotFound`, then start it with `Start`.

### WebServer.Middlewares

```go
func (s *WebServer) Middlewares(middlewares ...Middleware) *WebServer
```

`Middlewares` appends global middleware in execution order. The first middleware listed is the outermost and runs first for every request, including requests that do not match a route.

```go
server := castor.Server().Middlewares(
	middleware.RequestID,
	middleware.Recoverer,
)
```

### WebServer.Routes

```go
func (s *WebServer) Routes(routes ...Route) *WebServer
```

`Routes` appends one or more routes. Routes are registered on a `net/http.ServeMux` when the server starts.

```go
server := castor.Server().Routes(
	castor.Get("/", homeHandler),
	castor.Post("/todos", createTodoHandler),
	castor.Delete("/todos/{id}", deleteTodoHandler),
)
```

### WebServer.Groups

```go
func (s *WebServer) Groups(groups ...RouteGroup) *WebServer
```

`Groups` appends route groups. A group applies a path prefix and an optional middleware chain to every route it contains.

```go
contacts := castor.Group("/contacts",
	castor.Get("/", listContacts),
	castor.Get("/{id}", showContact),
).Middlewares(middleware.BasicAuth("Contacts", users))

server := castor.Server().Groups(contacts)
```

### WebServer.StaticAssets

```go
func (s *WebServer) StaticAssets(pattern, dir string, cfgs ...StaticConfig) *WebServer
```

`StaticAssets` serves files from `dir` under `pattern`. The pattern may end in `*` or `**`; both are stripped to form the URL prefix. Files are served with `http.FileServer` semantics, and only `GET` requests are registered.

The first `StaticConfig` in `cfgs` is used.

```go
server := castor.Server().StaticAssets("/assets/**", "./public", castor.StaticConfig{
	CacheMaxAge: 24 * time.Hour,
	ETag:        true,
})
```

A request for `/assets/css/site.css` reads `./public/css/site.css`.

### WebServer.NotFound

```go
func (s *WebServer) NotFound(handler NotFoundHandler) *WebServer
```

`NotFound` installs a handler that replaces any `404 Not Found` response, including 404s written by matched route handlers. 
The handler receives the response body and is responsible for writing the replacement response.

```go
server := castor.Server().
	NotFound(func(res http.ResponseWriter, message string) {
		castor.Json(res, map[string]any{
			"status":  http.StatusNotFound,
			"message": strings.TrimSpace(message),
		}, statusCode)
	}).
	Routes(castor.Get("/", homeHandler))
```

See [`NotFoundHandler`](#notfoundhandler) for the handler type.

### WebServer.Start

```go
func (s *WebServer) Start() error
```

`Start` builds the router, listens on the configured address, and serves requests. It blocks until the server fails or receives `SIGINT`/`SIGTERM`. Shutdown is graceful and gets 10 seconds to finish.

```go
if err := server.Start(); err != nil {
	log.Fatal(err)
}
```

### NotFoundHandler

```go
type NotFoundHandler func(res http.ResponseWriter, message string)
```

`NotFoundHandler` handles a customized 404 response. `message` is the original response body produced by the route or by `http.ServeMux`.

```go
var handler castor.NotFoundHandler = func(res http.ResponseWriter, message string) {
	http.Error(res, "custom: "+message, http.StatusNotFound)
}

server := castor.Server().
	NotFound(handler).
	Routes(castor.Get("/", indexHandler))
```

### StaticConfig

```go
type StaticConfig struct {
	CacheMaxAge time.Duration // Cache-Control max-age; zero omits the header.
	ETag        bool          // Enables ETag generation and If-None-Match handling.
}
```

`CacheMaxAge` adds `Cache-Control: public, max-age=<seconds>`. `ETag` adds an `ETag` derived from the file's modification time and size and answers matching `If-None-Match` requests with `304 Not Modified`.

```go
server := castor.Server().StaticAssets("/static/**", "./static", castor.StaticConfig{
	CacheMaxAge: time.Hour,
	ETag:        true,
})
```

See [WebServer.StaticAssets](#webserverstaticassets) for the complete example.

## Routing

Routes are constructed with helpers such as `Get` and `Post`, then passed to `WebServer.Routes` or `Group`. Castor uses `net/http.ServeMux` internally, so route patterns support method prefixes and path wildcards from the standard library.

### Get

```go
func Get(path string, handler http.HandlerFunc) Route
```

`Get` registers a `GET` route. A trailing slash on the path is kept exact instead of becoming a subtree pattern.

```go
server.Routes(castor.Get("/articles/{id}", func(res http.ResponseWriter, req *http.Request) {
	id := castor.GetPathParam(req, "id")
	castor.Json(res, map[string]string{"id": id})
}))
```

### Query

```go
func Query(pattern string, handler http.HandlerFunc) Route
```

`Query` is an alias for `Get`.

```go
server.Routes(castor.Query("/search", func(res http.ResponseWriter, req *http.Request) {
	castor.Json(res, map[string]string{"q": castor.GetQueryParam(req, "q")})
}))
```

### Post

```go
func Post(pattern string, handler http.HandlerFunc) Route
```

`Post` registers a `POST` route.

```go
server.Routes(castor.Post("/todos", func(res http.ResponseWriter, req *http.Request) {
	// create a todo
	res.WriteHeader(http.StatusCreated)
}))
```

### Put

```go
func Put(pattern string, handler http.HandlerFunc) Route
```

`Put` registers a `PUT` route.

```go
server.Routes(castor.Put("/todos/{id}", replaceTodo))
```

### Patch

```go
func Patch(pattern string, handler http.HandlerFunc) Route
```

`Patch` registers a `PATCH` route.

```go
server.Routes(castor.Patch("/todos/{id}", updateTodo))
```

### Delete

```go
func Delete(pattern string, handler http.HandlerFunc) Route
```

`Delete` registers a `DELETE` route.

```go
server.Routes(castor.Delete("/todos/{id}", deleteTodo))
```

### Options

```go
func Options(pattern string, handler http.HandlerFunc) Route
```

`Options` registers an `OPTIONS` route. This is useful when CORS uses `OptionsPassthrough` and the application handles preflight requests itself.

```go
server.Routes(castor.Options("/todos", func(res http.ResponseWriter, req *http.Request) {
	res.WriteHeader(http.StatusNoContent)
}))
```

### Route

```go
type Route func(mux *http.ServeMux, prefix string, middlewares []Middleware)
```

`Route` is a composable piece of the route tree. All route helpers return a `Route`, so routes can be collected in slices and passed to `Routes` together.

```go
routes := []castor.Route{
	castor.Get("/", homeHandler),
	castor.Get("/about", aboutHandler),
}
server := castor.Server().Routes(routes...)
```

### Route.Middlewares

```go
func (n Route) Middlewares(middlewares ...Middleware) Route
```

`Route.Middlewares` returns a route wrapped with middleware that applies only when that route matches.

```go
admin := castor.Get("/admin", adminPage).
	Middlewares(middleware.BasicAuth("Admin", users))
server := castor.Server().Routes(admin)
```

### Group

```go
func Group(prefix string, routes ...Route) RouteGroup
```

`Group` mounts routes under a shared path prefix. It's allow us creating modular web services or web applications.

```go
api := castor.Group("/api",
	castor.Get("/users", listUsers),
	castor.Post("/users", createUser),
)
server := castor.Server().Groups(api)
```

### RouteGroup

```go
type RouteGroup struct {
	Prefix string
	Routes []Route
}
```

`RouteGroup` combines a prefix and routes. Use `Group` to create one, then optionally add middleware with `RouteGroup.Middlewares`.

```go
group := castor.RouteGroup{
	Prefix: "/api",
	Routes: []castor.Route{castor.Get("/users", listUsers)},
}
server := castor.Server().Groups(group)
```

### RouteGroup.Middlewares

```go
func (rg RouteGroup) Middlewares(middlewares ...Middleware) RouteGroup
```

`RouteGroup.Middlewares` returns a copy of the group with middleware that applies to every route in the group.

```go
api := castor.Group("/api",
	castor.Get("/profile", profileHandler),
).Middlewares(middleware.BasicAuth("API", users))

server := castor.Server().Groups(api)
```

### Path patterns

Routes use the standard library's `ServeMux` pattern syntax:

```go
server.Routes(
	castor.Get("/articles/{id}", articleHandler),       // one segment
	castor.Get("/files/{path...}", fileHandler),        // remaining segments
	castor.Get("/exact/", exactTrailingSlashHandler),   // exact trailing slash
)
```

Inside a handler, read wildcards with [`GetPathParam`](#getpathparam). A path that matches but has no matching method produces the standard library's `405 Method Not Allowed` response.

## Requests

### GetPathParam

```go
func GetPathParam(r *http.Request, name string) string
```

`GetPathParam` returns a path wildcard from the route pattern, or `""` when it is absent.

```go
castor.Get("/users/{id}", func(res http.ResponseWriter, req *http.Request) {
	id := castor.GetPathParam(req, "id")
	castor.Json(res, map[string]string{"id": id})
})
```

### GetQueryParam

```go
func GetQueryParam(req *http.Request, name string) string
```

`GetQueryParam` returns the first value of a query parameter, or `""` when it is absent.

```go
filter := castor.GetQueryParam(req, "filter")
castor.Json(res, map[string]string{"filter": filter})
```

### ReadJson

```go
func ReadJson(req *http.Request, data any) error
```

`ReadJson` decodes the request body as JSON into `data`. `data` must be a non-nil pointer. Missing requests, missing bodies, malformed JSON, and JSON type errors are returned as errors.

```go
type createTodo struct {
	Title string `json:"title"`
}

func createTodoHandler(res http.ResponseWriter, req *http.Request) {
	var input createTodo
	if err := castor.ReadJson(req, &input); err != nil {
		http.Error(res, err.Error(), http.StatusBadRequest)
		return
	}
	castor.Json(res, input, http.StatusCreated)
}
```

### ReadFormData

```go
func ReadFormData(req *http.Request) (map[string]string, error)
```

`ReadFormData` parses URL-encoded or multipart form data from the request body. Query parameters are not included. For repeated fields, the first value wins. Files are ignored; use `ReadFormDataWithFiles` when uploads are needed.

```go
fields, err := castor.ReadFormData(req)
if err != nil {
	http.Error(w, "invalid form", http.StatusBadRequest)
	return
}
castor.Json(w, map[string]string{"name": fields["name"]})
```

### ReadFormDataWithFiles

```go
func ReadFormDataWithFiles(req *http.Request) (map[string]string, map[string][]*multipart.FileHeader, error)
```

`ReadFormDataWithFiles` returns both form fields and uploaded files keyed by field name. The same first-value rule applies to fields. Multipart data is kept in memory up to 32 MiB before it is written to temporary files by the standard library.

```go
fields, files, err := castor.ReadFormDataWithFiles(req)
if err != nil {
	http.Error(res, "invalid form", http.StatusBadRequest)
	return
}

if uploads := files["avatar"]; len(uploads) > 0 {
	file, err := uploads[0].Open()
	if err != nil {
		http.Error(res, "cannot read upload", http.StatusBadRequest)
		return
	}
	defer file.Close()
}
castor.Json(res, map[string]string{"name": fields["name"]})
```

## Responses

### Json

```go
func Json(res http.ResponseWriter, data any, status ...int)
```

`Json` marshals `data` as a JSON response, sets `Content-Type: application/json; charset=utf-8`, and writes the status code. With no status argument it uses `200 OK`. 
A marshaling failure produces a plain-text `500 Internal Server Error`.

```go
castor.Json(res, map[string]any{"ok": true})
castor.Json(res, map[string]string{"error": "not found"}, http.StatusNotFound)
```

### NotFound

```go
func NotFound(res http.ResponseWriter, message string)
```

`NotFound` writes `404 Not Found` and the supplied message as a plain response body.

```go
castor.NotFound(res, "Contact not found")
```

When the server has a custom [`NotFound`](#webservernotfound) handler, the body written here is passed to that handler as the original message.

### Redirect

```go
func Redirect(res http.ResponseWriter, uri string, status ...int)
```

`Redirect` sets the `Location` header and writes a redirect status. The default status is `302 Found`.

```go
castor.Redirect(res, "/login")                          // 302 Found
castor.Redirect(res, "/new-location", http.StatusMovedPermanently) // 301
```

## Cookies

`SetCookie`, `SetSignedCookie`, and `RemoveCookie` create cookies with `Path=/` and `HttpOnly` set. The regular cookie lifetime is two hours. Cookies are not marked `Secure` or `SameSite` by these helpers; add those attributes yourself when required.

### SetCookie

```go
func SetCookie(w http.ResponseWriter, name, value string)
```

`SetCookie` writes a root-path, HTTP-only cookie that expires after two hours.

```go
func login(res http.ResponseWriter, req *http.Request) {
	castor.SetCookie(res, "session", "abc123")
	castor.Redirect(res, "/")
}
```

### GetCookie

```go
func GetCookie(r *http.Request, name string) string
```

`GetCookie` returns a cookie's value, or `""` when the cookie is missing.

```go
session := castor.GetCookie(req, "session")
if session == "" {
	castor.Redirect(res, "/auth/login")
	return
}
```

### RemoveCookie

```go
func RemoveCookie(res http.ResponseWriter, name string)
```

`RemoveCookie` expires the named cookie immediately.

```go
castor.RemoveCookie(res, "session")
castor.Redirect(res, "/auth/login")
```

### SetSignedCookie

```go
func SetSignedCookie(res http.ResponseWriter, name, value, secret string)
```

`SetSignedCookie` signs the value with HMAC-SHA256 and stores it as `<base64-payload>.<base64-signature>`. The cookie uses the same defaults as `SetCookie`.

```go
const cookieSecret = "replace-with-a-long-random-secret"

castor.SetSignedCookie(w, "user", "alice", cookieSecret)
```

### GetSignedCookie

```go
func GetSignedCookie(req *http.Request, name, secret string) string
```

`GetSignedCookie` verifies a signed cookie with a constant-time HMAC comparison and returns its decoded value. It returns `""` when the cookie is missing, malformed, or has an invalid signature.

```go
user := castor.GetSignedCookie(req, "user", cookieSecret)
if user == "" {
	http.Error(res, "not signed in", http.StatusUnauthorized)
	return
}
castor.Json(res, map[string]string{"user": user})
```

## Flash messages

Flash messages are short-lived key/value pairs written to a cookie for the next request.

### Message

```go
func Message(key, value string) FlashMessage
```

`Message` creates a `FlashMessage`.

```go
notice := castor.Message("success", "Todo created")
```

### FlashMessage

```go
type FlashMessage struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
```

`FlashMessage` is a single flash key/value pair.

```go
message := castor.FlashMessage{Key: "error", Value: "Title is required"}
```

### Flash

```go
func Flash(res http.ResponseWriter) FlashWriter
```

`Flash` starts the flash-message DSL and returns a `FlashWriter` bound to the response.

```go
castor.Flash(res).Messages(
	castor.Message("success", "Todo created"),
	castor.Message("info", "You can add more"),
)
```

## Context values

### GetValue

```go
func GetValue[T any](ctx context.Context, key any) T
```

`GetValue` retrieves a typed value from a context and returns the zero value when the context is nil, the key is absent, or the stored value has a different type.

```go
type userKey struct{}

ctx := context.WithValue(req.Context(), userKey{}, "alice")
user := castor.GetValue[string](ctx, userKey{})
```

For request-scoped values, a `*castor.ContextKey` is a convenient stable key:

```go
var currentUserKey = &castor.ContextKey{Name: "currentUser"}

func withUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		ctx := context.WithValue(req.Context(), currentUserKey, "alice")
		next.ServeHTTP(res, req.WithContext(ctx))
	})
}

func profile(res http.ResponseWriter, req *http.Request) {
	user := castor.GetValue[string](req.Context(), currentUserKey)
	castor.Json(res, map[string]string{"user": user})
}
```

The middleware package also provides [`middleware.WithValue`](middleware.md#withvalue) for storing a value without writing a custom middleware.

### ContextKey

```go
type ContextKey struct {
	Name string
}
```

`ContextKey` is a named context key. It avoids collisions between context values from different parts of an application.

```go
var tenantKey = &castor.ContextKey{Name: "tenant"}
```

### ContextKey.String

```go
func (k *ContextKey) String() string
```

`ContextKey.String` implements `fmt.Stringer` for debugging context keys. The text keeps the historical `castor context value` prefix used by the implementation.

```go
fmt.Println(tenantKey) // castor context value tenant
```

### GetReqID

```go
func GetReqID(ctx context.Context) string
```

`GetReqID` returns the request ID stored by the request-ID middleware, or `""` when none was stored. It reads the same value as [`middleware.GetReqID`](middleware.md#getreqid).

```go
func handler(res http.ResponseWriter, req *http.Request) {
	castor.Json(res, map[string]string{"request_id": castor.GetReqID(r.Context())})
}
```

See [`middleware.RequestID`](middleware.md#requestid) for installing the middleware.

### RequestIDKey

```go
const RequestIDKey ctxKeyRequestID = 0
```

`RequestIDKey` is the context key used for request IDs. It is exported so applications can read the ID with `GetValue` when they do not want the `GetReqID` helper.

```go
id := castor.GetValue[string](req.Context(), castor.RequestIDKey)
```

### RequestIDHeader

```go
var RequestIDHeader = "X-Request-Id"
```

`RequestIDHeader` is the incoming header read by the request-ID middleware. Change it before serving if your infrastructure uses a different header.

```go
func main() {
	castor.RequestIDHeader = "X-Correlation-Id"
	// create and start the server
}
```

## Logging

Castor's logging support is built from small interfaces. The server installs `Logger` automatically, and `Logger` delegates to the replaceable `DefaultLogger` variable.

### Logger

`Logger` is the request logging installed by `Server`. It delegates to `DefaultLogger`.

`Logger` is already part of the default server chain; the snippet shows the explicit equivalent. Prefer adding only `Recoverer` and customizing `DefaultLogger` instead of adding `Logger` twice.

`DefaultLogger` is called by `Logger`. Replace it before creating/serving the server to customize request logging globally. The default is a `RequestLogger` using a `DefaultLogFormatter` that writes JSON when stdout is a terminal and text otherwise.

```go
castor.DefaultLogger = castor.RequestLogger(&castor.DefaultLogFormatter{
	Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
})

server := castor.Server()
```

### LogFormatter

```go
type LogFormatter interface {
	NewLogEntry(req *http.Request) LogEntry
}
```

`LogFormatter` creates a `LogEntry` for each request. Implement it to customize request log fields.

### DefaultLogFormatter

```go
type DefaultLogFormatter struct {
	Logger LoggerInterface
	Format LogFormat
}
```

`DefaultLogFormatter` is the built-in `LogFormatter`. `Logger` receives formatted output. `Format` is available to custom implementations; the built-in formatter writes through `Logger` and does not interpret `Format`.

```go
formatter := &castor.DefaultLogFormatter{
	Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	Format: castor.TextFormat,
}
```

### DefaultLogFormatter.NewLogEntry

```go
func (f *DefaultLogFormatter) NewLogEntry(req *http.Request) LogEntry
```

`NewLogEntry` creates the `LogEntry` used for one request. It is normally called by `RequestLogger`.

```go
formatter := &castor.DefaultLogFormatter{Logger: slog.Default()}
entry := formatter.NewLogEntry(r)
entry.Info("request started")
```

### LogFormat

```go
type LogFormat int

const (
	JsonFormat = iota + 1
	TextFormat
)
```

`LogFormat` identifies a logging format for formatters that use it. `JsonFormat` and `TextFormat` are the built-in values.

```go
formatter := &castor.DefaultLogFormatter{
	Logger: slog.Default(),
	Format: castor.JsonFormat,
}
```

### LoggerInterface

```go
type LoggerInterface interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}
```

`LoggerInterface` is satisfied by `*slog.Logger` and by any custom structured logger with the same three methods.

```go
logger.Info("server starting", "port", 3000)
```

### LogEntry

`LogEntry` represents one request's logging state. `RequestLogger` logs through `Info`; `Recoverer` calls `Panic` when it recovers a panic.

```go
log := castor.GetLog(req)
log.Info("handling request", "path", r.URL.Path)
```

### WithLogEntry

```go
func WithLogEntry(req *http.Request, entry LogEntry) *http.Request
```

`WithLogEntry` returns a request with `entry` stored in its context. `RequestLogger` uses it automatically.

```go
formatter := &castor.DefaultLogFormatter{Logger: slog.Default()}
r = castor.WithLogEntry(req, formatter.NewLogEntry(r))

castor.Get("/", func(res http.ResponseWriter, req *http.Request) {
	castor.GetLog(req).Info("inside handler")
})
```

### GetLog

```go
func GetLog(req *http.Request) LogEntry
```

`GetLog` returns the `LogEntry` stored in the request context, or `nil` when no logger is installed.

```go
log := castor.GetLog(req)
if log != nil {
	log.Warn("slow dependency", "name", "billing")
}
```
