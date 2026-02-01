package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/handler"
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

	userRepo := repository.NewUserRepository(db)
	authService := service.NewAuthService(userRepo, jwtSecret, jwtExpiry)

	authHandler := handler.NewAuthHandler(authService)

	appRouter := router.NewRouter(authHandler)
	mux := appRouter.Setup()
	log.Printf("Server starting on port %s", port)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
