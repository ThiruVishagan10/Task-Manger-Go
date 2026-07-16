package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// writeJSON sends v as a JSON body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	// The status and headers are already on the wire, so a failure here cannot
	// be turned into an error response; log it and move on.
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[ERROR] encode response: %v", err)
	}
}

// respondError writes message to the client as plain text and logs it at a
// severity derived from status. err is recorded in the log but never exposed to
// the client, so internal details cannot leak.
func respondError(w http.ResponseWriter, r *http.Request, status int, message string, err error) {
	if err != nil {
		log.Printf("[%s] %s %s -> %d %s | err=%v", logLevelForStatus(status), r.Method, r.URL.Path, status, message, err)
	} else {
		log.Printf("[%s] %s %s -> %d %s", logLevelForStatus(status), r.Method, r.URL.Path, status, message)
	}

	http.Error(w, message, status)
}

// logLevelForStatus maps an HTTP status to a log severity.
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
