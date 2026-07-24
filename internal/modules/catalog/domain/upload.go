package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UploadStatus string

const (
	UploadStatusUploading  UploadStatus = "uploading"
	UploadStatusQueued     UploadStatus = "queued"
	UploadStatusProcessing UploadStatus = "processing"
	UploadStatusCompleted  UploadStatus = "completed"
	UploadStatusFailed     UploadStatus = "failed"
	UploadStatusExpired    UploadStatus = "expired"
)

type UploadAssetKind string

const (
	UploadAssetImage   UploadAssetKind = "image"
	UploadAssetPreview UploadAssetKind = "preview"
	UploadAssetWAV     UploadAssetKind = "wav"
	UploadAssetStems   UploadAssetKind = "stems"
)

type ProcessingJobStatus string

const (
	ProcessingJobQueued     ProcessingJobStatus = "queued"
	ProcessingJobProcessing ProcessingJobStatus = "processing"
	ProcessingJobCompleted  ProcessingJobStatus = "completed"
	ProcessingJobFailed     ProcessingJobStatus = "failed"
)

type SpecUploadSession struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	SpecID       uuid.UUID       `json:"spec_id" db:"spec_id"`
	ProducerID   uuid.UUID       `json:"producer_id" db:"producer_id"`
	Metadata     json.RawMessage `json:"metadata" db:"metadata"`
	Status       UploadStatus    `json:"status" db:"status"`
	ErrorMessage *string         `json:"error_message,omitempty" db:"error_message"`
	ExpiresAt    time.Time       `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	Assets       []SpecUploadAsset
}

type SpecUploadAsset struct {
	ID                  uuid.UUID       `json:"id" db:"id"`
	SessionID           uuid.UUID       `json:"session_id" db:"session_id"`
	Kind                UploadAssetKind `json:"kind" db:"kind"`
	FileName            string          `json:"file_name" db:"file_name"`
	ObjectKey           string          `json:"object_key" db:"object_key"`
	FinalObjectKey      string          `json:"final_object_key" db:"final_object_key"`
	DeclaredContentType string          `json:"declared_content_type" db:"declared_content_type"`
	ActualContentType   *string         `json:"actual_content_type,omitempty" db:"actual_content_type"`
	ExpectedSize        int64           `json:"expected_size" db:"expected_size"`
	ActualSize          *int64          `json:"actual_size,omitempty" db:"actual_size"`
	ETag                *string         `json:"etag,omitempty" db:"etag"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at" db:"updated_at"`
}

type SpecProcessingJob struct {
	ID           uuid.UUID           `json:"id" db:"id"`
	SpecID       uuid.UUID           `json:"spec_id" db:"spec_id"`
	SessionID    uuid.UUID           `json:"session_id" db:"session_id"`
	Status       ProcessingJobStatus `json:"status" db:"status"`
	WorkerID     *string             `json:"worker_id,omitempty" db:"worker_id"`
	LockedAt     *time.Time          `json:"locked_at,omitempty" db:"locked_at"`
	StartedAt    *time.Time          `json:"started_at,omitempty" db:"started_at"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty" db:"completed_at"`
	ErrorMessage *string             `json:"error_message,omitempty" db:"error_message"`
	CreatedAt    time.Time           `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at" db:"updated_at"`
}

type ProcessingBundle struct {
	Job     SpecProcessingJob
	Session SpecUploadSession
	Spec    Spec
	Assets  []SpecUploadAsset
}

type ProcessedSpecFiles struct {
	ImageURL      string
	PreviewURL    string
	WAVURL        *string
	StemsURL      *string
	Duration      int
	WaveformPeaks pq.Int64Array
}

type SpecUploadStatus struct {
	UploadID         uuid.UUID
	SpecID           uuid.UUID
	ProducerID       uuid.UUID
	Status           UploadStatus
	ProcessingStatus ProcessingStatus
	ErrorMessage     *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	ExpiresAt        time.Time
}

type SpecUploadRepository interface {
	CreateSession(ctx context.Context, session *SpecUploadSession) error
	GetSession(ctx context.Context, uploadID, producerID uuid.UUID) (*SpecUploadSession, error)
	GetStatus(ctx context.Context, uploadID, producerID uuid.UUID) (*SpecUploadStatus, error)
	UpdateSessionMetadata(
		ctx context.Context,
		uploadID, producerID uuid.UUID,
		metadata json.RawMessage,
	) error
	ReplaceAsset(
		ctx context.Context,
		uploadID, producerID uuid.UUID,
		asset *SpecUploadAsset,
	) (*SpecUploadAsset, error)
	VerifyAsset(
		ctx context.Context,
		uploadID, producerID, assetID uuid.UUID,
		actualSize int64,
		actualContentType, etag string,
	) error
	MarkSessionFailed(ctx context.Context, uploadID uuid.UUID, reason string) error
	FinalizeUpload(ctx context.Context, session *SpecUploadSession, spec *Spec, job *SpecProcessingJob) error
	ExpireUploadSessions(ctx context.Context, expiredBefore time.Time) (int64, error)
	RequeueStaleJobs(ctx context.Context, staleBefore time.Time) (int64, error)
	ClaimNextJob(ctx context.Context, workerID string) (*ProcessingBundle, error)
	HeartbeatJob(ctx context.Context, jobID uuid.UUID, workerID string) error
	CompleteJob(ctx context.Context, jobID uuid.UUID, workerID string, result ProcessedSpecFiles) error
	FailJob(ctx context.Context, jobID uuid.UUID, workerID, reason string) error
}
