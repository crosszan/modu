// Package notebooklmvo defines value objects for NotebookLM API
package notebooklmvo

import (
	"time"
)

// Notebook represents a NotebookLM notebook
type Notebook struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	SourceCount int       `json:"source_count,omitempty"`
}

// Source represents a source document in a notebook
type Source struct {
	ID         string    `json:"id"`
	NotebookID string    `json:"notebook_id"`
	Title      string    `json:"title"`
	SourceType string    `json:"source_type"` // url, youtube, text, file
	URL        string    `json:"url,omitempty"`
	Status     string    `json:"status"` // processing, ready, error
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Artifact represents a generated artifact (audio, video, etc.)
type Artifact struct {
	ID           string    `json:"id"`
	NotebookID   string    `json:"notebook_id"`
	Title        string    `json:"title"`
	ArtifactType int       `json:"artifact_type"`
	Status       string    `json:"status"` // pending, in_progress, completed
	DownloadURL  string    `json:"download_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AskResult represents the response from a chat query
type AskResult struct {
	Answer         string          `json:"answer"`
	ConversationID string          `json:"conversation_id"`
	TurnNumber     int             `json:"turn_number"`
	References     []ChatReference `json:"references,omitempty"`
}

// ChatReference represents a citation in a chat response
type ChatReference struct {
	SourceID    string `json:"source_id"`
	SourceTitle string `json:"source_title"`
	Text        string `json:"text"`
}

// ConversationTurn represents a single turn in a conversation
type ConversationTurn struct {
	Query      string          `json:"query"`
	Answer     string          `json:"answer"`
	TurnNumber int             `json:"turn_number"`
	References []ChatReference `json:"references,omitempty"`
}

// GenerationStatus represents the status of artifact generation
type GenerationStatus struct {
	TaskID      string `json:"task_id"`
	Status      string `json:"status"` // pending, in_progress, completed, failed
	ArtifactID  string `json:"artifact_id,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

// AuthTokens holds authentication credentials
type AuthTokens struct {
	Cookies   map[string]string `json:"cookies"`
	CSRFToken string            `json:"csrf_token"`
	SessionID string            `json:"session_id"`
}

// CookieHeader returns cookies formatted as HTTP header value
func (a *AuthTokens) CookieHeader() string {
	if len(a.Cookies) == 0 {
		return ""
	}
	var result string
	for k, v := range a.Cookies {
		if result != "" {
			result += "; "
		}
		result += k + "=" + v
	}
	return result
}
