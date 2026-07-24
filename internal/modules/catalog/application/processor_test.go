package application

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeCoverImageCenterCropsRectangularCover(t *testing.T) {
	t.Parallel()

	source := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	draw.Draw(source, source.Bounds(), &image.Uniform{C: color.RGBA{R: 240, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(
		source,
		image.Rect(200, 0, 1000, 800),
		&image.Uniform{C: color.RGBA{G: 220, A: 255}},
		image.Point{},
		draw.Src,
	)
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, source))

	normalized, err := normalizeCoverImage(encoded.Bytes())
	require.NoError(t, err)

	config, format, err := image.DecodeConfig(bytes.NewReader(normalized))
	require.NoError(t, err)
	assert.Equal(t, "jpeg", format)
	assert.Equal(t, 500, config.Width)
	assert.Equal(t, 500, config.Height)

	result, _, err := image.Decode(bytes.NewReader(normalized))
	require.NoError(t, err)
	center := color.RGBAModel.Convert(result.At(250, 250)).(color.RGBA)
	assert.Greater(t, center.G, center.R)
}

func TestSpecUploadProcessor_ProcessFailureCleansOnlyAfterFencedFailure(t *testing.T) {
	t.Parallel()

	t.Run("successful fenced transition happens before cleanup", func(t *testing.T) {
		t.Parallel()

		bundle := failingProcessingBundle()
		var mu sync.Mutex
		var events []string
		record := func(event string) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, event)
		}
		uploads := &uploadRepositoryStub{
			claimNextJobFn: func(_ context.Context, workerID string) (*domain.ProcessingBundle, error) {
				assert.Equal(t, "worker-a", workerID)
				return bundle, nil
			},
			failJobFn: func(
				_ context.Context,
				jobID uuid.UUID,
				workerID, reason string,
			) error {
				assert.Equal(t, bundle.Job.ID, jobID)
				assert.Equal(t, "worker-a", workerID)
				assert.Contains(t, reason, "source unavailable")
				record("fail")
				return nil
			},
		}
		objects := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{}, errors.New("source unavailable")
			},
			deleteFn: func(_ context.Context, key string) error {
				record("delete:" + key)
				return nil
			},
		}

		processed, err := NewSpecUploadProcessor(uploads, objects, nil).
			ProcessNext(context.Background(), "worker-a", time.Hour)
		require.NoError(t, err)
		assert.True(t, processed)

		mu.Lock()
		gotEvents := append([]string(nil), events...)
		mu.Unlock()
		require.NotEmpty(t, gotEvents)
		assert.Equal(t, "fail", gotEvents[0])
		assert.Equal(t, []string{
			"fail",
			"delete:" + fmt.Sprintf("images/%s.jpg", bundle.Spec.ID),
			"delete:" + bundle.Assets[0].ObjectKey,
			"delete:" + bundle.Assets[0].FinalObjectKey,
		}, gotEvents)
	})

	t.Run("failed fencing leaves every object untouched", func(t *testing.T) {
		t.Parallel()

		bundle := failingProcessingBundle()
		deleteCalls := 0
		uploads := &uploadRepositoryStub{
			claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
				return bundle, nil
			},
			failJobFn: func(context.Context, uuid.UUID, string, string) error {
				return domain.ErrUploadState
			},
		}
		objects := &objectStoreStub{
			statObjectFn: func(context.Context, string) (filestorageDomain.ObjectInfo, error) {
				return filestorageDomain.ObjectInfo{}, errors.New("source unavailable")
			},
			deleteFn: func(context.Context, string) error {
				deleteCalls++
				return nil
			},
		}

		processed, err := NewSpecUploadProcessor(uploads, objects, nil).
			ProcessNext(context.Background(), "worker-b", time.Hour)
		require.ErrorIs(t, err, domain.ErrUploadState)
		assert.True(t, processed)
		assert.Zero(t, deleteCalls)
	})
}

func TestSpecUploadProcessor_HeartbeatLossDoesNotMutateOrCleanup(t *testing.T) {
	t.Parallel()

	bundle := failingProcessingBundle()
	leaseLost := errors.New("worker no longer owns job")
	failCalls := 0
	deleteCalls := 0
	heartbeatCalls := 0
	uploads := &uploadRepositoryStub{
		claimNextJobFn: func(context.Context, string) (*domain.ProcessingBundle, error) {
			return bundle, nil
		},
		heartbeatJobFn: func(_ context.Context, jobID uuid.UUID, workerID string) error {
			heartbeatCalls++
			assert.Equal(t, bundle.Job.ID, jobID)
			assert.Equal(t, "worker-c", workerID)
			return leaseLost
		},
		failJobFn: func(context.Context, uuid.UUID, string, string) error {
			failCalls++
			return nil
		},
	}
	objects := &objectStoreStub{
		statObjectFn: func(ctx context.Context, _ string) (filestorageDomain.ObjectInfo, error) {
			<-ctx.Done()
			return filestorageDomain.ObjectInfo{}, ctx.Err()
		},
		deleteFn: func(context.Context, string) error {
			deleteCalls++
			return nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	processed, err := NewSpecUploadProcessor(uploads, objects, nil).
		ProcessNext(ctx, "worker-c", time.Millisecond)
	require.Error(t, err)
	assert.ErrorContains(t, err, "upload processing lease lost")
	assert.ErrorIs(t, err, leaseLost)
	assert.True(t, processed)
	assert.Positive(t, heartbeatCalls)
	assert.Zero(t, failCalls)
	assert.Zero(t, deleteCalls)
}

func TestSpecUploadProcessor_ValidateWAVRejectsMagicOnlyFile(t *testing.T) {
	t.Parallel()

	valid := minimalPCM16WAV()
	magicOnly := make([]byte, 44)
	copy(magicOnly[0:4], "RIFF")
	binary.LittleEndian.PutUint32(magicOnly[4:8], uint32(len(magicOnly)-8))
	copy(magicOnly[8:12], "WAVE")

	tests := []struct {
		name    string
		content []byte
		wantErr string
	}{
		{
			name:    "minimal valid PCM WAV",
			content: valid,
		},
		{
			name:    "RIFF and WAVE magic without audio chunks",
			content: magicOnly,
			wantErr: "valid fmt and non-empty data chunks",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const key = "audio/wavs/test.wav"
			objects := &objectStoreStub{
				statObjectFn: func(_ context.Context, gotKey string) (filestorageDomain.ObjectInfo, error) {
					assert.Equal(t, key, gotKey)
					return filestorageDomain.ObjectInfo{
						Key:         key,
						Size:        int64(len(tt.content)),
						ContentType: "audio/wav",
					}, nil
				},
				openObjectFn: func(_ context.Context, gotKey string) (io.ReadCloser, error) {
					assert.Equal(t, key, gotKey)
					return io.NopCloser(bytes.NewReader(tt.content)), nil
				},
			}

			err := NewSpecUploadProcessor(&uploadRepositoryStub{}, objects, nil).
				validateWAV(context.Background(), key)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func failingProcessingBundle() *domain.ProcessingBundle {
	specID := uuid.New()
	size := int64(1024)
	contentType := "image/png"
	etag := "etag-image"
	return &domain.ProcessingBundle{
		Job: domain.SpecProcessingJob{
			ID:     uuid.New(),
			SpecID: specID,
			Status: domain.ProcessingJobProcessing,
		},
		Spec: domain.Spec{
			ID:         specID,
			ProducerID: uuid.New(),
			Title:      "Night Drive",
			Category:   domain.CategoryBeat,
		},
		Assets: []domain.SpecUploadAsset{
			{
				Kind:              domain.UploadAssetImage,
				ObjectKey:         "incoming/specs/upload/image.png",
				FinalObjectKey:    fmt.Sprintf("processing/specs/%s/cover-source.png", specID),
				ActualSize:        &size,
				ActualContentType: &contentType,
				ETag:              &etag,
			},
		},
	}
}

func minimalPCM16WAV() []byte {
	var data bytes.Buffer
	data.WriteString("RIFF")
	_ = binary.Write(&data, binary.LittleEndian, uint32(0))
	data.WriteString("WAVE")
	data.WriteString("fmt ")
	_ = binary.Write(&data, binary.LittleEndian, uint32(16))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint32(8_000))
	_ = binary.Write(&data, binary.LittleEndian, uint32(16_000))
	_ = binary.Write(&data, binary.LittleEndian, uint16(2))
	_ = binary.Write(&data, binary.LittleEndian, uint16(16))
	data.WriteString("data")
	_ = binary.Write(&data, binary.LittleEndian, uint32(4))
	data.Write([]byte{1, 0, 2, 0})

	result := data.Bytes()
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}
