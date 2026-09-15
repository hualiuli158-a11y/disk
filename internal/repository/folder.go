package repository

import (
	"context"
	"disk/internal/model"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var (
	// ErrRootFolderAlreadyExists 表示当前用户已经拥有根目录，不能重复创建。
	ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")
	ErrInvalidFolderName       = errors.New("文件夹名称不合法")
	ErrParentNotFound          = errors.New("父目录不存在")
	ErrFolderAlreadyExists     = errors.New("同名文件夹已存在")
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
