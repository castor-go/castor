package todos

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
)

type Task struct {
	ID        string
	Name      string
	Completed bool
}

var tasks = make([]Task, 0)

func NewID() string {
	b := make([]byte, 16) // 16 bytes = 128 bits = 32 hex chars
	rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b)[:7])
}

func filter(items []Task, predicate func(item Task) bool) []Task {
	list := make([]Task, 0)
	for _, i := range items {
		if predicate(i) {
			list = append(list, i)
		}
	}
	return list
}

func GetTasks() []Task {
	return tasks
}

func ApplyFilter(status string) []Task {
	if status == "active" {
		return filter(tasks, func(t Task) bool {
			return !t.Completed
		})
	}

	if status == "completed" {
		return filter(tasks, func(t Task) bool {
			return t.Completed
		})
	}

	return tasks
}

func AddTask(name string) Task {
	t := Task{
		ID:        NewID(),
		Name:      name,
		Completed: false,
	}
	tasks = append(tasks, t)
	return t
}

func CompleteTask(id string) Task {
	for idx := range tasks {
		if tasks[idx].ID == id {
			tasks[idx].Completed = !tasks[idx].Completed
			return tasks[idx]
		}
	}
	return Task{}
}

func DeleteTask(id string) {
	pos := -1
	for idx := range tasks {
		if tasks[idx].ID == id {
			pos = idx
			break
		}
	}

	if pos >= 0 {
		tasks = slices.Delete(tasks, pos, pos+1)
	}
}

func ClearTasks() {
	activeTasks := filter(tasks, func(t Task) bool {
		return !t.Completed
	})
	tasks = activeTasks
}
