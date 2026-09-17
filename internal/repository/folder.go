package repository

import (
	"context"
	"errors"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
)

var (
	ErrParentNotFound      = errors.New("父目录不存在")
	ErrFolderAlreadyExists = errors.New("同名文件夹已存在")
	ErrRootFolderNotFound  = errors.New("根目录不存在")
	ErrFolderNotFound      = errors.New("文件夹不存在")
)

// FoldersRepository 负责文件夹数据的持久化操作。
type FoldersRepository struct {
	db *gorm.DB
}

func NewFoldersRepository(db *gorm.DB) *FoldersRepository {
	return &FoldersRepository{db: db}
}

func (r *FoldersRepository) CreateChildFolder(ctx context.Context, userID uint64, parentID uint64, name string) (*model.Folder, error) {
	var child model.Folder

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent model.Folder

		err := tx.
			Select("id").
			Where("id = ? AND user_id = ?", parentID, userID).
			First(&parent).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return ErrParentNotFound
		case err != nil:
			return fmt.Errorf("find parent folder: %w", err)
		}

		child = model.Folder{
			UserID:   userID,
			ParentID: &parent.ID,
			Name:     name,
		}

		if err := tx.Create(&child).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrFolderAlreadyExists
			}
			return fmt.Errorf("create child folder: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &child, nil
}

// ListRootChildren 查询用户根目录及其直接子目录，不递归加载更深层级。
func (r *FoldersRepository) ListRootChildren(ctx context.Context, userID uint64) (*model.Folder, error) {
	var root model.Folder
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id IS NULL", userID).
		First(&root).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrRootFolderNotFound
	case err != nil:
		return nil, fmt.Errorf("find root folder: %w", err)
	}

	children := make([]model.Folder, 0)
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND parent_id = ?", userID, root.ID).
		Order("created_at ASC, id ASC").
		Find(&children).Error; err != nil {
		return nil, fmt.Errorf("find root children: %w", err)
	}

	root.Children = children
	return &root, nil
}

func (r *FoldersRepository) RenameFolder(ctx context.Context, userID uint64, folderID uint64, name string) error {
	result := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Where("id = ? AND user_id = ? AND parent_id IS NOT NULL", folderID, userID).
		Update("name", name)

	if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
		return ErrFolderAlreadyExists
	}
	if result.Error != nil {
		return fmt.Errorf("rename folder %d: %w", folderID, result.Error)
	}
	if result.RowsAffected == 0 {
		// MySQL 在名称没有发生变化时可能报告 0 行受影响；此时再次确认目录是否存在，
		// 使重复提交相同名称保持幂等。
		var count int64
		if err := r.db.WithContext(ctx).
			Model(&model.Folder{}).
			Where("id = ? AND user_id = ? AND parent_id IS NOT NULL", folderID, userID).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check folder %d after rename: %w", folderID, err)
		}
		if count == 0 {
			return ErrFolderNotFound
		}
	}

	return nil
}
