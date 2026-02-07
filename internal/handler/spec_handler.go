package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log" // Added log
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid" // Added db import
	"github.com/saransh1220/blueprint-audio/internal/db"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"golang.org/x/sync/errgroup"
)

type SpecHandler struct {
	service          service.SpecService
	fileService      service.FileService
	analyticsService service.AnalyticsServiceInterface
}

func NewSpecHandler(service service.SpecService, fileService service.FileService, analyticsService service.AnalyticsServiceInterface) *SpecHandler {
	return &SpecHandler{
		service:          service,
		fileService:      fileService,
		analyticsService: analyticsService,
	}
}

func (h *SpecHandler) Create(w http.ResponseWriter, r *http.Request) {
	// 1. Limit Total Request Size (1.5GB)
	r.Body = http.MaxBytesReader(w, r.Body, 1500<<20) // 1.5GB limit
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
	var uploadedKeysMu sync.Mutex
	var success bool
	defer func() {
		if !success {
			// Rollback: Delete all uploaded files if operation failed
			for _, key := range uploadedKeys {
				_ = h.fileService.Delete(context.Background(), key)
			}
		}
	}()

	g, ctx := errgroup.WithContext(r.Context())

	uploadAsync := func(formKey, folder string, limit int64, setUrl func(string)) func() error {
		return func() error {
			// Check for context cancellation
			if ctx.Err() != nil {
				return ctx.Err()
			}

			file, header, err := r.FormFile(formKey)
			if err == http.ErrMissingFile {
				return nil
			}
			if err != nil {
				return err
			}
			defer file.Close()

			if header.Size > limit {
				return errors.New(formKey + " file too large")
			}

			var url, key string

			if formKey == "image" {
				src, err := imaging.Decode(file)
				if err != nil {
					return fmt.Errorf("failed to decode image: %w", err)
				}

				dst := imaging.Fit(src, 800, 800, imaging.Lanczos)
				buf := new(bytes.Buffer)
				if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(90)); err != nil {
					return fmt.Errorf("failed to encode resized image: %w", err)
				}

				filename := fmt.Sprintf("%s.jpg", uuid.New().String())
				key = fmt.Sprintf("%s/%s", folder, filename)

				url, err = h.fileService.UploadWithKey(ctx, buf, key, "image/jpeg")
				if err != nil {
					return err
				}
			} else {
				url, key, err = h.fileService.Upload(ctx, file, header, folder)
				if err != nil {
					return err
				}
			}

			uploadedKeysMu.Lock()
			uploadedKeys = append(uploadedKeys, key)
			uploadedKeysMu.Unlock()

			setUrl(url)
			return nil
		}
	}

	// Image (5MB)
	g.Go(uploadAsync("image", "images", 5<<20, func(u string) { spec.ImageUrl = u }))

	// Preview (30MB)
	g.Go(uploadAsync("preview", "audio/previews", 30<<20, func(u string) { spec.PreviewUrl = u }))

	// WAV (300MB)
	g.Go(uploadAsync("wav", "audio/wavs", 300<<20, func(u string) {
		val := u
		spec.WavUrl = &val
	}))

	// Stems (1GB)
	g.Go(uploadAsync("stems", "audio/stems", 1<<30, func(u string) {
		val := u
		spec.StemsUrl = &val
	}))

	if err := g.Wait(); err != nil {
		http.Error(w, "upload failed: "+err.Error(), http.StatusInternalServerError)
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

	// 1. Try Cache
	cacheKey := "spec:" + idStr
	val, err := db.Rdb.Get(r.Context(), cacheKey).Result()
	if err == nil {
		// Cache Hit!
		log.Printf("[CACHE HIT] Spec ID: %s", idStr)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(val))
		return
	}

	log.Printf("[CACHE MISS] Spec ID: %s", idStr)

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

	// Get user ID if authenticated (optional)
	var userIDPtr *uuid.UUID
	if userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID); ok {
		userIDPtr = &userID
	}

	// Fetch analytics data
	analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), spec.ID, userIDPtr)
	if err == nil {
		spec.Analytics = &domain.SpecAnalytics{
			PlayCount:     analytics.PlayCount,
			FavoriteCount: analytics.FavoriteCount,
			IsFavorited:   analytics.IsFavorited,
		}
	}

	response := dto.ToSpecResponse(spec)

	// 3. Save to Cache (Async)
	go func() {
		jsonBytes, _ := json.Marshal(response)
		db.Rdb.Set(context.Background(), cacheKey, jsonBytes, 10*time.Minute)
	}()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
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

	search := q.Get("search")
	key := q.Get("key")
	if key == "All" {
		key = ""
	}

	minBPM, _ := strconv.Atoi(q.Get("min_bpm"))
	maxBPM, _ := strconv.Atoi(q.Get("max_bpm"))

	minPrice, _ := strconv.ParseFloat(q.Get("min_price"), 64)
	maxPrice, _ := strconv.ParseFloat(q.Get("max_price"), 64)

	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}

	limit := 20
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	filter := domain.SpecFilter{
		Category: category,
		Genres:   genres,
		Tags:     tags,
		Search:   search,
		MinBPM:   minBPM,
		MaxBPM:   maxBPM,
		MinPrice: minPrice,
		MaxPrice: maxPrice,
		Key:      key,
		Sort:     q.Get("sort"),
		Limit:    limit,
		Offset:   offset,
	}

	specs, total, err := h.service.ListSpecs(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get user ID if authenticated (optional)
	var userIDPtr *uuid.UUID
	if userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID); ok {
		userIDPtr = &userID
	}

	for i := range specs {
		h.sanitizeSpec(&specs[i])

		// Fetch analytics for each spec
		analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), specs[i].ID, userIDPtr)
		if err == nil {
			specs[i].Analytics = &domain.SpecAnalytics{
				PlayCount:     analytics.PlayCount,
				FavoriteCount: analytics.FavoriteCount,
				IsFavorited:   analytics.IsFavorited,
			}
		}
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
			"per_page": limit,
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

	// Invalidate Cache
	cacheKey := "spec:" + idStr
	db.Rdb.Del(context.Background(), cacheKey)
	log.Printf("[CACHE INVALIDATE] Deleted Spec ID: %s", idStr)

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
			return url, nil
		}
		presignedURL, err := h.fileService.GetPresignedURL(ctx, key, expiration)
		if err != nil {
			return url, err
		}
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

// Update handles PATCH /specs/:id - updates spec metadata and optionally the cover image
func (h *SpecHandler) Update(w http.ResponseWriter, r *http.Request) {
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

	// 1. Fetch existing spec first (to verify ownership and get old image URL)
	existingSpec, err := h.service.GetSpec(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if existingSpec == nil {
		http.Error(w, "spec not found", http.StatusNotFound)
		return
	}
	if existingSpec.ProducerID != producerID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// 2. Parse Multipart Form (10MB limit for metadata + image)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	// 3. Extract and Apply Metadata
	metadata := r.FormValue("metadata")
	var updateData domain.Spec
	if err := json.Unmarshal([]byte(metadata), &updateData); err != nil {
		http.Error(w, "invalid metadata json", http.StatusBadRequest)
		return
	}

	// Update fields
	existingSpec.Title = updateData.Title
	existingSpec.BasePrice = updateData.BasePrice
	existingSpec.BPM = updateData.BPM
	existingSpec.Key = updateData.Key
	existingSpec.Tags = updateData.Tags
	existingSpec.Description = updateData.Description
	// Add other fields as necessary

	// 4. Handle Image Replacement
	file, _, err := r.FormFile("image")
	if err != nil && err != http.ErrMissingFile {
		http.Error(w, "invalid file", http.StatusBadRequest)
		return
	}

	if file != nil {
		defer file.Close()

		// Resize and Upload New Image
		src, err := imaging.Decode(file)
		if err != nil {
			http.Error(w, "failed to decode image", http.StatusBadRequest)
			return
		}

		dst := imaging.Fit(src, 800, 800, imaging.Lanczos)
		buf := new(bytes.Buffer)
		if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(90)); err != nil {
			http.Error(w, "failed to encode image", http.StatusInternalServerError)
			return
		}

		filename := fmt.Sprintf("%s.jpg", uuid.New().String())
		key := fmt.Sprintf("images/%s", filename)

		url, err := h.fileService.UploadWithKey(r.Context(), buf, key, "image/jpeg")
		if err != nil {
			http.Error(w, "failed to upload image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Delete Old Image
		if existingSpec.ImageUrl != "" {
			// Helper to extract key and delete
			if oldKey, err := h.fileService.GetKeyFromUrl(existingSpec.ImageUrl); err == nil {
				// Fire and forget delete (or log error)
				_ = h.fileService.Delete(context.Background(), oldKey)
			}
		}

		// Update URL
		existingSpec.ImageUrl = url
	}

	// 5. Save Updates
	if err := h.service.UpdateSpec(r.Context(), existingSpec, producerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 6. Return Updated Spec
	h.sanitizeSpec(existingSpec)
	response := dto.ToSpecResponse(existingSpec)

	// Invalidate Cache
	cacheKey := "spec:" + idStr
	db.Rdb.Del(context.Background(), cacheKey)
	log.Printf("[CACHE INVALIDATE] Updated Spec ID: %s", idStr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUserSpecs handles GET /users/:id/specs - lists all specs by a user
func (h *SpecHandler) GetUserSpecs(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.PathValue("id")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	specs, total, err := h.service.GetUserSpecs(r.Context(), userID, page)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get current user ID (viewer) if authenticated
	var currentUserIDPtr *uuid.UUID
	if currentUserID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID); ok {
		currentUserIDPtr = &currentUserID
	}

	for i := range specs {
		h.sanitizeSpec(&specs[i])

		// Fetch analytics for each spec
		analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), specs[i].ID, currentUserIDPtr)
		if err == nil {
			specs[i].Analytics = &domain.SpecAnalytics{
				PlayCount:     analytics.PlayCount,
				FavoriteCount: analytics.FavoriteCount,
				IsFavorited:   analytics.IsFavorited,
			}
		}
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
