package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"first-project/internal/models"
	"first-project/internal/service"
)

// Response adalah format JSON standar untuk semua endpoint.
type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Meta    any    `json:"meta,omitempty"`
	Errors  any    `json:"errors,omitempty"`
}

// Meta berisi informasi pagination.
type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func writeJSON(w http.ResponseWriter, status int, payload Response) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("gagal menulis response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Response{Success: false, Message: message})
}

// handleServiceError menerjemahkan error dari service menjadi status HTTP yang tepat.
func handleServiceError(w http.ResponseWriter, err error) {
	var ve *service.ValidationError
	switch {
	case errors.As(err, &ve):
		writeJSON(w, http.StatusUnprocessableEntity, Response{
			Success: false, Message: "validasi gagal", Errors: ve.Fields,
		})
	case errors.Is(err, models.ErrNotFound):
		writeError(w, http.StatusNotFound, "data tidak ditemukan")
	default:
		log.Printf("internal error: %v", err)
		writeError(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}

// decodeJSON membaca body JSON dengan batas ukuran 1MB dan menolak field asing.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("body JSON tidak valid: %w", err)
	}
	return nil
}
