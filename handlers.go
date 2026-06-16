package main

import (
	"encoding/json"
	"net/http"
)

var nextID int = 1
var tasks []Task

func createTask(w http.ResponseWriter, r *http.Request) {
	var task Task

	err := json.NewDecoder(r.Body).Decode(&task)

	if err != nil {
		http.Error(w, "Invalid Data", http.StatusBadRequest)
	}

	task.ID = nextID
	nextID++

	tasks = append(tasks, task)

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(task)
}
