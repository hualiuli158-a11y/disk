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
