package api

import (
	"net/http"
)

// NewRouter registriert alle Routen. Go 1.22+ ServeMux-Pattern mit Methoden.
// Auth wird als Middleware-Kette in cmd/server/main.go vorgeschaltet
// (Deny-by-Default), daher tragen die Handler selbst keine Auth-Logik.
func NewRouter(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", h.Health)
	mux.HandleFunc("GET /api/v1/status", h.Status)
	mux.HandleFunc("GET /api/v1/data", h.Data)
	mux.HandleFunc("POST /api/v1/process", h.Process)

	// Fallback fuer alles andere.
	mux.HandleFunc("/", h.NotFound)

	return mux
}
