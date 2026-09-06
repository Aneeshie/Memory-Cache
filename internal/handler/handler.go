package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Aneeshie/shared-in-memory-index/internal/dto"
	"github.com/Aneeshie/shared-in-memory-index/internal/service"
	"github.com/go-chi/chi"
)

type Handler struct {
	service *service.CacheService
}

func NewHandler(service *service.CacheService) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) GetData(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	resp, err := h.service.GetData(r.Context(), key)
	if err != nil {
		http.Error(w, "could not get data", http.StatusNotExtended)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, "could not encode response", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) PutData(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")

	var req dto.PutDataRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.PutData(r.Context(), key, &req); err != nil {
		log.Println("PUT error:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
