package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func writeAPIError(w http.ResponseWriter, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{
		Code:    code,
		Message: message,
		Details: details,
	})
}

func mapDomainError(err error, fallbackStatus int, fallbackCode, fallbackMessage string) (int, string, string) {
	switch {
	case errors.Is(err, errNotFound):
		return http.StatusNotFound, "not_found", "not found"
	case errors.Is(err, errInvalidTransition):
		return http.StatusConflict, "invalid_state_transition", "invalid state transition"
	default:
		return fallbackStatus, fallbackCode, fallbackMessage
	}
}
