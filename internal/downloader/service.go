package downloader

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kkdai/youtube/v2"
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

func (s *Service) Download(urlStr, format string) (string, error) {
	if urlStr == "" {
		return "", fmt.Errorf("url is required")
	}

	// Try Native YouTube Downloader first for YouTube links
	if strings.Contains(urlStr, "youtube.com") || strings.Contains(urlStr, "youtu.be") {
		fmt.Println("DEBUG: Detected YouTube link. Trying native downloader...")
		path, err := s.downloadYouTubeNative(urlStr, format)
		if err == nil {
			return path, nil
		}
		fmt.Printf("DEBUG: Native downloader failed: %v. Falling back to yt-dlp...\n", err)
	}

	return s.downloadWithYtDlp(urlStr, format)
}

func (s *Service) parseCookiesFile(path string) []*http.Cookie {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var cookies []*http.Cookie
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		// Fields: domain, flag, path, secure, expiration, name, value
		cookies = append(cookies, &http.Cookie{
			Name:   fields[5],
			Value:  fields[6],
			Domain: fields[0],
			Path:   fields[2],
		})
	}
	return cookies
}

func (s *Service) downloadYouTubeNative(urlStr, format string) (string, error) {
	jar, _ := cookiejar.New(nil)
	cookiesPath := "cookies.txt"

	// Ensure cookies are fresh if env var exists
	if envCookies := os.Getenv("YOUTUBE_COOKIES"); envCookies != "" {
		var cookieData []byte
		decoded, decodeErr := base64.StdEncoding.DecodeString(envCookies)
		if decodeErr == nil {
			cookieData = decoded
		} else {
			cookieData = []byte(envCookies)
		}
		_ = os.WriteFile(cookiesPath, cookieData, 0644)
	}

	if fileExists(cookiesPath) {
		cookies := s.parseCookiesFile(cookiesPath)
		u, _ := url.Parse("https://youtube.com")
		jar.SetCookies(u, cookies)
		fmt.Printf("DEBUG: Native downloader loaded %d cookies.\n", len(cookies))
	}

	// Setup client with proxy if available
	client := &http.Client{Jar: jar}
	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(u),
			}
			fmt.Println("DEBUG: Native downloader using proxy:", proxyURL)
		}
	}

	yclient := youtube.Client{HTTPClient: client}
	video, err := yclient.GetVideo(urlStr)
	if err != nil {
		return "", err
	}

	// Select best format
	formats := video.Formats
	if format == "mp3" {
		formats = formats.Type("audio")
	} else if format == "mp4" {
		formats = formats.Type("video").WithAudioChannels()
	}
	formats.Sort()

	if len(formats) == 0 {
		return "", fmt.Errorf("no suitable formats found")
	}

	targetFormat := &formats[0] // Best quality after sort

	// Sanitize filename
	cleanTitle := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == '_' {
			return r
		}
		return -1
	}, video.Title)
	cleanTitle = strings.TrimSpace(cleanTitle)

	ext := "mp4"
	if format == "mp3" {
		ext = "mp3"
	}
	finalPath := filepath.Join(s.OutDir, fmt.Sprintf("%s.%s", cleanTitle, ext))

	fmt.Printf("DEBUG: Downloading native to: %s\n", finalPath)
	stream, size, err := yclient.GetStream(video, targetFormat)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	file, err := os.Create(finalPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		return "", err
	}

	fmt.Printf("DEBUG: Downloaded %d bytes natively\n", size)
	return finalPath, nil
}

func (s *Service) downloadWithYtDlp(urlStr, format string) (string, error) {
	// Output template
	outputTemplate := filepath.Join(s.OutDir, "%(title)s.%(ext)s")

	// Check for cookies
	cookiesPath := "cookies.txt"
	if envCookies := os.Getenv("YOUTUBE_COOKIES"); envCookies != "" {
		var cookieData []byte
		decoded, decodeErr := base64.StdEncoding.DecodeString(envCookies)
		if decodeErr == nil {
			cookieData = decoded
		} else {
			cookieData = []byte(envCookies)
		}
		_ = os.WriteFile(cookiesPath, cookieData, 0644)
	}

	hasCookies := fileExists(cookiesPath)

	// Common Args
	// Re-added common User-Agent as most cookies are exported from Desktop Chrome
	commonArgs := []string{
		"--force-ipv4",
		"--no-playlist",
		"--no-warnings",
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}

	if proxyURL := os.Getenv("HTTP_PROXY"); proxyURL != "" {
		commonArgs = append(commonArgs, "--proxy", proxyURL)
		fmt.Println("DEBUG: yt-dlp using proxy:", proxyURL)
	}

	if hasCookies {
		commonArgs = append(commonArgs, "--cookies", cookiesPath)
	}

	if format == "mp3" {
		commonArgs = append(commonArgs, "-x", "--audio-format", "mp3", "-f", "bestaudio/best")
	} else if format == "mp4" {
		commonArgs = append(commonArgs, "-f", "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best", "-S", "res:1080,ext:mp4:m4a", "--merge-output-format", "mp4")
	} else {
		commonArgs = append(commonArgs, "-f", "best[height<=1080]/best")
	}

	// 1. Get Filename
	getNameArgs := append([]string{"--get-filename", "-o", outputTemplate}, commonArgs...)
	getNameArgs = append(getNameArgs, urlStr)

	nameCmd := exec.Command(s.BinPath, getNameArgs...)
	outBytes, err := nameCmd.CombinedOutput()
	if err != nil {
		fmt.Printf("DEBUG: yt-dlp filename error: %v, Output: %s\n", err, string(outBytes))
		return "", fmt.Errorf("failed to resolve filename: %s", string(outBytes))
	}
	finalPath := strings.TrimSpace(string(outBytes))
	lines := strings.Split(finalPath, "\n")
	if len(lines) > 0 {
		finalPath = strings.TrimSpace(lines[len(lines)-1])
	}

	if format == "mp3" && !strings.HasSuffix(finalPath, ".mp3") {
		ext := filepath.Ext(finalPath)
		if ext != "" {
			finalPath = strings.TrimSuffix(finalPath, ext) + ".mp3"
		} else {
			finalPath = finalPath + ".mp3"
		}
	}

	// 2. Download
	dlArgs := append([]string{"-o", outputTemplate}, commonArgs...)
	dlArgs = append(dlArgs, urlStr)

	dlCmd := exec.Command(s.BinPath, dlArgs...)
	dlCmd.Stdout = os.Stdout
	dlCmd.Stderr = os.Stderr

	fmt.Printf("Downloading with yt-dlp to: %s\n", finalPath)
	if err := dlCmd.Run(); err != nil {
		fmt.Printf("DEBUG: yt-dlp download error: %v\n", err)
		return "", fmt.Errorf("download failed: %w", err)
	}

	if !fileExists(finalPath) {
		return "", fmt.Errorf("download finished but file not found at expected path: %s", finalPath)
	}

	return finalPath, nil
}
