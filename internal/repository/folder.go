package repository

import (
	"context"
	"errors"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")

const RootFolderName = "/"

type FoldersRepository struct {
	db *gorm.DB
}

func NewFoldersRepository(db *gorm.DB) *FoldersRepository {
	return &FoldersRepository{db: db}
}

// CreateFoldersByID 查询指定用户，并为其创建根目录。
func (f *FoldersRepository) CreateFoldersByID(ctx context.Context, userID uint64) (*model.Folder, error) {
	var folder model.Folder

	err := f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		err := tx.
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id").
			Where("id = ?", userID).
			First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		if err != nil {
			return fmt.Errorf("find user %d: %w", userID, err)
		}

		var existing model.Folder
		err = tx.
			Select("id").
			Where("user_id = ? AND parent_id IS NULL", user.ID).
			First(&existing).Error
		switch {
		case err == nil:
			return ErrRootFolderAlreadyExists
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 当前用户还没有根目录，可以继续创建。
		default:
			return fmt.Errorf("find root folder for user %d: %w", user.ID, err)
		}

		folder = model.Folder{
			UserID:   user.ID,
			ParentID: nil,
			Name:     RootFolderName,
		}
		if err := tx.Create(&folder).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrRootFolderAlreadyExists
			}
			return fmt.Errorf("create root folder for user %d: %w", user.ID, err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &folder, nil
}
