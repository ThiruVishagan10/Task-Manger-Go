package main

import (
	"log"
	"net/http"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

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

	http.HandleFunc("/tasks/",  func(w http.ResponseWriter,r *http.Request){
		
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

	log.Println("Server running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
