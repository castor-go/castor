package contacts

import (
	"net/http"

	cg "github.com/castor-go/castor"
)

func contactsHandler(res http.ResponseWriter, req *http.Request) {
	cg.Json(res, getContacts())
}

func viewContactHandler(res http.ResponseWriter, req *http.Request) {
	id := cg.GetPathParam(req, "id")
	for _, c := range getContacts() {
		if c.ID == id {
			cg.Json(res, c)
			return
		}
	}
	cg.NotFound(res, "Contact not found")
}

func Routes() cg.RouteGroup {
	return cg.Group("/contacts",
		cg.Get("/", contactsHandler),
		cg.Get("/{id}", viewContactHandler),
	)
}
