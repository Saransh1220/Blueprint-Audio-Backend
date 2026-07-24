package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	catalogPersistence "github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage"
	notificationDomain "github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	notificationPersistence "github.com/saransh1220/blueprint-audio/internal/modules/notification/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
)

type databaseNotifier struct {
	repository notificationDomain.NotificationRepository
}

func (n *databaseNotifier) Create(
	ctx context.Context,
	userID uuid.UUID,
	title, message string,
	notificationType notificationDomain.NotificationType,
) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	return n.repository.Create(ctx, &notificationDomain.Notification{
		ID:        id,
		UserID:    userID,
		Title:     title,
		Message:   message,
		Type:      notificationType,
		IsRead:    false,
		CreatedAt: time.Now().UTC(),
	})
}

func main() {
	cfg := config.Load()
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	files, err := filestorage.NewModule(ctx, cfg.FileStorage)
	if err != nil {
		log.Fatalf("initialize file storage: %v", err)
	}
	uploads := catalogPersistence.NewSpecUploadRepository(db)
	notifier := &databaseNotifier{
		repository: notificationPersistence.NewPgNotificationRepository(db),
	}
	processor := application.NewSpecUploadProcessor(uploads, files.Service(), notifier)

	workerPrefix := strings.TrimSpace(os.Getenv("WORKER_ID"))
	if workerPrefix == "" {
		host, _ := os.Hostname()
		workerPrefix = host
	}
	workerPrefix = normalizeWorkerPrefix(workerPrefix)
	// The configured value is a human-readable prefix, not the fencing token.
	// Every process gets a distinct identity even when replicas share config.
	workerID := workerPrefix + "-" + uuid.NewString()
	pollInterval := durationEnv("WORKER_POLL_INTERVAL", 2*time.Second)
	lease := durationEnv("WORKER_LEASE_DURATION", 30*time.Minute)
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
	log.Printf("spec upload worker stopped")
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	if parsed, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name))); err == nil && parsed > 0 {
		return parsed
	}
	return fallback
}

func normalizeWorkerPrefix(value string) string {
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
