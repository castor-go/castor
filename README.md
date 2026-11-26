# Castor

Castor is a small, function-based DSL for building HTTP services in Go using only the standard library. Servers, routes, middleware, and helpers are ordinary Go functions, so applications stay close to `net/http` without giving up convenient composition.

- Go 1.27+
- No third-party dependencies
- Built-in routing, middleware, JSON, forms, cookies, logging, and static-file serving

## Motivation

Castor was built with simple mindset, define a library that uses Golang standard library, to build stable, reliable, scalable, maintainable and composable applications. 
After using it to build production projects and battle tested, I decided to share it as open-source.

> Note: Castor was not built to be the fastest library to built web services or web applications. However, if you can contribute, just open a PR ❤️

## Installation

```bash
go get github.com/castor-go/castor
```

## Build a basic app

Create a new module and a `main.go`:

```bash
mkdir hello-castor
cd hello-castor
go mod init hello-castor
go get github.com/castor-go/castor
```

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
				castor.Json(res, map[string]string{
					"message": "Hello, Castor!",
				})
			}),
			castor.Get("/hello/{name}", func(res http.ResponseWriter, req *http.Request) {
				castor.Json(res, map[string]string{
					"message": "Hello, " + castor.GetPathParam(req, "name") + "!",
				})
			}),
		)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
```

The server listens on `0.0.0.0:3000`. Use `castor.Config(castor.ServerConfig{...})` when you need the complete configuration, including timeouts.

Run the app:

```bash
go run .
```

Try the routes from another terminal:

```bash
curl http://localhost:3000/
# {"message":"Hello, Castor!"}

curl http://localhost:3000/hello/Ada
# {"message":"Hello, Ada!"}
```

Press `Ctrl+C` to stop the server. Castor handles the signal and shuts down gracefully.

## Documentation

- [Core library reference](docs/castor.md) — servers, routing, requests, responses, cookies, flash messages, context, and logging.
- [Middleware reference](docs/middleware.md) — applying middleware, CORS, authentication, compression, request IDs, client IPs, throttling, and more.

## Examples

- [Basic](https://github.com/castor-go/castor/tree/main/examples/basic)
- [TODO](https://github.com/castor-go/castor/tree/main/examples/todo)

## License

MIT — see [LICENSE](LICENSE).
