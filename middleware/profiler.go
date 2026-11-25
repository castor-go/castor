//go:build !tinygo

package middleware

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"strings"
)

// Profiler returns a handler serving net/http/pprof and expvar endpoints.
// Mount it behind http.StripPrefix, for example at "/debug".
func Profiler() http.Handler {
	mux := http.NewServeMux()

	requestURI := func(r *http.Request) string {
		uri, _, _ := strings.Cut(r.RequestURI, "?")
		if uri == "" {
			uri = r.URL.Path
		}
		// Keep redirects local even if RequestURI is absolute or protocol-relative.
		uri = strings.ReplaceAll(uri, `\`, "/")
		return "/" + strings.TrimLeft(uri, "/")
	}
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		target := strings.TrimSuffix(requestURI(r), "/") + "/pprof/"
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
	mux.HandleFunc("/pprof", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, requestURI(r)+"/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/pprof/", pprof.Index)
	mux.HandleFunc("/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/pprof/profile", pprof.Profile)
	mux.HandleFunc("/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/pprof/trace", pprof.Trace)
	mux.Handle("/vars", expvar.Handler())

	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		mux.Handle("/pprof/"+name, pprof.Handler(name))
	}

	return NoCache(mux)
}
