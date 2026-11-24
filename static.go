package castor

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// StaticConfig configures static asset serving.
type StaticConfig struct {
	CacheMaxAge time.Duration // Cache-Control max-age; zero omits the header.
	ETag        bool          // Enables ETag generation and If-None-Match handling.
}

func staticNode(pattern, dir string, cfgs ...StaticConfig) Route {
	cfg := StaticConfig{}
	if len(cfgs) > 0 {
		cfg = cfgs[0]
	}

	return func(mux *http.ServeMux, prefix string, middlewares []Middleware) {
		route := joinPath(prefix, pattern)
		route = strings.TrimSuffix(route, "**")
		route = strings.TrimSuffix(route, "*")
		if !strings.HasSuffix(route, "/") {
			route += "/"
		}

		patternPath := strings.TrimSuffix(route, "/")
		fsHandler := http.StripPrefix(patternPath, http.FileServer(http.Dir(dir)))
		mux.Handle(http.MethodGet+" "+route, chain(staticHandler(fsHandler, dir, patternPath, cfg), middlewares))
	}
}

func staticHandler(next http.Handler, dir, pattern string, cfg StaticConfig) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if cfg.CacheMaxAge > 0 {
			res.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", cfg.CacheMaxAge/time.Second))
		}
		if cfg.ETag {
			if etag, ok := fileETag(pattern, dir, req.URL.Path); ok {
				res.Header().Set("ETag", etag)
				if req.Header.Get("If-None-Match") == etag {
					res.WriteHeader(http.StatusNotModified)
					return
				}
			}
		}

		next.ServeHTTP(res, req)
	})
}

func fileETag(pattern, dir, urlPath string) (string, bool) {
	clean := path.Clean("/" + urlPath)
	if !strings.HasPrefix(clean, pattern) {
		return "", false
	}

	resource, _ := strings.CutPrefix(clean, pattern)
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(resource)))
	if err != nil || info.IsDir() {
		return "", false
	}
	return fmt.Sprintf(`"%x-%x"`, info.ModTime().UnixNano(), info.Size()), true
}
