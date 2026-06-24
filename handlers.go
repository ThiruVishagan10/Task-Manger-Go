package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

var nextID int = 1
var tasks []Task

func createTask(w http.ResponseWriter, r *http.Request) {
	var task Task

	err := json.NewDecoder(r.Body).Decode(&task)

	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid Data", err)
		return
	}

	task.ID = nextID
	nextID++

	tasks = append(tasks, task)

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(task)
}

func getTasksByID(w http.ResponseWriter, r *http.Request) {
	idstr := strings.TrimPrefix(r.URL.Path, "/tasks/")
	idstr = strings.Trim(idstr, "/")

	if idstr == "" {
		respondError(w, r, http.StatusBadRequest, "Invalid task ID", nil)
		return
	}

	id, err := strconv.Atoi(idstr)

	if err != nil {
		respondError(w, r, http.StatusBadRequest, "Invalid task ID", err)
		return
	}

	for _, tasks := range tasks {
		if tasks.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tasks)
			return
		}
	}

	respondError(w, r, http.StatusNotFound, "Task Not Found", nil)
}

func getTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}
