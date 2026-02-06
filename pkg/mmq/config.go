package mmq

import (
	"os"
	"path/filepath"
)

// Config MMQ配置
type Config struct {
	// DBPath 数据库路径
	DBPath string
	// CacheDir 模型缓存目录
	CacheDir string
	// EmbeddingModel 嵌入模型
	EmbeddingModel string
	// RerankModel 重排模型
	RerankModel string
	// ChunkSize 分块大小（字符数）
	ChunkSize int
	// ChunkOverlap 分块重叠（字符数）
	ChunkOverlap int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	homeDir, _ := os.UserHomeDir()

	return Config{
		DBPath:         filepath.Join(homeDir, ".modu", "memory.db"),
		CacheDir:       filepath.Join(homeDir, ".cache", "modu", "models"),
		EmbeddingModel: "embeddinggemma-300M-Q8_0",
		RerankModel:    "qwen3-reranker-0.6b-q8_0",
		ChunkSize:      3200, // ~800 tokens
		ChunkOverlap:   480,  // 15% overlap
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.DBPath == "" {
		c.DBPath = DefaultConfig().DBPath
	}

	if c.CacheDir == "" {
		c.CacheDir = DefaultConfig().CacheDir
	}

	// 创建必要的目录
	if err := os.MkdirAll(filepath.Dir(c.DBPath), 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(c.CacheDir, 0755); err != nil {
		return err
	}

	// 设置默认值
	if c.ChunkSize == 0 {
		c.ChunkSize = 3200
	}

	if c.ChunkOverlap == 0 {
		c.ChunkOverlap = 480
	}

	if c.EmbeddingModel == "" {
		c.EmbeddingModel = "embeddinggemma-300M-Q8_0"
	}

	if c.RerankModel == "" {
		c.RerankModel = "qwen3-reranker-0.6b-q8_0"
	}

	return nil
}
