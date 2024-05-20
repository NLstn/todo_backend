package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

type Todo struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

var idCounter int32
var todos []Todo
var mutex = &sync.Mutex{}

func main() {

	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/todos", func(w http.ResponseWriter, r *http.Request) {

		var todo Todo
		err := json.NewDecoder(r.Body).Decode(&todo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		todo.ID = int(atomic.AddInt32(&idCounter, 1))

		mutex.Lock()
		todos = append(todos, todo)
		mutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(todo)
	})

	mux.HandleFunc("GET /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {

		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}

		for _, todo := range todos {
			if fmt.Sprint(todo.ID) == id {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(todo)
				return
			}
		}

		http.Error(w, "todo not found", http.StatusNotFound)
	})

	mux.HandleFunc("PATCH /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}

		var updatedTodo Todo
		err := json.NewDecoder(r.Body).Decode(&updatedTodo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		for i, todo := range todos {
			if fmt.Sprint(todo.ID) == id {
				mutex.Lock()
				if updatedTodo.Description != "" {
					todos[i].Description = updatedTodo.Description
				}
				mutex.Unlock()
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(todos[i])
				return
			}
		}

		http.Error(w, "todo not found", http.StatusNotFound)
	})

	mux.HandleFunc("DELETE /v1/todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}

		for i, todo := range todos {
			if fmt.Sprint(todo.ID) == id {
				mutex.Lock()
				todos = append(todos[:i], todos[i+1:]...)
				mutex.Unlock()
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		http.Error(w, "todo not found", http.StatusNotFound)
	})

	mux.HandleFunc("GET /v1/todos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(todos)
	})

	if err := http.ListenAndServe("localhost:8080", mux); err != nil {
		fmt.Println(err.Error())
	}

}
