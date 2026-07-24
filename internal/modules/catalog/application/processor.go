package application

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	notificationDomain "github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
)

const (
	maxCoverDimension = 8000
	maxCoverPixels    = int64(40_000_000)
	minStemsSize      = int64(1024)
)

type UploadNotifier interface {
	Create(
		ctx context.Context,
		userID uuid.UUID,
		title, message string,
		notificationType notificationDomain.NotificationType,
	) error
}

type SpecUploadProcessor struct {
	uploads  domain.SpecUploadRepository
	objects  SpecObjectStore
	notifier UploadNotifier
}

func NewSpecUploadProcessor(
	uploads domain.SpecUploadRepository,
	objects SpecObjectStore,
	notifier UploadNotifier,
) *SpecUploadProcessor {
	return &SpecUploadProcessor{uploads: uploads, objects: objects, notifier: notifier}
}

func (p *SpecUploadProcessor) RequeueStale(ctx context.Context, lease time.Duration) (int64, error) {
	return p.uploads.RequeueStaleJobs(ctx, time.Now().UTC().Add(-lease))
}

func (p *SpecUploadProcessor) ExpireUploads(ctx context.Context) (int64, error) {
	return p.uploads.ExpireUploadSessions(ctx, time.Now().UTC())
}

// ProcessNext claims and processes at most one job. A processing failure is
// persisted as a failed job and does not stop the worker loop. The heartbeat
// keeps long audio/archive operations from being reclaimed by another worker.
func (p *SpecUploadProcessor) ProcessNext(
	ctx context.Context,
	workerID string,
	heartbeatInterval time.Duration,
) (bool, error) {
	bundle, err := p.uploads.ClaimNextJob(ctx, workerID)
	if errors.Is(err, domain.ErrNoProcessingJob) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	processCtx, stopProcessing := context.WithCancel(ctx)
	heartbeatDone := make(chan error, 1)
	go p.heartbeat(processCtx, stopProcessing, heartbeatDone, bundle.Job.ID, workerID, heartbeatInterval)

	result, cleanupKeys, processErr := p.process(processCtx, bundle)
	stopProcessing()
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil {
		// The worker can no longer prove ownership. A later claimant may now be
		// processing this job, so do not mutate its state or delete any objects.
		return true, fmt.Errorf("upload processing lease lost: %w", heartbeatErr)
	}

	err = processErr
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return true, err
		}
		if markErr := p.uploads.FailJob(
			context.Background(), bundle.Job.ID, workerID, err.Error(),
		); markErr != nil {
			return true, fmt.Errorf("process upload: %v; mark failed: %w", err, markErr)
		}
		// Cleanup is safe only after the fenced state transition proves that
		// this worker still owns the job.
		p.cleanup(context.Background(), cleanupKeys)
		p.notify(
			context.Background(),
			bundle.Spec.ProducerID,
			"Upload Failed",
			fmt.Sprintf("Processing for '%s' failed. Please try again.", bundle.Spec.Title),
			notificationDomain.NotificationTypeError,
		)
		return true, nil
	}

	// An ambiguous database error must leave the job leased for recovery. Do
	// not delete final objects because the completion transaction may have
	// committed even if the caller received an error.
	if err := p.uploads.CompleteJob(ctx, bundle.Job.ID, workerID, result); err != nil {
		return true, err
	}

	for _, asset := range bundle.Assets {
		_ = p.objects.Delete(context.Background(), asset.ObjectKey)
		if asset.Kind == domain.UploadAssetImage {
			_ = p.objects.Delete(context.Background(), asset.FinalObjectKey)
		}
	}
	p.notify(
		context.Background(),
		bundle.Spec.ProducerID,
		"Upload Complete",
		fmt.Sprintf("Your beat '%s' is now live!", bundle.Spec.Title),
		notificationDomain.NotificationTypeSuccess,
	)
	return true, nil
}

func (p *SpecUploadProcessor) heartbeat(
	ctx context.Context,
	cancelProcessing context.CancelFunc,
	done chan<- error,
	jobID uuid.UUID,
	workerID string,
	interval time.Duration,
) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- nil
			return
		case <-ticker.C:
			if err := p.uploads.HeartbeatJob(ctx, jobID, workerID); err != nil {
				if ctx.Err() != nil {
					done <- nil
					return
				}
				cancelProcessing()
				done <- err
				return
			}
		}
	}
}

func (p *SpecUploadProcessor) process(
	ctx context.Context,
	bundle *domain.ProcessingBundle,
) (domain.ProcessedSpecFiles, []string, error) {
	assets := make(map[domain.UploadAssetKind]domain.SpecUploadAsset, len(bundle.Assets))
	processedImageKey := fmt.Sprintf("images/%s.jpg", bundle.Spec.ID)
	cleanupKeys := make([]string, 0, len(bundle.Assets)*2+1)
	cleanupKeys = append(cleanupKeys, processedImageKey)
	for _, asset := range bundle.Assets {
		// Deterministic final keys may have been left by an earlier crashed
		// attempt. A fenced failure cleans the complete known key set, even
		// when this attempt fails before its first copy.
		cleanupKeys = append(cleanupKeys, asset.ObjectKey, asset.FinalObjectKey)
		if _, exists := assets[asset.Kind]; exists {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("duplicate %s asset", asset.Kind)
		}
		sourceInfo, err := p.objects.StatObject(ctx, asset.ObjectKey)
		if err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("verify %s asset before copy: %w", asset.Kind, err)
		}
		if asset.ActualSize == nil || sourceInfo.Size != *asset.ActualSize ||
			asset.ActualContentType == nil ||
			normalizeContentType(sourceInfo.ContentType) != *asset.ActualContentType ||
			asset.ETag == nil || sourceInfo.ETag == "" || sourceInfo.ETag != *asset.ETag {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("%s asset changed after upload completion", asset.Kind)
		}
		if _, err := p.objects.CopyObject(
			ctx, asset.ObjectKey, asset.FinalObjectKey, *asset.ETag,
		); err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("copy %s asset: %w", asset.Kind, err)
		}
		destinationInfo, err := p.objects.StatObject(ctx, asset.FinalObjectKey)
		if err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("verify copied %s asset: %w", asset.Kind, err)
		}
		if destinationInfo.Size != sourceInfo.Size {
			return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf(
				"copied %s asset size does not match its verified source", asset.Kind,
			)
		}
		assets[asset.Kind] = asset
	}

	imageAsset, ok := assets[domain.UploadAssetImage]
	if !ok {
		return domain.ProcessedSpecFiles{}, cleanupKeys, errors.New("cover image is missing")
	}
	previewAsset, ok := assets[domain.UploadAssetPreview]
	if !ok {
		return domain.ProcessedSpecFiles{}, cleanupKeys, errors.New("MP3 preview is missing")
	}

	imageBytes, err := p.readBounded(ctx, imageAsset.FinalObjectKey, uploadSizeLimits[domain.UploadAssetImage])
	if err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("read cover image: %w", err)
	}
	processedImage, err := normalizeCoverImage(imageBytes)
	if err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, err
	}
	imageKey := processedImageKey
	if _, err := p.objects.UploadWithKey(ctx, bytes.NewReader(processedImage), imageKey, "image/jpeg"); err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("upload processed cover: %w", err)
	}

	previewReader, err := p.objects.OpenObject(ctx, previewAsset.FinalObjectKey)
	if err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("open preview: %w", err)
	}
	analysis, analysisErr := AnalyzeMP3(previewReader, 64)
	closeErr := previewReader.Close()
	if analysisErr != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, analysisErr
	}
	if closeErr != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, fmt.Errorf("close preview: %w", closeErr)
	}

	var wavURL *string
	var stemsURL *string
	wavAsset, hasWAV := assets[domain.UploadAssetWAV]
	if bundle.Spec.Category == domain.CategoryBeat && !hasWAV {
		return domain.ProcessedSpecFiles{}, cleanupKeys, errors.New("WAV asset is missing")
	}
	if hasWAV {
		if err := p.validateWAV(ctx, wavAsset.FinalObjectKey); err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, err
		}
		url, err := p.objects.ObjectURL(wavAsset.FinalObjectKey)
		if err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, err
		}
		wavURL = &url
	}

	stemsAsset, hasStems := assets[domain.UploadAssetStems]
	if bundle.Spec.Category == domain.CategoryBeat && !hasStems {
		return domain.ProcessedSpecFiles{}, cleanupKeys, errors.New("stems asset is missing")
	}
	if hasStems {
		if err := p.validateStems(ctx, stemsAsset.FinalObjectKey); err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, err
		}
		url, err := p.objects.ObjectURL(stemsAsset.FinalObjectKey)
		if err != nil {
			return domain.ProcessedSpecFiles{}, cleanupKeys, err
		}
		stemsURL = &url
	}

	imageURL, err := p.objects.ObjectURL(imageKey)
	if err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, err
	}
	previewURL, err := p.objects.ObjectURL(previewAsset.FinalObjectKey)
	if err != nil {
		return domain.ProcessedSpecFiles{}, cleanupKeys, err
	}
	return domain.ProcessedSpecFiles{
		ImageURL:      imageURL,
		PreviewURL:    previewURL,
		WAVURL:        wavURL,
		StemsURL:      stemsURL,
		Duration:      analysis.Duration,
		WaveformPeaks: pq.Int64Array(analysis.WaveformPeaks),
	}, cleanupKeys, nil
}

func normalizeCoverImage(imageBytes []byte) ([]byte, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, errors.New("cover image must be a valid JPEG or PNG")
	}
	if config.Width <= 0 || config.Height <= 0 ||
		config.Width > maxCoverDimension || config.Height > maxCoverDimension ||
		int64(config.Width)*int64(config.Height) > maxCoverPixels {
		return nil, errors.New("cover image dimensions are too large")
	}

	sourceImage, err := imaging.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode cover image: %w", err)
	}
	bounds := sourceImage.Bounds()
	squareSize := min(bounds.Dx(), bounds.Dy())
	cropped := imaging.CropCenter(sourceImage, squareSize, squareSize)
	resized := imaging.Fit(cropped, 500, 500, imaging.Lanczos)

	var output bytes.Buffer
	if err := imaging.Encode(&output, resized, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
		return nil, fmt.Errorf("encode cover image: %w", err)
	}
	return output.Bytes(), nil
}

func (p *SpecUploadProcessor) readBounded(ctx context.Context, key string, limit int64) ([]byte, error) {
	info, err := p.objects.StatObject(ctx, key)
	if err != nil {
		return nil, err
	}
	if info.Size <= 0 || info.Size > limit {
		return nil, errors.New("object exceeds its size limit")
	}
	reader, err := p.objects.OpenObject(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("object exceeds its size limit")
	}
	return data, nil
}

func (p *SpecUploadProcessor) validateWAV(ctx context.Context, key string) error {
	info, err := p.objects.StatObject(ctx, key)
	if err != nil {
		return fmt.Errorf("stat WAV: %w", err)
	}
	if info.Size < 44 {
		return errors.New("wav is too small to contain audio")
	}
	reader, err := p.objects.OpenObject(ctx, key)
	if err != nil {
		return fmt.Errorf("open WAV: %w", err)
	}
	defer reader.Close()

	var header [12]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return fmt.Errorf("read WAV header: %w", err)
	}
	if string(header[:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return errors.New("wav must be a WAV file")
	}
	riffSize := int64(binary.LittleEndian.Uint32(header[4:8]))
	if riffSize < 4 || riffSize+8 != info.Size {
		return errors.New("wav RIFF size does not match the object")
	}

	remaining := riffSize - 4 // WAVE form type was consumed with the header.
	var foundFormat, foundAudioData bool
	for remaining > 0 {
		if remaining < 8 {
			return errors.New("wav has a truncated chunk header")
		}
		var chunkHeader [8]byte
		if _, err := io.ReadFull(reader, chunkHeader[:]); err != nil {
			return fmt.Errorf("read WAV chunk header: %w", err)
		}
		remaining -= 8
		chunkSize := int64(binary.LittleEndian.Uint32(chunkHeader[4:8]))
		paddedSize := chunkSize + chunkSize%2
		if chunkSize < 0 || paddedSize > remaining {
			return errors.New("wav chunk exceeds RIFF bounds")
		}

		switch string(chunkHeader[:4]) {
		case "fmt ":
			if foundFormat || chunkSize < 16 {
				return errors.New("wav must contain one valid fmt chunk")
			}
			var format [16]byte
			if _, err := io.ReadFull(reader, format[:]); err != nil {
				return fmt.Errorf("read WAV format: %w", err)
			}
			audioFormat := binary.LittleEndian.Uint16(format[0:2])
			channels := binary.LittleEndian.Uint16(format[2:4])
			sampleRate := binary.LittleEndian.Uint32(format[4:8])
			byteRate := binary.LittleEndian.Uint32(format[8:12])
			blockAlign := binary.LittleEndian.Uint16(format[12:14])
			bitsPerSample := binary.LittleEndian.Uint16(format[14:16])
			if audioFormat != 1 && audioFormat != 3 && audioFormat != 0xfffe {
				return errors.New("wav audio encoding is not supported")
			}
			if channels < 1 || channels > 8 ||
				sampleRate < 8_000 || sampleRate > 384_000 {
				return errors.New("wav channel count or sample rate is invalid")
			}
			if bitsPerSample != 8 && bitsPerSample != 16 &&
				bitsPerSample != 24 && bitsPerSample != 32 {
				return errors.New("wav bit depth is invalid")
			}
			expectedBlockAlign := uint32(channels) * uint32((bitsPerSample+7)/8)
			if uint32(blockAlign) != expectedBlockAlign ||
				uint64(byteRate) != uint64(sampleRate)*uint64(blockAlign) {
				return errors.New("wav byte rate or block alignment is invalid")
			}
			if _, err := io.CopyN(io.Discard, reader, chunkSize-16); err != nil {
				return fmt.Errorf("read WAV format extension: %w", err)
			}
			foundFormat = true
		case "data":
			if chunkSize == 0 {
				return errors.New("wav data chunk is empty")
			}
			if _, err := io.CopyN(io.Discard, reader, chunkSize); err != nil {
				return fmt.Errorf("read WAV audio data: %w", err)
			}
			foundAudioData = true
		default:
			if _, err := io.CopyN(io.Discard, reader, chunkSize); err != nil {
				return fmt.Errorf("read WAV chunk: %w", err)
			}
		}
		if chunkSize%2 != 0 {
			var padding [1]byte
			if _, err := io.ReadFull(reader, padding[:]); err != nil {
				return fmt.Errorf("read WAV chunk padding: %w", err)
			}
		}
		remaining -= paddedSize
	}
	if !foundFormat || !foundAudioData {
		return errors.New("wav must contain valid fmt and non-empty data chunks")
	}
	return nil
}

func (p *SpecUploadProcessor) validateStems(ctx context.Context, key string) error {
	info, err := p.objects.StatObject(ctx, key)
	if err != nil {
		return fmt.Errorf("stat stems archive: %w", err)
	}
	if info.Size < minStemsSize {
		return errors.New("stems archive is too small")
	}
	header, err := p.readHeader(ctx, key, 30)
	if err != nil {
		return fmt.Errorf("read stems header: %w", err)
	}
	extension := strings.ToLower(filepath.Ext(key))
	if extension == ".zip" && bytes.HasPrefix(header, []byte{'P', 'K', 0x03, 0x04}) {
		if len(header) < 30 {
			return errors.New("stems ZIP has a truncated local file header")
		}
		flags := binary.LittleEndian.Uint16(header[6:8])
		compression := binary.LittleEndian.Uint16(header[8:10])
		fileNameLength := binary.LittleEndian.Uint16(header[26:28])
		extraLength := binary.LittleEndian.Uint16(header[28:30])
		if flags&1 != 0 {
			return errors.New("encrypted stems ZIP archives are not supported")
		}
		if compression != 0 && compression != 8 {
			return errors.New("stems ZIP compression method is not supported")
		}
		if fileNameLength == 0 || int64(30)+int64(fileNameLength)+int64(extraLength) > info.Size {
			return errors.New("stems ZIP local file header is invalid")
		}
		return nil
	}
	if extension == ".rar" && isRARHeader(header) {
		return nil
	}
	return errors.New("stems archive contents do not match its extension")
}

func (p *SpecUploadProcessor) readHeader(ctx context.Context, key string, size int64) ([]byte, error) {
	reader, err := p.objects.OpenObject(ctx, key)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, size))
}

func isRARHeader(header []byte) bool {
	return bytes.HasPrefix(header, []byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x00}) ||
		bytes.HasPrefix(header, []byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x00})
}

func (p *SpecUploadProcessor) cleanup(ctx context.Context, keys []string) {
	for _, key := range keys {
		_ = p.objects.Delete(ctx, key)
	}
}

func (p *SpecUploadProcessor) notify(
	ctx context.Context,
	userID uuid.UUID,
	title, message string,
	notificationType notificationDomain.NotificationType,
) {
	if p.notifier != nil {
		_ = p.notifier.Create(ctx, userID, title, message, notificationType)
	}
}
