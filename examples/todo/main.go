package main

import (
	"log"
	"todo/web"

	wg "github.com/webgo/webgo"
)

func main() {
	server := wg.Server().
		StaticAssets("/assets/**", "./public", wg.StaticConfig{
			ETag: true,
		}).
		Routes(
			wg.Get("/", web.IndexHandler),
			wg.Post("/todos", web.AddTaskHandler),
			wg.Post("/clear", web.ClearTasksHandler),
			wg.Patch("/{id}", web.CompleteTaskHandler),
			wg.Delete("/{id}", web.DestroyTaskHandler),
		)

	if err := server.Start(); err != nil {
		log.Fatal(err)
	}
}
