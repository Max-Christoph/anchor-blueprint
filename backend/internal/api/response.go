package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Max-Christoph/anchor-blueprint/backend/internal/httpx"
)

// errorBody ist das einheitliche Fehlerformat der API.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// respondJSON serialisiert payload mit status.
func respondJSON(logger *slog.Logger, w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("encode response", "error", err)
	}
}

// respondError liefert ein einheitliches Fehlerobjekt.
func respondError(logger *slog.Logger, w http.ResponseWriter, r *http.Request, status int, code, message string) {
	respondJSON(logger, w, status, errorBody{Error: errorDetail{
		Code:      code,
		Message:   message,
		RequestID: httpx.RequestIDFrom(r.Context()),
	}})
}
