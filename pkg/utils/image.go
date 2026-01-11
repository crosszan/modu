package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const DefaultImageDir = "./images"

func SaveImageData(data []byte, filename string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}
	return os.WriteFile(filename, data, 0644)
}

func SaveImageFromURL(url string, filename ...string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response failed: %w", err)
	}

	var path string
	if len(filename) > 0 && filename[0] != "" {
		path = filename[0]
	} else {
		path = filepath.Join(DefaultImageDir, generateFilename(".png"))
	}

	if err := SaveImageData(data, path); err != nil {
		return "", err
	}
	return path, nil
}

func SaveImages(images [][]byte, mimeTypes []string, dir ...string) ([]string, error) {
	targetDir := DefaultImageDir
	if len(dir) > 0 && dir[0] != "" {
		targetDir = dir[0]
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("create directory failed: %w", err)
	}

	var savedFiles []string
	for i, data := range images {
		mimeType := "image/png"
		if i < len(mimeTypes) && mimeTypes[i] != "" {
			mimeType = mimeTypes[i]
		}
		ext := GetExtFromMimeType(mimeType)
		filename := filepath.Join(targetDir, generateFilename(ext))
		if err := os.WriteFile(filename, data, 0644); err != nil {
			return savedFiles, fmt.Errorf("save image %d failed: %w", i+1, err)
		}
		savedFiles = append(savedFiles, filename)
	}
	return savedFiles, nil
}

func GetExtFromMimeType(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	default:
		return ".png"
	}
}

func GetFileSizeKB(filename string) (float64, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return 0, err
	}
	return float64(info.Size()) / 1024, nil
}

func generateFilename(ext string) string {
	return fmt.Sprintf("img_%d%s", time.Now().UnixNano(), ext)
}
