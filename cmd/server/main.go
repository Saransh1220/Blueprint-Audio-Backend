package main

import (
	"context"
	"log"

	"github.com/redis/go-redis/v9"
	"github.com/saransh1220/blueprint-audio/internal/gateway"
	gatewayMiddleware "github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog"
	catalogPersistence "github.com/saransh1220/blueprint-audio/internal/modules/catalog/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/filestorage"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment"
	"github.com/saransh1220/blueprint-audio/internal/modules/user"
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
	authMiddleware := gatewayMiddleware.NewAuthMiddleware(cfg.JWT.Secret)

	// 6. Setup Routes
	mux := gateway.SetupRoutes(gateway.RouterConfig{
		AuthHandler:      authModule.HTTPHandler(),
		AuthMiddleware:   authMiddleware,
		SpecHandler:      catalogModule.HTTPHandler(),
		UserHandler:      userModule.HTTPHandler(),
		PaymentHandler:   paymentModule.HTTPHandler(),
		AnalyticsHandler: analyticsModule.AnalyticsHandler,
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
