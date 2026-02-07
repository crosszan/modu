package memory

import (
	"fmt"
	"time"
)

// ConversationTurn 对话轮次
type ConversationTurn struct {
	User      string
	Assistant string
	SessionID string
	Timestamp time.Time
	Metadata  map[string]interface{}
}

// ConversationMemory 对话记忆管理
type ConversationMemory struct {
	manager *Manager
}

// NewConversationMemory 创建对话记忆管理器
func NewConversationMemory(manager *Manager) *ConversationMemory {
	return &ConversationMemory{manager: manager}
}

// StoreTurn 存储对话轮次
func (c *ConversationMemory) StoreTurn(turn ConversationTurn) error {
	content := fmt.Sprintf("用户: %s\n助手: %s", turn.User, turn.Assistant)

	metadata := turn.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["user_msg"] = turn.User
	metadata["assistant_msg"] = turn.Assistant
	metadata["session_id"] = turn.SessionID

	mem := Memory{
		Type:       MemoryTypeConversation,
		Content:    content,
		Metadata:   metadata,
		Timestamp:  turn.Timestamp,
		Importance: 0.5, // 默认重要性
	}

	return c.manager.Store(mem)
}

// GetHistory 获取会话历史
func (c *ConversationMemory) GetHistory(sessionID string, maxTurns int) ([]ConversationTurn, error) {
	// 从store获取指定会话的记忆
	memories, err := c.manager.store.GetMemoriesBySession(sessionID, maxTurns)
	if err != nil {
		return nil, err
	}

	turns := make([]ConversationTurn, 0, len(memories))
	for _, mem := range memories {
		turn := ConversationTurn{
			SessionID: sessionID,
			Timestamp: mem.Timestamp,
			Metadata:  mem.Metadata,
		}

		if userMsg, ok := mem.Metadata["user_msg"].(string); ok {
			turn.User = userMsg
		}
		if assistantMsg, ok := mem.Metadata["assistant_msg"].(string); ok {
			turn.Assistant = assistantMsg
		}

		turns = append(turns, turn)
	}

	return turns, nil
}

// SearchHistory 语义搜索历史对话
func (c *ConversationMemory) SearchHistory(query string, limit int) ([]ConversationTurn, error) {
	opts := RecallOptions{
		Limit:              limit,
		MemoryTypes:        []MemoryType{MemoryTypeConversation},
		ApplyDecay:         true,
		DecayHalflife:      7 * 24 * time.Hour, // 7天半衰期
		WeightByImportance: false,
	}

	memories, err := c.manager.Recall(query, opts)
	if err != nil {
		return nil, err
	}

	turns := make([]ConversationTurn, 0, len(memories))
	for _, mem := range memories {
		turn := ConversationTurn{
			Timestamp: mem.Timestamp,
			Metadata:  mem.Metadata,
		}

		if sessionID, ok := mem.Metadata["session_id"].(string); ok {
			turn.SessionID = sessionID
		}
		if userMsg, ok := mem.Metadata["user_msg"].(string); ok {
			turn.User = userMsg
		}
		if assistantMsg, ok := mem.Metadata["assistant_msg"].(string); ok {
			turn.Assistant = assistantMsg
		}

		turns = append(turns, turn)
	}

	return turns, nil
}

// GetRecentTurns 获取最近的对话轮次（所有会话）
func (c *ConversationMemory) GetRecentTurns(limit int) ([]ConversationTurn, error) {
	memories, err := c.manager.store.GetRecentMemoriesByType(string(MemoryTypeConversation), limit)
	if err != nil {
		return nil, err
	}

	turns := make([]ConversationTurn, 0, len(memories))
	for _, mem := range memories {
		turn := ConversationTurn{
			Timestamp: mem.Timestamp,
			Metadata:  mem.Metadata,
		}

		if sessionID, ok := mem.Metadata["session_id"].(string); ok {
			turn.SessionID = sessionID
		}
		if userMsg, ok := mem.Metadata["user_msg"].(string); ok {
			turn.User = userMsg
		}
		if assistantMsg, ok := mem.Metadata["assistant_msg"].(string); ok {
			turn.Assistant = assistantMsg
		}

		turns = append(turns, turn)
	}

	return turns, nil
}

// ClearSession 清除指定会话的历史
func (c *ConversationMemory) ClearSession(sessionID string) (int, error) {
	return c.manager.store.DeleteMemoriesBySession(sessionID)
}

// GetSessionIDs 获取所有会话ID
func (c *ConversationMemory) GetSessionIDs() ([]string, error) {
	return c.manager.store.GetSessionIDs()
}

// CountBySession 统计指定会话的对话轮次数
func (c *ConversationMemory) CountBySession(sessionID string) (int, error) {
	return c.manager.store.CountMemoriesBySession(sessionID)
}
