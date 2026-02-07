package mmq

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreAndRecallMemory(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 存储记忆
	now := time.Now()
	mem := Memory{
		Type:       MemoryTypeConversation,
		Content:    "用户问：什么是RAG？助手答：RAG是检索增强生成技术。",
		Metadata: map[string]interface{}{
			"session_id": "test-session",
			"user_msg":   "什么是RAG？",
		},
		Timestamp:  now,
		Importance: 0.8,
	}

	err = m.StoreMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	// 回忆记忆
	opts := RecallOptions{
		Limit:              5,
		ApplyDecay:         false,
		WeightByImportance: true,
		MinRelevance:       0.0,
	}

	memories, err := m.RecallMemories("RAG", opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(memories) == 0 {
		t.Error("Expected at least one memory")
	}

	t.Logf("Recalled %d memories", len(memories))
	if len(memories) > 0 {
		t.Logf("Top memory: %s (relevance: %.2f)", memories[0].Content, memories[0].Metadata["relevance"])
	}
}

func TestMemoryTypes(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储不同类型的记忆
	memories := []Memory{
		{
			Type:       MemoryTypeConversation,
			Content:    "对话: 讨论了Go语言",
			Timestamp:  now,
			Importance: 0.5,
		},
		{
			Type:       MemoryTypeFact,
			Content:    "Go 是 静态类型语言",
			Timestamp:  now,
			Importance: 0.9,
		},
		{
			Type:       MemoryTypePreference,
			Content:    "用户偏好 编程语言: Go",
			Timestamp:  now,
			Importance: 1.0,
		},
	}

	for _, mem := range memories {
		if err := m.StoreMemory(mem); err != nil {
			t.Fatalf("Failed to store %s memory: %v", mem.Type, err)
		}
	}

	// 测试按类型过滤
	t.Run("Filter by type", func(t *testing.T) {
		opts := RecallOptions{
			Limit:        10,
			MemoryTypes:  []MemoryType{MemoryTypeFact},
			MinRelevance: 0.0,
		}

		factMemories, err := m.RecallMemories("Go", opts)
		if err != nil {
			t.Fatal(err)
		}

		if len(factMemories) == 0 {
			t.Error("Expected at least one fact memory")
		}

		// 验证所有返回的都是fact类型
		for _, mem := range factMemories {
			if mem.Type != MemoryTypeFact {
				t.Errorf("Expected fact memory, got %s", mem.Type)
			}
		}

		t.Logf("Found %d fact memories", len(factMemories))
	})
}

func TestTimeDecay(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储新旧记忆
	oldMem := Memory{
		Type:       MemoryTypeConversation,
		Content:    "旧的对话内容",
		Timestamp:  now.Add(-7 * 24 * time.Hour), // 7天前
		Importance: 0.5,
	}

	newMem := Memory{
		Type:       MemoryTypeConversation,
		Content:    "新的对话内容",
		Timestamp:  now,
		Importance: 0.5,
	}

	m.StoreMemory(oldMem)
	m.StoreMemory(newMem)

	// 测试时间衰减
	t.Run("With decay", func(t *testing.T) {
		opts := RecallOptions{
			Limit:         10,
			ApplyDecay:    true,
			DecayHalflife: 7 * 24 * time.Hour, // 7天半衰期
		}

		memories, err := m.RecallMemories("对话", opts)
		if err != nil {
			t.Fatal(err)
		}

		if len(memories) < 2 {
			t.Error("Expected at least 2 memories")
		}

		// 新记忆应该排在前面（应用衰减后）
		t.Logf("Memory 1 (timestamp: %v): relevance = %.4f",
			memories[0].Timestamp, memories[0].Metadata["relevance"])
		t.Logf("Memory 2 (timestamp: %v): relevance = %.4f",
			memories[1].Timestamp, memories[1].Metadata["relevance"])
	})

	t.Run("Without decay", func(t *testing.T) {
		opts := RecallOptions{
			Limit:      10,
			ApplyDecay: false,
		}

		memories, err := m.RecallMemories("对话", opts)
		if err != nil {
			t.Fatal(err)
		}

		t.Logf("Without decay: found %d memories", len(memories))
	})
}

func TestImportanceWeighting(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储不同重要性的记忆
	highImportance := Memory{
		Type:       MemoryTypeFact,
		Content:    "重要事实",
		Timestamp:  now,
		Importance: 1.0,
	}

	lowImportance := Memory{
		Type:       MemoryTypeFact,
		Content:    "普通事实",
		Timestamp:  now,
		Importance: 0.2,
	}

	m.StoreMemory(highImportance)
	m.StoreMemory(lowImportance)

	// 测试重要性加权
	opts := RecallOptions{
		Limit:              10,
		WeightByImportance: true,
		ApplyDecay:         false,
	}

	memories, err := m.RecallMemories("事实", opts)
	if err != nil {
		t.Fatal(err)
	}

	if len(memories) < 2 {
		t.Error("Expected at least 2 memories")
	}

	// 高重要性的应该排在前面
	t.Logf("Memory 1 (importance: %.1f): %s", memories[0].Importance, memories[0].Content)
	t.Logf("Memory 2 (importance: %.1f): %s", memories[1].Importance, memories[1].Content)
}

func TestUpdateMemory(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储记忆
	mem := Memory{
		Type:       MemoryTypeFact,
		Content:    "原始内容",
		Timestamp:  now,
		Importance: 0.5,
	}

	err = m.StoreMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	// 获取记忆ID（通过回忆）
	opts := RecallOptions{Limit: 1}
	memories, _ := m.RecallMemories("原始", opts)
	if len(memories) == 0 {
		t.Fatal("Failed to retrieve stored memory")
	}

	memID := memories[0].ID

	// 更新记忆
	updatedMem := Memory{
		Type:       MemoryTypeFact,
		Content:    "更新后的内容",
		Timestamp:  now,
		Importance: 0.8,
	}

	err = m.UpdateMemory(memID, updatedMem)
	if err != nil {
		t.Fatal(err)
	}

	// 验证更新
	retrieved, err := m.GetMemoryByID(memID)
	if err != nil {
		t.Fatal(err)
	}

	if retrieved.Content != "更新后的内容" {
		t.Errorf("Expected updated content, got: %s", retrieved.Content)
	}

	if retrieved.Importance != 0.8 {
		t.Errorf("Expected importance 0.8, got: %.1f", retrieved.Importance)
	}

	t.Logf("Memory updated successfully: %s", retrieved.Content)
}

func TestDeleteMemory(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储记忆
	mem := Memory{
		Type:       MemoryTypeConversation,
		Content:    "要删除的记忆",
		Timestamp:  now,
		Importance: 0.5,
	}

	err = m.StoreMemory(mem)
	if err != nil {
		t.Fatal(err)
	}

	// 获取记忆ID
	opts := RecallOptions{Limit: 1}
	memories, _ := m.RecallMemories("删除", opts)
	if len(memories) == 0 {
		t.Fatal("Failed to retrieve stored memory")
	}

	memID := memories[0].ID

	// 删除记忆
	err = m.DeleteMemory(memID)
	if err != nil {
		t.Fatal(err)
	}

	// 验证删除
	_, err = m.GetMemoryByID(memID)
	if err == nil {
		t.Error("Expected error when retrieving deleted memory")
	}

	t.Log("Memory deleted successfully")
}

func TestExpiredMemories(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()
	pastTime := now.Add(-1 * time.Hour)

	// 存储过期记忆
	expiredMem := Memory{
		Type:       MemoryTypeConversation,
		Content:    "已过期的记忆",
		Timestamp:  now,
		ExpiresAt:  &pastTime,
		Importance: 0.5,
	}

	err = m.StoreMemory(expiredMem)
	if err != nil {
		t.Fatal(err)
	}

	// 清理过期记忆
	count, err := m.CleanupExpiredMemories()
	if err != nil {
		t.Fatal(err)
	}

	if count < 1 {
		t.Error("Expected at least 1 expired memory to be deleted")
	}

	t.Logf("Cleaned up %d expired memories", count)
}

func TestCountMemories(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	now := time.Now()

	// 存储多个记忆
	for i := 0; i < 5; i++ {
		mem := Memory{
			Type:       MemoryTypeConversation,
			Content:    "测试记忆",
			Timestamp:  now,
			Importance: 0.5,
		}
		m.StoreMemory(mem)
	}

	// 统计记忆
	count, err := m.CountMemories()
	if err != nil {
		t.Fatal(err)
	}

	if count < 5 {
		t.Errorf("Expected at least 5 memories, got %d", count)
	}

	t.Logf("Total memories: %d", count)
}

func BenchmarkStoreMemory(b *testing.B) {
	tmpDir := b.TempDir()
	m, _ := NewWithDB(filepath.Join(tmpDir, "bench.db"))
	defer m.Close()

	now := time.Now()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mem := Memory{
			Type:       MemoryTypeConversation,
			Content:    "benchmark memory content",
			Timestamp:  now,
			Importance: 0.5,
		}
		m.StoreMemory(mem)
	}
}

func BenchmarkRecallMemories(b *testing.B) {
	tmpDir := b.TempDir()
	m, _ := NewWithDB(filepath.Join(tmpDir, "bench.db"))
	defer m.Close()

	// 准备数据
	now := time.Now()
	for i := 0; i < 100; i++ {
		mem := Memory{
			Type:       MemoryTypeConversation,
			Content:    "memory content for benchmark",
			Timestamp:  now,
			Importance: 0.5,
		}
		m.StoreMemory(mem)
	}

	opts := RecallOptions{
		Limit:      10,
		ApplyDecay: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.RecallMemories("memory", opts)
	}
}
