package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CleanupRepository struct {
	db *gorm.DB
}

// NewCleanupRepository 使用指定数据库连接创建物理文件清理仓储。
func NewCleanupRepository(db *gorm.DB) *CleanupRepository {
	return &CleanupRepository{db: db}
}

// ListUnreferencedPhysicalFiles 查询创建时间早于指定阈值且未被逻辑文件引用的物理文件。
func (r *CleanupRepository) ListUnreferencedPhysicalFiles(ctx context.Context, olderThan time.Time, limit int) ([]model.PhysicalFile, error) {
	files := make([]model.PhysicalFile, 0)
	err := r.db.WithContext(ctx).
		Where("created_at < ?", olderThan).
		Where("NOT EXISTS (SELECT 1 FROM files WHERE files.physical_file_id = physical_files.id)").
		Where("NOT EXISTS (SELECT 1 FROM file_versions WHERE file_versions.physical_file_id = physical_files.id)").
		Order("id ASC").
		Limit(limit).
		Find(&files).Error
	if err != nil {
		return nil, fmt.Errorf("list unreferenced physical files: %w", err)
	}
	return files, nil
}

// DeletePhysicalFileIfUnreferenced 在事务中再次确认物理文件无引用后删除其记录。
// 返回存储键、是否实际删除以及执行过程中产生的错误。
func (r *CleanupRepository) DeletePhysicalFileIfUnreferenced(ctx context.Context, physicalID uint64) (string, bool, error) {
	var storageKey string
	deleted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var physical model.PhysicalFile
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", physicalID).
			First(&physical).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("lock physical file %d: %w", physicalID, err)
		}

		var references int64
		if err := tx.Unscoped().Model(&model.File{}).
			Where("physical_file_id = ?", physicalID).
			Count(&references).Error; err != nil {
			return fmt.Errorf("count references to physical file %d: %w", physicalID, err)
		}
		if references > 0 {
			return nil
		}
		if err := tx.Model(&model.FileVersion{}).
			Where("physical_file_id = ?", physicalID).
			Count(&references).Error; err != nil {
			return fmt.Errorf("count version references to physical file %d: %w", physicalID, err)
		}
		if references > 0 {
			return nil
		}
		if err := tx.Delete(&physical).Error; err != nil {
			return fmt.Errorf("delete orphan physical file %d: %w", physicalID, err)
		}
		storageKey = physical.StorageKey
		deleted = true
		return nil
	})
	return storageKey, deleted, err
}

// PhysicalStorageKeyExists 判断指定存储键是否仍被物理文件记录使用。
func (r *CleanupRepository) PhysicalStorageKeyExists(ctx context.Context, storageKey string) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.PhysicalFile{}).
		Where("storage_key = ?", storageKey).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check physical storage key %q: %w", storageKey, err)
	}
	return count > 0, nil
}
