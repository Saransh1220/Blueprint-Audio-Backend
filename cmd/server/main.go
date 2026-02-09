package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	cachedb "github.com/saransh1220/blueprint-audio/internal/db" // Aliased to avoid shadowing
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/saransh1220/blueprint-audio/internal/router"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

type appConfig struct {
	dsn            string
	port           string
	jwtSecret      string
	jwtExpiry      time.Duration
	allowedOrigins string
}

func loadAppConfig() appConfig {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default-dev-secret"
	}

	jwtExpiryStr := os.Getenv("JWT_EXPIRATION")
	jwtExpiry, _ := time.ParseDuration(jwtExpiryStr)
	if jwtExpiry == 0 {
		jwtExpiry = 24 * time.Hour
	}

	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:4200"
	}

	return appConfig{
		dsn:            fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPass, dbName),
		port:           port,
		jwtSecret:      jwtSecret,
		jwtExpiry:      jwtExpiry,
		allowedOrigins: allowedOrigins,
	}
}

func main() {
	cfg := loadAppConfig()

	log.Println("Connecting to DB...")

	db, err := sqlx.Connect("postgres", cfg.dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)

	}
	defer db.Close()

	log.Printf("Database Connected Successfully!")

	// Initialize Redis
	if err := cachedb.InitRedis(); err != nil {
		log.Printf("Warning: Failed to connect to Redis: %v", err)
		// We don't fatal here, allowing app to run without cache if needed
	} else {
		log.Println("Redis Connected Successfully!")
	}

	fileService, err := service.NewFileService(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize file service: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	specRepo := repository.NewSpecRepository(db)
	analyticsRepo := repository.NewAnalyticsRepository(db)

	specService := service.NewSpecService(specRepo)
	authService := service.NewAuthService(userRepo, cfg.jwtSecret, cfg.jwtExpiry)
	userService := service.NewUserService(userRepo)

	orderRepo := repository.NewOrderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	licenseRepo := repository.NewLicenseRepository(db)

	paymentService := service.NewPaymentService(orderRepo, paymentRepo, licenseRepo, specRepo, fileService)
	analyticsService := service.NewAnalyticsService(analyticsRepo, specRepo)

	specHandler := handler.NewSpecHandler(specService, fileService, analyticsService)
	authHandler := handler.NewAuthHandler(authService, fileService)
	userHandler := handler.NewUserHandler(userService, fileService)

	authMiddleware := middleware.NewAuthMiddleware(cfg.jwtSecret)

	paymentHandler := handler.NewPaymentHandler(paymentService)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService, specRepo, fileService)

	appRouter := router.NewRouter(authHandler, authMiddleware, specHandler, userHandler, paymentHandler, analyticsHandler)
	mux := appRouter.Setup()
	log.Printf("Server starting on port %s", cfg.port)
	handler := middleware.CORSMiddleware(mux, cfg.allowedOrigins)

	if err := http.ListenAndServe(":"+cfg.port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
