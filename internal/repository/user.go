package repository

import (
	"context"
	"errors"
	"fmt"

	"disk/internal/model"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound          = errors.New("用户不存在")
	ErrUsernameAlreadyExists = errors.New("用户名已存在")
	ErrCreateUserFailed      = errors.New("create user failed")
)

// RootFolderName 是系统为每个用户创建的根目录名称。
const RootFolderName = "/"

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// 通过名称查询查询用户
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User

	err := r.db.WithContext(ctx).Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByID 根据主键查询用户。
func (r *UserRepository) FindByID(ctx context.Context, id uint64) (*model.User, error) {
	var user model.User

	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// 创建用户,创建用户并为用户创建根目录
func (r *UserRepository) CreateRoot(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// username 的唯一索引负责处理并发注册。
		if err := tx.Create(user).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrUsernameAlreadyExists
			}
			return fmt.Errorf("create user: %w", err)
		}

		root := model.Folder{
			UserID:   user.ID,
			ParentID: nil,
			Name:     RootFolderName,
		}

		if err := tx.Create(&root).Error; err != nil {
			return fmt.Errorf(
				"create root folder for user %d: %w",
				user.ID,
				err,
			)
		}

		return nil
	})
}
