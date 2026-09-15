package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrRootFolderAlreadyExists 表示当前用户已经拥有根目录，不能重复创建。
var ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")

// FoldersRepository 负责文件夹数据的持久化操作。
type FoldersRepository struct {
	db *gorm.DB
}

// NewFoldersRepository 创建一个文件夹仓储实例。
func NewFoldersRepository(db *gorm.DB) *FoldersRepository {
	return &FoldersRepository{db: db}
}

func (r *FoldersRepository) CreateChildFolders(ctx context.Context) error {
	return nil
}
