// Package memory 提供记忆管理功能
package memory

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/crosszan/modu/pkg/mmq/llm"
	"github.com/crosszan/modu/pkg/mmq/store"
)

// MemoryType 记忆类型
type MemoryType string

const (
	MemoryTypeConversation MemoryType = "conversation"
	MemoryTypeFact         MemoryType = "fact"
	MemoryTypePreference   MemoryType = "preference"
	MemoryTypeEpisodic     MemoryType = "episodic"
)

// Memory 记忆结构
type Memory struct {
	ID         string
	Type       MemoryType
	Content    string
	Metadata   map[string]interface{}
	Tags       []string
	Timestamp  time.Time
	ExpiresAt  *time.Time
	Importance float64   // 0.0-1.0
	Relevance  float64   // 检索时的相关度
}

// RecallOptions 回忆选项
type RecallOptions struct {
	Limit              int
	MemoryTypes        []MemoryType
	ApplyDecay         bool
	DecayHalflife      time.Duration
	WeightByImportance bool
	MinRelevance       float64
}

// DefaultRecallOptions 默认回忆选项
func DefaultRecallOptions() RecallOptions {
	return RecallOptions{
		Limit:              10,
		MemoryTypes:        nil, // 所有类型
		ApplyDecay:         true,
		DecayHalflife:      24 * time.Hour * 30, // 30天半衰期
		WeightByImportance: true,
		MinRelevance:       0.0,
	}
}

// Manager 记忆管理器
type Manager struct {
	store     *store.Store
	embedding *llm.EmbeddingGenerator
}

// NewManager 创建记忆管理器
func NewManager(st *store.Store, embedding *llm.EmbeddingGenerator) *Manager {
	return &Manager{
		store:     st,
		embedding: embedding,
	}
}

// Store 存储记忆
func (m *Manager) Store(mem Memory) error {
	// 1. 生成嵌入
	embedding, err := m.embedding.Generate(mem.Content, false)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	// 2. 如果未设置重要性，使用默认值
	if mem.Importance == 0 {
		mem.Importance = 0.5 // 默认中等重要性
	}

	// 3. 存储到数据库
	return m.store.InsertMemory(string(mem.Type), mem.Content, mem.Metadata, mem.Tags,
		mem.Timestamp, mem.ExpiresAt, mem.Importance, embedding)
}

// Recall 回忆记忆
func (m *Manager) Recall(query string, opts RecallOptions) ([]Memory, error) {
	// 1. 生成查询向量
	queryEmbedding, err := m.embedding.Generate(query, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 2. 向量搜索
	// 转换MemoryType到string
	var memTypes []string
	if opts.MemoryTypes != nil {
		memTypes = make([]string, len(opts.MemoryTypes))
		for i, mt := range opts.MemoryTypes {
			memTypes[i] = string(mt)
		}
	}

	results, err := m.store.SearchMemories(queryEmbedding, opts.Limit*2, memTypes)
	if err != nil {
		return nil, err
	}

	// 3. 转换为Memory类型
	memories := make([]Memory, len(results))
	for i, r := range results {
		memories[i] = Memory{
			ID:         r.ID,
			Type:       MemoryType(r.Type),
			Content:    r.Content,
			Metadata:   r.Metadata,
			Tags:       r.Tags,
			Timestamp:  r.Timestamp,
			ExpiresAt:  r.ExpiresAt,
			Importance: r.Importance,
			Relevance:  r.Relevance,
		}
	}

	// 4. 应用时间衰减
	if opts.ApplyDecay {
		memories = m.applyTimeDecay(memories, opts.DecayHalflife)
	}

	// 5. 按重要性加权
	if opts.WeightByImportance {
		memories = m.weightByImportance(memories)
	}

	// 6. 重新排序
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Relevance > memories[j].Relevance
	})

	// 7. 应用最小相关度过滤
	if opts.MinRelevance > 0 {
		filtered := make([]Memory, 0, len(memories))
		for _, mem := range memories {
			if mem.Relevance >= opts.MinRelevance {
				filtered = append(filtered, mem)
			}
		}
		memories = filtered
	}

	// 8. 限制返回数量
	if len(memories) > opts.Limit {
		memories = memories[:opts.Limit]
	}

	return memories, nil
}

// applyTimeDecay 应用时间衰减
func (m *Manager) applyTimeDecay(memories []Memory, halflife time.Duration) []Memory {
	now := time.Now()

	for i := range memories {
		age := now.Sub(memories[i].Timestamp)
		decayFactor := math.Exp(-age.Hours() / halflife.Hours())

		// 调整相关性分数
		memories[i].Relevance *= decayFactor
	}

	return memories
}

// weightByImportance 按重要性加权
func (m *Manager) weightByImportance(memories []Memory) []Memory {
	for i := range memories {
		// 重要性作为乘数（0.5-1.5的范围）
		importanceMultiplier := 0.5 + memories[i].Importance
		memories[i].Relevance *= importanceMultiplier
	}

	return memories
}

// Update 更新记忆
func (m *Manager) Update(id string, mem Memory) error {
	// 生成新的嵌入
	embedding, err := m.embedding.Generate(mem.Content, false)
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}

	return m.store.UpdateMemory(id, mem.Content, mem.Metadata, mem.Tags,
		mem.ExpiresAt, mem.Importance, embedding)
}

// Delete 删除记忆
func (m *Manager) Delete(id string) error {
	return m.store.DeleteMemory(id)
}

// GetByID 根据ID获取记忆
func (m *Manager) GetByID(id string) (*Memory, error) {
	result, err := m.store.GetMemoryByID(id)
	if err != nil {
		return nil, err
	}

	mem := &Memory{
		ID:         result.ID,
		Type:       MemoryType(result.Type),
		Content:    result.Content,
		Metadata:   result.Metadata,
		Tags:       result.Tags,
		Timestamp:  result.Timestamp,
		ExpiresAt:  result.ExpiresAt,
		Importance: result.Importance,
		Relevance:  0,
	}

	return mem, nil
}

// GetByType 获取指定类型的所有记忆
func (m *Manager) GetByType(memType MemoryType) ([]Memory, error) {
	results, err := m.store.GetMemoriesByType(string(memType))
	if err != nil {
		return nil, err
	}

	memories := make([]Memory, len(results))
	for i, r := range results {
		memories[i] = Memory{
			ID:         r.ID,
			Type:       MemoryType(r.Type),
			Content:    r.Content,
			Metadata:   r.Metadata,
			Tags:       r.Tags,
			Timestamp:  r.Timestamp,
			ExpiresAt:  r.ExpiresAt,
			Importance: r.Importance,
			Relevance:  0,
		}
	}

	return memories, nil
}

// CleanupExpired 清理过期记忆
func (m *Manager) CleanupExpired() (int, error) {
	return m.store.DeleteExpiredMemories()
}

// Count 统计记忆数量
func (m *Manager) Count() (int, error) {
	return m.store.CountMemories()
}

// CountByType 按类型统计记忆数量
func (m *Manager) CountByType(memType MemoryType) (int, error) {
	return m.store.CountMemoriesByType(string(memType))
}
