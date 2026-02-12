package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog"
	catalogPersistence "github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment"
	"github.com/saransh1220/blueprint-audio/internal/modules/user"
	"github.com/saransh1220/blueprint-audio/internal/router"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/config"
	"github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/database"
)

func main() {
	// 1. Load Configuration
	cfg := config.Load()

	// 2. Database Connection
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// 3. Redis Connection
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Host + ":" + cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	// 4. Initialize Modules

	// Filestorage Module
	fsModule, err := filestorage.NewModule(ctx, cfg.FileStorage)
	if err != nil {
		log.Fatalf("Failed to initialize filestorage module: %v", err)
	}

	// Auth Module
	authModule, err := auth.NewModule(db, cfg.JWT.Secret, cfg.JWT.Expiry, fsModule.Service())
	if err != nil {
		log.Fatalf("Failed to initialize auth module: %v", err)
	}

	// User Module
	userModule := user.NewModule(authModule.UserRepository(), fsModule.Service())

	// Catalog Module Prerequisites
	// We need to instantiate the SpecRepository explicitly to share it between Catalog and Analytics
	specRepo := catalogPersistence.NewSpecRepository(db)

	// Analytics Module (Likely needs SpecRepo)
	analyticsModule := analytics.NewModule(db, specRepo, fsModule.Service())

	// Catalog Module
	catalogModule := catalog.NewModule(db, specRepo, fsModule.Service(), analyticsModule.AnalyticsService, redisClient)

	// Payment Module
	paymentModule := payment.NewModule(db, catalogModule.SpecFinder(), fsModule.Service())

	// 5. Middleware
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWT.Secret)

	// 6. Router
	appRouter := router.NewRouter(
		authModule.HTTPHandler(),
		authMiddleware,
		catalogModule.HTTPHandler(),
		userModule.HTTPHandler(),
		paymentModule.HTTPHandler(),
		analyticsModule.AnalyticsHandler,
	)

	// 7. Server Setup with Middleware
	mux := appRouter.Setup()

	// Apply CORS middleware
	handler := middleware.CORSMiddleware(mux, cfg.Server.AllowedOrigins)
	// Apply Prometheus middleware
	handler = middleware.PrometheusMiddleware(handler)

	srv := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: handler,
	}

	// Graceful Shutdown
	go func() {
		log.Printf("Server starting on port %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exiting")
}
