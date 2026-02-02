package downloader

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Service struct {
	BinPath string
	OutDir  string
}

func NewService() *Service {
	outDir := "downloads"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating download dir: %v\n", err)
	}

	// Check for yt-dlp in system path
	binPath := "yt-dlp"

	return &Service{
		BinPath: binPath,
		OutDir:  outDir,
	}
}

type SearchResult struct {
	Title    string `json:"title"`
	VideoID  string `json:"videoId"`
	Author   string `json:"author"`
	URL      string `json:"url"`
	Duration int    `json:"duration"`
}

// Search YouTube using yt-dlp's built-in search
func (s *Service) Search(query string) (*SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	fmt.Printf("DEBUG: Searching for: %s\n", query)

	// Use yt-dlp's ytsearch to find first result
	searchQuery := fmt.Sprintf("ytsearch1:%s", query)

	// Get video ID
	cmd := exec.Command(s.BinPath, "--get-id", searchQuery)
	idOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	videoID := strings.TrimSpace(string(idOutput))
	if videoID == "" {
		return nil, fmt.Errorf("no results found for: %s", query)
	}

	// Get video title
	cmd = exec.Command(s.BinPath, "--get-title", searchQuery)
	titleOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get title: %w", err)
	}
	title := strings.TrimSpace(string(titleOutput))

	// Get video duration
	cmd = exec.Command(s.BinPath, "--get-duration", searchQuery)
	durationOutput, _ := cmd.Output()
	durationStr := strings.TrimSpace(string(durationOutput))

	// Get uploader
	cmd = exec.Command(s.BinPath, "--get-filename", "-o", "%(uploader)s", searchQuery)
	uploaderOutput, _ := cmd.Output()
	uploader := strings.TrimSpace(string(uploaderOutput))

	result := &SearchResult{
		Title:    title,
		VideoID:  videoID,
		Author:   uploader,
		URL:      "https://youtube.com/watch?v=" + videoID,
		Duration: parseDuration(durationStr),
	}

	fmt.Printf("DEBUG: Found video: %s (ID: %s)\n", result.Title, result.VideoID)
	return result, nil
}

func parseDuration(dur string) int {
	// Parse duration like "3:45" to seconds
	parts := strings.Split(dur, ":")
	if len(parts) == 2 {
		var min, sec int
		fmt.Sscanf(parts[0], "%d", &min)
		fmt.Sscanf(parts[1], "%d", &sec)
		return min*60 + sec
	} else if len(parts) == 3 {
		var hour, min, sec int
		fmt.Sscanf(parts[0], "%d", &hour)
		fmt.Sscanf(parts[1], "%d", &min)
		fmt.Sscanf(parts[2], "%d", &sec)
		return hour*3600 + min*60 + sec
	}
	return 0
}

func (s *Service) Download(urlStr, format string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required")
	}

	fmt.Printf("DEBUG: Downloading %s (format: %s)\n", urlStr, format)

	outputTemplate := filepath.Join(s.OutDir, "%(title)s.%(ext)s")

	// Build yt-dlp arguments
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"-o", outputTemplate,
	}

	if format == "mp3" {
		args = append(args, "-x", "--audio-format", "mp3", "-f", "bestaudio/best")
	} else if format == "mp4" {
		args = append(args, "-f", "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best", "--merge-output-format", "mp4")
	} else {
		args = append(args, "-f", "best[height<=1080]/best")
	}

	// Get final filename first
	getNameArgs := append([]string{"--get-filename"}, args...)
	getNameArgs = append(getNameArgs, urlStr)

	nameCmd := exec.Command(s.BinPath, getNameArgs...)
	outBytes, err := nameCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to resolve filename: %s", string(outBytes))
	}

	finalPath := strings.TrimSpace(string(outBytes))
	lines := strings.Split(finalPath, "\n")
	if len(lines) > 0 {
		finalPath = strings.TrimSpace(lines[len(lines)-1])
	}

	// Fix extension for MP3
	if format == "mp3" && !strings.HasSuffix(finalPath, ".mp3") {
		ext := filepath.Ext(finalPath)
		if ext != "" {
			finalPath = strings.TrimSuffix(finalPath, ext) + ".mp3"
		} else {
			finalPath = finalPath + ".mp3"
		}
	}

	// Download
	dlArgs := append(args, urlStr)
	dlCmd := exec.Command(s.BinPath, dlArgs...)
	dlCmd.Stdout = os.Stdout
	dlCmd.Stderr = os.Stderr

	fmt.Printf("DEBUG: Downloading to: %s\n", finalPath)
	if err := dlCmd.Run(); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Verify file exists
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		return "", fmt.Errorf("download finished but file not found: %s", finalPath)
	}

	fmt.Printf("DEBUG: Successfully downloaded to %s\n", finalPath)
	return finalPath, nil
}
