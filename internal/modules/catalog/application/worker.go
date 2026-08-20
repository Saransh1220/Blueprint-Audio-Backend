package application

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
)

// StartUploadWorker starts the durable background upload processor in a loop until ctx is canceled.
func StartUploadWorker(
	ctx context.Context,
	processor *SpecUploadProcessor,
	cfg config.WorkerConfig,
) {
	workerPrefix := strings.TrimSpace(cfg.ID)
	if workerPrefix == "" {
		host, _ := os.Hostname()
		workerPrefix = host
	}
	workerPrefix = NormalizeWorkerPrefix(workerPrefix)
	workerID := workerPrefix + "-" + uuid.NewString()

	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	lease := cfg.LeaseDuration
	if lease <= 0 {
		lease = 30 * time.Minute
	}
	requeueInterval := lease / 2
	if requeueInterval < time.Minute {
		requeueInterval = time.Minute
	}
	nextRequeue := time.Time{}

	log.Printf("spec upload worker started id=%s poll=%s lease=%s", workerID, pollInterval, lease)
	for ctx.Err() == nil {
		now := time.Now()
		if nextRequeue.IsZero() || now.After(nextRequeue) {
			expired, err := processor.ExpireUploads(ctx)
			if err != nil && ctx.Err() == nil {
				log.Printf("expire abandoned upload sessions: %v", err)
			} else if expired > 0 {
				log.Printf("expired %d abandoned upload sessions", expired)
			}
			count, err := processor.RequeueStale(ctx, lease)
			if err != nil && ctx.Err() == nil {
				log.Printf("requeue stale upload jobs: %v", err)
			} else if count > 0 {
				log.Printf("requeued %d stale upload jobs", count)
			}
			nextRequeue = now.Add(requeueInterval)
		}

		processed, err := processor.ProcessNext(ctx, workerID, lease/3)
		if err != nil && ctx.Err() == nil {
			log.Printf("process upload job: %v", err)
		}
		if processed {
			continue
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	log.Printf("spec upload worker stopped (id=%s)", workerID)
}

func NormalizeWorkerPrefix(value string) string {
	const maxPrefixLength = 83 // 120-column limit minus "-" and UUID.
	var normalized strings.Builder
	normalized.Grow(min(len(value), maxPrefixLength))
	for _, char := range value {
		if normalized.Len() >= maxPrefixLength {
			break
		}
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char == '-', char == '_', char == '.':
			normalized.WriteRune(char)
		default:
			normalized.WriteByte('-')
		}
	}
	result := strings.Trim(normalized.String(), "-")
	if result == "" {
		return "worker"
	}
	return result
}
