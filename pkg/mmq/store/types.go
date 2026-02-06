package store

import "time"

// Document store内部使用的文档类型
type Document struct {
	ID         string
	Collection string
	Path       string
	Title      string
	Content    string
	Hash       string
	CreatedAt  time.Time
	ModifiedAt time.Time
	Active     bool
}

// SearchResult store内部使用的搜索结果类型
type SearchResult struct {
	ID         string
	Score      float64
	Title      string
	Content    string
	Snippet    string
	Source     string
	Collection string
	Path       string
	Timestamp  time.Time
}

// Status 索引状态
type Status struct {
	TotalDocuments int
	NeedsEmbedding int
	Collections    []string
	DBPath         string
	CacheDir       string
}
