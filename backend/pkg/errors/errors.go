package errors

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e APIError) Error() string { return e.Message }

func WriteError(w http.ResponseWriter, err APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Status)
	json.NewEncoder(w).Encode(err)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

var (
	ErrNotFound      = APIError{Status: 404, Code: "not_found", Message: "Resource not found"}
	ErrUnauthorized  = APIError{Status: 401, Code: "unauthorized", Message: "Unauthorized"}
	ErrForbidden     = APIError{Status: 403, Code: "forbidden", Message: "Forbidden"}
	ErrBadRequest    = APIError{Status: 400, Code: "bad_request", Message: "Bad request"}
	ErrInternal      = APIError{Status: 500, Code: "internal_error", Message: "Internal server error"}
	ErrQuotaExceeded = APIError{Status: 429, Code: "quota_exceeded", Message: "Plan quota exceeded"}
)
