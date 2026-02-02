package main

import (
	"log"
	"media-downloader-api/internal/api"
	"media-downloader-api/internal/downloader"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	app := fiber.New(fiber.Config{
		AppName: "Media Downloader API",
	})

	// Services
	svc := downloader.NewService()

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())

	// Serve Static Files
	app.Static("/", "./web")
	app.Static("/downloads", "./downloads")

	// Rate Limiting (20 requests per minute)
	app.Use(limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Minute,
	}))

	// Routes
	api.SetupRoutes(app, svc)

	// Start server with graceful shutdown
	go func() {
		port := os.Getenv("PORT")
		if port == "" {
			port = "3000"
		}
		if err := app.Listen(":" + port); err != nil {
			log.Panic(err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c // Block until signal received
	log.Println("Gracefully shutting down...")
	_ = app.Shutdown()
	log.Println("Server stopped")
}
