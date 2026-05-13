package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/gateway"
	gatewayMiddleware "github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog"
	catalogPersistence "github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage"
	"github.com/saransh1220/blueprint-audio/internal/modules/notification"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment"
	"github.com/saransh1220/blueprint-audio/internal/modules/user"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
	"github.com/saransh1220/blueprint-audio/pkg/migration"
)

func main() {
	// 1. Load Configuration
	cfg := config.Load()

	if cfg.Migration.AutoRun {
		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
		if err := migration.AutoMigrate(postgresURL(cfg.Database), cfg.Migration.Path, logger); err != nil {
			log.Fatalf("Failed to run database migrations: %v", err)
		}
	}

	// 2. Database Connection
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	var redisClient *redis.Client
	if cfg.Redis.Enabled {
		// 3. Redis Connection
		redisClient = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		if err := redisClient.Ping(ctx).Err(); err != nil {
			log.Printf("Warning: Failed to connect to Redis; cache disabled: %v", err)
			redisClient.Close()
			redisClient = nil
		} else {
			defer redisClient.Close()
		}
	} else {
		log.Printf("Redis disabled; running without cache")
	}

	// 4. Initialize Modules
	if cfg.Email.Enabled && (strings.TrimSpace(cfg.Email.ResendAPIKey) == "" || strings.TrimSpace(cfg.Email.From) == "") {
		log.Fatal("Email is enabled but RESEND_API_KEY or EMAIL_FROM is missing")
	}

	emailSender := sharedemail.NewSender(sharedemail.Config{
		APIKey:  cfg.Email.ResendAPIKey,
		From:    cfg.Email.From,
		ReplyTo: cfg.Email.ReplyTo,
		Enabled: cfg.Email.Enabled,
	})

	// Filestorage Module
	fsModule, err := filestorage.NewModule(ctx, cfg.FileStorage)
	if err != nil {
		log.Fatalf("Failed to initialize filestorage module: %v", err)
	}

	// Auth Module
	authModule, err := auth.NewModule(db, cfg.JWT.Secret, cfg.JWT.Expiry, cfg.JWT.RefreshExpiry, fsModule.Service(), cfg.Google.ClientID, cfg.Server.SecureCookies, emailSender, cfg.AppBaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize auth module: %v", err)
	}

	// User Module
	userModule := user.NewModule(authModule.UserRepository(), fsModule.Service())

	// Notification Module
	notificationModule := notification.NewModule(db)

	// Catalog Module Prerequisites
	// We need to instantiate the SpecRepository explicitly to share it between Catalog and Analytics
	specRepo := catalogPersistence.NewSpecRepository(db)

	// Analytics Module (Likely needs SpecRepo)
	analyticsModule := analytics.NewModule(db, specRepo, fsModule.Service())

	// Catalog Module
	catalogModule := catalog.NewModule(db, specRepo, fsModule.Service(), analyticsModule.AnalyticsService, notificationModule.Service(), redisClient)

	// Payment Module
	paymentModule := payment.NewModule(db, catalogModule.SpecFinder(), authModule.UserFinder(), fsModule.Service(), emailSender, cfg.AppBaseURL)

	// 5. Middleware
	authMiddleware := gatewayMiddleware.NewAuthMiddleware(cfg.JWT.Secret)

	// 6. Setup Routes
	mux := gateway.SetupRoutes(gateway.RouterConfig{
		AuthHandler:         authModule.HTTPHandler(),
		AuthMiddleware:      authMiddleware,
		SpecHandler:         catalogModule.HTTPHandler(),
		UserHandler:         userModule.HTTPHandler(),
		PaymentHandler:      paymentModule.HTTPHandler(),
		AnalyticsHandler:    analyticsModule.AnalyticsHandler,
		NotificationHandler: notificationModule.HTTPHandler(),
	})

	// 7. Apply Middleware
	handler := gatewayMiddleware.CORSMiddleware(mux, cfg.Server.AllowedOrigins)
	handler = gatewayMiddleware.PrometheusMiddleware(handler)

	// 8. Start Server
	srv := gateway.NewServer(cfg.Server.Port, handler)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func postgresURL(cfg database.PostgresConfig) string {
	u := &url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   net.JoinHostPort(cfg.Host, cfg.Port),
		Path:   "/" + cfg.DBName,
	}

	q := u.Query()
	q.Set("sslmode", cfg.SSLMode)
	u.RawQuery = q.Encode()

	return u.String()
}
