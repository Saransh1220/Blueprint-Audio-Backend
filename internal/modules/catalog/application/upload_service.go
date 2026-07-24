package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
)

const (
	specUploadURLTTL     = 4 * time.Hour
	specUploadSessionTTL = 24 * time.Hour
	maxUploadTotalSize   = int64(1500 << 20)
)

var uploadSizeLimits = map[domain.UploadAssetKind]int64{
	domain.UploadAssetImage:   5 << 20,
	domain.UploadAssetPreview: 30 << 20,
	domain.UploadAssetWAV:     300 << 20,
	domain.UploadAssetStems:   1 << 30,
}

type SpecObjectStore interface {
	CreatePresignedUpload(ctx context.Context, key, contentType string, expectedSize int64, ttl time.Duration) (filestorageDomain.PresignedUpload, error)
	StatObject(ctx context.Context, key string) (filestorageDomain.ObjectInfo, error)
	OpenObject(ctx context.Context, key string) (io.ReadCloser, error)
	CopyObject(ctx context.Context, source, destination, expectedETag string) (string, error)
	ObjectURL(key string) (string, error)
	UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error)
	Delete(ctx context.Context, key string) error
}

type UploadFileCommand struct {
	Kind        domain.UploadAssetKind
	FileName    string
	ContentType string
	SizeBytes   int64
}

type UploadInstruction struct {
	AssetID   uuid.UUID
	Kind      domain.UploadAssetKind
	ObjectKey string
	Upload    filestorageDomain.PresignedUpload
}

type InitiateSpecUploadResult struct {
	UploadID  uuid.UUID
	SpecID    uuid.UUID
	ExpiresAt time.Time
}

type SpecUploadService interface {
	Initiate(ctx context.Context, producerID uuid.UUID) (*InitiateSpecUploadResult, error)
	SaveMetadata(
		ctx context.Context,
		uploadID, producerID uuid.UUID,
		spec domain.Spec,
	) error
	PrepareFile(
		ctx context.Context,
		uploadID, producerID uuid.UUID,
		file UploadFileCommand,
	) (*UploadInstruction, error)
	ConfirmFile(
		ctx context.Context,
		uploadID, producerID, assetID uuid.UUID,
	) (*domain.SpecUploadAsset, error)
	Complete(ctx context.Context, uploadID, producerID uuid.UUID) (*domain.Spec, error)
	Status(ctx context.Context, uploadID, producerID uuid.UUID) (*domain.SpecUploadStatus, error)
}

type specUploadService struct {
	uploads domain.SpecUploadRepository
	specs   domain.SpecRepository
	objects SpecObjectStore
}

func NewSpecUploadService(
	uploads domain.SpecUploadRepository,
	specs domain.SpecRepository,
	objects SpecObjectStore,
) SpecUploadService {
	return &specUploadService{uploads: uploads, specs: specs, objects: objects}
}

func (s *specUploadService) Initiate(
	ctx context.Context,
	producerID uuid.UUID,
) (*InitiateSpecUploadResult, error) {
	specID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	uploadID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expiresAt := now.Add(specUploadSessionTTL)
	session := &domain.SpecUploadSession{
		ID:         uploadID,
		SpecID:     specID,
		ProducerID: producerID,
		Metadata:   json.RawMessage(`{}`),
		Status:     domain.UploadStatusUploading,
		ExpiresAt:  expiresAt,
		CreatedAt:  now,
		UpdatedAt:  now,
		Assets:     []domain.SpecUploadAsset{},
	}
	result := &InitiateSpecUploadResult{
		UploadID:  uploadID,
		SpecID:    specID,
		ExpiresAt: expiresAt,
	}

	if err := s.uploads.CreateSession(ctx, session); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *specUploadService) SaveMetadata(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	spec domain.Spec,
) error {
	session, err := s.requireUploadingSession(ctx, uploadID, producerID)
	if err != nil {
		return err
	}
	if err := s.prepareUploadSpec(ctx, session.SpecID, producerID, &spec); err != nil {
		return err
	}
	metadata, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("encode upload metadata: %w", err)
	}
	return s.uploads.UpdateSessionMetadata(ctx, uploadID, producerID, metadata)
}

func (s *specUploadService) PrepareFile(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	file UploadFileCommand,
) (*UploadInstruction, error) {
	session, err := s.requireUploadingSession(ctx, uploadID, producerID)
	if err != nil {
		return nil, err
	}
	normalized, err := validateUploadFile(file)
	if err != nil {
		return nil, err
	}
	extension, err := uploadExtension(
		normalized.Kind, normalized.FileName, normalized.ContentType,
	)
	if err != nil {
		return nil, err
	}
	assetID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	stagingKey := fmt.Sprintf(
		"incoming/specs/%s/%s/%s/%s%s",
		producerID, uploadID, normalized.Kind, assetID, extension,
	)
	presigned, err := s.objects.CreatePresignedUpload(
		ctx, stagingKey, normalized.ContentType, normalized.SizeBytes, specUploadURLTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("create upload URL for %s: %w", normalized.Kind, err)
	}
	now := time.Now().UTC()
	asset := &domain.SpecUploadAsset{
		ID:                  assetID,
		SessionID:           uploadID,
		Kind:                normalized.Kind,
		FileName:            normalized.FileName,
		ObjectKey:           stagingKey,
		FinalObjectKey:      finalAssetKey(session.SpecID, normalized.Kind, extension),
		DeclaredContentType: normalized.ContentType,
		ExpectedSize:        normalized.SizeBytes,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	previous, err := s.uploads.ReplaceAsset(ctx, uploadID, producerID, asset)
	if err != nil {
		return nil, err
	}
	if previous != nil && previous.ObjectKey != "" && previous.ObjectKey != stagingKey {
		_ = s.objects.Delete(context.Background(), previous.ObjectKey)
	}
	return &UploadInstruction{
		AssetID:   assetID,
		Kind:      normalized.Kind,
		ObjectKey: stagingKey,
		Upload:    presigned,
	}, nil
}

func (s *specUploadService) ConfirmFile(
	ctx context.Context,
	uploadID, producerID, assetID uuid.UUID,
) (*domain.SpecUploadAsset, error) {
	session, err := s.requireUploadingSession(ctx, uploadID, producerID)
	if err != nil {
		return nil, err
	}
	var asset *domain.SpecUploadAsset
	for i := range session.Assets {
		if session.Assets[i].ID == assetID {
			asset = &session.Assets[i]
			break
		}
	}
	if asset == nil {
		return nil, domain.ErrUploadState
	}
	info, err := s.verifyUploadedObject(ctx, asset)
	if err != nil {
		return nil, err
	}
	actualContentType := normalizeContentType(info.ContentType)
	if err := s.uploads.VerifyAsset(
		ctx,
		uploadID,
		producerID,
		assetID,
		info.Size,
		actualContentType,
		info.ETag,
	); err != nil {
		return nil, err
	}
	actualSize := info.Size
	asset.ActualSize = &actualSize
	asset.ActualContentType = &actualContentType
	asset.ETag = &info.ETag
	return asset, nil
}

func (s *specUploadService) requireUploadingSession(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.SpecUploadSession, error) {
	session, err := s.uploads.GetSession(ctx, uploadID, producerID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.UploadStatusUploading {
		return nil, domain.ErrUploadState
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, domain.ErrUploadExpired
	}
	return session, nil
}

func (s *specUploadService) prepareUploadSpec(
	ctx context.Context,
	specID, producerID uuid.UUID,
	spec *domain.Spec,
) error {
	if spec.Category != domain.CategoryBeat {
		return invalidUpload(errors.New("direct upload currently supports beats only"))
	}
	spec.ID = specID
	spec.ProducerID = producerID
	spec.Type = "beat"
	spec.ProcessingStatus = domain.ProcessingStatusProcessing
	spec.ImageUrl = ""
	spec.PreviewUrl = ""
	spec.WavUrl = nil
	spec.StemsUrl = nil
	spec.Duration = 0
	spec.WaveformPeaks = nil
	spec.CreatedAt = time.Time{}
	spec.UpdatedAt = time.Time{}
	spec.DeletedAt = nil
	spec.IsDeleted = false
	if err := deriveBeatUploadMetadata(spec); err != nil {
		return invalidUpload(err)
	}

	preparer := &specService{repo: s.specs}
	if err := preparer.prepareSpecForCreate(ctx, spec); err != nil {
		return invalidUpload(err)
	}
	// Draft sessions are not represented in specs yet. Tie public identifiers
	// to the reserved spec ID so concurrent drafts with the same title cannot
	// race on slug or short-code uniqueness.
	idSegment := strings.ReplaceAll(specID.String(), "-", "")
	slugSuffix := idSegment[len(idSegment)-12:]
	shortCode := idSegment[len(idSegment)-8:]
	baseSlug := "untitled-beat"
	if spec.Slug != nil && strings.TrimSpace(*spec.Slug) != "" {
		baseSlug = strings.TrimSpace(*spec.Slug)
	}
	uploadSlug := fmt.Sprintf("%s-%s", baseSlug, slugSuffix)
	spec.Slug = &uploadSlug
	spec.ShortCode = &shortCode
	return nil
}

func (s *specUploadService) verifyUploadedObject(
	ctx context.Context,
	asset *domain.SpecUploadAsset,
) (filestorageDomain.ObjectInfo, error) {
	info, err := s.objects.StatObject(ctx, asset.ObjectKey)
	if err != nil {
		return filestorageDomain.ObjectInfo{}, invalidUpload(
			fmt.Errorf("%s upload is missing: %w", asset.Kind, err),
		)
	}
	limit := uploadSizeLimits[asset.Kind]
	if info.Size <= 0 || info.Size > limit || info.Size != asset.ExpectedSize {
		return filestorageDomain.ObjectInfo{}, invalidUpload(fmt.Errorf(
			"%s size does not match the declared file", asset.Kind,
		))
	}
	actualContentType := normalizeContentType(info.ContentType)
	if actualContentType == "" || actualContentType != asset.DeclaredContentType {
		return filestorageDomain.ObjectInfo{}, invalidUpload(fmt.Errorf(
			"%s content type does not match the declared file", asset.Kind,
		))
	}
	if info.ETag == "" {
		return filestorageDomain.ObjectInfo{}, invalidUpload(
			fmt.Errorf("%s object checksum is missing", asset.Kind),
		)
	}
	return info, nil
}

func deriveBeatUploadMetadata(spec *domain.Spec) error {
	if len(spec.Licenses) == 0 {
		return errors.New("at least one license is required")
	}
	currency := ""
	minimumPrice := spec.Licenses[0].Price
	for i := range spec.Licenses {
		licenseCurrency, err := normalizeCurrency(spec.Licenses[i].PriceCurrency, "")
		if err != nil {
			return fmt.Errorf("every license must declare a supported currency")
		}
		if currency == "" {
			currency = licenseCurrency
		} else if currency != licenseCurrency {
			return errors.New("all licenses must use the same currency")
		}
		spec.Licenses[i].PriceCurrency = licenseCurrency
		if spec.Licenses[i].Price < minimumPrice {
			minimumPrice = spec.Licenses[i].Price
		}
	}
	spec.BasePrice = minimumPrice
	spec.PriceCurrency = currency
	for i := range spec.Genres {
		spec.Genres[i].Name = strings.ToUpper(strings.TrimSpace(spec.Genres[i].Name))
		// Preserve the canonical slugs created by the previous uploader
		// (notably R&B -> r&b) so existing UNIQUE name/slug rows are reused.
		spec.Genres[i].Slug = legacyGenreSlug(spec.Genres[i].Name)
	}
	return nil
}

func legacyGenreSlug(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "-"))
}

func (s *specUploadService) Complete(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.Spec, error) {
	session, err := s.uploads.GetSession(ctx, uploadID, producerID)
	if err != nil {
		return nil, err
	}
	switch session.Status {
	case domain.UploadStatusQueued, domain.UploadStatusProcessing, domain.UploadStatusCompleted:
		return s.getOwnedSessionSpec(ctx, session, producerID)
	case domain.UploadStatusExpired:
		return nil, domain.ErrUploadExpired
	case domain.UploadStatusFailed:
		return nil, domain.ErrUploadState
	case domain.UploadStatusUploading:
		// Continue with object verification and the atomic queue transition.
	default:
		return nil, domain.ErrUploadState
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		return nil, domain.ErrUploadExpired
	}
	if err := validateReadyAssets(domain.CategoryBeat, session.Assets); err != nil {
		return nil, err
	}

	for i := range session.Assets {
		asset := &session.Assets[i]
		info, err := s.verifyUploadedObject(ctx, asset)
		if err != nil {
			return nil, err
		}
		actualSize := info.Size
		actualContentType := normalizeContentType(info.ContentType)
		etag := info.ETag
		asset.ActualSize = &actualSize
		asset.ActualContentType = &actualContentType
		asset.ETag = &etag
	}

	if string(bytes.TrimSpace(session.Metadata)) == "{}" {
		return nil, invalidUpload(errors.New("beat metadata has not been saved"))
	}
	var spec domain.Spec
	if err := json.Unmarshal(session.Metadata, &spec); err != nil {
		return nil, fmt.Errorf("decode stored upload metadata: %w", err)
	}
	if spec.ID != session.SpecID || spec.ProducerID != producerID {
		return nil, domain.ErrUploadState
	}
	spec.ProcessingStatus = domain.ProcessingStatusProcessing
	spec.ImageUrl = ""
	spec.PreviewUrl = ""
	spec.WavUrl = nil
	spec.StemsUrl = nil
	spec.Duration = 0
	spec.WaveformPeaks = nil

	jobID, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	job := &domain.SpecProcessingJob{
		ID:        jobID,
		SpecID:    spec.ID,
		SessionID: uploadID,
		Status:    domain.ProcessingJobQueued,
	}
	if err := s.uploads.FinalizeUpload(ctx, session, &spec, job); err != nil {
		// A concurrent completion request may have committed after this request
		// loaded the session. Treat that transition like a retry after a lost
		// HTTP response and return the already-created spec.
		if errors.Is(err, domain.ErrUploadState) {
			current, getErr := s.uploads.GetSession(ctx, uploadID, producerID)
			if getErr == nil {
				switch current.Status {
				case domain.UploadStatusQueued, domain.UploadStatusProcessing, domain.UploadStatusCompleted:
					return s.getOwnedSessionSpec(ctx, current, producerID)
				}
			}
		}
		return nil, err
	}
	return &spec, nil
}

func (s *specUploadService) getOwnedSessionSpec(
	ctx context.Context,
	session *domain.SpecUploadSession,
	producerID uuid.UUID,
) (*domain.Spec, error) {
	spec, err := s.specs.GetByIDSystem(ctx, session.SpecID)
	if err != nil {
		return nil, err
	}
	if spec.ProducerID != producerID {
		return nil, domain.ErrUploadForbidden
	}
	return spec, nil
}

func (s *specUploadService) Status(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.SpecUploadStatus, error) {
	status, err := s.uploads.GetStatus(ctx, uploadID, producerID)
	if err != nil {
		return nil, err
	}
	if status.Status == domain.UploadStatusUploading && time.Now().UTC().After(status.ExpiresAt) {
		status.Status = domain.UploadStatusExpired
	}
	return status, nil
}

func validateUploadFile(file UploadFileCommand) (UploadFileCommand, error) {
	if _, recognized := uploadSizeLimits[file.Kind]; !recognized {
		return UploadFileCommand{}, invalidUpload(fmt.Errorf("unexpected asset kind %q", file.Kind))
	}
	file.FileName = strings.TrimSpace(filepath.Base(file.FileName))
	file.ContentType = normalizeContentType(file.ContentType)
	if file.FileName == "" || file.FileName == "." {
		return UploadFileCommand{}, invalidUpload(
			fmt.Errorf("%s file_name is required", file.Kind),
		)
	}
	if !utf8.ValidString(file.FileName) ||
		len(file.FileName) > 255 ||
		utf8.RuneCountInString(file.FileName) > 255 {
		return UploadFileCommand{}, invalidUpload(
			fmt.Errorf("%s file_name is too long", file.Kind),
		)
	}
	limit := uploadSizeLimits[file.Kind]
	if file.SizeBytes <= 0 || file.SizeBytes > limit {
		return UploadFileCommand{}, invalidUpload(
			fmt.Errorf("%s exceeds its size limit", file.Kind),
		)
	}
	if _, err := uploadExtension(file.Kind, file.FileName, file.ContentType); err != nil {
		return UploadFileCommand{}, err
	}
	return file, nil
}

func validateUploadManifest(category domain.Category, files []UploadFileCommand) ([]UploadFileCommand, error) {
	if category != domain.CategoryBeat {
		return nil, invalidUpload(errors.New("direct upload currently supports beats only"))
	}
	required := map[domain.UploadAssetKind]bool{
		domain.UploadAssetImage:   true,
		domain.UploadAssetPreview: true,
		domain.UploadAssetWAV:     true,
		domain.UploadAssetStems:   true,
	}
	if len(files) < len(required) || len(files) > len(uploadSizeLimits) {
		return nil, invalidUpload(errors.New("upload manifest has missing or extra files"))
	}

	seen := make(map[domain.UploadAssetKind]bool, len(files))
	var total int64
	normalized := make([]UploadFileCommand, 0, len(files))
	for _, file := range files {
		if seen[file.Kind] {
			return nil, invalidUpload(fmt.Errorf("duplicate asset kind %q", file.Kind))
		}
		seen[file.Kind] = true
		var err error
		file, err = validateUploadFile(file)
		if err != nil {
			return nil, err
		}
		total += file.SizeBytes
		normalized = append(normalized, file)
	}
	for kind := range required {
		if !seen[kind] {
			return nil, invalidUpload(fmt.Errorf("required %s asset is missing", kind))
		}
	}
	if total > maxUploadTotalSize {
		return nil, invalidUpload(errors.New("combined upload exceeds its size limit"))
	}
	return normalized, nil
}

func validateReadyAssets(
	category domain.Category,
	assets []domain.SpecUploadAsset,
) error {
	files := make([]UploadFileCommand, 0, len(assets))
	for _, asset := range assets {
		if asset.ActualSize == nil ||
			asset.ActualContentType == nil ||
			asset.ETag == nil ||
			*asset.ActualSize <= 0 ||
			strings.TrimSpace(*asset.ActualContentType) == "" ||
			strings.TrimSpace(*asset.ETag) == "" {
			return invalidUpload(fmt.Errorf("%s upload has not finished", asset.Kind))
		}
		files = append(files, UploadFileCommand{
			Kind:        asset.Kind,
			FileName:    asset.FileName,
			ContentType: asset.DeclaredContentType,
			SizeBytes:   asset.ExpectedSize,
		})
	}
	_, err := validateUploadManifest(category, files)
	return err
}

func uploadExtension(kind domain.UploadAssetKind, fileName, contentType string) (string, error) {
	extension := strings.ToLower(filepath.Ext(fileName))
	contentType = normalizeContentType(contentType)
	valid := false
	switch kind {
	case domain.UploadAssetImage:
		valid = (extension == ".jpg" || extension == ".jpeg") && contentType == "image/jpeg"
		valid = valid || (extension == ".png" && contentType == "image/png")
	case domain.UploadAssetPreview:
		valid = extension == ".mp3" && (contentType == "audio/mpeg" || contentType == "audio/mp3")
		extension = ".mp3"
	case domain.UploadAssetWAV:
		valid = extension == ".wav" && (contentType == "audio/wav" || contentType == "audio/x-wav" || contentType == "audio/wave")
		extension = ".wav"
	case domain.UploadAssetStems:
		valid = extension == ".zip" && (contentType == "application/zip" || contentType == "application/x-zip-compressed")
		valid = valid || (extension == ".rar" && (contentType == "application/vnd.rar" || contentType == "application/x-rar-compressed"))
	}
	if !valid {
		return "", invalidUpload(fmt.Errorf("%s file type is not supported", kind))
	}
	return extension, nil
}

func finalAssetKey(specID uuid.UUID, kind domain.UploadAssetKind, extension string) string {
	switch kind {
	case domain.UploadAssetImage:
		return fmt.Sprintf("processing/specs/%s/cover-source%s", specID, extension)
	case domain.UploadAssetPreview:
		return fmt.Sprintf("audio/previews/%s.mp3", specID)
	case domain.UploadAssetWAV:
		return fmt.Sprintf("audio/wavs/%s.wav", specID)
	case domain.UploadAssetStems:
		return fmt.Sprintf("audio/stems/%s%s", specID, extension)
	default:
		return fmt.Sprintf("processing/specs/%s/%s%s", specID, kind, extension)
	}
}

func normalizeContentType(value string) string {
	value, _, _ = strings.Cut(value, ";")
	return strings.ToLower(strings.TrimSpace(value))
}

func invalidUpload(err error) error {
	return fmt.Errorf("%w: %v", domain.ErrInvalidUpload, err)
}

var _ SpecUploadService = (*specUploadService)(nil)
