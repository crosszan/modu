package mmq

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateCollection(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection("test-coll", "/tmp/test", CollectionOptions{
		Mask: "**/*.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证集合已创建
	coll, err := m.GetCollection("test-coll")
	if err != nil {
		t.Fatal(err)
	}

	if coll.Name != "test-coll" {
		t.Errorf("Expected name 'test-coll', got '%s'", coll.Name)
	}

	if coll.Path != "/tmp/test" {
		t.Errorf("Expected path '/tmp/test', got '%s'", coll.Path)
	}

	if coll.Mask != "**/*.md" {
		t.Errorf("Expected mask '**/*.md', got '%s'", coll.Mask)
	}

	t.Logf("Collection created: %+v", coll)
}

func TestListCollections(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建多个集合
	collections := []struct {
		name string
		path string
	}{
		{"docs", "/tmp/docs"},
		{"notes", "/tmp/notes"},
		{"articles", "/tmp/articles"},
	}

	for _, c := range collections {
		err := m.CreateCollection(c.name, c.path, CollectionOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}

	// 列出集合
	list, err := m.ListCollections()
	if err != nil {
		t.Fatal(err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 collections, got %d", len(list))
	}

	t.Logf("Collections:")
	for i, c := range list {
		t.Logf("  [%d] %s (%s) - %d docs", i+1, c.Name, c.Path, c.DocCount)
	}
}

func TestRemoveCollection(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection("temp-coll", "/tmp/temp", CollectionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 添加一些文档
	doc := Document{
		Collection: "temp-coll",
		Path:       "test.md",
		Title:      "Test",
		Content:    "Test content",
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}
	m.IndexDocument(doc)

	// 删除集合
	err = m.RemoveCollection("temp-coll")
	if err != nil {
		t.Fatal(err)
	}

	// 验证集合已删除
	_, err = m.GetCollection("temp-coll")
	if err == nil {
		t.Error("Expected error when getting deleted collection")
	}

	t.Log("Collection removed successfully")
}

func TestRenameCollection(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection("old-name", "/tmp/test", CollectionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 添加文档
	doc := Document{
		Collection: "old-name",
		Path:       "test.md",
		Title:      "Test",
		Content:    "Test content",
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}
	m.IndexDocument(doc)

	// 重命名
	err = m.RenameCollection("old-name", "new-name")
	if err != nil {
		t.Fatal(err)
	}

	// 验证新名称存在
	coll, err := m.GetCollection("new-name")
	if err != nil {
		t.Fatal(err)
	}

	if coll.Name != "new-name" {
		t.Errorf("Expected name 'new-name', got '%s'", coll.Name)
	}

	// 验证旧名称不存在
	_, err = m.GetCollection("old-name")
	if err == nil {
		t.Error("Expected error when getting old collection name")
	}

	t.Logf("Collection renamed: old-name -> new-name")
}

func TestIndexDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建测试目录结构
	testDir := filepath.Join(tmpDir, "test-docs")
	os.MkdirAll(filepath.Join(testDir, "subdir"), 0755)

	// 创建测试文件
	files := map[string]string{
		"doc1.md":          "# Document 1\nThis is the first document.",
		"doc2.md":          "# Document 2\nThis is the second document.",
		"subdir/doc3.md":   "# Document 3\nThis is in a subdirectory.",
		"readme.txt":       "This is not markdown.",
		".hidden.md":       "Hidden file.",
		"subdir/doc4.html": "HTML file",
	}

	for path, content := range files {
		fullPath := filepath.Join(testDir, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	// 索引目录
	err = m.IndexDirectory(testDir, IndexOptions{
		Collection: "test-docs",
		Mask:       "**/*.md",
		Recursive:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// 验证集合已创建
	coll, err := m.GetCollection("test-docs")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Collection: %s (%d docs)", coll.Name, coll.DocCount)

	// 验证文档数量（应该索引3个.md文件，跳过.hidden.md）
	if coll.DocCount < 3 {
		t.Errorf("Expected at least 3 documents, got %d", coll.DocCount)
	}

	// 搜索验证
	results, err := m.Search("document", SearchOptions{
		Limit:      10,
		Collection: "test-docs",
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Search found %d results", len(results))
	for i, r := range results {
		t.Logf("  [%d] %s - %s", i+1, r.Title, r.Path)
	}
}

func TestCollectionDocCount(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection("count-test", "/tmp/test", CollectionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 初始文档数应该是0
	coll, _ := m.GetCollection("count-test")
	if coll.DocCount != 0 {
		t.Errorf("Expected 0 docs initially, got %d", coll.DocCount)
	}

	// 添加文档
	for i := 0; i < 5; i++ {
		doc := Document{
			Collection: "count-test",
			Path:       filepath.Join("doc", string(rune(i))+".md"),
			Title:      "Doc " + string(rune(i)),
			Content:    "Content",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		}
		m.IndexDocument(doc)
	}

	// 验证文档数
	coll, _ = m.GetCollection("count-test")
	if coll.DocCount != 5 {
		t.Errorf("Expected 5 docs, got %d", coll.DocCount)
	}

	t.Logf("Collection has %d documents", coll.DocCount)
}

func TestCollectionWithDocuments(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewWithDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection("with-docs", "/tmp/test", CollectionOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// 添加文档
	docs := []Document{
		{
			Collection: "with-docs",
			Path:       "go.md",
			Title:      "Go Language",
			Content:    "Go programming language documentation",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
		{
			Collection: "with-docs",
			Path:       "python.md",
			Title:      "Python Language",
			Content:    "Python programming language documentation",
			CreatedAt:  time.Now(),
			ModifiedAt: time.Now(),
		},
	}

	for _, doc := range docs {
		if err := m.IndexDocument(doc); err != nil {
			t.Fatal(err)
		}
	}

	// 列出集合
	collections, err := m.ListCollections()
	if err != nil {
		t.Fatal(err)
	}

	// 找到我们的集合
	var found *Collection
	for _, c := range collections {
		if c.Name == "with-docs" {
			found = &c
			break
		}
	}

	if found == nil {
		t.Fatal("Collection not found in list")
	}

	if found.DocCount != 2 {
		t.Errorf("Expected 2 docs, got %d", found.DocCount)
	}

	t.Logf("Collection: %s has %d documents", found.Name, found.DocCount)
}
