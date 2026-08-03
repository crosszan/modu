package coding_agent

import (
	"github.com/openmodu/modu/pkg/coding_agent/services/session"
	"github.com/openmodu/modu/pkg/types"
)

// ForkMessage represents a user message available for forking.
type ForkMessage struct {
	EntryID string `json:"entryId"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SessionInfo = session.SessionInfo

// SessionBranchInfo describes one visible branch in the session tree.
type SessionBranchInfo struct {
	ID         string `json:"id"`
	ParentID   string `json:"parentId,omitempty"`
	Label      string `json:"label,omitempty"`
	EntryCount int    `json:"entryCount"`
}

// SessionTreeNode describes one visible entry in the current session tree.
type SessionTreeNode struct {
	ID            string `json:"id"`
	ParentID      string `json:"parentId,omitempty"`
	Type          string `json:"type"`
	Role          string `json:"role,omitempty"`
	Label         string `json:"label,omitempty"`
	Preview       string `json:"preview,omitempty"`
	Depth         int    `json:"depth"`
	ChildCount    int    `json:"childCount"`
	Current       bool   `json:"current"`
	InCurrentPath bool   `json:"inCurrentPath"`
	Timestamp     int64  `json:"timestamp"`
}

// SessionTranscriptEntry is one displayable item from the persisted current
// path. Message is set only when Type is "message".
type SessionTranscriptEntry struct {
	Type      string             `json:"type"`
	Message   types.AgentMessage `json:"message,omitempty"`
	Timestamp int64              `json:"timestamp"`
}

// SessionStats contains aggregate statistics for the current session.
type SessionStats struct {
	TotalTokens    int   `json:"totalTokens"`
	MessageCount   int   `json:"messageCount"`
	SessionStarted int64 `json:"sessionStarted"`
	DurationMs     int64 `json:"durationMs"`
	// CacheReadTokens and FreshInputTokens are the lifetime sums across every
	// assistant turn in the session (not just the current context window),
	// used to report a prompt-cache hit rate.
	CacheReadTokens  int `json:"cacheReadTokens"`
	FreshInputTokens int `json:"freshInputTokens"`
}

// CacheHitRate returns the fraction of input tokens served from the prompt
// cache, in [0, 1]. Returns (0, false) when no input tokens have been
// recorded yet (a fresh session), so callers can distinguish "no data" from
// "0% hit rate".
func (s SessionStats) CacheHitRate() (float64, bool) {
	total := s.CacheReadTokens + s.FreshInputTokens
	if total <= 0 {
		return 0, false
	}
	return float64(s.CacheReadTokens) / float64(total), true
}
