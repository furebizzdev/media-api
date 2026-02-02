package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Service struct {
	OutDir string
}

func NewService() *Service {
	outDir := "downloads"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating download dir: %v\n", err)
	}

	return &Service{
		OutDir: outDir,
	}
}

// Public Invidious instances (will try in order if one fails)
var invidiousInstances = []string{
	"https://invidious.nerdvpn.de",
	"https://inv.nadeko.net",
	"https://invidious.privacyredirect.com",
	"https://invidious.protokolla.fi",
	"https://iv.odysfvr.com",
}

type InvidiousVideo struct {
	Title           string            `json:"title"`
	VideoID         string            `json:"videoId"`
	AdaptiveFormats []InvidiousFormat `json:"adaptiveFormats"`
	FormatStreams   []InvidiousFormat `json:"formatStreams"`
}

type InvidiousFormat struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	Quality      string `json:"quality"`
	Container    string `json:"container"`
	Encoding     string `json:"encoding"`
	AudioQuality string `json:"audioQuality,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
}

type SearchResult struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	VideoID string `json:"videoId"`
	Author  string `json:"author"`
	Length  int    `json:"lengthSeconds"`
}

func extractVideoID(urlStr string) string {
	// Handle youtu.be short links
	if strings.Contains(urlStr, "youtu.be/") {
		parts := strings.Split(urlStr, "youtu.be/")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "?")[0]
			return strings.TrimSpace(id)
		}
	}

	// Handle youtube.com/watch?v= links
	if strings.Contains(urlStr, "watch?v=") {
		parts := strings.Split(urlStr, "watch?v=")
		if len(parts) > 1 {
			id := strings.Split(parts[1], "&")[0]
			return strings.TrimSpace(id)
		}
	}

	return ""
}

// Search YouTube using Invidious API
func (s *Service) Search(query string) (*SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	fmt.Printf("DEBUG: Searching for: %s\n", query)

	// Try each Invidious instance until one works
	var lastErr error
	for i, instance := range invidiousInstances {
		fmt.Printf("DEBUG: Trying Invidious instance %d/%d for search: %s\n", i+1, len(invidiousInstances), instance)

		result, err := s.searchFromInvidious(instance, query)
		if err == nil && result != nil {
			return result, nil
		}

		fmt.Printf("DEBUG: Search instance failed: %v\n", err)
		lastErr = err
	}

	return nil, fmt.Errorf("all Invidious instances failed for search, last error: %w", lastErr)
}

func (s *Service) searchFromInvidious(instance, query string) (*SearchResult, error) {
	// Build search API URL
	searchURL := fmt.Sprintf("%s/api/v1/search?q=%s&type=video", instance, url.QueryEscape(query))

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("invidious search API returned status %d", resp.StatusCode)
	}

	var results []SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Return first video result
	for _, result := range results {
		if result.Type == "video" && result.VideoID != "" {
			fmt.Printf("DEBUG: Found video: %s (ID: %s)\n", result.Title, result.VideoID)
			return &result, nil
		}
	}

	return nil, fmt.Errorf("no video results found for query: %s", query)
}

func (s *Service) Download(urlStr, format string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required")
	}

	// Extract video ID from URL
	videoID := extractVideoID(urlStr)
	if videoID == "" {
		return "", fmt.Errorf("could not extract video ID from URL")
	}

	fmt.Printf("DEBUG: Extracted video ID: %s, requesting format: %s\n", videoID, format)

	// Try each Invidious instance until one works
	var lastErr error
	for i, instance := range invidiousInstances {
		fmt.Printf("DEBUG: Trying Invidious instance %d/%d: %s\n", i+1, len(invidiousInstances), instance)

		path, err := s.downloadFromInvidious(instance, videoID, format)
		if err == nil {
			return path, nil
		}

		fmt.Printf("DEBUG: Instance failed: %v\n", err)
		lastErr = err
	}

	return "", fmt.Errorf("all Invidious instances failed, last error: %w", lastErr)
}

func (s *Service) downloadFromInvidious(instance, videoID, format string) (string, error) {
	// Fetch video info from Invidious API
	apiURL := fmt.Sprintf("%s/api/v1/videos/%s", instance, videoID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch video info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("invidious API returned status %d", resp.StatusCode)
	}

	var video InvidiousVideo
	if err := json.NewDecoder(resp.Body).Decode(&video); err != nil {
		return "", fmt.Errorf("failed to parse video info: %w", err)
	}

	// Select best format
	var downloadURL string
	if format == "mp3" {
		// Find best audio-only format
		for _, f := range video.AdaptiveFormats {
			if strings.Contains(f.Type, "audio") {
				downloadURL = f.URL
				break
			}
		}
	} else {
		// Find best video+audio format (prefer 1080p or lower)
		for _, f := range video.FormatStreams {
			if strings.Contains(f.Quality, "1080") || strings.Contains(f.Quality, "720") {
				downloadURL = f.URL
				break
			}
		}
		// Fallback to any format stream
		if downloadURL == "" && len(video.FormatStreams) > 0 {
			downloadURL = video.FormatStreams[0].URL
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no suitable format found")
	}

	fmt.Printf("DEBUG: Found download URL, fetching file...\n")

	// Download the file
	fileResp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer fileResp.Body.Close()

	if fileResp.StatusCode != 200 {
		return "", fmt.Errorf("file download failed with status: %d", fileResp.StatusCode)
	}

	// Sanitize title for filename
	cleanTitle := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, video.Title)
	cleanTitle = strings.TrimSpace(cleanTitle)
	if cleanTitle == "" {
		cleanTitle = video.VideoID
	}

	ext := ".mp4"
	if format == "mp3" {
		ext = ".mp3"
	}

	finalPath := filepath.Join(s.OutDir, cleanTitle+ext)

	// Save file
	file, err := os.Create(finalPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, fileResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	fmt.Printf("DEBUG: Successfully downloaded %d bytes to %s\n", written, finalPath)
	return finalPath, nil
}
