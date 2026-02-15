package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
)

type SpecHandler struct {
	service             application.SpecService
	fileService         FileService
	analyticsService    AnalyticsService
	notificationService NotificationService
	redisClient         *redis.Client
}

func NewSpecHandler(service application.SpecService, fileService FileService, analyticsService AnalyticsService, notificationService NotificationService, redisClient *redis.Client) *SpecHandler {
	return &SpecHandler{
		service:             service,
		fileService:         fileService,
		analyticsService:    analyticsService,
		notificationService: notificationService,
		redisClient:         redisClient,
	}
}

func (h *SpecHandler) Create(w http.ResponseWriter, r *http.Request) {
	log.Printf("[SpecHandler.Create] Started")

	// 1. Limit Total Request Size (1.5GB)
	r.Body = http.MaxBytesReader(w, r.Body, 1500<<20) // 1.5GB limit
	if err := r.ParseMultipartForm(50 << 20); err != nil {
		log.Printf("[SpecHandler.Create] ParseMultipartForm error: %v", err)
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}

	// 2. Extract Metadata (JSON)
	metadata := r.FormValue("metadata")
	var spec domain.Spec
	if err := json.Unmarshal([]byte(metadata), &spec); err != nil {
		log.Printf("[SpecHandler.Create] Metadata unmarshal error: %v", err)
		http.Error(w, "invalid metadata json", http.StatusBadRequest)
		return
	}

	// 3. Auth Check
	producerId, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	if !ok {
		log.Printf("[SpecHandler.Create] Unauthorized")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	spec.ProducerID = producerId

	// 4. Persist Files to Temp Storage for Async Processing
	tempFiles := make(map[string]string)

	// Helper to persist file
	persistFile := func(formKey string) error {
		file, _, err := r.FormFile(formKey)
		if err == http.ErrMissingFile {
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()

		// Create temp file
		tempFile, err := os.CreateTemp("", "upload-*")
		if err != nil {
			return fmt.Errorf("failed to create temp file: %w", err)
		}
		defer tempFile.Close()

		// Copy content
		if _, err := io.Copy(tempFile, file); err != nil {
			os.Remove(tempFile.Name()) // Clean up on error
			return fmt.Errorf("failed to save temp file: %w", err)
		}

		tempFiles[formKey] = tempFile.Name()
		return nil
	}

	keysToProcess := []string{"image", "preview", "wav", "stems"}
	for _, key := range keysToProcess {
		if err := persistFile(key); err != nil {
			// Cleanup any created temp files
			for _, path := range tempFiles {
				os.Remove(path)
			}
			log.Printf("[SpecHandler.Create] File persistence failed: %v", err)
			http.Error(w, "file upload failed", http.StatusInternalServerError)
			return
		}
	}

	// 4b. Validate required files presence
	if spec.Category == domain.CategoryBeat {
		if _, ok := tempFiles["wav"]; !ok {
			// Cleanup
			for _, path := range tempFiles {
				os.Remove(path)
			}
			http.Error(w, "wav file is required for beats", http.StatusBadRequest)
			return
		}
		if _, ok := tempFiles["stems"]; !ok {
			// Cleanup
			for _, path := range tempFiles {
				os.Remove(path)
			}
			http.Error(w, "stems file is required for beats", http.StatusBadRequest)
			return
		}
	}

	// 5. Initial DB Save
	spec.ProcessingStatus = domain.ProcessingStatusProcessing
	if spec.ID == uuid.Nil {
		spec.ID = uuid.New()
	}

	if err := h.service.CreateSpec(r.Context(), &spec); err != nil {
		// Cleanup temp files
		for _, path := range tempFiles {
			os.Remove(path)
		}
		log.Printf("[SpecHandler.Create] Database save failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 6. Respond 202
	h.sanitizeSpec(&spec)
	response := ToSpecResponse(&spec)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[SpecHandler.Create] Response encode error: %v", err)
	}

	// 7. Async Processing
	go func(specID uuid.UUID, producerID uuid.UUID, filePaths map[string]string) {
		asyncCtx := context.Background()
		jobStart := time.Now()
		log.Printf("[SpecHandler.Create] Starting async job for spec %s", specID)

		var uploadedKeys []string
		var funcErr error
		filesToUpdate := make(map[string]*string)

		// Final Cleanup & Status Update
		defer func() {
			// Always remove temp files
			for _, path := range filePaths {
				os.Remove(path)
			}

			if funcErr != nil {
				log.Printf("[SpecHandler.Create] Async job failed: %v", funcErr)
				// Rollback uploads
				for _, key := range uploadedKeys {
					_ = h.fileService.Delete(asyncCtx, key)
				}
				// Update Status Failed
				_ = h.service.UpdateFilesAndStatus(asyncCtx, specID, nil, domain.ProcessingStatusFailed)

				// Invalidate Cache
				h.redisClient.Del(asyncCtx, "spec:"+specID.String())

				// Notify Failure
				_ = h.notificationService.Create(asyncCtx, producerID, "Upload Failed", fmt.Sprintf("Processing for '%s' failed. Please try again.", spec.Title), "error")
			} else {
				log.Printf("[SpecHandler.Create] Async job success in %v", time.Since(jobStart))

				// Update Status Completed & URLs
				if err := h.service.UpdateFilesAndStatus(asyncCtx, specID, filesToUpdate, domain.ProcessingStatusCompleted); err != nil {
					log.Printf("Failed to update status: %v", err)

					// Treat as failure
					// Rollback uploads
					for _, key := range uploadedKeys {
						_ = h.fileService.Delete(asyncCtx, key)
					}
					// Update Status Failed
					_ = h.service.UpdateFilesAndStatus(asyncCtx, specID, nil, domain.ProcessingStatusFailed)

					// Invalidate Cache (to reflect failed status)
					cacheKey := "spec:" + specID.String()
					h.redisClient.Del(asyncCtx, cacheKey)

					// Notify Failure
					_ = h.notificationService.Create(asyncCtx, producerID, "Upload Failed", fmt.Sprintf("Processing for '%s' failed. Please try again.", spec.Title), "error")
					return
				}

				// Invalidate Cache (to reflect new status and files)
				cacheKey := "spec:" + specID.String()
				h.redisClient.Del(asyncCtx, cacheKey)

				// Notify Success
				_ = h.notificationService.Create(asyncCtx, producerID, "Upload Complete", fmt.Sprintf("Your beat '%s' is now live!", spec.Title), "success")
			}
		}()

		// Process each file
		processFile := func(key, path string) (string, string, error) {
			f, err := os.Open(path)
			if err != nil {
				return "", "", err
			}
			defer f.Close()

			stat, _ := f.Stat()
			fileSize := stat.Size()
			log.Printf("Processing %s (%d bytes)", key, fileSize)

			if key == "image" {
				// Resize Image
				src, err := imaging.Decode(f)
				if err != nil {
					return "", "", fmt.Errorf("image decode error: %w", err)
				}
				dst := imaging.Fit(src, 500, 500, imaging.Lanczos)
				buf := new(bytes.Buffer)
				if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
					return "", "", fmt.Errorf("image encode error: %w", err)
				}

				filename := fmt.Sprintf("%s.jpg", uuid.New().String())
				s3Key := fmt.Sprintf("images/%s", filename)
				url, err := h.fileService.UploadWithKey(asyncCtx, buf, s3Key, "image/jpeg")
				return url, s3Key, err
			} else {
				// Regular Upload
				// We need a proper filename/ext. Since we lost original filename, we assume extensions based on key
				// Or we just generate UUIDs
				folder := "audio/misc"
				if key == "preview" {
					folder = "audio/previews"
				}
				if key == "wav" {
					folder = "audio/wavs"
				}
				if key == "stems" {
					folder = "audio/stems"
				}

				// construct a fake header or use UploadWithKey
				// UploadWithKey reads io.Reader.
				// existing h.fileService.Upload takes *multipart.FileHeader which we don't have.
				// We should use UploadWithKey or a new method.
				// SpecHandler uses FileService interface. Let's see if we can use UploadWithKey.

				ext := ".bin"
				mime := "application/octet-stream"
				if key == "preview" {
					ext = ".mp3"
					mime = "audio/mpeg"
				}
				if key == "wav" {
					ext = ".wav"
					mime = "audio/wav"
				}
				if key == "stems" {
					ext = ".zip"
					mime = "application/zip"
				}

				filename := fmt.Sprintf("%s%s", uuid.New().String(), ext)
				s3Key := fmt.Sprintf("%s/%s", folder, filename)

				url, err := h.fileService.UploadWithKey(asyncCtx, f, s3Key, mime)
				return url, s3Key, err
			}
		}

		for key, path := range filePaths {
			url, s3Key, err := processFile(key, path)
			if err != nil {
				funcErr = err
				return
			}

			uploadedKeys = append(uploadedKeys, s3Key)

			// Map to DB field
			val := url
			if key == "image" {
				filesToUpdate["image_url"] = &val
			}
			if key == "preview" {
				filesToUpdate["preview_url"] = &val
			}
			if key == "wav" {
				filesToUpdate["wav_url"] = &val
			}
			if key == "stems" {
				filesToUpdate["stems_url"] = &val
			}
		}

	}(spec.ID, spec.ProducerID, tempFiles)
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
	val, err := h.redisClient.Get(r.Context(), cacheKey).Result()
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
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

	response := ToSpecResponse(spec)

	// Fetch analytics data
	analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), spec.ID, userIDPtr)
	if err == nil {
		response.Analytics = &SpecAnalytics{
			PlayCount:          analytics.PlayCount,
			FavoriteCount:      analytics.FavoriteCount,
			TotalDownloadCount: analytics.TotalDownloadCount,
			IsFavorited:        analytics.IsFavorited,
		}
	}

	// 3. Save to Cache (Async)
	go func() {
		jsonBytes, _ := json.Marshal(response)
		h.redisClient.Set(context.Background(), cacheKey, jsonBytes, 10*time.Minute)
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get user ID if authenticated (optional)
	var userIDPtr *uuid.UUID
	if userID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID); ok {
		userIDPtr = &userID
	}

	responses := make([]SpecResponse, len(specs))

	for i := range specs {
		h.sanitizeSpec(&specs[i])
		responses[i] = *ToSpecResponse(&specs[i])

		// Fetch analytics for each spec
		analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), specs[i].ID, userIDPtr)
		if err == nil {
			responses[i].Analytics = &SpecAnalytics{
				PlayCount:          analytics.PlayCount,
				FavoriteCount:      analytics.FavoriteCount,
				TotalDownloadCount: analytics.TotalDownloadCount,
				IsFavorited:        analytics.IsFavorited,
			}
		}
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
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
		if err == domain.ErrSpecSoftDeleted {
			// Spec was soft-deleted because it has existing purchases.
			// Do NOT delete files from storage.
			log.Printf("Spec %s was soft deleted (purchased). Skipping file deletion.", idStr)

			// Invalidate Cache
			cacheKey := "spec:" + idStr
			h.redisClient.Del(context.Background(), cacheKey)
			log.Printf("[CACHE INVALIDATE] Deleted Spec ID: %s", idStr)

			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
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
	h.redisClient.Del(context.Background(), cacheKey)
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
		presignedURL, err := h.fileService.GetPresignedDownloadURL(ctx, key, spec.Title, expiration)
		if err != nil {
			return url, err
		}
		return presignedURL, nil
	}

	// Generate presigned URLs for audio files
	if presignedURL, err := generatePresignedURL(spec.PreviewUrl); err == nil && presignedURL != "" {
		spec.PreviewUrl = presignedURL
	}

	// Use regular presigned URL for image preview.
	imageKey, err := h.fileService.GetKeyFromUrl(spec.ImageUrl)
	if err == nil {
		if presignedURL, err := h.fileService.GetPresignedURL(ctx, imageKey, expiration); err == nil {
			spec.ImageUrl = presignedURL
		}
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

	// 1. Fetch existing spec first
	existingSpec, err := h.service.GetSpec(r.Context(), id)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
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

	// 2. Parse Multipart Form
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
	existingSpec.FreeMp3Enabled = updateData.FreeMp3Enabled
	existingSpec.Licenses = updateData.Licenses

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

		dst := imaging.Fit(src, 500, 500, imaging.Lanczos)
		buf := new(bytes.Buffer)
		if err := imaging.Encode(buf, dst, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
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
			if oldKey, err := h.fileService.GetKeyFromUrl(existingSpec.ImageUrl); err == nil {
				_ = h.fileService.Delete(context.Background(), oldKey)
			}
		}

		// Update URL
		existingSpec.ImageUrl = url
	}

	// 5. Save Updates
	if err := h.service.UpdateSpec(r.Context(), existingSpec, producerID); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// 6. Return Updated Spec
	h.sanitizeSpec(existingSpec)
	response := ToSpecResponse(existingSpec)

	// Invalidate Cache
	cacheKey := "spec:" + idStr
	h.redisClient.Del(context.Background(), cacheKey)
	log.Printf("[CACHE INVALIDATE] Updated Spec ID: %s", idStr)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

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
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Get current user ID (viewer) if authenticated
	var currentUserIDPtr *uuid.UUID
	if currentUserID, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID); ok {
		currentUserIDPtr = &currentUserID
	}

	responses := make([]SpecResponse, len(specs))

	for i := range specs {
		h.sanitizeSpec(&specs[i])
		responses[i] = *ToSpecResponse(&specs[i])

		// Fetch analytics for each spec
		analytics, err := h.analyticsService.GetPublicAnalytics(r.Context(), specs[i].ID, currentUserIDPtr)
		if err == nil {
			responses[i].Analytics = &SpecAnalytics{
				PlayCount:          analytics.PlayCount,
				FavoriteCount:      analytics.FavoriteCount,
				TotalDownloadCount: analytics.TotalDownloadCount,
				IsFavorited:        analytics.IsFavorited,
			}
		}
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

// DownloadFree handles POST /specs/{id}/download-free
func (h *SpecHandler) DownloadFree(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	// 1. Get Spec
	spec, err := h.service.GetSpec(r.Context(), id)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if spec == nil {
		http.Error(w, "spec not found", http.StatusNotFound)
		return
	}

	// 2. Check if Free Download is enabled
	if !spec.FreeMp3Enabled {
		http.Error(w, "free download not enabled for this spec", http.StatusForbidden)
		return
	}

	// 3. Track Download (Async)
	go func() {
		if err := h.analyticsService.TrackFreeDownload(context.Background(), id); err != nil {
			log.Printf("Failed to track free download for spec %s: %v", id, err)
		}
	}()

	// 4. Generate Presigned URL
	// Using PreviewUrl as the free MP3 source
	if spec.PreviewUrl == "" {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	key, err := h.fileService.GetKeyFromUrl(spec.PreviewUrl)
	if err != nil {
		// Log error and return regular URL if key extraction fails
		log.Printf("Failed to extract key from url %s: %v", spec.PreviewUrl, err)
		json.NewEncoder(w).Encode(map[string]string{"url": spec.PreviewUrl})
		return
	}

	// Generate download URL with 1 hour expiration
	// We use the spec title as the filename
	filename := fmt.Sprintf("%s.mp3", spec.Title) // Assuming MP3
	downloadUrl, err := h.fileService.GetPresignedDownloadURL(r.Context(), key, filename, time.Hour)
	if err != nil {
		http.Error(w, "failed to generate download url", http.StatusInternalServerError)
		return
	}

	// 5. Return URL
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"url": downloadUrl})
}
