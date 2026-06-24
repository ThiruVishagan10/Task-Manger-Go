package main

import (
	"log"
	"net/http"
)

func respondError(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil {
		log.Printf("[%s] %s %s -> %d %s | err=%v", logLevelForStatus(status), r.Method, r.URL.Path, status, message, err)
	} else {
		log.Printf("[%s] %s %s -> %d %s", logLevelForStatus(status), r.Method, r.URL.Path, status, message)
	}

	http.Error(w, message, status)
}

func logLevelForStatus(status int) string {
	switch {
	case status >= 500:
		return "ERROR"
	case status >= 400:
		return "WARN"
	default:
		return "INFO"
	}
}
