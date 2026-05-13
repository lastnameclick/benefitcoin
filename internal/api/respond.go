package api

import (
	"encoding/json"
	"log"
	"net/http"
)

// errorEnvelope is the consistent shape for all API errors.
type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

// writeErr emits the standard error envelope. It matches auth.ErrorWriter.
func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: msg}})
}

// decodeJSON parses the request body into dst, returning false (and writing a
// 400) on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
