package main

import (
	"log"
	"todo/web"

	cg "github.com/castor-go/castor"
)

func main() {
	server := cg.Server().
		StaticAssets("/assets/**", "./public", cg.StaticConfig{
			ETag: true,
		}).
		Routes(
			cg.Get("/", web.IndexHandler),
			cg.Post("/todos", web.AddTaskHandler),
			cg.Post("/clear", web.ClearTasksHandler),
			cg.Patch("/{id}", web.CompleteTaskHandler),
			cg.Delete("/{id}", web.DestroyTaskHandler),
		)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
