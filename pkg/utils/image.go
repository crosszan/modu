package utils

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func SaveImageData(data []byte, filename string) error {
	return os.WriteFile(filename, data, 0644)
}

func SaveImageFromURL(url, filename string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response failed: %w", err)
	}
	return os.WriteFile(filename, data, 0644)
}

func SaveImages(dir, prefix string, images [][]byte, mimeTypes []string) ([]string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory failed: %w", err)
	}

	var savedFiles []string
	for i, data := range images {
		mimeType := "image/png"
		if i < len(mimeTypes) {
			mimeType = mimeTypes[i]
		}
		ext := GetExtFromMimeType(mimeType)
		filename := filepath.Join(dir, fmt.Sprintf("%s_%d%s", prefix, i+1, ext))
		if err := SaveImageData(data, filename); err != nil {
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
