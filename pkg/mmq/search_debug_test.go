package mmq

import (
	"testing"
	"time"
)

func TestSearchDebug(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 索引一个简单的文档
	doc := Document{
		Collection: "test",
		Path:       "test.md",
		Title:      "Test",
		Content:    "Go is a programming language developed by Google",
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}

	err = m.IndexDocument(doc)
	if err != nil {
		t.Fatal(err)
	}

	// 测试搜索单词"Go"
	t.Log("Searching for 'Go'...")
	results, err := m.Search("Go", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Found %d results", len(results))
	for i, r := range results {
		snippet := r.Content
		if len(snippet) > 50 {
			snippet = snippet[:50]
		}
		t.Logf("[%d] Score: %.4f, Title: %s, Content: %s",
			i, r.Score, r.Title, snippet)
	}

	if len(results) == 0 {
		t.Error("Expected at least 1 result for 'Go'")
	}

	// 测试搜索"Google"
	t.Log("Searching for 'Google'...")
	results, err = m.Search("Google", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Found %d results", len(results))
	if len(results) == 0 {
		t.Error("Expected at least 1 result for 'Google'")
	}

	// 测试搜索"programming"
	t.Log("Searching for 'programming'...")
	results, err = m.Search("programming", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Found %d results", len(results))
	if len(results) == 0 {
		t.Error("Expected at least 1 result for 'programming'")
	}

	// 打印buildFTS5Query的结果
	query := "Go programming"
	t.Logf("buildFTS5Query('%s') would produce: ...", query)
}
