package main

import (
	"log"
	"net/http"
	"fmt"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	cfg := loadConfig()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Welcome to the Task Manager API!")
	})

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {

		case http.MethodGet:
			getTasks(w, r)

		case http.MethodPost:
			createTask(w, r)

		default:
			respondError(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", nil)
		}
	})

	http.HandleFunc("/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if r.URL.Path == "/tasks/" {
				getTasks(w, r)
				return
			}
			getTasksByID(w, r)
			return
		}

		respondError(w, r, http.StatusMethodNotAllowed, "Method Not Allowed", nil)
	})

	log.Printf("Server running on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
