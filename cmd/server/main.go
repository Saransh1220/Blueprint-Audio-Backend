package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/handler"
	"github.com/saransh1220/blueprint-audio/internal/middleware"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/saransh1220/blueprint-audio/internal/router"
	"github.com/saransh1220/blueprint-audio/internal/service"
)

func main() {
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", dbHost, dbPort, dbUser, dbPass, dbName)

	log.Println("Connecting to DB...")

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)

	}
	defer db.Close()

	log.Printf("Database Connected Successfully!")

	jwtSecret := os.Getenv("JWT_SECRET")
	jwtExpiryStr := os.Getenv("JWT_EXPIRATION")
	if jwtSecret == "" {
		jwtSecret = "default-dev-secret"
	}
	jwtExpiry, _ := time.ParseDuration(jwtExpiryStr)
	if jwtExpiry == 0 {
		jwtExpiry = 24 * time.Hour
	}

	fileService, err := service.NewFileService(context.Background())
	if err != nil {
		log.Fatalf("Failed to initialize file service: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	specRepo := repository.NewSpecRepository(db)

	specService := service.NewSpecService(specRepo)
	authService := service.NewAuthService(userRepo, jwtSecret, jwtExpiry)
	userService := service.NewUserService(userRepo)

	specHandler := handler.NewSpecHandler(specService, fileService)
	authHandler := handler.NewAuthHandler(authService, fileService)
	userHandler := handler.NewUserHandler(userService, fileService)

	authMiddleware := middleware.NewAuthMiddleware(jwtSecret)

	orderRepo := repository.NewOrderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)
	licenseRepo := repository.NewLicenseRepository(db)

	paymentService := service.NewPaymentService(orderRepo, paymentRepo, licenseRepo, specRepo, fileService)

	paymentHandler := handler.NewPaymentHandler(paymentService)

	appRouter := router.NewRouter(authHandler, authMiddleware, specHandler, userHandler, paymentHandler)
	mux := appRouter.Setup()
	log.Printf("Server starting on port %s", port)

	// Wrap specific handler with CORS middleware
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins == "" {
		allowedOrigins = "http://localhost:4200" // Default fallback
	}
	handler := middleware.CORSMiddleware(mux, allowedOrigins)

	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
