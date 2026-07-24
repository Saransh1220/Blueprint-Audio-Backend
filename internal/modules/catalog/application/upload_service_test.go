package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uploadRepositoryStub struct {
	domain.SpecUploadRepository

	createSessionFn  func(context.Context, *domain.SpecUploadSession) error
	getSessionFn     func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error)
	updateMetadataFn func(context.Context, uuid.UUID, uuid.UUID, json.RawMessage) error
	replaceAssetFn   func(
		context.Context,
		uuid.UUID,
		uuid.UUID,
		*domain.SpecUploadAsset,
	) (*domain.SpecUploadAsset, error)
	verifyAssetFn  func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, string, string) error
	finalizeFn     func(context.Context, *domain.SpecUploadSession, *domain.Spec, *domain.SpecProcessingJob) error
	claimNextJobFn func(context.Context, string) (*domain.ProcessingBundle, error)
	heartbeatJobFn func(context.Context, uuid.UUID, string) error
	completeJobFn  func(context.Context, uuid.UUID, string, domain.ProcessedSpecFiles) error
	failJobFn      func(context.Context, uuid.UUID, string, string) error
}

func (s *uploadRepositoryStub) CreateSession(
	ctx context.Context,
	session *domain.SpecUploadSession,
) error {
	if s.createSessionFn == nil {
		return errors.New("unexpected CreateSession call")
	}
	return s.createSessionFn(ctx, session)
}

func (s *uploadRepositoryStub) GetSession(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.SpecUploadSession, error) {
	if s.getSessionFn == nil {
		return nil, errors.New("unexpected GetSession call")
	}
	return s.getSessionFn(ctx, uploadID, producerID)
}

func (s *uploadRepositoryStub) UpdateSessionMetadata(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	metadata json.RawMessage,
) error {
	if s.updateMetadataFn == nil {
		return errors.New("unexpected UpdateSessionMetadata call")
	}
	return s.updateMetadataFn(ctx, uploadID, producerID, metadata)
}

func (s *uploadRepositoryStub) ReplaceAsset(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	asset *domain.SpecUploadAsset,
) (*domain.SpecUploadAsset, error) {
	if s.replaceAssetFn == nil {
		return nil, errors.New("unexpected ReplaceAsset call")
	}
	return s.replaceAssetFn(ctx, uploadID, producerID, asset)
}

func (s *uploadRepositoryStub) VerifyAsset(
	ctx context.Context,
	uploadID, producerID, assetID uuid.UUID,
	actualSize int64,
	actualContentType, etag string,
) error {
	if s.verifyAssetFn == nil {
		return errors.New("unexpected VerifyAsset call")
	}
	return s.verifyAssetFn(
		ctx,
		uploadID,
		producerID,
		assetID,
		actualSize,
		actualContentType,
		etag,
	)
}

func (s *uploadRepositoryStub) FinalizeUpload(
	ctx context.Context,
	session *domain.SpecUploadSession,
	spec *domain.Spec,
	job *domain.SpecProcessingJob,
) error {
	if s.finalizeFn == nil {
		return errors.New("unexpected FinalizeUpload call")
	}
	return s.finalizeFn(ctx, session, spec, job)
}

func (s *uploadRepositoryStub) ClaimNextJob(
	ctx context.Context,
	workerID string,
) (*domain.ProcessingBundle, error) {
	if s.claimNextJobFn == nil {
		return nil, errors.New("unexpected ClaimNextJob call")
	}
	return s.claimNextJobFn(ctx, workerID)
}

func (s *uploadRepositoryStub) HeartbeatJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
) error {
	if s.heartbeatJobFn == nil {
		return nil
	}
	return s.heartbeatJobFn(ctx, jobID, workerID)
}

func (s *uploadRepositoryStub) CompleteJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID string,
	result domain.ProcessedSpecFiles,
) error {
	if s.completeJobFn == nil {
		return errors.New("unexpected CompleteJob call")
	}
	return s.completeJobFn(ctx, jobID, workerID, result)
}

func (s *uploadRepositoryStub) FailJob(
	ctx context.Context,
	jobID uuid.UUID,
	workerID, reason string,
) error {
	if s.failJobFn == nil {
		return errors.New("unexpected FailJob call")
	}
	return s.failJobFn(ctx, jobID, workerID, reason)
}

type uploadSpecRepositoryStub struct {
	domain.SpecRepository

	getByIDSystemFn  func(context.Context, uuid.UUID) (*domain.Spec, error)
	getBySlugFn      func(context.Context, string) (*domain.Spec, error)
	getByShortCodeFn func(context.Context, string) (*domain.Spec, error)
}

func (s *uploadSpecRepositoryStub) GetByIDSystem(
	ctx context.Context,
	id uuid.UUID,
) (*domain.Spec, error) {
	if s.getByIDSystemFn == nil {
		return nil, errors.New("unexpected GetByIDSystem call")
	}
	return s.getByIDSystemFn(ctx, id)
}

func (s *uploadSpecRepositoryStub) GetBySlug(
	ctx context.Context,
	slug string,
) (*domain.Spec, error) {
	if s.getBySlugFn == nil {
		return nil, nil
	}
	return s.getBySlugFn(ctx, slug)
}

func (s *uploadSpecRepositoryStub) GetByShortCode(
	ctx context.Context,
	shortCode string,
) (*domain.Spec, error) {
	if s.getByShortCodeFn == nil {
		return nil, nil
	}
	return s.getByShortCodeFn(ctx, shortCode)
}

type objectStoreStub struct {
	createPresignedUploadFn func(context.Context, string, string, int64, time.Duration) (filestorageDomain.PresignedUpload, error)
	statObjectFn            func(context.Context, string) (filestorageDomain.ObjectInfo, error)
	openObjectFn            func(context.Context, string) (io.ReadCloser, error)
	copyObjectFn            func(context.Context, string, string, string) (string, error)
	objectURLFn             func(string) (string, error)
	uploadWithKeyFn         func(context.Context, io.Reader, string, string) (string, error)
	deleteFn                func(context.Context, string) error
}

func (s *objectStoreStub) CreatePresignedUpload(
	ctx context.Context,
	key, contentType string,
	expectedSize int64,
	ttl time.Duration,
) (filestorageDomain.PresignedUpload, error) {
	if s.createPresignedUploadFn == nil {
		return filestorageDomain.PresignedUpload{}, errors.New("unexpected CreatePresignedUpload call")
	}
	return s.createPresignedUploadFn(ctx, key, contentType, expectedSize, ttl)
}

func (s *objectStoreStub) StatObject(
	ctx context.Context,
	key string,
) (filestorageDomain.ObjectInfo, error) {
	if s.statObjectFn == nil {
		return filestorageDomain.ObjectInfo{}, errors.New("unexpected StatObject call")
	}
	return s.statObjectFn(ctx, key)
}

func (s *objectStoreStub) OpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if s.openObjectFn == nil {
		return nil, errors.New("unexpected OpenObject call")
	}
	return s.openObjectFn(ctx, key)
}

func (s *objectStoreStub) CopyObject(
	ctx context.Context,
	source, destination, expectedETag string,
) (string, error) {
	if s.copyObjectFn == nil {
		return "", errors.New("unexpected CopyObject call")
	}
	return s.copyObjectFn(ctx, source, destination, expectedETag)
}

func (s *objectStoreStub) ObjectURL(key string) (string, error) {
	if s.objectURLFn == nil {
		return "", errors.New("unexpected ObjectURL call")
	}
	return s.objectURLFn(key)
}

func (s *objectStoreStub) UploadWithKey(
	ctx context.Context,
	file io.Reader,
	key, contentType string,
) (string, error) {
	if s.uploadWithKeyFn == nil {
		return "", errors.New("unexpected UploadWithKey call")
	}
	return s.uploadWithKeyFn(ctx, file, key, contentType)
}

func (s *objectStoreStub) Delete(ctx context.Context, key string) error {
	if s.deleteFn == nil {
		return errors.New("unexpected Delete call")
	}
	return s.deleteFn(ctx, key)
}

func TestValidateUploadManifest_BeatsOnlyAndLimits(t *testing.T) {
	t.Parallel()

	valid := validBeatUploadFiles()
	normalized, err := validateUploadManifest(domain.CategoryBeat, valid)
	require.NoError(t, err)
	require.Len(t, normalized, 4)
	assert.Equal(t, "audio/mpeg", normalized[1].ContentType)

	tests := []struct {
		name     string
		category domain.Category
		files    []UploadFileCommand
	}{
		{
			name:     "samples are not accepted by this endpoint",
			category: domain.CategorySample,
			files:    valid,
		},
		{
			name:     "every beat asset is required",
			category: domain.CategoryBeat,
			files:    valid[:3],
		},
		{
			name:     "an asset cannot exceed its own limit",
			category: domain.CategoryBeat,
			files: func() []UploadFileCommand {
				files := append([]UploadFileCommand(nil), valid...)
				files[0].SizeBytes = uploadSizeLimits[domain.UploadAssetImage] + 1
				return files
			}(),
		},
		{
			name:     "duplicate kinds are rejected",
			category: domain.CategoryBeat,
			files: func() []UploadFileCommand {
				files := append([]UploadFileCommand(nil), valid...)
				files[3] = files[0]
				return files
			}(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := validateUploadManifest(tt.category, tt.files)
			require.ErrorIs(t, err, domain.ErrInvalidUpload)
		})
	}
}

func TestSpecUploadService_StagedDraftFlow(t *testing.T) {
	t.Parallel()

	t.Run("initiate creates an empty draft session", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		var captured *domain.SpecUploadSession
		uploads := &uploadRepositoryStub{
			createSessionFn: func(_ context.Context, session *domain.SpecUploadSession) error {
				captured = session
				return nil
			},
		}

		result, err := NewSpecUploadService(
			uploads, &uploadSpecRepositoryStub{}, &objectStoreStub{},
		).Initiate(context.Background(), producerID)
		require.NoError(t, err)
		require.NotNil(t, captured)
		assert.Equal(t, result.UploadID, captured.ID)
		assert.Equal(t, result.SpecID, captured.SpecID)
		assert.Equal(t, producerID, captured.ProducerID)
		assert.JSONEq(t, `{}`, string(captured.Metadata))
		assert.Empty(t, captured.Assets)
		assert.Equal(t, domain.UploadStatusUploading, captured.Status)
		assert.Equal(t, specUploadSessionTTL, captured.ExpiresAt.Sub(captured.CreatedAt))
	})

	t.Run("metadata is normalized and stored against the reserved spec", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		var stored domain.Spec
		uploads := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				return session, nil
			},
			updateMetadataFn: func(
				_ context.Context,
				uploadID, ownerID uuid.UUID,
				metadata json.RawMessage,
			) error {
				assert.Equal(t, session.ID, uploadID)
				assert.Equal(t, producerID, ownerID)
				return json.Unmarshal(metadata, &stored)
			},
		}
		spec := domain.Spec{
			Title:         "Night Drive",
			Category:      domain.CategoryBeat,
			BPM:           124,
			Key:           "C# MINOR",
			Genres:        []domain.Genre{{Name: "Trap", Slug: "client-slug"}},
			Moods:         []string{},
			Instruments:   []string{},
			PriceCurrency: "USD",
			Licenses: []domain.LicenseOption{
				{
					LicenseType:   domain.LicenseBasic,
					Name:          "Basic",
					Price:         19,
					PriceCurrency: "USD",
					Features:      []string{},
					FileTypes:     []string{"MP3"},
				},
			},
		}

		err := NewSpecUploadService(
			uploads, &uploadSpecRepositoryStub{}, &objectStoreStub{},
		).SaveMetadata(context.Background(), session.ID, producerID, spec)
		require.NoError(t, err)
		assert.Equal(t, session.SpecID, stored.ID)
		assert.Equal(t, producerID, stored.ProducerID)
		assert.Equal(t, "beat", stored.Type)
		assert.Equal(t, "trap", stored.Genres[0].Slug)
		require.NotNil(t, stored.Slug)
		assert.Contains(t, *stored.Slug, "night-drive-")
	})

	t.Run("one selected file receives a URL and is verified independently", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		session.Assets = nil
		var prepared *domain.SpecUploadAsset
		uploads := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				return session, nil
			},
			replaceAssetFn: func(
				_ context.Context,
				uploadID, ownerID uuid.UUID,
				asset *domain.SpecUploadAsset,
			) (*domain.SpecUploadAsset, error) {
				assert.Equal(t, session.ID, uploadID)
				assert.Equal(t, producerID, ownerID)
				prepared = asset
				session.Assets = []domain.SpecUploadAsset{*asset}
				return nil, nil
			},
			verifyAssetFn: func(
				_ context.Context,
				uploadID, ownerID, assetID uuid.UUID,
				size int64,
				contentType, etag string,
			) error {
				assert.Equal(t, session.ID, uploadID)
				assert.Equal(t, producerID, ownerID)
				assert.Equal(t, prepared.ID, assetID)
				assert.Equal(t, prepared.ExpectedSize, size)
				assert.Equal(t, prepared.DeclaredContentType, contentType)
				assert.Equal(t, "etag-cover", etag)
				return nil
			},
		}
		objects := &objectStoreStub{
			createPresignedUploadFn: func(
				_ context.Context,
				key, contentType string,
				expectedSize int64,
				ttl time.Duration,
			) (filestorageDomain.PresignedUpload, error) {
				assert.Equal(t, specUploadURLTTL, ttl)
				return filestorageDomain.PresignedUpload{
					URL:     "https://storage.invalid/" + key,
					Method:  "PUT",
					Headers: map[string]string{"Content-Type": contentType},
				}, nil
			},
			statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
				assert.Equal(t, prepared.ObjectKey, key)
				return filestorageDomain.ObjectInfo{
					Key:         key,
					Size:        prepared.ExpectedSize,
					ContentType: prepared.DeclaredContentType,
					ETag:        "etag-cover",
				}, nil
			},
		}
		service := NewSpecUploadService(uploads, &uploadSpecRepositoryStub{}, objects)

		instruction, err := service.PrepareFile(
			context.Background(),
			session.ID,
			producerID,
			validBeatUploadFiles()[0],
		)
		require.NoError(t, err)
		require.NotNil(t, prepared)
		assert.Equal(t, prepared.ID, instruction.AssetID)
		assert.Contains(t, prepared.ObjectKey, prepared.ID.String())

		confirmed, err := service.ConfirmFile(
			context.Background(), session.ID, producerID, prepared.ID,
		)
		require.NoError(t, err)
		require.NotNil(t, confirmed.ActualSize)
		assert.Equal(t, prepared.ExpectedSize, *confirmed.ActualSize)
	})
}

func TestSpecUploadService_CompleteVerifiesObjectsAndIsIdempotent(t *testing.T) {
	t.Parallel()

	t.Run("verified HEAD metadata is persisted before queueing", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		statCalls := 0
		finalized := false
		uploads := &uploadRepositoryStub{
			getSessionFn: func(_ context.Context, uploadID, ownerID uuid.UUID) (*domain.SpecUploadSession, error) {
				assert.Equal(t, session.ID, uploadID)
				assert.Equal(t, producerID, ownerID)
				return session, nil
			},
			finalizeFn: func(
				_ context.Context,
				gotSession *domain.SpecUploadSession,
				spec *domain.Spec,
				job *domain.SpecProcessingJob,
			) error {
				finalized = true
				assert.Same(t, session, gotSession)
				assert.Equal(t, session.SpecID, spec.ID)
				assert.Equal(t, producerID, spec.ProducerID)
				assert.Equal(t, domain.ProcessingStatusProcessing, spec.ProcessingStatus)
				assert.Equal(t, domain.ProcessingJobQueued, job.Status)
				assert.Equal(t, session.ID, job.SessionID)
				for _, asset := range gotSession.Assets {
					require.NotNil(t, asset.ActualSize)
					assert.Equal(t, asset.ExpectedSize, *asset.ActualSize)
					require.NotNil(t, asset.ActualContentType)
					assert.Equal(t, asset.DeclaredContentType, *asset.ActualContentType)
					require.NotNil(t, asset.ETag)
					assert.Equal(t, "etag-"+string(asset.Kind), *asset.ETag)
				}
				return nil
			},
		}
		objects := &objectStoreStub{
			statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
				statCalls++
				asset := assetByObjectKey(t, session.Assets, key)
				return filestorageDomain.ObjectInfo{
					Key:         key,
					Size:        asset.ExpectedSize,
					ContentType: asset.DeclaredContentType + "; charset=binary",
					ETag:        "etag-" + string(asset.Kind),
				}, nil
			},
		}

		spec, err := NewSpecUploadService(uploads, &uploadSpecRepositoryStub{}, objects).
			Complete(context.Background(), session.ID, producerID)
		require.NoError(t, err)
		require.NotNil(t, spec)
		assert.True(t, finalized)
		assert.Len(t, session.Assets, statCalls)
	})

	t.Run("a HEAD size mismatch prevents queueing", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		finalized := false
		uploads := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				return session, nil
			},
			finalizeFn: func(
				context.Context,
				*domain.SpecUploadSession,
				*domain.Spec,
				*domain.SpecProcessingJob,
			) error {
				finalized = true
				return nil
			},
		}
		objects := &objectStoreStub{
			statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
				asset := assetByObjectKey(t, session.Assets, key)
				return filestorageDomain.ObjectInfo{
					Key:         key,
					Size:        asset.ExpectedSize + 1,
					ContentType: asset.DeclaredContentType,
					ETag:        "etag",
				}, nil
			},
		}

		_, err := NewSpecUploadService(uploads, &uploadSpecRepositoryStub{}, objects).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, domain.ErrInvalidUpload)
		assert.False(t, finalized)
	})

	t.Run("retry after queueing returns the existing spec without HEAD calls", func(t *testing.T) {
		t.Parallel()

		producerID := uuid.New()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusQueued)
		existing := &domain.Spec{
			ID:               session.SpecID,
			ProducerID:       producerID,
			Title:            "Night Drive",
			ProcessingStatus: domain.ProcessingStatusProcessing,
		}
		uploads := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				return session, nil
			},
		}
		specs := &uploadSpecRepositoryStub{
			getByIDSystemFn: func(_ context.Context, id uuid.UUID) (*domain.Spec, error) {
				assert.Equal(t, session.SpecID, id)
				return existing, nil
			},
		}
		objects := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				t.Fatal("queued completion retry must not HEAD storage")
				return filestorageDomain.ObjectInfo{}, nil
			},
		}

		spec, err := NewSpecUploadService(uploads, specs, objects).
			Complete(context.Background(), session.ID, producerID)
		require.NoError(t, err)
		assert.Same(t, existing, spec)
	})
}

func validBeatUploadFiles() []UploadFileCommand {
	return []UploadFileCommand{
		{
			Kind:        domain.UploadAssetImage,
			FileName:    "cover.png",
			ContentType: "image/png",
			SizeBytes:   1 << 20,
		},
		{
			Kind:        domain.UploadAssetPreview,
			FileName:    "preview.mp3",
			ContentType: "audio/mpeg; charset=binary",
			SizeBytes:   2 << 20,
		},
		{
			Kind:        domain.UploadAssetWAV,
			FileName:    "master.wav",
			ContentType: "audio/wav",
			SizeBytes:   3 << 20,
		},
		{
			Kind:        domain.UploadAssetStems,
			FileName:    "stems.zip",
			ContentType: "application/zip",
			SizeBytes:   4 << 20,
		},
	}
}

func verifiedUploadSession(
	t *testing.T,
	producerID uuid.UUID,
	status domain.UploadStatus,
) *domain.SpecUploadSession {
	t.Helper()

	uploadID := uuid.New()
	specID := uuid.New()
	spec := domain.Spec{
		ID:               specID,
		ProducerID:       producerID,
		Title:            "Night Drive",
		Category:         domain.CategoryBeat,
		ProcessingStatus: domain.ProcessingStatusProcessing,
	}
	metadata, err := json.Marshal(spec)
	require.NoError(t, err)

	files := validBeatUploadFiles()
	assets := make([]domain.SpecUploadAsset, 0, len(files))
	for _, file := range files {
		extension, extensionErr := uploadExtension(file.Kind, file.FileName, file.ContentType)
		require.NoError(t, extensionErr)
		actualSize := file.SizeBytes
		actualContentType := normalizeContentType(file.ContentType)
		etag := "verified-" + string(file.Kind)
		assets = append(assets, domain.SpecUploadAsset{
			ID:                  uuid.New(),
			SessionID:           uploadID,
			Kind:                file.Kind,
			FileName:            file.FileName,
			ObjectKey:           fmt.Sprintf("incoming/specs/%s/%s/%s%s", producerID, uploadID, file.Kind, extension),
			FinalObjectKey:      finalAssetKey(specID, file.Kind, extension),
			DeclaredContentType: normalizeContentType(file.ContentType),
			ExpectedSize:        file.SizeBytes,
			ActualSize:          &actualSize,
			ActualContentType:   &actualContentType,
			ETag:                &etag,
		})
	}

	now := time.Now().UTC()
	return &domain.SpecUploadSession{
		ID:         uploadID,
		SpecID:     specID,
		ProducerID: producerID,
		Metadata:   metadata,
		Status:     status,
		ExpiresAt:  now.Add(time.Hour),
		CreatedAt:  now,
		UpdatedAt:  now,
		Assets:     assets,
	}
}

func assetByObjectKey(
	t *testing.T,
	assets []domain.SpecUploadAsset,
	key string,
) domain.SpecUploadAsset {
	t.Helper()
	for _, asset := range assets {
		if asset.ObjectKey == key {
			return asset
		}
	}
	t.Fatalf("unexpected object key %q", key)
	return domain.SpecUploadAsset{}
}
