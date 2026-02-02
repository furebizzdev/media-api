package api

import (
	"media-downloader-api/internal/downloader"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	Service *downloader.Service
}

func SetupRoutes(app *fiber.App, service *downloader.Service) {
	h := &Handler{Service: service}

	api := app.Group("/api/v1")

	api.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	api.Post("/download", h.HandleDownload)
	api.Get("/search", h.HandleSearch)
}

func (h *Handler) HandleSearch(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Query parameter 'q' is required",
		})
	}

	result, err := h.Service.Search(query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Return search result with YouTube URL
	return c.JSON(fiber.Map{
		"title":    result.Title,
		"author":   result.Author,
		"url":      "https://youtube.com/watch?v=" + result.VideoID,
		"videoId":  result.VideoID,
		"duration": result.Duration,
	})
}

func (h *Handler) HandleDownload(c *fiber.Ctx) error {
	type DownloadRequest struct {
		URL    string `json:"url"`
		Format string `json:"format"` // mp3, mp4
	}

	var req DownloadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.URL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "URL is required",
		})
	}

	path, err := h.Service.Download(req.URL, req.Format)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// For now, return the path.
	// In a real app we might verify if we want to serve it directly.
	// Let's add a download header so the user can download it?
	// Or just return JSON info.
	return c.JSON(fiber.Map{
		"message": "Download successful",
		"file":    path,
	})
}
