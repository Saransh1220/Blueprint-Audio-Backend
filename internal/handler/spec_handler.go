package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type SpecHandler struct {
	service service.SpecService
}

func NewSpecHandler(service service.SpecService) *SpecHandler {
	return &SpecHandler{service: service}
}

func (h *SpecHandler) Create(w http.ResponseWriter, r *http.Request) {
	var spec domain.Spec

	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	producerId, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized",
			http.StatusUnauthorized)
		return
	}
	spec.ProducerID = producerId

	if err := h.service.CreateSpec(r.Context(), &spec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(spec)
}

func (h *SpecHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	spec, err := h.service.GetSpec(r.Context(), id)
	if err != nil {
		// Differentiate between 404 and 500 if possible, for now 500 or 404 based on error
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if spec == nil {
		http.Error(w, "spec not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(spec)
}

func (h *SpecHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	category := domain.Category(q.Get("category"))

	var genres []string
	if g := q.Get("genres"); g != "" {
		genres = strings.Split(g, ",")
	}
	var tags []string
	if t := q.Get("tags"); t != "" {
		tags = strings.Split(t, ",")
	}

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	specs, err := h.service.ListSpecs(r.Context(), category, genres, tags, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(specs)
}

func (h *SpecHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	producerID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.service.DeleteSpec(r.Context(), id, producerID); err != nil {
		if err == domain.ErrSpecNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
