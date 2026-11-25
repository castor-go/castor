package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/castor-go/castor"
)

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestBasicAuth(t *testing.T) {
	h := BasicAuth("test", map[string]string{"alice": "secret"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := serve(h, req)
	if rec.Code != http.StatusUnauthorized || !strings.Contains(rec.Header().Get("WWW-Authenticate"), `realm="test"`) {
		t.Fatalf("unauthenticated response: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("alice", "wrong")
	if rec = serve(h, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("alice", "secret")
	if rec = serve(h, req); rec.Code != http.StatusNoContent {
		t.Fatalf("valid auth: %d", rec.Code)
	}
}

func TestContentRules(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	tests := []struct {
		name    string
		handler http.Handler
		body    string
		header  [2]string
		want    int
	}{
		{"content type", AllowContentType("application/json")(ok), "{}", [2]string{"Content-Type", "application/json; charset=utf-8"}, 204},
		{"bad content type", AllowContentType("application/json")(ok), "{}", [2]string{"Content-Type", "text/plain"}, 415},
		{"empty body", AllowContentType("application/json")(ok), "", [2]string{}, 204},
		{"content encoding", AllowContentEncoding("gzip")(ok), "x", [2]string{"Content-Encoding", "gzip"}, 204},
		{"bad content encoding", AllowContentEncoding("gzip")(ok), "x", [2]string{"Content-Encoding", "br"}, 415},
		{"mixed content encoding", AllowContentEncoding("gzip")(ok), "x", [2]string{"Content-Encoding", "gzip, br"}, 415},
		{"charset", ContentCharset("utf-8")(ok), "{}", [2]string{"Content-Type", "application/json; charset=UTF-8"}, 204},
		{"bad charset", ContentCharset("utf-8")(ok), "{}", [2]string{"Content-Type", "application/json"}, 415},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			if test.header[0] != "" {
				req.Header.Set(test.header[0], test.header[1])
			}
			if rec := serve(test.handler, req); rec.Code != test.want {
				t.Fatalf("code = %d, want %d", rec.Code, test.want)
			}
		})
	}
}

func TestSimpleHeaderMiddleware(t *testing.T) {
	rec := serve(SetHeader("X-Test", "yes")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("X-Test") != "yes" {
		t.Fatal("SetHeader did not set header")
	}

	rec = serve(NoCache(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Cache-Control") == "" || rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("NoCache headers: %#v", rec.Header())
	}

	when := time.Unix(1_700_000_000, 0).UTC()
	rec = serve(Deprecation(when, `<https://example.com/migrate>; rel="deprecation"`)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Deprecation") != "@1700000000" || rec.Header().Get("Link") == "" {
		t.Fatalf("Deprecation headers: %#v", rec.Header())
	}

	rec = serve(Sunset(when)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Header().Get("Sunset") != when.Format(http.TimeFormat) {
		t.Fatalf("Sunset = %q", rec.Header().Get("Sunset"))
	}
}

func TestHeartbeatPageRouteMaybeValue(t *testing.T) {
	nextCalled := false
	h := Heartbeat("/ping")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { nextCalled = true }))
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/PING", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "." || nextCalled {
		t.Fatalf("heartbeat: code=%d body=%q next=%v", rec.Code, rec.Body.String(), nextCalled)
	}

	nextCalled = false
	rec = serve(h, httptest.NewRequest(http.MethodPost, "/ping", nil))
	if !nextCalled || rec.Code != http.StatusOK {
		t.Fatalf("heartbeat POST should pass through: %d, %v", rec.Code, nextCalled)
	}

	h = PageRoute("/page", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }),
	)
	if rec = serve(h, httptest.NewRequest(http.MethodGet, "/page", nil)); rec.Code != http.StatusTeapot {
		t.Fatalf("page route: %d", rec.Code)
	}

	applied := false
	h = Maybe(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { applied = true; next.ServeHTTP(w, r) })
	}, func(r *http.Request) bool { return r.URL.Query().Get("yes") == "1" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	serve(h, httptest.NewRequest(http.MethodGet, "/?yes=1", nil))
	if !applied {
		t.Fatal("Maybe did not apply middleware")
	}

	key := &castor.ContextKey{Name: "value"}
	var got string
	h = WithValue(key, "hello")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = castor.GetValue[string](r.Context(), key)
	}))
	serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if got != "hello" {
		t.Fatalf("context value = %q", got)
	}
}

func TestPathMiddleware(t *testing.T) {
	var gotPath, gotFormat string
	h := CleanPath(URLFormat(PathRewrite("/old", "/new")(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath, gotFormat = r.URL.Path, GetURLFormat(r)
		}),
	)))

	serve(h, httptest.NewRequest(http.MethodGet, "/old//articles/1.json", nil))
	if gotPath != "/new/articles/1" || gotFormat != "json" {
		t.Fatalf("path=%q format=%q", gotPath, gotFormat)
	}

	gotPath = ""
	h = StripSlashes(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gotPath = r.URL.Path }))
	serve(h, httptest.NewRequest(http.MethodGet, "/users/", nil))
	if gotPath != "/users" {
		t.Fatalf("strip slash path = %q", gotPath)
	}

	rec := serve(RedirectSlashes(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), httptest.NewRequest(http.MethodGet, "/users/?a=1", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/users?a=1" {
		t.Fatalf("redirect slash: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestRequestID(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = GetReqID(r.Context()) }))

	serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if got == "" || !strings.Contains(got, "/") {
		t.Fatalf("generated request ID = %q", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(castor.RequestIDHeader, "incoming")
	serve(h, req)
	if got != "incoming" {
		t.Fatalf("request ID header = %q", got)
	}
}

func TestURLFormatBeforeMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /articles/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id := r.PathValue("id"); id != "42" {
			t.Errorf("path value = %q", id)
		}
		if format := GetURLFormat(r); format != "json" {
			t.Errorf("format = %q", format)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	rec := serve(URLFormat(mux), httptest.NewRequest(http.MethodGet, "/articles/42.json", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("URLFormat routed status = %d", rec.Code)
	}
}

func TestRequestSize(t *testing.T) {
	var readErr error
	h := RequestSize(4)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	serve(h, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("12345")))
	if readErr == nil {
		t.Fatal("expected body size error")
	}
}

func TestRouteHeaders(t *testing.T) {
	mark := func(value string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Matched", value)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := RouteHeaders().
		Route("X-Role", "admin", mark("admin")).
		Route("X-Role", "user-*", mark("user")).
		RouteAny("X-Env", []string{"prod", "staging"}, mark("env")).
		RouteDefault(mark("default")).
		Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	check := func(header, value, want string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set(header, value)
		}
		if got := serve(h, req).Header().Get("X-Matched"); got != want {
			t.Fatalf("%s=%q matched %q, want %q", header, value, got, want)
		}
	}
	check("X-Role", "admin", "admin")
	check("X-Role", "user-42", "user")
	check("X-Env", "prod", "env")
	check("", "", "default")

	hostRouter := RouteHeaders().Route("Host", "*.example.com", mark("host")).Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/", nil)
	if got := serve(hostRouter, req).Header().Get("X-Matched"); got != "host" {
		t.Fatalf("Host route matched %q", got)
	}
}

func TestClientIP(t *testing.T) {
	var got string
	h := ClientIPFromXFF("10.0.0.0/8")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.1.2.3:4567"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.2.3.4")
	serve(h, req)
	if got != "203.0.113.9" {
		t.Fatalf("XFF client = %q", got)
	}

	got = ""
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.5:4567"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	serve(h, req)
	if got != "198.51.100.5" {
		t.Fatalf("untrusted peer client = %q", got)
	}

	got = ""
	h = ClientIPFromHeader("X-Real-IP")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
	}))
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "2001:db8::1")
	serve(h, req)
	if got != "2001:db8::1" {
		t.Fatalf("header client = %q", got)
	}

	got = ""
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.10:1234"
	serve(ClientIPFromRemoteAddr(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = GetClientIP(r.Context())
	})), req)
	if got != "192.0.2.10" {
		t.Fatalf("remote addr client = %q", got)
	}
}

type readerFromRecorder struct {
	*httptest.ResponseRecorder
}

func (r *readerFromRecorder) ReadFrom(src io.Reader) (int64, error) {
	return io.Copy(r.ResponseRecorder.Body, src)
}

func TestWrapResponseWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := castor.NewWrapResponseWriter(rec, 1)
	if _, ok := wrapped.(http.Flusher); !ok {
		t.Fatal("wrapper must implement http.Flusher")
	}

	n, err := wrapped.Write([]byte("hello"))
	if err != nil || n != 5 || wrapped.Status() != http.StatusOK || wrapped.BytesWritten() != 5 || rec.Body.String() != "hello" {
		t.Fatalf("write: n=%d err=%v status=%d bytes=%d body=%q", n, err, wrapped.Status(), wrapped.BytesWritten(), rec.Body.String())
	}

	rec = httptest.NewRecorder()
	wrapped = castor.NewWrapResponseWriter(rec, 1)
	var tee bytes.Buffer
	wrapped.Tee(&tee)
	wrapped.Discard()
	_, _ = wrapped.Write([]byte("hidden"))
	if rec.Body.Len() != 0 || tee.String() != "hidden" || wrapped.BytesWritten() != 6 {
		t.Fatalf("discard: body=%q tee=%q bytes=%d", rec.Body.String(), tee.String(), wrapped.BytesWritten())
	}

	source := &readerFromRecorder{ResponseRecorder: httptest.NewRecorder()}
	wrapped = castor.NewWrapResponseWriter(source, 1)
	copied, err := io.Copy(wrapped, strings.NewReader("fast"))
	if err != nil || copied != 4 || wrapped.BytesWritten() != 4 || source.Body.String() != "fast" {
		t.Fatalf("readfrom: n=%d err=%v bytes=%d body=%q", copied, err, wrapped.BytesWritten(), source.Body.String())
	}
}

func TestRecovererAbortHandler(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", recovered)
		}
	}()
	h := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Fatal("expected panic")
}

func TestCompress(t *testing.T) {
	body := `{"ok":true}`
	h := Compress(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := serve(h, req)
	if rec.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Fatalf("gzip headers: %#v", rec.Header())
	}
	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil || string(decoded) != body {
		t.Fatalf("gzip body: %q err=%v", decoded, err)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip;q=0")
	rec = serve(h, req)
	if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != body {
		t.Fatalf("q=0 should not compress: headers=%#v body=%q", rec.Header(), rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = serve(h, req)
	if rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != body {
		t.Fatalf("no accept-encoding should not compress: %#v %q", rec.Header(), rec.Body.String())
	}

	nobody := Compress(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	}))
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = serve(nobody, req)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Content-Encoding") != "" {
		t.Fatalf("204 compression: code=%d headers=%#v", rec.Code, rec.Header())
	}
}

func TestThrottle(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := Throttle(1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))

	first := make(chan int, 1)
	go func() {
		rec := serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
		first <- rec.Code
	}()
	<-started

	rec := serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttle second request: %d", rec.Code)
	}
	close(release)
	if code := <-first; code != http.StatusNoContent {
		t.Fatalf("throttle first request: %d", code)
	}
}

func TestTimeout(t *testing.T) {
	h := Timeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("timeout code: %d", rec.Code)
	}

	h = Timeout(20 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	rec = serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("completed request code: %d", rec.Code)
	}
}

func TestSupressNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	called := false
	h := SupressNotFound(mux)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := serve(h, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound || called {
		t.Fatalf("missing: code=%d called=%v", rec.Code, called)
	}

	rec = serve(h, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("existing: code=%d called=%v", rec.Code, called)
	}

	called = false
	rec = serve(h, httptest.NewRequest(http.MethodPost, "/ok", nil))
	if rec.Code != http.StatusMethodNotAllowed || called {
		t.Fatalf("method: code=%d called=%v", rec.Code, called)
	}
}

func TestProfiler(t *testing.T) {
	h := Profiler()
	rec := serve(h, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/pprof/" {
		t.Fatalf("profiler root: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = serve(h, httptest.NewRequest(http.MethodGet, "/pprof", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/pprof/" {
		t.Fatalf("profiler pprof redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = serve(h, httptest.NewRequest(http.MethodGet, "/pprof/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("profiler index: %d", rec.Code)
	}

	mounted := http.StripPrefix("/debug", Profiler())
	rec = serve(mounted, httptest.NewRequest(http.MethodGet, "/debug/", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/debug/pprof/" {
		t.Fatalf("mounted profiler: %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestNewIsAHandlerMiddleware(t *testing.T) {
	h := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) }))
	rec := serve(h(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("New middleware: %d", rec.Code)
	}
}

func TestConcurrencySmoke(t *testing.T) {
	var wg sync.WaitGroup
	h := Compress(1)(RequestID(Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok")
	}))))
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			rec := serve(h, req)
			if rec.Code != http.StatusOK {
				t.Errorf("concurrent status: %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}
