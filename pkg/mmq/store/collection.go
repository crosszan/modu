package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Collection 集合信息
type Collection struct {
	Name       string
	Path       string    // 文件系统路径
	Mask       string    // Glob匹配模式，如 "**/*.md"
	CreatedAt  time.Time
	UpdatedAt  time.Time
	DocCount   int // 文档数量（统计信息）
}

// CreateCollection 创建集合
func (s *Store) CreateCollection(name, path, mask string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// 检查是否已存在
	var exists int
	err := s.db.QueryRow("SELECT COUNT(*) FROM collections WHERE name = ?", name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check collection: %w", err)
	}

	if exists > 0 {
		return fmt.Errorf("collection '%s' already exists", name)
	}

	// 插入集合
	_, err = s.db.Exec(`
		INSERT INTO collections (name, path, mask, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, path, mask, now, now)

	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

// ListCollections 列出所有集合
func (s *Store) ListCollections() ([]Collection, error) {
	rows, err := s.db.Query(`
		SELECT
			c.name,
			c.path,
			c.mask,
			c.created_at,
			c.updated_at,
			COUNT(DISTINCT d.id) as doc_count
		FROM collections c
		LEFT JOIN documents d ON d.collection = c.name AND d.active = 1
		GROUP BY c.name
		ORDER BY c.created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query collections: %w", err)
	}
	defer rows.Close()

	var collections []Collection
	for rows.Next() {
		var c Collection
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&c.Name,
			&c.Path,
			&c.Mask,
			&createdAtStr,
			&updatedAtStr,
			&c.DocCount,
		)
		if err != nil {
			continue
		}

		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

		collections = append(collections, c)
	}

	return collections, nil
}

// GetCollection 获取集合信息
func (s *Store) GetCollection(name string) (*Collection, error) {
	var c Collection
	var createdAtStr, updatedAtStr string
	var docCount sql.NullInt64

	err := s.db.QueryRow(`
		SELECT
			c.name,
			c.path,
			c.mask,
			c.created_at,
			c.updated_at,
			COUNT(DISTINCT d.id) as doc_count
		FROM collections c
		LEFT JOIN documents d ON d.collection = c.name AND d.active = 1
		WHERE c.name = ?
		GROUP BY c.name
	`, name).Scan(
		&c.Name,
		&c.Path,
		&c.Mask,
		&createdAtStr,
		&updatedAtStr,
		&docCount,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("collection '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	if docCount.Valid {
		c.DocCount = int(docCount.Int64)
	}

	return &c, nil
}

// RemoveCollection 删除集合
func (s *Store) RemoveCollection(name string) error {
	// 先检查是否存在
	_, err := s.GetCollection(name)
	if err != nil {
		return err
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 删除集合的所有文档（设置为inactive）
	_, err = tx.Exec("UPDATE documents SET active = 0 WHERE collection = ?", name)
	if err != nil {
		return fmt.Errorf("failed to deactivate documents: %w", err)
	}

	// 删除集合记录
	_, err = tx.Exec("DELETE FROM collections WHERE name = ?", name)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RenameCollection 重命名集合
func (s *Store) RenameCollection(oldName, newName string) error {
	// 检查旧集合是否存在
	_, err := s.GetCollection(oldName)
	if err != nil {
		return err
	}

	// 检查新名称是否已存在
	var exists int
	err = s.db.QueryRow("SELECT COUNT(*) FROM collections WHERE name = ?", newName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check new name: %w", err)
	}

	if exists > 0 {
		return fmt.Errorf("collection '%s' already exists", newName)
	}

	// 开始事务
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 更新集合名称
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`
		UPDATE collections
		SET name = ?, updated_at = ?
		WHERE name = ?
	`, newName, now, oldName)
	if err != nil {
		return fmt.Errorf("failed to update collection name: %w", err)
	}

	// 更新所有文档的collection字段
	_, err = tx.Exec("UPDATE documents SET collection = ? WHERE collection = ?", newName, oldName)
	if err != nil {
		return fmt.Errorf("failed to update documents: %w", err)
	}

	// 提交事务
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateCollectionTimestamp 更新集合的更新时间
func (s *Store) UpdateCollectionTimestamp(name string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`
		UPDATE collections
		SET updated_at = ?
		WHERE name = ?
	`, now, name)

	if err != nil {
		return fmt.Errorf("failed to update collection timestamp: %w", err)
	}

	return nil
}

// GetCollectionNames 获取所有集合名称列表
func (s *Store) GetCollectionNames() ([]string, error) {
	rows, err := s.db.Query("SELECT name FROM collections ORDER BY created_at DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to query collection names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}

	return names, nil
}

// CollectionExists 检查集合是否存在
func (s *Store) CollectionExists(name string) (bool, error) {
	var exists int
	err := s.db.QueryRow("SELECT COUNT(*) FROM collections WHERE name = ?", name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check collection: %w", err)
	}

	return exists > 0, nil
}
