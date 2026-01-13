package notebooklm

import (
	"fmt"
	"time"

	vo "github.com/crosszan/modu/vo/notebooklm_vo"
)

// parseNotebookList parses the list notebooks response
func parseNotebookList(data any) ([]vo.Notebook, error) {
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid notebook list response")
	}

	if len(arr) == 0 {
		return []vo.Notebook{}, nil
	}

	// Notebooks are in the first element
	nbList, ok := arr[0].([]any)
	if !ok {
		return []vo.Notebook{}, nil
	}

	var notebooks []vo.Notebook
	for _, item := range nbList {
		nb, err := parseNotebook(item)
		if err != nil {
			continue // Skip malformed entries
		}
		notebooks = append(notebooks, *nb)
	}

	return notebooks, nil
}

// parseNotebook parses a single notebook from API response
func parseNotebook(data any) (*vo.Notebook, error) {
	arr, ok := data.([]any)
	if !ok || len(arr) < 3 {
		return nil, fmt.Errorf("invalid notebook data")
	}

	nb := &vo.Notebook{}

	// Find ID (UUID format) and Title (human readable)
	// The response structure has ID and Title in first few positions
	// but order may vary - ID is always UUID format
	for i := 0; i < len(arr) && i < 5; i++ {
		if str, ok := arr[i].(string); ok && str != "" {
			if isUUID(str) {
				nb.ID = str
			} else if nb.Title == "" {
				nb.Title = str
			}
		}
	}

	// Parse timestamps if available
	nb.CreatedAt = time.Now()
	nb.UpdatedAt = time.Now()

	// Count sources if available
	if len(arr) > 1 {
		if sources, ok := arr[1].([]any); ok {
			nb.SourceCount = len(sources)
		}
	}

	return nb, nil
}

// parseSource parses a source from API response
func parseSource(data any, notebookID string) (*vo.Source, error) {
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid source data")
	}

	source := &vo.Source{
		NotebookID: notebookID,
		Status:     "processing",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Source structure varies, try to extract key fields
	if len(arr) > 0 {
		// First element often contains source info array
		if sourceArr, ok := arr[0].([]any); ok && len(sourceArr) > 0 {
			// ID is often triple-nested
			if idArr, ok := sourceArr[0].([]any); ok && len(idArr) > 0 {
				if idArr2, ok := idArr[0].([]any); ok && len(idArr2) > 0 {
					if id, ok := idArr2[0].(string); ok {
						source.ID = id
					}
				} else if id, ok := idArr[0].(string); ok {
					source.ID = id
				}
			}

			// Title is often at index 1
			if len(sourceArr) > 1 {
				if title, ok := sourceArr[1].(string); ok {
					source.Title = title
				}
			}

			// Look for URL in metadata
			if len(sourceArr) > 2 {
				if meta, ok := sourceArr[2].([]any); ok {
					source.URL = findURLInMeta(meta)
				}
			}
		}
	}

	// Determine source type from URL
	if source.URL != "" {
		source.SourceType = "url"
		if containsYouTube(source.URL) {
			source.SourceType = "youtube"
		}
	} else {
		source.SourceType = "text"
	}

	return source, nil
}

// findURLInMeta searches for URL in metadata array
func findURLInMeta(meta []any) string {
	for _, item := range meta {
		if urlArr, ok := item.([]any); ok && len(urlArr) > 0 {
			if url, ok := urlArr[0].(string); ok {
				if isURL(url) {
					return url
				}
			}
		}
	}
	return ""
}

// isURL checks if a string looks like a URL
func isURL(s string) bool {
	return len(s) > 8 && (s[:7] == "http://" || s[:8] == "https://")
}

// isUUID checks if a string looks like a UUID
func isUUID(s string) bool {
	// UUID format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (36 chars with 4 dashes)
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// containsYouTube checks if URL is a YouTube link
func containsYouTube(url string) bool {
	return len(url) > 0 && (contains(url, "youtube.com") || contains(url, "youtu.be"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// parseGenerationStatus parses artifact generation status
func parseGenerationStatus(data any) (*vo.GenerationStatus, error) {
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid generation status data")
	}

	status := &vo.GenerationStatus{
		Status: "pending",
	}

	// Parse status structure
	if len(arr) > 0 {
		if taskID, ok := arr[0].(string); ok {
			status.TaskID = taskID
		}
	}

	if len(arr) > 1 {
		// Status code: 1=processing, 2=pending, 3=completed
		if statusCode, ok := arr[1].(float64); ok {
			switch int(statusCode) {
			case 1:
				status.Status = "in_progress"
			case 2:
				status.Status = "pending"
			case 3:
				status.Status = "completed"
			}
		}
	}

	// Look for download URL
	if len(arr) > 3 {
		if url, ok := arr[3].(string); ok && isURL(url) {
			status.DownloadURL = url
			status.Status = "completed"
		}
	}

	return status, nil
}

// parseArtifactList parses the list artifacts response
func parseArtifactList(data any) ([]vo.Artifact, error) {
	arr, ok := data.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid artifact list response")
	}

	var artifacts []vo.Artifact
	for _, item := range arr {
		artifact, err := parseArtifact(item)
		if err != nil {
			continue
		}
		artifacts = append(artifacts, *artifact)
	}

	return artifacts, nil
}

// parseArtifact parses a single artifact
func parseArtifact(data any) (*vo.Artifact, error) {
	arr, ok := data.([]any)
	if !ok || len(arr) < 2 {
		return nil, fmt.Errorf("invalid artifact data")
	}

	artifact := &vo.Artifact{
		Status:    "completed",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if len(arr) > 0 {
		if id, ok := arr[0].(string); ok {
			artifact.ID = id
		}
	}

	if len(arr) > 1 {
		if title, ok := arr[1].(string); ok {
			artifact.Title = title
		}
	}

	if len(arr) > 2 {
		if artType, ok := arr[2].(float64); ok {
			artifact.ArtifactType = int(artType)
		}
	}

	if len(arr) > 3 {
		if url, ok := arr[3].(string); ok && isURL(url) {
			artifact.DownloadURL = url
		}
	}

	return artifact, nil
}
