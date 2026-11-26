package web

import (
	"net/http"
	"todo/pages"
	"todo/todos"

	cg "github.com/castor-go/castor"
)

func IndexHandler(res http.ResponseWriter, req *http.Request) {
	filter := cg.GetQueryParam(req, "filter")
	if filter != "" && IsHtmx(req) {
		todoList := todos.ApplyFilter(filter)
		todos := pages.TodoList(todos.ApplyFilter(filter))
		fragment := pages.Partial("#filters", pages.TodoFilters(filter, todoList))
		Render(res, todos, fragment)
		return
	}

	// full page load
	page := pages.Index(todos.GetTasks(), filter)
	page.Render(req.Context(), res)
}

func AddTaskHandler(res http.ResponseWriter, req *http.Request) {
	logger := cg.GetLog(req)
	filter := cg.GetQueryParam(req, "filter")

	data, err := cg.ReadFormData(req)
	if err != nil {
		logger.Error("fail to read request body", "cause", err.Error())
		return
	}

	newTask := todos.AddTask(data["name"])
	logger.Info("New Task", "title", newTask.Name)

	todoList := todos.GetTasks()
	todos := pages.TodoList(todoList)
	fragment := pages.Partial("#filters", pages.TodoFilters(filter, todoList))
	Render(res, todos, fragment)
}

func CompleteTaskHandler(res http.ResponseWriter, req *http.Request) {
	id := cg.GetPathParam(req, "id")
	if id == "" {
		res.WriteHeader(http.StatusNoContent)
		return
	}

	task := todos.CompleteTask(id)
	item := pages.TaskItem(task)
	filterComp := pages.Partial("#filters", pages.TodoFilters("", todos.GetTasks()))
	Render(res, item, filterComp)
}

func DestroyTaskHandler(res http.ResponseWriter, req *http.Request) {
	id := cg.GetPathParam(req, "id")
	if id != "" {
		todos.DeleteTask(id)
	}

	filterComp := pages.Partial("#filters", pages.TodoFilters("", todos.GetTasks()))
	Render(res, filterComp)
}

func ClearTasksHandler(res http.ResponseWriter, req *http.Request) {
	todos.ClearTasks()

	todoList := todos.GetTasks()
	todos := pages.TodoList(todoList)
	fragment := pages.Partial("#filters", pages.TodoFilters("", todoList))
	Render(res, todos, fragment)
}
