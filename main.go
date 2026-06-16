package main

import (
	"fmt"
	"net/http"
)

func main() {

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {

		case http.MethodGet:
			getTasks(w, r)

		case http.MethodPost:
			getTasks(w, r)

		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})

	fmt.Println("Server running on :8080")
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	panic("unimplemented")
}
