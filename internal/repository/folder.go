package repository

import (
	"context"
	"errors"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrRootFolderAlreadyExists 表示当前用户已经拥有根目录，不能重复创建。
var ErrRootFolderAlreadyExists = errors.New("用户根目录已存在")

// RootFolderName 是系统为每个用户创建的根目录名称。
const RootFolderName = "/"

// FoldersRepository 负责文件夹数据的持久化操作。
type FoldersRepository struct {
	db *gorm.DB
}

// NewFoldersRepository 创建一个文件夹仓储实例。
func NewFoldersRepository(db *gorm.DB) *FoldersRepository {
	return &FoldersRepository{db: db}
}

// CreateFoldersByID 根据用户 ID 查询用户，并为该用户创建唯一的根目录。
// 用户查询、根目录查重和目录创建都在同一个事务中完成；任一步失败时，事务都会回滚。
func (f *FoldersRepository) CreateFoldersByID(ctx context.Context, userID uint64) (*model.Folder, error) {
	var folder model.Folder

	// 将请求上下文传入数据库操作，使客户端取消请求或超时时能够终止事务。
	err := f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		// 锁定用户记录，避免同一用户的多个并发请求同时创建根目录。
		// 这里只查询 id，因为创建根目录不需要用户名、密码等其他用户字段。
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

		// ParentID 为 NULL 的文件夹代表根目录；创建之前先确认该用户还没有根目录。
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

		// 根目录没有父目录，因此 ParentID 必须为 nil。
		folder = model.Folder{
			UserID:   user.ID,
			ParentID: nil,
			Name:     RootFolderName,
		}
		// 数据库唯一键冲突也统一转换成“根目录已存在”，避免向上层泄露数据库错误。
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

	// Create 成功后 GORM 会回填 folder.ID、CreatedAt 和 UpdatedAt。
	return &folder, nil
}
