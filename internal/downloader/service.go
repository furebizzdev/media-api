package downloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

type CobaltRequest struct {
	URL          string `json:"url"`
	VideoQuality string `json:"vQuality,omitempty"`
	AudioFormat  string `json:"aFormat,omitempty"`
	IsAudioOnly  bool   `json:"isAudioOnly"`
}

type CobaltResponse struct {
	Status string `json:"status"`
	URL    string `json:"url"`
	Text   string `json:"text,omitempty"`
}

func (s *Service) Download(urlStr, format string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required")
	}

	fmt.Printf("DEBUG: Using Cobalt API for: %s (format: %s)\n", urlStr, format)

	// Prepare Cobalt request
	isAudio := format == "mp3"
	cobaltReq := CobaltRequest{
		URL:         urlStr,
		IsAudioOnly: isAudio,
	}

	if isAudio {
		cobaltReq.AudioFormat = "mp3"
	} else {
		cobaltReq.VideoQuality = "1080"
	}

	reqBody, _ := json.Marshal(cobaltReq)

	// Call Cobalt API
	req, err := http.NewRequest("POST", "https://co.wuk.sh/api/json", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cobalt api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("cobalt api error (status %d): %s", resp.StatusCode, string(body))
	}

	var cobaltResp CobaltResponse
	if err := json.NewDecoder(resp.Body).Decode(&cobaltResp); err != nil {
		return "", fmt.Errorf("failed to parse cobalt response: %w", err)
	}

	if cobaltResp.Status != "stream" && cobaltResp.Status != "redirect" {
		return "", fmt.Errorf("cobalt returned status: %s, text: %s", cobaltResp.Status, cobaltResp.Text)
	}

	if cobaltResp.URL == "" {
		return "", fmt.Errorf("cobalt did not return a download URL")
	}

	fmt.Printf("DEBUG: Cobalt returned download URL, fetching file...\n")

	// Download the file from Cobalt's URL
	fileResp, err := http.Get(cobaltResp.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download from cobalt url: %w", err)
	}
	defer fileResp.Body.Close()

	if fileResp.StatusCode != 200 {
		return "", fmt.Errorf("file download failed with status: %d", fileResp.StatusCode)
	}

	// Generate filename from Content-Disposition or use generic name
	filename := "download"
	if cd := fileResp.Header.Get("Content-Disposition"); cd != "" {
		if idx := strings.Index(cd, "filename="); idx != -1 {
			filename = strings.Trim(cd[idx+9:], "\"")
		}
	}

	// Ensure correct extension
	ext := filepath.Ext(filename)
	if ext == "" {
		if format == "mp3" {
			ext = ".mp3"
		} else {
			ext = ".mp4"
		}
		filename = filename + ext
	}

	finalPath := filepath.Join(s.OutDir, filename)

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
