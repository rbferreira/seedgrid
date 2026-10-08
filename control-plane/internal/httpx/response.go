// Package httpx contains shared HTTP utilities: uniform response
// envelope, standardized errors and middlewares (API.md §21, §27).
package httpx

import (
	"encoding/json"
	"net/http"
	"time"
)

// WriteJSON serializes v as JSON with the provided status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Params: the data that the code phrase uses (ADR-041 §4) — the Portal builds the text in
	// the reader's language from code + params; Message is the fallback and the detail.
	Params    map[string]any `json:"params,omitempty"`
	Timestamp string         `json:"timestamp"`
}

// WriteError responds with the platform's standard error envelope.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteErrorParams(w, status, code, message, nil)
}

// WriteErrorParams is WriteError with the data the code phrase uses (error.params).
func WriteErrorParams(w http.ResponseWriter, status int, code, message string, params map[string]any) {
	WriteJSON(w, status, errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   message,
		Params:    params,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}})
}

// DecodeJSON reads the request body as JSON into dst.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
