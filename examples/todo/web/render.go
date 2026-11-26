package web

import (
	"context"
	"net/http"
	"strings"

	"github.com/a-h/templ"
)

func IsHtmx(req *http.Request) bool {
	return req.Header.Get("Hx-Request") != ""
}

func Render(res http.ResponseWriter, components ...templ.Component) {
	var out strings.Builder
	for _, c := range components {
		c.Render(context.Background(), &out)
	}
	res.WriteHeader(http.StatusOK)
	res.Write([]byte(out.String()))
}
