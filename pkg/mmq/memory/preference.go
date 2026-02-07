package memory

import (
	"encoding/json"
	"fmt"
	"time"
)

// Preference 偏好结构
type Preference struct {
	Category string
	Key      string
	Value    interface{}
	Source   string // 偏好来源（如 "user", "inferred"）
	Timestamp time.Time
}

// PreferenceMemory 偏好记忆管理
type PreferenceMemory struct {
	manager *Manager
}

// NewPreferenceMemory 创建偏好记忆管理器
func NewPreferenceMemory(manager *Manager) *PreferenceMemory {
	return &PreferenceMemory{manager: manager}
}

// RecordPreference 记录偏好
func (p *PreferenceMemory) RecordPreference(pref Preference) error {
	// 序列化value为JSON字符串
	valueJSON, err := json.Marshal(pref.Value)
	if err != nil {
		return fmt.Errorf("failed to marshal preference value: %w", err)
	}

	content := fmt.Sprintf("用户偏好 %s: %s = %s", pref.Category, pref.Key, string(valueJSON))

	metadata := map[string]interface{}{
		"category": pref.Category,
		"key":      pref.Key,
		"value":    pref.Value,
	}

	if pref.Source != "" {
		metadata["source"] = pref.Source
	}

	mem := Memory{
		Type:       MemoryTypePreference,
		Content:    content,
		Metadata:   metadata,
		Timestamp:  pref.Timestamp,
		Importance: 1.0, // 偏好总是重要的
	}

	return p.manager.Store(mem)
}

// GetPreference 获取偏好
func (p *PreferenceMemory) GetPreference(category, key string) (interface{}, error) {
	query := fmt.Sprintf("%s %s", category, key)

	opts := RecallOptions{
		Limit:              1,
		MemoryTypes:        []MemoryType{MemoryTypePreference},
		ApplyDecay:         false, // 偏好不衰减
		WeightByImportance: false,
		MinRelevance:       0.0,
	}

	memories, err := p.manager.Recall(query, opts)
	if err != nil {
		return nil, err
	}

	if len(memories) == 0 {
		return nil, fmt.Errorf("preference not found: %s/%s", category, key)
	}

	// 验证category和key匹配
	mem := memories[0]
	if mem.Metadata["category"] != category || mem.Metadata["key"] != key {
		// 如果最相关的结果不是精确匹配，尝试精确查找
		return p.getPreferenceExact(category, key)
	}

	return mem.Metadata["value"], nil
}

// getPreferenceExact 精确查找偏好
func (p *PreferenceMemory) getPreferenceExact(category, key string) (interface{}, error) {
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return nil, err
	}

	for _, mem := range memories {
		if mem.Metadata["category"] == category && mem.Metadata["key"] == key {
			return mem.Metadata["value"], nil
		}
	}

	return nil, fmt.Errorf("preference not found: %s/%s", category, key)
}

// GetAllPreferences 获取所有偏好
func (p *PreferenceMemory) GetAllPreferences() (map[string]map[string]interface{}, error) {
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return nil, err
	}

	prefs := make(map[string]map[string]interface{})

	for _, mem := range memories {
		category, ok1 := mem.Metadata["category"].(string)
		key, ok2 := mem.Metadata["key"].(string)
		value := mem.Metadata["value"]

		if !ok1 || !ok2 {
			continue
		}

		if prefs[category] == nil {
			prefs[category] = make(map[string]interface{})
		}

		prefs[category][key] = value
	}

	return prefs, nil
}

// GetPreferencesByCategory 获取指定类别的所有偏好
func (p *PreferenceMemory) GetPreferencesByCategory(category string) (map[string]interface{}, error) {
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return nil, err
	}

	prefs := make(map[string]interface{})

	for _, mem := range memories {
		if cat, ok := mem.Metadata["category"].(string); ok && cat == category {
			if key, ok := mem.Metadata["key"].(string); ok {
				prefs[key] = mem.Metadata["value"]
			}
		}
	}

	if len(prefs) == 0 {
		return nil, fmt.Errorf("no preferences found for category: %s", category)
	}

	return prefs, nil
}

// UpdatePreference 更新偏好
func (p *PreferenceMemory) UpdatePreference(category, key string, newValue interface{}) error {
	// 查找现有偏好
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return err
	}

	for _, mem := range memories {
		if mem.Metadata["category"] == category && mem.Metadata["key"] == key {
			// 更新值
			mem.Metadata["value"] = newValue

			// 重新生成content
			valueJSON, _ := json.Marshal(newValue)
			mem.Content = fmt.Sprintf("用户偏好 %s: %s = %s", category, key, string(valueJSON))

			return p.manager.Update(mem.ID, mem)
		}
	}

	// 如果不存在，创建新偏好
	return p.RecordPreference(Preference{
		Category:  category,
		Key:       key,
		Value:     newValue,
		Source:    "updated",
		Timestamp: time.Now(),
	})
}

// DeletePreference 删除偏好
func (p *PreferenceMemory) DeletePreference(category, key string) error {
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return err
	}

	for _, mem := range memories {
		if mem.Metadata["category"] == category && mem.Metadata["key"] == key {
			return p.manager.Delete(mem.ID)
		}
	}

	return fmt.Errorf("preference not found: %s/%s", category, key)
}

// DeleteCategory 删除整个类别的偏好
func (p *PreferenceMemory) DeleteCategory(category string) (int, error) {
	memories, err := p.manager.GetByType(MemoryTypePreference)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, mem := range memories {
		if cat, ok := mem.Metadata["category"].(string); ok && cat == category {
			if err := p.manager.Delete(mem.ID); err == nil {
				count++
			}
		}
	}

	return count, nil
}

// SearchPreferences 语义搜索偏好
func (p *PreferenceMemory) SearchPreferences(query string, limit int) ([]Preference, error) {
	opts := RecallOptions{
		Limit:              limit,
		MemoryTypes:        []MemoryType{MemoryTypePreference},
		ApplyDecay:         false,
		WeightByImportance: false,
		MinRelevance:       0.3,
	}

	memories, err := p.manager.Recall(query, opts)
	if err != nil {
		return nil, err
	}

	prefs := make([]Preference, 0, len(memories))
	for _, mem := range memories {
		pref := Preference{
			Timestamp: mem.Timestamp,
		}

		if category, ok := mem.Metadata["category"].(string); ok {
			pref.Category = category
		}
		if key, ok := mem.Metadata["key"].(string); ok {
			pref.Key = key
		}
		if value := mem.Metadata["value"]; value != nil {
			pref.Value = value
		}
		if source, ok := mem.Metadata["source"].(string); ok {
			pref.Source = source
		}

		prefs = append(prefs, pref)
	}

	return prefs, nil
}

// ExportPreferences 导出偏好为JSON
func (p *PreferenceMemory) ExportPreferences() (string, error) {
	prefs, err := p.GetAllPreferences()
	if err != nil {
		return "", err
	}

	jsonData, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal preferences: %w", err)
	}

	return string(jsonData), nil
}

// ImportPreferences 从JSON导入偏好
func (p *PreferenceMemory) ImportPreferences(jsonData string) error {
	var prefs map[string]map[string]interface{}

	if err := json.Unmarshal([]byte(jsonData), &prefs); err != nil {
		return fmt.Errorf("failed to unmarshal preferences: %w", err)
	}

	now := time.Now()
	for category, kvs := range prefs {
		for key, value := range kvs {
			pref := Preference{
				Category:  category,
				Key:       key,
				Value:     value,
				Source:    "imported",
				Timestamp: now,
			}

			if err := p.RecordPreference(pref); err != nil {
				return fmt.Errorf("failed to import preference %s/%s: %w", category, key, err)
			}
		}
	}

	return nil
}
