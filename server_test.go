package castor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveTestRequest(server *WebServer, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	server.handler().ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestDefaultNotFound(t *testing.T) {
	server := Server().Routes(Get("/", func(http.ResponseWriter, *http.Request) {}))
	if server.notFound == nil {
		t.Fatal("server must have a default not-found handler")
	}

	rec := serveTestRequest(server, http.MethodGet, "/missing")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
		t.Fatalf("unmatched: code=%d body=%q", rec.Code, rec.Body.String())
	}

	server = Server().Routes(Get("/domain-miss", func(w http.ResponseWriter, r *http.Request) {
		NotFound(w, "domain not found")
	}))
	rec = serveTestRequest(server, http.MethodGet, "/domain-miss")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "domain not found" {
		t.Fatalf("route 404: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCustomNotFound(t *testing.T) {
	called := 0
	var gotCode int
	var gotMessage string
	server := Server().
		Middlewares(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Middleware", "yes")
				next.ServeHTTP(w, r)
			})
		}).
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			called++
			gotCode, gotMessage = statusCode, message
			w.Header().Set("X-NotFound", "yes")
			w.WriteHeader(statusCode)
			_, _ = io.WriteString(w, "custom not found")
		}).
		Routes(Get("/", func(http.ResponseWriter, *http.Request) {}))

	rec := serveTestRequest(server, http.MethodGet, "/missing")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "custom not found" {
		t.Fatalf("custom not found: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("not-found calls = %d, want 1", called)
	}
	if gotCode != http.StatusNotFound || gotMessage != "404 page not found\n" {
		t.Fatalf("original details: code=%d message=%q", gotCode, gotMessage)
	}
	if rec.Header().Get("X-NotFound") != "yes" || rec.Header().Get("X-Middleware") != "yes" {
		t.Fatalf("headers = %#v", rec.Header())
	}
}

func TestCustomNotFoundHandlesRoute404(t *testing.T) {
	called := 0
	var gotCode int
	var gotMessage string
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			called++
			gotCode, gotMessage = statusCode, message
			w.Header().Set("X-Custom", "yes")
			w.WriteHeader(statusCode)
			_, _ = io.WriteString(w, "custom: "+message)
		}).
		Routes(Get("/contacts/{id}", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Route", "discard-me")
			NotFound(w, "Contact not found")
		}))

	rec := serveTestRequest(server, http.MethodGet, "/contacts/999")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "custom: Contact not found" {
		t.Fatalf("route 404: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("not-found calls = %d, want 1", called)
	}
	if gotCode != http.StatusNotFound || gotMessage != "Contact not found" {
		t.Fatalf("original details: code=%d message=%q", gotCode, gotMessage)
	}
	if rec.Header().Get("X-Custom") != "yes" {
		t.Fatalf("missing custom header: %#v", rec.Header())
	}
	if rec.Header().Get("X-Route") != "" {
		t.Fatalf("route 404 headers leaked: %#v", rec.Header())
	}
}

func TestCustomNotFoundHandlesHTTPError(t *testing.T) {
	var gotMessage string
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			gotMessage = message
			w.WriteHeader(statusCode)
			_, _ = io.WriteString(w, "handled")
		}).
		Routes(Get("/missing-entity", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "entity missing", http.StatusNotFound)
		}))

	rec := serveTestRequest(server, http.MethodGet, "/missing-entity")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "handled" {
		t.Fatalf("http.Error 404: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if gotMessage != "entity missing\n" {
		t.Fatalf("original message = %q", gotMessage)
	}
}

func TestCustomNotFoundBuffersFullMessage(t *testing.T) {
	var gotMessage string
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			gotMessage = message
			w.WriteHeader(statusCode)
		}).
		Routes(Get("/split", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "part one ")
			_, _ = io.WriteString(w, "part two")
		}))

	rec := serveTestRequest(server, http.MethodGet, "/split")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if gotMessage != "part one part two" {
		t.Fatalf("message = %q", gotMessage)
	}
}

func TestNotFoundPreservesRoutesAndPathValues(t *testing.T) {
	customCalled := false
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			customCalled = true
			w.WriteHeader(statusCode)
		}).
		Routes(Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, GetPathParam(r, "id"))
		}))

	rec := serveTestRequest(server, http.MethodGet, "/users/42")
	if rec.Code != http.StatusOK || rec.Body.String() != "42" {
		t.Fatalf("matched route: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if customCalled {
		t.Fatal("not-found handler called for a matched route")
	}
}

func TestNotFoundPreservesMethodNotAllowed(t *testing.T) {
	customCalled := false
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			customCalled = true
			w.WriteHeader(http.StatusTeapot)
		}).
		Routes(Get("/only", func(http.ResponseWriter, *http.Request) {}))

	rec := serveTestRequest(server, http.MethodPost, "/only")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if allow := rec.Header().Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Fatalf("Allow = %q", allow)
	}
	if customCalled {
		t.Fatal("not-found handler called for a method-not-allowed response")
	}
}

func TestNotFoundDoesNotOverrideNon404(t *testing.T) {
	customCalled := false
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			customCalled = true
		}).
		Routes(Get("/created", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "created")
		}))

	rec := serveTestRequest(server, http.MethodGet, "/created")
	if rec.Code != http.StatusCreated || rec.Body.String() != "created" {
		t.Fatalf("non-404: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if customCalled {
		t.Fatal("not-found handler called for a non-404 response")
	}
}

func TestNotFoundNilRestoresDefault(t *testing.T) {
	customCalled := false
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			customCalled = true
		}).
		NotFound(nil).
		Routes(
			Get("/", func(http.ResponseWriter, *http.Request) {}),
			Get("/domain-miss", func(w http.ResponseWriter, r *http.Request) {
				NotFound(w, "domain not found")
			}),
		)

	rec := serveTestRequest(server, http.MethodGet, "/missing")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "404 page not found\n" {
		t.Fatalf("unmatched: code=%d body=%q", rec.Code, rec.Body.String())
	}

	rec = serveTestRequest(server, http.MethodGet, "/domain-miss")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "domain not found" {
		t.Fatalf("route 404: code=%d body=%q", rec.Code, rec.Body.String())
	}

	if customCalled {
		t.Fatal("custom not-found handler called after reset")
	}
}

func TestCustomNotFoundHandlesStatic404(t *testing.T) {
	called := 0
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			called++
			w.WriteHeader(statusCode)
			_, _ = io.WriteString(w, "static custom")
		}).
		StaticAssets("/assets/**", t.TempDir())

	rec := serveTestRequest(server, http.MethodGet, "/assets/missing.txt")
	if rec.Code != http.StatusNotFound || rec.Body.String() != "static custom" {
		t.Fatalf("static 404: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if called != 1 {
		t.Fatalf("not-found calls = %d, want 1", called)
	}
}

func TestNotFoundCanSetStatus(t *testing.T) {
	server := Server().
		NotFound(func(w http.ResponseWriter, statusCode int, message string) {
			w.WriteHeader(http.StatusGone)
			_, _ = io.WriteString(w, "gone")
		}).
		Routes(Get("/", func(http.ResponseWriter, *http.Request) {}))

	rec := serveTestRequest(server, http.MethodGet, "/missing")
	if rec.Code != http.StatusGone || rec.Body.String() != "gone" {
		t.Fatalf("custom status: code=%d body=%q", rec.Code, rec.Body.String())
	}
}
