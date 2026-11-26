package main

import (
	"basic/contacts"
	"fmt"
	"log"
	"net/http"

	"github.com/castor-go/castor"
	"github.com/castor-go/castor/middleware"
)

func indexHandler(res http.ResponseWriter, req *http.Request) {
	castor.Json(res, map[string]string{
		"status":  "OK",
		"message": "Castor!",
	})
}

func greetHandler(res http.ResponseWriter, req *http.Request) {
	name := castor.GetPathParam(req, "name")
	castor.Json(res, map[string]string{
		"message": fmt.Sprintf("Hey %s!", name),
	})
}

func main() {
	server := castor.Server().
		Middlewares(
			middleware.StripSlashes,
			middleware.Heartbeat("/ping"),
			middleware.Recoverer,
		).
		Routes(
			castor.Get("/", indexHandler),
			castor.Get("/greet/{name}", greetHandler),
		).Groups(contacts.Routes())

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
