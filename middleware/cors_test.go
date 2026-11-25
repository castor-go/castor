package middleware

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

var corsResponseHeaders = [...]string{
	"Vary",
	"Access-Control-Allow-Origin",
	"Access-Control-Allow-Methods",
	"Access-Control-Allow-Headers",
	"Access-Control-Allow-Credentials",
	"Access-Control-Max-Age",
	"Access-Control-Expose-Headers",
}

func corsRequest(method string, headers map[string]string) *http.Request {
	req := httptest.NewRequest(method, "/", nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	return req
}

func assertCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder, want map[string]string) {
	t.Helper()
	for _, name := range corsResponseHeaders {
		got := strings.Join(rec.Header().Values(name), ", ")
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestCORS(t *testing.T) {
	const (
		origin      = "https://example.com"
		otherOrigin = "https://other.example.com"
	)

	tests := []struct {
		name       string
		opts       CORSOpts
		method     string
		headers    map[string]string
		want       map[string]string
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "actual defaults",
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": "*"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual no origin",
			method:     http.MethodGet,
			want:       map[string]string{"Vary": "Origin"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual empty origin",
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": ""},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": "*"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual exact origin",
			opts:       CORSOpts{AllowedOrigins: []string{origin}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": origin},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual origin case insensitive",
			opts:       CORSOpts{AllowedOrigins: []string{"HTTPS://EXAMPLE.COM"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://Example.com"},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": "https://Example.com"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual wildcard origin",
			opts:       CORSOpts{AllowedOrigins: []string{"https://*.example.com"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://api.example.com"},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": "https://api.example.com"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual wildcard origin case insensitive",
			opts:       CORSOpts{AllowedOrigins: []string{"HTTPS://*.EXAMPLE.COM"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://API.example.com"},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": "https://API.example.com"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual wildcard origin miss",
			opts:       CORSOpts{AllowedOrigins: []string{"https://*.example.com"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://api.example.org"},
			want:       map[string]string{"Vary": "Origin"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual disallowed origin",
			opts:       CORSOpts{AllowedOrigins: []string{origin}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": otherOrigin},
			want:       map[string]string{"Vary": "Origin"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name: "actual allow origin func",
			opts: CORSOpts{AllowOriginFunc: func(r *http.Request, origin string) bool {
				return origin == "https://example.com" && r.Header.Get("X-Token") == "secret"
			}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin, "X-Token": "secret"},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": origin},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name: "actual allow origin func denies",
			opts: CORSOpts{AllowOriginFunc: func(r *http.Request, origin string) bool {
				return r.Header.Get("X-Token") == "secret"
			}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual disallowed method",
			opts:       CORSOpts{AllowedOrigins: []string{origin}, AllowedMethods: []string{http.MethodPost}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual exposed headers",
			opts:       CORSOpts{AllowedOrigins: []string{origin}, ExposedHeaders: []string{"X-Header-1", "x-header-2"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": origin, "Access-Control-Expose-Headers": "X-Header-1, X-Header-2"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:       "actual credentials",
			opts:       CORSOpts{AllowedOrigins: []string{origin}, AllowCredentials: true},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": origin},
			want:       map[string]string{"Vary": "Origin", "Access-Control-Allow-Origin": origin, "Access-Control-Allow-Credentials": "true"},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:   "preflight defaults",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": http.MethodGet,
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight empty origin",
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "",
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": http.MethodGet,
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight wildcard origin",
			opts:   CORSOpts{AllowedOrigins: []string{"*"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": http.MethodGet,
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight credentials",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowCredentials: true},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                             "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":      origin,
				"Access-Control-Allow-Methods":     http.MethodGet,
				"Access-Control-Allow-Credentials": "true",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight max age",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, MaxAge: 10},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
				"Access-Control-Max-Age":       "10",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight disallowed origin",
			opts:   CORSOpts{AllowedOrigins: []string{origin}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        otherOrigin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary": "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight disallowed method",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowedMethods: []string{http.MethodPost}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodPatch,
			},
			want: map[string]string{
				"Vary": "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name: "preflight method case insensitive",
			opts: CORSOpts{
				AllowedOrigins: []string{origin},
				AllowedMethods: []string{"get"},
			},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": "GET",
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight disallowed headers",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowedHeaders: []string{"X-Header-1"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         origin,
				"Access-Control-Request-Method":  http.MethodGet,
				"Access-Control-Request-Headers": "X-Header-3, X-Header-1",
			},
			want: map[string]string{
				"Vary": "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight allowed headers",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowedHeaders: []string{"X-Header-1", "x-header-2"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         origin,
				"Access-Control-Request-Method":  http.MethodGet,
				"Access-Control-Request-Headers": "X-Header-2, X-HEADER-1",
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
				"Access-Control-Allow-Headers": "X-Header-2, X-Header-1",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight default headers",
			opts:   CORSOpts{AllowedOrigins: []string{origin}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         origin,
				"Access-Control-Request-Method":  http.MethodGet,
				"Access-Control-Request-Headers": "content-type",
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
				"Access-Control-Allow-Headers": "Content-Type",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight wildcard headers",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowedHeaders: []string{"*"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         origin,
				"Access-Control-Request-Method":  http.MethodGet,
				"Access-Control-Request-Headers": "X-Header-2, X-HEADER-1",
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
				"Access-Control-Allow-Headers": "X-Header-2, X-Header-1",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight origin header always allowed",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, AllowedHeaders: []string{"X-Header-1"}},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                         origin,
				"Access-Control-Request-Method":  http.MethodGet,
				"Access-Control-Request-Headers": "origin",
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
				"Access-Control-Allow-Headers": "Origin",
			},
			wantStatus: http.StatusOK,
			wantNext:   false,
		},
		{
			name:   "preflight passthrough",
			opts:   CORSOpts{AllowedOrigins: []string{origin}, OptionsPassthrough: true},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        origin,
				"Access-Control-Request-Method": http.MethodGet,
			},
			want: map[string]string{
				"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
				"Access-Control-Allow-Origin":  origin,
				"Access-Control-Allow-Methods": http.MethodGet,
			},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:    "options without request method",
			opts:    CORSOpts{AllowedOrigins: []string{origin}},
			method:  http.MethodOptions,
			headers: map[string]string{"Origin": origin},
			want: map[string]string{
				"Vary":                        "Origin",
				"Access-Control-Allow-Origin": origin,
			},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
		{
			name:    "options without origin",
			opts:    CORSOpts{AllowedOrigins: []string{origin}},
			method:  http.MethodOptions,
			headers: map[string]string{"Access-Control-Request-Method": http.MethodGet},
			want: map[string]string{
				"Vary": "Origin",
			},
			wantStatus: http.StatusNoContent,
			wantNext:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			})

			rec := serve(CORS(test.opts)(next), corsRequest(test.method, test.headers))
			if rec.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, test.wantStatus)
			}
			if nextCalled != test.wantNext {
				t.Errorf("next called = %v, want %v", nextCalled, test.wantNext)
			}
			assertCORSHeaders(t, rec, test.want)
		})
	}
}

func TestCORSAllowAll(t *testing.T) {
	nextCalled := false
	handler := CORSAllowAll()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	}))

	req := corsRequest(http.MethodOptions, map[string]string{
		"Origin":                         "https://example.com",
		"Access-Control-Request-Method":  http.MethodPut,
		"Access-Control-Request-Headers": "X-Custom",
	})
	rec := serve(handler, req)
	if rec.Code != http.StatusOK || nextCalled {
		t.Fatalf("preflight: status=%d next=%v", rec.Code, nextCalled)
	}
	assertCORSHeaders(t, rec, map[string]string{
		"Vary":                         "Origin, Access-Control-Request-Method, Access-Control-Request-Headers",
		"Access-Control-Allow-Origin":  "*",
		"Access-Control-Allow-Methods": http.MethodPut,
		"Access-Control-Allow-Headers": "X-Custom",
	})

	nextCalled = false
	rec = serve(handler, corsRequest(http.MethodGet, map[string]string{"Origin": "https://example.com"}))
	if rec.Code != http.StatusOK || !nextCalled {
		t.Fatalf("actual: status=%d next=%v", rec.Code, nextCalled)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q", got)
	}
}

func TestCORSPreflightHeaderTokenPunctuation(t *testing.T) {
	for _, punctuation := range "!#$%&'*+-.^_`|~" {
		t.Run(string(punctuation), func(t *testing.T) {
			name := "X" + string(punctuation) + "Custom"
			handler := CORS(CORSOpts{
				AllowedOrigins: []string{"https://example.com"},
				AllowedMethods: []string{http.MethodPost},
				AllowedHeaders: []string{"Content-Type", name},
			})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

			rec := serve(handler, corsRequest(http.MethodOptions, map[string]string{
				"Origin":                         "https://example.com",
				"Access-Control-Request-Method":  http.MethodPost,
				"Access-Control-Request-Headers": "content-type, " + strings.ToLower(name),
			}))
			if got, want := rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type, "+http.CanonicalHeaderKey(name); got != want {
				t.Fatalf("allow headers = %q, want %q", got, want)
			}
		})
	}
}

func TestCORSMultipleRequestHeaderValues(t *testing.T) {
	handler := CORS(CORSOpts{
		AllowedOrigins: []string{"https://example.com"},
		AllowedHeaders: []string{"X-One", "X-Two"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Add("Access-Control-Request-Headers", "X-One")
	req.Header.Add("Access-Control-Request-Headers", "X-Two")

	rec := serve(handler, req)
	if got, want := rec.Header().Get("Access-Control-Allow-Headers"), "X-One, X-Two"; got != want {
		t.Fatalf("allow headers = %q, want %q", got, want)
	}
}

func TestParseCORSHeaderList(t *testing.T) {
	tests := []struct {
		name  string
		value []string
		want  []string
	}{
		{"empty", nil, nil},
		{"blank", []string{" , "}, nil},
		{"headers", []string{"header, second-header, THIRD-HEADER, Numb3r3d-H34d3r"}, []string{"Header", "Second-Header", "Third-Header", "Numb3r3d-H34d3r"}},
		{"space separated", []string{"Header_with_underscore Header.with.full.stop"}, []string{"Header_with_underscore", "Header.with.full.stop"}},
		{"multiple values", []string{"X-One", "x-two, X-THREE"}, []string{"X-One", "X-Two", "X-Three"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := parseHeaderList(test.value); !slices.Equal(got, test.want) {
				t.Fatalf("parseHeaderList(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
