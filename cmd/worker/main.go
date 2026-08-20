package main

import (
	"context"
	"log"
	"os"
	"os/signal"
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

	application.StartUploadWorker(ctx, processor, cfg.Worker)
}
