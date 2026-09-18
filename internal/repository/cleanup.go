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

func NewCleanupRepository(db *gorm.DB) *CleanupRepository {
	return &CleanupRepository{db: db}
}

func (r *CleanupRepository) ListUnreferencedPhysicalFiles(ctx context.Context, olderThan time.Time, limit int) ([]model.PhysicalFile, error) {
	files := make([]model.PhysicalFile, 0)
	err := r.db.WithContext(ctx).
		Where("created_at < ?", olderThan).
		Where("NOT EXISTS (SELECT 1 FROM files WHERE files.physical_file_id = physical_files.id)").
		Order("id ASC").
		Limit(limit).
		Find(&files).Error
	if err != nil {
		return nil, fmt.Errorf("list unreferenced physical files: %w", err)
	}
	return files, nil
}

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
		if err := tx.Delete(&physical).Error; err != nil {
			return fmt.Errorf("delete orphan physical file %d: %w", physicalID, err)
		}
		storageKey = physical.StorageKey
		deleted = true
		return nil
	})
	return storageKey, deleted, err
}

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
