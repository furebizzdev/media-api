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
	// Ensure downloads directory exists
	outDir := "downloads"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating download dir: %v\n", err)
	}

	// Check for local bin first, then system path
	binPath, err := filepath.Abs("bin/yt-dlp")
	if err != nil || !fileExists(binPath) {
		binPath = "yt-dlp" // Fallback to system path
	}

	return &Service{
		BinPath: binPath,
		OutDir:  outDir,
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (s *Service) Download(url, format string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("url is required")
	}

	// Output template
	// Use %(title)s for readability, but sanitize it? yt-dlp sanitizes by default.
	// %(id)s is safest for uniqueness if we don't want collisions easily, but title is friendlier.
	outputTemplate := filepath.Join(s.OutDir, "%(title)s.%(ext)s")

	// Common Args
	commonArgs := []string{"--no-playlist", "--no-warnings"}
	if format == "mp3" {
		commonArgs = append(commonArgs, "-x", "--audio-format", "mp3")
	} else if format == "mp4" {
		// Limit to 1080p, prefer H.264 (avc1) for compatibility and smaller size compared to VP9/AV1 at high bitrates?
		// Actually VP9 is smaller but H.264 is standard.
		// User said "too heavy", so maybe just capping resolution is enough.
		// We use -S "res:1080" to prioritize 1080p, and "codec:h264" if available.
		commonArgs = append(commonArgs, "-f", "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best", "-S", "res:1080,ext:mp4:m4a", "--merge-output-format", "mp4")
	} else {
		// Default
		commonArgs = append(commonArgs, "-f", "best[height<=1080]/best")
	}

	// 1. Get Filename
	// We add -o outputTemplate and --get-filename
	getNameArgs := append([]string{"--get-filename", "-o", outputTemplate}, commonArgs...)
	getNameArgs = append(getNameArgs, url)

	nameCmd := exec.Command(s.BinPath, getNameArgs...)
	outBytes, err := nameCmd.Output()
	if err != nil {
		var errMsg string
		if exitErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exitErr.Stderr)
		} else {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("failed to resolve filename: %s", errMsg)
	}
	finalPath := strings.TrimSpace(string(outBytes))

	// Fix extension mismatch for MP3
	// yt-dlp --get-filename often returns the video extension even with -x
	if format == "mp3" && !strings.HasSuffix(finalPath, ".mp3") {
		// Replace extension with .mp3
		ext := filepath.Ext(finalPath)
		if ext != "" {
			finalPath = strings.TrimSuffix(finalPath, ext) + ".mp3"
		} else {
			finalPath = finalPath + ".mp3"
		}
	}

	// 2. Download
	dlArgs := append([]string{"-o", outputTemplate}, commonArgs...)
	dlArgs = append(dlArgs, url)

	dlCmd := exec.Command(s.BinPath, dlArgs...)
	dlCmd.Stdout = os.Stdout // Stream to server log for debug
	dlCmd.Stderr = os.Stderr

	fmt.Printf("Downloading to: %s\n", finalPath)
	if err := dlCmd.Run(); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Verify file exists
	if !fileExists(finalPath) {
		// Fallback check: sometimes extension differs slightly or path issues.
		return "", fmt.Errorf("download finished but file not found at expected path: %s", finalPath)
	}

	return finalPath, nil
}
