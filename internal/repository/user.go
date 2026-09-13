package repository

import (
	"context"
	"errors"

	"disk/internal/model"

	"gorm.io/gorm"
)

var (
	ErrUserNotFound          = errors.New("用户不存在")
	ErrUsernameAlreadyExists = errors.New("用户名已存在")
)

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

// 创建用户
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrUsernameAlreadyExists
	}
	return err
}
