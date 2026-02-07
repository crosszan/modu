package memory

import (
	"fmt"
	"time"
)

// Fact 事实结构（主谓宾三元组）
type Fact struct {
	Subject    string
	Predicate  string
	Object     string
	Confidence float64 // 置信度 0.0-1.0
	Source     string  // 事实来源
	Timestamp  time.Time
}

// FactMemory 事实记忆管理
type FactMemory struct {
	manager *Manager
}

// NewFactMemory 创建事实记忆管理器
func NewFactMemory(manager *Manager) *FactMemory {
	return &FactMemory{manager: manager}
}

// StoreFact 存储事实
func (f *FactMemory) StoreFact(fact Fact) error {
	content := fmt.Sprintf("%s %s %s", fact.Subject, fact.Predicate, fact.Object)

	metadata := map[string]interface{}{
		"subject":    fact.Subject,
		"predicate":  fact.Predicate,
		"object":     fact.Object,
		"confidence": fact.Confidence,
	}

	if fact.Source != "" {
		metadata["source"] = fact.Source
	}

	mem := Memory{
		Type:       MemoryTypeFact,
		Content:    content,
		Metadata:   metadata,
		Timestamp:  fact.Timestamp,
		Importance: fact.Confidence, // 使用置信度作为重要性
	}

	return f.manager.Store(mem)
}

// QueryFact 查询事实
func (f *FactMemory) QueryFact(subject, predicate string) ([]Fact, error) {
	query := fmt.Sprintf("%s %s", subject, predicate)

	opts := RecallOptions{
		Limit:              10,
		MemoryTypes:        []MemoryType{MemoryTypeFact},
		ApplyDecay:         false, // 事实不衰减
		WeightByImportance: true,  // 按置信度加权
		MinRelevance:       0.3,   // 最小相关度阈值
	}

	memories, err := f.manager.Recall(query, opts)
	if err != nil {
		return nil, err
	}

	facts := make([]Fact, 0, len(memories))
	for _, mem := range memories {
		fact := Fact{
			Timestamp: mem.Timestamp,
		}

		if subject, ok := mem.Metadata["subject"].(string); ok {
			fact.Subject = subject
		}
		if predicate, ok := mem.Metadata["predicate"].(string); ok {
			fact.Predicate = predicate
		}
		if object, ok := mem.Metadata["object"].(string); ok {
			fact.Object = object
		}
		if confidence, ok := mem.Metadata["confidence"].(float64); ok {
			fact.Confidence = confidence
		}
		if source, ok := mem.Metadata["source"].(string); ok {
			fact.Source = source
		}

		facts = append(facts, fact)
	}

	return facts, nil
}

// GetFactsBySubject 获取关于某主体的所有事实
func (f *FactMemory) GetFactsBySubject(subject string) ([]Fact, error) {
	opts := RecallOptions{
		Limit:              50,
		MemoryTypes:        []MemoryType{MemoryTypeFact},
		ApplyDecay:         false,
		WeightByImportance: true,
		MinRelevance:       0.2,
	}

	memories, err := f.manager.Recall(subject, opts)
	if err != nil {
		return nil, err
	}

	facts := make([]Fact, 0, len(memories))
	for _, mem := range memories {
		// 过滤：只返回subject匹配的事实
		if subj, ok := mem.Metadata["subject"].(string); !ok || subj != subject {
			continue
		}

		fact := Fact{
			Subject:   subject,
			Timestamp: mem.Timestamp,
		}

		if predicate, ok := mem.Metadata["predicate"].(string); ok {
			fact.Predicate = predicate
		}
		if object, ok := mem.Metadata["object"].(string); ok {
			fact.Object = object
		}
		if confidence, ok := mem.Metadata["confidence"].(float64); ok {
			fact.Confidence = confidence
		}
		if source, ok := mem.Metadata["source"].(string); ok {
			fact.Source = source
		}

		facts = append(facts, fact)
	}

	return facts, nil
}

// UpdateFactConfidence 更新事实的置信度
func (f *FactMemory) UpdateFactConfidence(subject, predicate, object string, newConfidence float64) error {
	// 查找匹配的事实
	facts, err := f.QueryFact(subject, predicate)
	if err != nil {
		return err
	}

	// 找到匹配的事实并更新
	for _, fact := range facts {
		if fact.Object == object {
			// 从metadata获取memory ID
			memories, _ := f.manager.GetByType(MemoryTypeFact)
			for _, mem := range memories {
				if mem.Metadata["subject"] == subject &&
					mem.Metadata["predicate"] == predicate &&
					mem.Metadata["object"] == object {

					mem.Importance = newConfidence
					mem.Metadata["confidence"] = newConfidence

					return f.manager.Update(mem.ID, mem)
				}
			}
		}
	}

	return fmt.Errorf("fact not found: %s %s %s", subject, predicate, object)
}

// DeleteFact 删除事实
func (f *FactMemory) DeleteFact(subject, predicate, object string) error {
	// 查找并删除匹配的事实
	memories, err := f.manager.GetByType(MemoryTypeFact)
	if err != nil {
		return err
	}

	for _, mem := range memories {
		if mem.Metadata["subject"] == subject &&
			mem.Metadata["predicate"] == predicate &&
			mem.Metadata["object"] == object {

			return f.manager.Delete(mem.ID)
		}
	}

	return fmt.Errorf("fact not found: %s %s %s", subject, predicate, object)
}

// GetAllFacts 获取所有事实
func (f *FactMemory) GetAllFacts() ([]Fact, error) {
	memories, err := f.manager.GetByType(MemoryTypeFact)
	if err != nil {
		return nil, err
	}

	facts := make([]Fact, 0, len(memories))
	for _, mem := range memories {
		fact := Fact{
			Timestamp: mem.Timestamp,
		}

		if subject, ok := mem.Metadata["subject"].(string); ok {
			fact.Subject = subject
		}
		if predicate, ok := mem.Metadata["predicate"].(string); ok {
			fact.Predicate = predicate
		}
		if object, ok := mem.Metadata["object"].(string); ok {
			fact.Object = object
		}
		if confidence, ok := mem.Metadata["confidence"].(float64); ok {
			fact.Confidence = confidence
		}
		if source, ok := mem.Metadata["source"].(string); ok {
			fact.Source = source
		}

		facts = append(facts, fact)
	}

	return facts, nil
}

// SearchFacts 语义搜索事实
func (f *FactMemory) SearchFacts(query string, limit int) ([]Fact, error) {
	opts := RecallOptions{
		Limit:              limit,
		MemoryTypes:        []MemoryType{MemoryTypeFact},
		ApplyDecay:         false,
		WeightByImportance: true,
		MinRelevance:       0.3,
	}

	memories, err := f.manager.Recall(query, opts)
	if err != nil {
		return nil, err
	}

	facts := make([]Fact, 0, len(memories))
	for _, mem := range memories {
		fact := Fact{
			Timestamp: mem.Timestamp,
		}

		if subject, ok := mem.Metadata["subject"].(string); ok {
			fact.Subject = subject
		}
		if predicate, ok := mem.Metadata["predicate"].(string); ok {
			fact.Predicate = predicate
		}
		if object, ok := mem.Metadata["object"].(string); ok {
			fact.Object = object
		}
		if confidence, ok := mem.Metadata["confidence"].(float64); ok {
			fact.Confidence = confidence
		}
		if source, ok := mem.Metadata["source"].(string); ok {
			fact.Source = source
		}

		facts = append(facts, fact)
	}

	return facts, nil
}
