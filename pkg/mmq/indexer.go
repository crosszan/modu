package mmq

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

// IndexDirectory 索引目录（批量索引）
func (m *MMQ) IndexDirectory(path string, opts IndexOptions) error {
	// 展开路径
	absPath, err := filepath.Abs(expandPath(path))
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 检查路径是否存在
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("path not found: %w", err)
	}

	// 设置默认值
	mask := opts.Mask
	if mask == "" {
		mask = "**/*.md"
	}

	collection := opts.Collection
	if collection == "" {
		// 使用目录名作为collection名
		collection = filepath.Base(absPath)
	}

	// 确保集合存在
	exists, _ := m.store.CollectionExists(collection)
	if !exists {
		if err := m.store.CreateCollection(collection, absPath, mask); err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
	}

	// 遍历目录，找到匹配的文件
	var indexed int
	var skipped int

	err = filepath.WalkDir(absPath, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录
		if d.IsDir() {
			// 跳过隐藏目录和node_modules等
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		// 计算相对路径
		relPath, err := filepath.Rel(absPath, filePath)
		if err != nil {
			return err
		}

		// 检查是否匹配mask
		matched, err := doublestar.Match(mask, relPath)
		if err != nil || !matched {
			skipped++
			return nil
		}

		// 读取文件内容
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Printf("Warning: failed to read %s: %v\n", relPath, err)
			skipped++
			return nil
		}

		// 获取文件信息
		info, _ := d.Info()
		modTime := time.Now()
		if info != nil {
			modTime = info.ModTime()
		}

		// 提取标题（从文件名或内容）
		title := extractTitle(string(content), relPath)

		// 索引文档
		doc := Document{
			Collection: collection,
			Path:       relPath,
			Title:      title,
			Content:    string(content),
			CreatedAt:  modTime,
			ModifiedAt: modTime,
		}

		if err := m.IndexDocument(doc); err != nil {
			fmt.Printf("Warning: failed to index %s: %v\n", relPath, err)
			skipped++
			return nil
		}

		indexed++

		// 显示进度
		if indexed%10 == 0 {
			fmt.Printf("Indexed %d files...\n", indexed)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	// 更新集合时间戳
	m.store.UpdateCollectionTimestamp(collection)

	fmt.Printf("\nIndexing complete: %d files indexed, %d skipped\n", indexed, skipped)

	return nil
}

// IndexCollection 索引整个集合（重新索引）
func (m *MMQ) IndexCollection(name string) error {
	// 获取集合信息
	coll, err := m.store.GetCollection(name)
	if err != nil {
		return err
	}

	// 使用集合的路径和mask重新索引
	return m.IndexDirectory(coll.Path, IndexOptions{
		Collection: name,
		Mask:       coll.Mask,
		Recursive:  true,
	})
}

// UpdateCollection 更新集合（可选git pull）
func (m *MMQ) UpdateCollection(name string, pull bool) error {
	// 获取集合信息
	coll, err := m.store.GetCollection(name)
	if err != nil {
		return err
	}

	// 如果需要，执行git pull
	if pull {
		if err := gitPull(coll.Path); err != nil {
			fmt.Printf("Warning: git pull failed: %v\n", err)
			// 继续索引，不中断
		}
	}

	// 重新索引
	return m.IndexCollection(name)
}

// extractTitle 从内容或文件名提取标题
func extractTitle(content, filename string) string {
	// 尝试从markdown中提取h1标题
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimPrefix(line, "# ")
			title = strings.TrimSpace(title)
			if title != "" {
				return title
			}
		}
	}

	// 使用文件名（去掉扩展名）
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// expandPath 展开路径（处理~）
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// gitPull 执行git pull
func gitPull(path string) error {
	// 检查是否是git仓库
	gitDir := filepath.Join(path, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return fmt.Errorf("not a git repository")
	}

	// TODO: 实际执行git pull命令
	// 这里暂时跳过，因为需要exec包
	fmt.Printf("Git pull in %s (skipped in current implementation)\n", path)
	return nil
}

// hashContent 计算内容哈希
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash[:])
}
