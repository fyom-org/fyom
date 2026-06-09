// Package response provides helpers for writing JSON HTTP responses.
package response

import (
	"encoding/json"
	"net/http"
)

// Response is the standard API response envelope.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

// Success writes a successful response.
func Success(w http.ResponseWriter, data interface{}) {
	JSON(w, 200, Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Created writes a 201 response.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, 201, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(204)
}

// Error writes an error response.
func Error(w http.ResponseWriter, code int, message string) {
	JSON(w, code, Response{
		Code:    code,
		Message: message,
	})
}
