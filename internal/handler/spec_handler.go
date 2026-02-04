package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type SpecHandler struct {
	service     service.SpecService
	fileService service.FileService
}

func NewSpecHandler(service service.SpecService, fileService service.FileService) *SpecHandler {
	return &SpecHandler{
		service:     service,
		fileService: fileService}
}

func (h *SpecHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 1. Parse Multipart Form (Max 50MB)
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	// 2. Extract Metadata (JSON)
	metadata := r.FormValue("metadata")
	var spec domain.Spec
	if err := json.Unmarshal([]byte(metadata), &spec); err != nil {
		http.Error(w, "invalid metadata json", http.StatusBadRequest)
		return
	}

	// 3. Auth Check
	producerId, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spec.ProducerID = producerId

	// 4. Handle File Uploads with Rollback
	var uploadedKeys []string
	var success bool
	defer func() {
		if !success {
			// Rollback: Delete all uploaded files if operation failed
			for _, key := range uploadedKeys {
				_ = h.fileService.Delete(context.Background(), key)
			}
		}
	}()

	upload := func(formKey, folder string, setUrl func(string)) error {
		file, header, err := r.FormFile(formKey)
		if err == http.ErrMissingFile {
			return nil // Optional (or handled by service validation)
		}
		if err != nil {
			return err
		}
		defer file.Close()

		url, key, err := h.fileService.Upload(r.Context(), file, header, folder)
		if err != nil {
			return err
		}
		uploadedKeys = append(uploadedKeys, key)
		setUrl(url)
		return nil
	}

	// Upload Image
	if err := upload("image", "images", func(u string) { spec.ImageUrl = u }); err != nil {
		http.Error(w, "upload image failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Upload Preview (MP3)
	if err := upload("preview", "audio/previews", func(u string) { spec.PreviewUrl = u }); err != nil {
		http.Error(w, "upload preview failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Upload WAV
	if err := upload("wav", "audio/wavs", func(u string) {
		val := u
		spec.WavUrl = &val
	}); err != nil {
		http.Error(w, "upload wav failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Upload Stems
	if err := upload("stems", "audio/stems", func(u string) {
		val := u
		spec.StemsUrl = &val
	}); err != nil {
		http.Error(w, "upload stems failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 5. Call Service to Save DB Record
	if err := h.service.CreateSpec(r.Context(), &spec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Mark success to prevent rollback
	success = true

	// 6. Return Created Spec
	h.sanitizeSpec(&spec)
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

	h.sanitizeSpec(spec)
	response := dto.ToSpecResponse(spec)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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

	specs, total, err := h.service.ListSpecs(r.Context(), category, genres, tags, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for i := range specs {
		h.sanitizeSpec(&specs[i])
	}
	responses := make([]dto.SpecResponse, len(specs))
	for i := range specs {
		responses[i] = *dto.ToSpecResponse(&specs[i])
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": responses,
		"metadata": map[string]interface{}{
			"total":    total,
			"page":     page,
			"per_page": 20,
		},
	})
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

	// 1. Get Spec details before deletion (to get file URLs)
	spec, err := h.service.GetSpec(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if spec == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// 2. Verify ownership
	if spec.ProducerID != producerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 3. Delete from DB
	if err := h.service.DeleteSpec(r.Context(), id, producerID); err != nil {
		if err == domain.ErrSpecNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. Delete files from Storage (Async or Sync - here Sync for simplicity)
	ctx := context.Background() // Use background context for cleanup to ensure it runs even if request cancels

	// Helper to delete file by URL
	deleteFile := func(url string) {
		if url == "" {
			return
		}
		key, err := h.fileService.GetKeyFromUrl(url)
		if err != nil {
			// Log error but continue
			return
		}
		_ = h.fileService.Delete(ctx, key)
	}

	deleteFile(spec.ImageUrl)
	deleteFile(spec.PreviewUrl)
	if spec.WavUrl != nil {
		deleteFile(*spec.WavUrl)
	}
	if spec.StemsUrl != nil {
		deleteFile(*spec.StemsUrl)
	}

	w.WriteHeader(http.StatusNoContent)
}

// sanitizeSpec generates presigned URLs for audio files to enable range request streaming
func (h *SpecHandler) sanitizeSpec(spec *domain.Spec) {
	ctx := context.Background()

	// For audio files, generate presigned URLs (1 hour expiration)
	// This enables range requests and chunked downloading
	expiration := time.Hour * 1

	// Helper to get key from URL and generate presigned URL
	generatePresignedURL := func(url string) (string, error) {
		if url == "" {
			return "", nil
		}
		key, err := h.fileService.GetKeyFromUrl(url)
		if err != nil {
			// If we can't parse the key, return original URL
			fmt.Printf("DEBUG: GetKeyFromUrl error for %s: %v\n", url, err)
			return url, nil
		}
		fmt.Printf("DEBUG: Generating presigned URL for key: %s\n", key)
		presignedURL, err := h.fileService.GetPresignedURL(ctx, key, expiration)
		if err != nil {
			fmt.Printf("DEBUG: GetPresignedURL error for %s: %v\n", key, err)
			return url, err
		}
		fmt.Printf("DEBUG: Generated presigned URL: %s\n", presignedURL)
		return presignedURL, nil
	}

	// Generate presigned URLs for audio files
	if presignedURL, err := generatePresignedURL(spec.PreviewUrl); err == nil && presignedURL != "" {
		spec.PreviewUrl = presignedURL
	}

	// if spec.WavUrl != nil {
	// 	if presignedURL, err := generatePresignedURL(*spec.WavUrl); err == nil && presignedURL != "" {
	// 		spec.WavUrl = &presignedURL
	// 	}
	// }

	// if spec.StemsUrl != nil {
	// 	if presignedURL, err := generatePresignedURL(*spec.StemsUrl); err == nil && presignedURL != "" {
	// 		spec.StemsUrl = &presignedURL
	// 	}
	// }

	// For images, we can keep direct URLs or also use presigned URLs
	// Using presigned for consistency and security
	if presignedURL, err := generatePresignedURL(spec.ImageUrl); err == nil && presignedURL != "" {
		spec.ImageUrl = presignedURL
	}
}
