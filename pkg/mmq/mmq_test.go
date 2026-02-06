package mmq

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMMQBasic(t *testing.T) {
	// 创建临时数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 初始化MMQ
	cfg := DefaultConfig()
	cfg.DBPath = dbPath

	m, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create MMQ: %v", err)
	}
	defer m.Close()

	// 测试索引文档
	doc := Document{
		Collection: "test",
		Path:       "readme.md",
		Title:      "Test Document",
		Content:    "This is a test document about Golang and RAG systems. It contains information about vector search and BM25 algorithms.",
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	err = m.IndexDocument(doc)
	if err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// 测试获取文档（使用path）
	retrieved, err := m.GetDocument("readme.md")
	if err != nil {
		t.Fatalf("Failed to get document: %v", err)
	}

	if retrieved.Title != doc.Title {
		t.Errorf("Expected title %q, got %q", doc.Title, retrieved.Title)
	}

	// 测试状态
	status, err := m.Status()
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if status.TotalDocuments != 1 {
		t.Errorf("Expected 1 document, got %d", status.TotalDocuments)
	}

	if len(status.Collections) != 1 || status.Collections[0] != "test" {
		t.Errorf("Expected collection [test], got %v", status.Collections)
	}

	// 测试搜索
	results, err := m.Search("Golang", SearchOptions{
		Limit:      5,
		Collection: "test",
	})
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one search result")
	}

	if len(results) > 0 {
		if results[0].Title != doc.Title {
			t.Errorf("Expected result title %q, got %q", doc.Title, results[0].Title)
		}
		t.Logf("Search result score: %.4f", results[0].Score)
	}

	// 测试删除文档（使用path）
	err = m.DeleteDocument("readme.md")
	if err != nil {
		t.Fatalf("Failed to delete document: %v", err)
	}

	// 验证删除
	status, err = m.Status()
	if err != nil {
		t.Fatalf("Failed to get status after delete: %v", err)
	}

	if status.TotalDocuments != 0 {
		t.Errorf("Expected 0 documents after delete, got %d", status.TotalDocuments)
	}
}

func TestMMQMultipleDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := DefaultConfig()
	cfg.DBPath = dbPath

	m, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create MMQ: %v", err)
	}
	defer m.Close()

	// 索引多个文档
	docs := []Document{
		{
			Collection: "tech",
			Path:       "golang.md",
			Title:      "Go Programming",
			Content:    "Go is a statically typed, compiled programming language. It has goroutines for concurrency.",
		},
		{
			Collection: "tech",
			Path:       "python.md",
			Title:      "Python Programming",
			Content:    "Python is a high-level, interpreted programming language. It's great for data science.",
		},
		{
			Collection: "docs",
			Path:       "rag.md",
			Title:      "RAG Systems",
			Content:    "Retrieval-Augmented Generation combines search with language models for better responses.",
		},
	}

	for _, doc := range docs {
		doc.CreatedAt = time.Now()
		doc.ModifiedAt = time.Now()
		if err := m.IndexDocument(doc); err != nil {
			t.Fatalf("Failed to index document %s: %v", doc.Path, err)
		}
	}

	// 测试集合过滤
	results, err := m.Search("programming", SearchOptions{
		Limit:      10,
		Collection: "tech",
	})
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 results in 'tech' collection, got %d", len(results))
	}

	// 测试全局搜索
	results, err = m.Search("language", SearchOptions{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results, got %d", len(results))
	}

	// 测试状态
	status, err := m.Status()
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if status.TotalDocuments != 3 {
		t.Errorf("Expected 3 documents, got %d", status.TotalDocuments)
	}

	if len(status.Collections) != 2 {
		t.Errorf("Expected 2 collections, got %d", len(status.Collections))
	}
}

func TestMMQNewWithDB(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "quick.db")

	// 测试快速初始化
	m, err := NewWithDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create MMQ with NewWithDB: %v", err)
	}
	defer m.Close()

	// 验证配置
	status, err := m.Status()
	if err != nil {
		t.Fatalf("Failed to get status: %v", err)
	}

	if status.DBPath != dbPath {
		t.Errorf("Expected DBPath %q, got %q", dbPath, status.DBPath)
	}
}

func TestChunking(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "chunk.db")

	m, err := NewWithDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create MMQ: %v", err)
	}
	defer m.Close()

	// 创建一个大文档
	longContent := ""
	for i := 0; i < 100; i++ {
		longContent += "This is paragraph number " + string(rune(i)) + ". It contains some information about various topics including Go programming, RAG systems, and vector databases. "
	}

	doc := Document{
		Collection: "test",
		Path:       "long.md",
		Title:      "Long Document",
		Content:    longContent,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	err = m.IndexDocument(doc)
	if err != nil {
		t.Fatalf("Failed to index long document: %v", err)
	}

	// 搜索应该能找到
	results, err := m.Search("paragraph", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected to find the long document")
	}
}

func BenchmarkSearch(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")

	m, _ := NewWithDB(dbPath)
	defer m.Close()

	// 准备测试数据
	for i := 0; i < 100; i++ {
		doc := Document{
			Collection: "bench",
			Path:       filepath.Join("doc", string(rune(i))+".md"),
			Title:      "Document " + string(rune(i)),
			Content:    "This is document number " + string(rune(i)) + " about Go programming and search systems.",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
		m.IndexDocument(doc)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Search("programming", SearchOptions{Limit: 10})
	}
}
