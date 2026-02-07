package mmq

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRetrieveContext(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 索引文档
	docs := []Document{
		{
			Collection: "tech",
			Path:       "go.md",
			Title:      "Go Programming",
			Content:    "Go is a statically typed compiled language. It has excellent concurrency support with goroutines.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "tech",
			Path:       "python.md",
			Title:      "Python Programming",
			Content:    "Python is a dynamically typed interpreted language. It is great for data science and machine learning.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "ai",
			Path:       "rag.md",
			Title:      "RAG Systems",
			Content:    "Retrieval-Augmented Generation combines search with language models for better responses.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
	}

	for _, doc := range docs {
		if err := m.IndexDocument(doc); err != nil {
			t.Fatal(err)
		}
	}

	// 生成嵌入
	if err := m.GenerateEmbeddings(); err != nil {
		t.Fatal(err)
	}

	// 测试BM25检索
	t.Run("FTS Strategy", func(t *testing.T) {
		contexts, err := m.RetrieveContext("Go programming", RetrieveOptions{
			Limit:    5,
			Strategy: StrategyFTS,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(contexts) == 0 {
			t.Error("Expected at least one context")
		}

		t.Logf("FTS Strategy returned %d contexts", len(contexts))
		if len(contexts) > 0 {
			t.Logf("Top result: %s (%.2f)", contexts[0].Source, contexts[0].Relevance)
		}
	})

	// 测试向量检索
	t.Run("Vector Strategy", func(t *testing.T) {
		contexts, err := m.RetrieveContext("concurrent programming", RetrieveOptions{
			Limit:    5,
			Strategy: StrategyVector,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(contexts) == 0 {
			t.Error("Expected at least one context")
		}

		t.Logf("Vector Strategy returned %d contexts", len(contexts))
		if len(contexts) > 0 {
			t.Logf("Top result: %s (%.2f)", contexts[0].Source, contexts[0].Relevance)
		}
	})

	// 测试混合检索
	t.Run("Hybrid Strategy", func(t *testing.T) {
		contexts, err := m.RetrieveContext("language model search", RetrieveOptions{
			Limit:    5,
			Strategy: StrategyHybrid,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(contexts) == 0 {
			t.Error("Expected at least one context")
		}

		t.Logf("Hybrid Strategy returned %d contexts", len(contexts))
		if len(contexts) > 0 {
			t.Logf("Top result: %s (%.2f)", contexts[0].Source, contexts[0].Relevance)
		}
	})

	// 测试集合过滤
	t.Run("Collection Filter", func(t *testing.T) {
		contexts, err := m.RetrieveContext("programming", RetrieveOptions{
			Limit:      5,
			Strategy:   StrategyHybrid,
			Collection: "tech",
		})
		if err != nil {
			t.Fatal(err)
		}

		// 验证所有结果都来自tech集合
		for _, ctx := range contexts {
			if collection, ok := ctx.Metadata["collection"].(string); ok {
				if collection != "tech" {
					t.Errorf("Expected collection 'tech', got '%s'", collection)
				}
			}
		}

		t.Logf("Collection filter returned %d contexts", len(contexts))
	})

	// 测试分数阈值
	t.Run("MinScore Filter", func(t *testing.T) {
		contexts, err := m.RetrieveContext("test", RetrieveOptions{
			Limit:    10,
			Strategy: StrategyHybrid,
			MinScore: 0.5,
		})
		if err != nil {
			t.Fatal(err)
		}

		// 验证所有结果分数都 >= 0.5
		for _, ctx := range contexts {
			if ctx.Relevance < 0.5 {
				t.Errorf("Expected relevance >= 0.5, got %.2f", ctx.Relevance)
			}
		}

		t.Logf("MinScore filter returned %d contexts", len(contexts))
	})
}

func TestHybridSearch(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 索引文档
	docs := []Document{
		{
			Collection: "docs",
			Path:       "doc1.md",
			Title:      "Document 1",
			Content:    "This document is about Go programming and concurrent systems.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "docs",
			Path:       "doc2.md",
			Title:      "Document 2",
			Content:    "This document discusses Python data analysis and visualization.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
	}

	for _, doc := range docs {
		if err := m.IndexDocument(doc); err != nil {
			t.Fatal(err)
		}
	}

	// 生成嵌入
	if err := m.GenerateEmbeddings(); err != nil {
		t.Fatal(err)
	}

	// 测试混合搜索
	results, err := m.HybridSearch("programming", SearchOptions{
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	t.Logf("HybridSearch returned %d results", len(results))
	for i, res := range results {
		t.Logf("[%d] Score: %.2f, Title: %s", i+1, res.Score, res.Title)
	}

	// 验证结果按分数排序
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Error("Results not sorted by score")
		}
	}
}

func TestRetrieveContextMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 索引文档
	doc := Document{
		Collection: "test",
		Path:       "test.md",
		Title:      "Test Document",
		Content:    "This is a test document with metadata.",
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	if err := m.IndexDocument(doc); err != nil {
		t.Fatal(err)
	}

	if err := m.GenerateEmbeddings(); err != nil {
		t.Fatal(err)
	}

	// 检索
	contexts, err := m.RetrieveContext("test", RetrieveOptions{
		Limit:    1,
		Strategy: StrategyFTS,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(contexts) == 0 {
		t.Fatal("Expected at least one context")
	}

	// 验证元数据
	ctx := contexts[0]
	if ctx.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	// 检查必要的元数据字段
	requiredFields := []string{"title", "collection", "path", "source"}
	for _, field := range requiredFields {
		if _, ok := ctx.Metadata[field]; !ok {
			t.Errorf("Missing metadata field: %s", field)
		}
	}

	t.Logf("Context metadata: %v", ctx.Metadata)
}

func BenchmarkRetrieveContext(b *testing.B) {
	tmpDir := b.TempDir()
	m, _ := NewWithDB(filepath.Join(tmpDir, "bench.db"))
	defer m.Close()

	// 准备文档
	for i := 0; i < 50; i++ {
		doc := Document{
			Collection: "bench",
			Path:       filepath.Join("doc", string(rune(i))+".md"),
			Title:      "Doc " + string(rune(i)),
			Content:    "Content about programming and software development for document " + string(rune(i)),
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
		m.IndexDocument(doc)
	}

	m.GenerateEmbeddings()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.RetrieveContext("programming", RetrieveOptions{
			Limit:    10,
			Strategy: StrategyHybrid,
		})
	}
}

func BenchmarkHybridSearch(b *testing.B) {
	tmpDir := b.TempDir()
	m, _ := NewWithDB(filepath.Join(tmpDir, "bench.db"))
	defer m.Close()

	// 准备文档
	for i := 0; i < 50; i++ {
		doc := Document{
			Collection: "bench",
			Path:       filepath.Join("doc", string(rune(i))+".md"),
			Title:      "Doc " + string(rune(i)),
			Content:    "Software development and programming techniques in document " + string(rune(i)),
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
		m.IndexDocument(doc)
	}

	m.GenerateEmbeddings()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.HybridSearch("programming", SearchOptions{Limit: 10})
	}
}
