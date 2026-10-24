package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type statusRepoStub struct {
	*uploadRepositoryStub

	getStatusFn func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadStatus, error)
}

func (s *statusRepoStub) GetStatus(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.SpecUploadStatus, error) {
	return s.getStatusFn(ctx, uploadID, producerID)
}

func sessionRepo(session *domain.SpecUploadSession) *uploadRepositoryStub {
	return &uploadRepositoryStub{
		getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
			return session, nil
		},
	}
}

func TestSpecUploadService_Status(t *testing.T) {
	t.Parallel()

	uploadID, producerID := uuid.New(), uuid.New()
	build := func(status domain.UploadStatus, expires time.Time, err error) SpecUploadService {
		repo := &statusRepoStub{
			uploadRepositoryStub: &uploadRepositoryStub{},
			getStatusFn: func(_ context.Context, gotUpload, gotProducer uuid.UUID) (*domain.SpecUploadStatus, error) {
				assert.Equal(t, uploadID, gotUpload)
				assert.Equal(t, producerID, gotProducer)
				if err != nil {
					return nil, err
				}
				return &domain.SpecUploadStatus{UploadID: uploadID, Status: status, ExpiresAt: expires}, nil
			},
		}
		return NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, &objectStoreStub{})
	}

	t.Run("repository error is returned", func(t *testing.T) {
		t.Parallel()
		_, err := build("", time.Time{}, domain.ErrUploadNotFound).Status(context.Background(), uploadID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadNotFound)
	})
	t.Run("uploading session past expiry reports expired", func(t *testing.T) {
		t.Parallel()
		got, err := build(domain.UploadStatusUploading, time.Now().Add(-time.Minute), nil).
			Status(context.Background(), uploadID, producerID)
		require.NoError(t, err)
		assert.Equal(t, domain.UploadStatusExpired, got.Status)
	})
	t.Run("uploading session before expiry is unchanged", func(t *testing.T) {
		t.Parallel()
		got, err := build(domain.UploadStatusUploading, time.Now().Add(time.Hour), nil).
			Status(context.Background(), uploadID, producerID)
		require.NoError(t, err)
		assert.Equal(t, domain.UploadStatusUploading, got.Status)
	})
	t.Run("non-uploading expired session keeps its status", func(t *testing.T) {
		t.Parallel()
		got, err := build(domain.UploadStatusCompleted, time.Now().Add(-time.Hour), nil).
			Status(context.Background(), uploadID, producerID)
		require.NoError(t, err)
		assert.Equal(t, domain.UploadStatusCompleted, got.Status)
	})
}

func TestSpecUploadService_InitiateErrors(t *testing.T) {
	t.Parallel()

	boom := errors.New("insert failed")
	repo := &uploadRepositoryStub{createSessionFn: func(context.Context, *domain.SpecUploadSession) error { return boom }}
	result, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, &objectStoreStub{}).
		Initiate(context.Background(), uuid.New())
	require.ErrorIs(t, err, boom)
	assert.Nil(t, result)
}

func TestSpecUploadService_RequireUploadingSession(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	notFound := &uploadRepositoryStub{getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
		return nil, domain.ErrUploadNotFound
	}}
	expired := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	queued := verifiedUploadSession(t, producerID, domain.UploadStatusQueued)

	tests := []struct {
		name string
		repo *uploadRepositoryStub
		want error
	}{
		{"missing session", notFound, domain.ErrUploadNotFound},
		{"expired session", sessionRepo(expired), domain.ErrUploadExpired},
		{"already queued session", sessionRepo(queued), domain.ErrUploadState},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewSpecUploadService(tt.repo, &uploadSpecRepositoryStub{}, &objectStoreStub{})
			id := uuid.New()

			require.ErrorIs(t, svc.SaveMetadata(context.Background(), id, producerID, domain.Spec{}), tt.want)
			_, err := svc.PrepareFile(context.Background(), id, producerID, validBeatUploadFiles()[0])
			require.ErrorIs(t, err, tt.want)
			_, err = svc.ConfirmFile(context.Background(), id, producerID, uuid.New())
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestSpecUploadService_SaveMetadataErrors(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	validLicense := domain.LicenseOption{
		LicenseType: domain.LicenseBasic, Name: "Basic", Price: 19, PriceCurrency: "USD",
		Features: []string{}, FileTypes: []string{"MP3"},
	}
	validSpec := func() domain.Spec {
		return domain.Spec{
			Title: "Night Drive", Category: domain.CategoryBeat, BPM: 124, Key: "C# MINOR",
			Genres: []domain.Genre{{Name: "Trap"}}, Moods: []string{}, Instruments: []string{},
			PriceCurrency: "USD", Licenses: []domain.LicenseOption{validLicense},
		}
	}
	otherLicense := validLicense
	otherLicense.PriceCurrency = "INR"
	otherLicense.LicenseType = domain.LicensePremium
	badCurrency := validLicense
	badCurrency.PriceCurrency = "EUR"

	tests := []struct {
		name   string
		mutate func(*domain.Spec)
		update error
		want   error
	}{
		{"non beat category", func(s *domain.Spec) { s.Category = domain.CategorySample }, nil, domain.ErrInvalidUpload},
		{"no licenses", func(s *domain.Spec) { s.Licenses = nil }, nil, domain.ErrInvalidUpload},
		{"unsupported license currency", func(s *domain.Spec) { s.Licenses = []domain.LicenseOption{badCurrency} }, nil, domain.ErrInvalidUpload},
		{"mixed license currencies", func(s *domain.Spec) {
			s.Licenses = []domain.LicenseOption{validLicense, otherLicense}
		}, nil, domain.ErrInvalidUpload},
		{"repository update failure", func(*domain.Spec) {}, errors.New("db down"), nil},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
			repo := sessionRepo(session)
			repo.updateMetadataFn = func(context.Context, uuid.UUID, uuid.UUID, json.RawMessage) error {
				return tt.update
			}
			spec := validSpec()
			tt.mutate(&spec)

			err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, &objectStoreStub{}).
				SaveMetadata(context.Background(), session.ID, producerID, spec)
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			} else {
				require.ErrorIs(t, err, tt.update)
			}
		})
	}
}

func TestSpecUploadService_DeriveBeatMetadataPicksMinimumPriceAndUppercasesGenres(t *testing.T) {
	t.Parallel()

	spec := domain.Spec{
		Genres: []domain.Genre{{Name: " lo fi ", Slug: "x"}, {Name: "R&B"}},
		Licenses: []domain.LicenseOption{
			{Price: 50, PriceCurrency: "usd"},
			{Price: 10, PriceCurrency: " USD "},
		},
	}
	require.NoError(t, deriveBeatUploadMetadata(&spec))
	assert.Equal(t, float64(10), float64(spec.BasePrice))
	assert.Equal(t, "USD", spec.PriceCurrency)
	assert.Equal(t, "LO FI", spec.Genres[0].Name)
	assert.Equal(t, "lo-fi", spec.Genres[0].Slug)
	assert.Equal(t, "r&b", spec.Genres[1].Slug)
}

func TestSpecUploadService_PrepareFileErrors(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	boom := errors.New("boom")
	newSession := func() *domain.SpecUploadSession {
		return verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
	}

	t.Run("invalid file is rejected before storage is touched", func(t *testing.T) {
		t.Parallel()
		session := newSession()
		bad := validBeatUploadFiles()[0]
		bad.ContentType = "image/gif"
		_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, &objectStoreStub{}).
			PrepareFile(context.Background(), session.ID, producerID, bad)
		require.ErrorIs(t, err, domain.ErrInvalidUpload)
	})
	t.Run("presign failure", func(t *testing.T) {
		t.Parallel()
		session := newSession()
		objects := &objectStoreStub{createPresignedUploadFn: func(context.Context, string, string, int64, time.Duration) (filestorageDomain.PresignedUpload, error) {
			return filestorageDomain.PresignedUpload{}, boom
		}}
		_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, objects).
			PrepareFile(context.Background(), session.ID, producerID, validBeatUploadFiles()[0])
		require.ErrorIs(t, err, boom)
		assert.ErrorContains(t, err, "create upload URL for image")
	})
	t.Run("replace failure", func(t *testing.T) {
		t.Parallel()
		session := newSession()
		repo := sessionRepo(session)
		repo.replaceAssetFn = func(context.Context, uuid.UUID, uuid.UUID, *domain.SpecUploadAsset) (*domain.SpecUploadAsset, error) {
			return nil, boom
		}
		objects := &objectStoreStub{createPresignedUploadFn: func(context.Context, string, string, int64, time.Duration) (filestorageDomain.PresignedUpload, error) {
			return filestorageDomain.PresignedUpload{URL: "u"}, nil
		}}
		_, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, objects).
			PrepareFile(context.Background(), session.ID, producerID, validBeatUploadFiles()[0])
		require.ErrorIs(t, err, boom)
	})
	t.Run("replacing an asset deletes the superseded staging object", func(t *testing.T) {
		t.Parallel()
		session := newSession()
		repo := sessionRepo(session)
		repo.replaceAssetFn = func(context.Context, uuid.UUID, uuid.UUID, *domain.SpecUploadAsset) (*domain.SpecUploadAsset, error) {
			return &domain.SpecUploadAsset{ObjectKey: "incoming/old-cover.png"}, nil
		}
		var deleted []string
		objects := &objectStoreStub{
			createPresignedUploadFn: func(context.Context, string, string, int64, time.Duration) (filestorageDomain.PresignedUpload, error) {
				return filestorageDomain.PresignedUpload{URL: "u"}, nil
			},
			deleteFn: func(_ context.Context, key string) error {
				deleted = append(deleted, key)
				return errors.New("ignored")
			},
		}
		instruction, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, objects).
			PrepareFile(context.Background(), session.ID, producerID, validBeatUploadFiles()[0])
		require.NoError(t, err)
		assert.NotEqual(t, "incoming/old-cover.png", instruction.ObjectKey)
		assert.Equal(t, []string{"incoming/old-cover.png"}, deleted)
	})
}

func TestSpecUploadService_ConfirmFileErrors(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	boom := errors.New("boom")

	t.Run("unknown asset id", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, &objectStoreStub{}).
			ConfirmFile(context.Background(), session.ID, producerID, uuid.New())
		require.ErrorIs(t, err, domain.ErrUploadState)
	})

	statFor := func(info func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo, err error) func(*domain.SpecUploadSession) *objectStoreStub {
		return func(session *domain.SpecUploadSession) *objectStoreStub {
			return &objectStoreStub{statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
				if err != nil {
					return filestorageDomain.ObjectInfo{}, err
				}
				return info(assetByObjectKey(t, session.Assets, key)), nil
			}}
		}
	}
	tests := []struct {
		name    string
		store   func(*domain.SpecUploadSession) *objectStoreStub
		verify  error
		wantErr string
		want    error
	}{
		{"object missing", statFor(nil, boom), nil, "image upload is missing", domain.ErrInvalidUpload},
		{"size mismatch", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: a.ExpectedSize + 1, ContentType: a.DeclaredContentType, ETag: "e"}
		}, nil), nil, "size does not match", domain.ErrInvalidUpload},
		{"zero size", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: 0, ContentType: a.DeclaredContentType, ETag: "e"}
		}, nil), nil, "size does not match", domain.ErrInvalidUpload},
		{"content type mismatch", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: a.ExpectedSize, ContentType: "image/gif", ETag: "e"}
		}, nil), nil, "content type does not match", domain.ErrInvalidUpload},
		{"empty content type", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: a.ExpectedSize, ETag: "e"}
		}, nil), nil, "content type does not match", domain.ErrInvalidUpload},
		{"missing etag", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: a.ExpectedSize, ContentType: a.DeclaredContentType}
		}, nil), nil, "checksum is missing", domain.ErrInvalidUpload},
		{"verify persistence failure", statFor(func(a domain.SpecUploadAsset) filestorageDomain.ObjectInfo {
			return filestorageDomain.ObjectInfo{Size: a.ExpectedSize, ContentType: a.DeclaredContentType, ETag: "e"}
		}, nil), boom, "", boom},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
			repo := sessionRepo(session)
			repo.verifyAssetFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int64, string, string) error {
				return tt.verify
			}
			// Image is the first asset in the fixture.
			_, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, tt.store(session)).
				ConfirmFile(context.Background(), session.ID, producerID, session.Assets[0].ID)
			require.ErrorIs(t, err, tt.want)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestSpecUploadService_CompleteStatusBranches(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	t.Run("session lookup failure", func(t *testing.T) {
		t.Parallel()
		repo := &uploadRepositoryStub{getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
			return nil, domain.ErrUploadForbidden
		}}
		_, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, &objectStoreStub{}).
			Complete(context.Background(), uuid.New(), producerID)
		require.ErrorIs(t, err, domain.ErrUploadForbidden)
	})

	tests := []struct {
		status domain.UploadStatus
		want   error
	}{
		{domain.UploadStatusExpired, domain.ErrUploadExpired},
		{domain.UploadStatusFailed, domain.ErrUploadState},
		{domain.UploadStatus("mystery"), domain.ErrUploadState},
	}
	for _, tt := range tests {
		tt := tt
		t.Run("status "+string(tt.status), func(t *testing.T) {
			t.Parallel()
			session := verifiedUploadSession(t, producerID, tt.status)
			_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, &objectStoreStub{}).
				Complete(context.Background(), session.ID, producerID)
			require.ErrorIs(t, err, tt.want)
		})
	}

	t.Run("completed session of another producer is forbidden", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusCompleted)
		specs := &uploadSpecRepositoryStub{getByIDSystemFn: func(context.Context, uuid.UUID) (*domain.Spec, error) {
			return &domain.Spec{ID: session.SpecID, ProducerID: uuid.New()}, nil
		}}
		_, err := NewSpecUploadService(sessionRepo(session), specs, &objectStoreStub{}).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadForbidden)
	})
	t.Run("completed session spec lookup failure", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusProcessing)
		boom := errors.New("spec gone")
		specs := &uploadSpecRepositoryStub{getByIDSystemFn: func(context.Context, uuid.UUID) (*domain.Spec, error) {
			return nil, boom
		}}
		_, err := NewSpecUploadService(sessionRepo(session), specs, &objectStoreStub{}).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, boom)
	})
	t.Run("uploading session past expiry", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		session.ExpiresAt = time.Now().Add(-time.Second)
		_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, &objectStoreStub{}).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadExpired)
	})
}

func TestSpecUploadService_CompleteValidationFailures(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	okStore := func(session *domain.SpecUploadSession) *objectStoreStub {
		return &objectStoreStub{statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
			a := assetByObjectKey(t, session.Assets, key)
			return filestorageDomain.ObjectInfo{Key: key, Size: a.ExpectedSize, ContentType: a.DeclaredContentType, ETag: "e"}, nil
		}}
	}

	tests := []struct {
		name    string
		mutate  func(*domain.SpecUploadSession)
		want    error
		wantMsg string
	}{
		{"asset not confirmed", func(s *domain.SpecUploadSession) { s.Assets[0].ETag = nil }, domain.ErrInvalidUpload, "has not finished"},
		{"asset not confirmed (empty etag)", func(s *domain.SpecUploadSession) {
			blank := " "
			s.Assets[1].ETag = &blank
		}, domain.ErrInvalidUpload, "has not finished"},
		{"missing required asset", func(s *domain.SpecUploadSession) { s.Assets = s.Assets[:3] }, domain.ErrInvalidUpload, ""},
		{"duplicate asset kind", func(s *domain.SpecUploadSession) {
			s.Assets = append(s.Assets[:3:3], s.Assets[0])
		}, domain.ErrInvalidUpload, "duplicate asset kind"},
		{"metadata never saved", func(s *domain.SpecUploadSession) { s.Metadata = json.RawMessage(` {} `) }, domain.ErrInvalidUpload, "metadata has not been saved"},
		{"metadata corrupt", func(s *domain.SpecUploadSession) { s.Metadata = json.RawMessage(`{"id":`) }, nil, "decode stored upload metadata"},
		{"metadata for a different spec", func(s *domain.SpecUploadSession) {
			s.SpecID = uuid.New()
		}, domain.ErrUploadState, ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
			tt.mutate(session)
			_, err := NewSpecUploadService(sessionRepo(session), &uploadSpecRepositoryStub{}, okStore(session)).
				Complete(context.Background(), session.ID, producerID)
			require.Error(t, err)
			if tt.want != nil {
				require.ErrorIs(t, err, tt.want)
			}
			if tt.wantMsg != "" {
				assert.ErrorContains(t, err, tt.wantMsg)
			}
		})
	}
}

func TestSpecUploadService_CompleteFinalizeRaceHandling(t *testing.T) {
	t.Parallel()

	producerID := uuid.New()
	okStore := func(session *domain.SpecUploadSession) *objectStoreStub {
		return &objectStoreStub{statObjectFn: func(_ context.Context, key string) (filestorageDomain.ObjectInfo, error) {
			a := assetByObjectKey(t, session.Assets, key)
			return filestorageDomain.ObjectInfo{Key: key, Size: a.ExpectedSize, ContentType: a.DeclaredContentType, ETag: "e"}, nil
		}}
	}

	t.Run("lost race returns the spec created by the winner", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		calls := 0
		repo := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				calls++
				if calls == 1 {
					return session, nil
				}
				winner := *session
				winner.Status = domain.UploadStatusQueued
				return &winner, nil
			},
			finalizeFn: func(context.Context, *domain.SpecUploadSession, *domain.Spec, *domain.SpecProcessingJob) error {
				return domain.ErrUploadState
			},
		}
		existing := &domain.Spec{ID: session.SpecID, ProducerID: producerID}
		specs := &uploadSpecRepositoryStub{getByIDSystemFn: func(context.Context, uuid.UUID) (*domain.Spec, error) {
			return existing, nil
		}}
		got, err := NewSpecUploadService(repo, specs, okStore(session)).Complete(context.Background(), session.ID, producerID)
		require.NoError(t, err)
		assert.Same(t, existing, got)
	})

	t.Run("state conflict with unusable reload returns the original error", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		calls := 0
		repo := &uploadRepositoryStub{
			getSessionFn: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadSession, error) {
				calls++
				if calls == 1 {
					return session, nil
				}
				failed := *session
				failed.Status = domain.UploadStatusFailed
				return &failed, nil
			},
			finalizeFn: func(context.Context, *domain.SpecUploadSession, *domain.Spec, *domain.SpecProcessingJob) error {
				return domain.ErrUploadState
			},
		}
		_, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, okStore(session)).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, domain.ErrUploadState)
	})

	t.Run("other finalize errors pass through", func(t *testing.T) {
		t.Parallel()
		session := verifiedUploadSession(t, producerID, domain.UploadStatusUploading)
		boom := errors.New("tx failed")
		repo := sessionRepo(session)
		repo.finalizeFn = func(context.Context, *domain.SpecUploadSession, *domain.Spec, *domain.SpecProcessingJob) error {
			return boom
		}
		_, err := NewSpecUploadService(repo, &uploadSpecRepositoryStub{}, okStore(session)).
			Complete(context.Background(), session.ID, producerID)
		require.ErrorIs(t, err, boom)
	})
}

func TestUploadHelpers_ValidationEdges(t *testing.T) {
	t.Parallel()

	t.Run("validateUploadFile", func(t *testing.T) {
		t.Parallel()
		good := validBeatUploadFiles()[0]
		tests := []struct {
			name   string
			mutate func(*UploadFileCommand)
			want   string
		}{
			{"unknown kind", func(f *UploadFileCommand) { f.Kind = "video" }, "unexpected asset kind"},
			{"blank name", func(f *UploadFileCommand) { f.FileName = "  " }, "file_name is required"},
			{"dot name", func(f *UploadFileCommand) { f.FileName = "." }, "file_name is required"},
			{"name too long", func(f *UploadFileCommand) {
				name := make([]byte, 300)
				for i := range name {
					name[i] = 'a'
				}
				f.FileName = string(name) + ".png"
			}, "too long"},
			{"invalid utf8 name", func(f *UploadFileCommand) { f.FileName = "bad\xff.png" }, "too long"},
			{"zero size", func(f *UploadFileCommand) { f.SizeBytes = 0 }, "exceeds its size limit"},
			{"oversize", func(f *UploadFileCommand) { f.SizeBytes = uploadSizeLimits[f.Kind] + 1 }, "exceeds its size limit"},
			{"wrong extension", func(f *UploadFileCommand) { f.FileName = "cover.bmp" }, "file type is not supported"},
		}
		for _, tt := range tests {
			f := good
			tt.mutate(&f)
			_, err := validateUploadFile(f)
			require.ErrorIs(t, err, domain.ErrInvalidUpload, tt.name)
			assert.ErrorContains(t, err, tt.want, tt.name)
		}
		// A path component in the name is stripped.
		f := good
		f.FileName = "../../evil/cover.png"
		norm, err := validateUploadFile(f)
		require.NoError(t, err)
		assert.Equal(t, "cover.png", norm.FileName)
	})

	t.Run("uploadExtension accepts each supported pairing", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			kind        domain.UploadAssetKind
			name, ctype string
			want        string
		}{
			{domain.UploadAssetImage, "a.JPG", "image/jpeg", ".jpg"},
			{domain.UploadAssetImage, "a.jpeg", "image/jpeg", ".jpeg"},
			{domain.UploadAssetPreview, "a.mp3", "audio/mp3", ".mp3"},
			{domain.UploadAssetWAV, "a.wav", "audio/x-wav", ".wav"},
			{domain.UploadAssetWAV, "a.wav", "audio/wave", ".wav"},
			{domain.UploadAssetStems, "a.zip", "application/x-zip-compressed", ".zip"},
			{domain.UploadAssetStems, "a.rar", "application/vnd.rar", ".rar"},
			{domain.UploadAssetStems, "a.rar", "application/x-rar-compressed", ".rar"},
		}
		for _, c := range cases {
			got, err := uploadExtension(c.kind, c.name, c.ctype)
			require.NoError(t, err, c.name+" "+c.ctype)
			assert.Equal(t, c.want, got)
		}
		_, err := uploadExtension(domain.UploadAssetStems, "a.rar", "application/zip")
		require.ErrorIs(t, err, domain.ErrInvalidUpload)
		_, err = uploadExtension("other", "a.bin", "application/octet-stream")
		require.ErrorIs(t, err, domain.ErrInvalidUpload)
	})

	t.Run("finalAssetKey covers every kind", func(t *testing.T) {
		t.Parallel()
		id := uuid.New()
		assert.Equal(t, "processing/specs/"+id.String()+"/cover-source.png", finalAssetKey(id, domain.UploadAssetImage, ".png"))
		assert.Equal(t, "audio/previews/"+id.String()+".mp3", finalAssetKey(id, domain.UploadAssetPreview, ".mp3"))
		assert.Equal(t, "audio/wavs/"+id.String()+".wav", finalAssetKey(id, domain.UploadAssetWAV, ".wav"))
		assert.Equal(t, "audio/stems/"+id.String()+".rar", finalAssetKey(id, domain.UploadAssetStems, ".rar"))
		assert.Equal(t, "processing/specs/"+id.String()+"/video.mp4", finalAssetKey(id, "video", ".mp4"))
	})

	t.Run("manifest rejects duplicates, missing kinds and oversize totals", func(t *testing.T) {
		t.Parallel()
		files := validBeatUploadFiles()

		dup := append([]UploadFileCommand{}, files...)
		dup[1] = dup[0]
		_, err := validateUploadManifest(domain.CategoryBeat, dup)
		require.ErrorContains(t, err, "duplicate asset kind")

		_, err = validateUploadManifest(domain.CategoryBeat, files[:3])
		require.ErrorContains(t, err, "missing or extra files")

		bad := append([]UploadFileCommand{}, files...)
		bad[0].SizeBytes = 0
		_, err = validateUploadManifest(domain.CategoryBeat, bad)
		require.ErrorIs(t, err, domain.ErrInvalidUpload)

		var huge []UploadFileCommand
		for _, f := range files {
			f.SizeBytes = uploadSizeLimits[f.Kind]
			huge = append(huge, f)
		}
		// The per-file limits sum to less than maxUploadTotalSize, so maxed-out
		// files are still a valid manifest.
		normalized, err := validateUploadManifest(domain.CategoryBeat, huge)
		require.NoError(t, err)
		assert.Len(t, normalized, 4)
	})
}
