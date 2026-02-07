package mmq

import (
	"path/filepath"
	"testing"
	"time"
)

func TestVectorSearch(t *testing.T) {
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
			Content:    "Go is a statically typed compiled language. It has excellent concurrency support with goroutines and channels.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "tech",
			Path:       "python.md",
			Title:      "Python Programming",
			Content:    "Python is a dynamically typed interpreted language. It is great for data science and machine learning with NumPy and Pandas.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "ai",
			Path:       "rag.md",
			Title:      "RAG Systems",
			Content:    "Retrieval-Augmented Generation combines search with language models for better responses. It uses vector embeddings for semantic search.",
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

	// 检查状态
	status, _ := m.Status()
	t.Logf("Status: %d documents, %d need embedding", status.TotalDocuments, status.NeedsEmbedding)

	// 测试向量搜索
	t.Run("Basic VectorSearch", func(t *testing.T) {
		results, err := m.VectorSearch("concurrent programming", SearchOptions{
			Limit: 5,
		})
		if err != nil {
			t.Fatal(err)
		}

		if len(results) == 0 {
			t.Error("Expected at least one result")
		}

		t.Logf("VectorSearch returned %d results", len(results))
		for i, res := range results {
			t.Logf("[%d] Score: %.3f, Title: %s, Source: %s",
				i+1, res.Score, res.Title, res.Source)
		}

		// 验证返回完整文档
		if len(results) > 0 {
			if results[0].Content == "" {
				t.Error("Expected full document content")
			}
			if results[0].Source != "vector" {
				t.Errorf("Expected source 'vector', got '%s'", results[0].Source)
			}
		}
	})

	// 测试集合过滤
	t.Run("VectorSearch with Collection Filter", func(t *testing.T) {
		results, err := m.VectorSearch("programming language", SearchOptions{
			Limit:      5,
			Collection: "tech",
		})
		if err != nil {
			t.Fatal(err)
		}

		// 验证所有结果都来自tech集合
		for _, res := range results {
			if res.Collection != "tech" {
				t.Errorf("Expected collection 'tech', got '%s'", res.Collection)
			}
		}

		t.Logf("Collection filter returned %d results", len(results))
	})

	// 测试分数排序
	t.Run("VectorSearch Score Ordering", func(t *testing.T) {
		results, err := m.VectorSearch("machine learning", SearchOptions{
			Limit: 5,
		})
		if err != nil {
			t.Fatal(err)
		}

		// 验证按分数降序排列
		for i := 1; i < len(results); i++ {
			if results[i].Score > results[i-1].Score {
				t.Error("Results not sorted by score")
			}
		}

		t.Logf("Score ordering verified, top score: %.3f", results[0].Score)
	})
}

func TestVectorSearchVsRetrieveContext(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 索引一个长文档
	doc := Document{
		Collection: "docs",
		Path:       "long.md",
		Title:      "Long Document",
		Content: `
			This is a very long document about programming.
			It talks about Go concurrency with goroutines.
			It also discusses Python data science libraries.
			And mentions machine learning frameworks.
			The document covers many different topics in detail.
			Each section provides comprehensive information.
			Readers can learn a lot from this content.
		`,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	m.IndexDocument(doc)
	m.GenerateEmbeddings()

	// 对比VectorSearch和RetrieveContext
	t.Run("VectorSearch returns full document", func(t *testing.T) {
		results, _ := m.VectorSearch("goroutines", SearchOptions{
			Limit: 1,
		})

		if len(results) == 0 {
			t.Fatal("No results")
		}

		// VectorSearch应该返回完整文档
		if len(results[0].Content) < 100 {
			t.Error("VectorSearch should return full document content")
		}

		t.Logf("VectorSearch returned document of length: %d", len(results[0].Content))
	})

	t.Run("RetrieveContext returns chunks", func(t *testing.T) {
		contexts, _ := m.RetrieveContext("goroutines", RetrieveOptions{
			Limit:    1,
			Strategy: StrategyVector,
		})

		if len(contexts) == 0 {
			t.Fatal("No contexts")
		}

		// RetrieveContext返回文本块（可能是片段）
		t.Logf("RetrieveContext returned chunk of length: %d", len(contexts[0].Text))
	})
}

func BenchmarkVectorSearch(b *testing.B) {
	tmpDir := b.TempDir()
	m, _ := NewWithDB(filepath.Join(tmpDir, "bench.db"))
	defer m.Close()

	// 准备文档
	for i := 0; i < 20; i++ {
		doc := Document{
			Collection: "bench",
			Path:       filepath.Join("doc", string(rune(i))+".md"),
			Title:      "Doc " + string(rune(i)),
			Content:    "This is a document about software development and programming techniques.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
		m.IndexDocument(doc)
	}

	m.GenerateEmbeddings()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.VectorSearch("programming", SearchOptions{Limit: 5})
	}
}
