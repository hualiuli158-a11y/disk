package service

import (
	"context"
	"errors"
	"strings"

	"disk/internal/model"
	"disk/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidRegisterInput  = errors.New("注册参数不合法")
	ErrUsernameAlreadyExists = errors.New("用户名已存在")
)

type UserRegister interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
}

type RegisterService struct {
	users  UserRegister
	tokens TokenGenerator
}

type RegisterInput struct {
	Username string
	Password string
}

type RegisterResult struct {
	AccessToken string
}

func NewRegisterService(users UserRegister, tokens TokenGenerator) *RegisterService {
	return &RegisterService{users: users, tokens: tokens}
}

// Register 创建用户，并在创建成功后签发访问令牌。
func (s *RegisterService) Register(ctx context.Context, input RegisterInput) (*RegisterResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" || len(username) > 100 || len(input.Password) > 72 {
		return nil, ErrInvalidRegisterInput
	}

	_, err := s.users.FindByUsername(ctx, username)
	switch {
	case err == nil:
		return nil, ErrUsernameAlreadyExists
	case errors.Is(err, repository.ErrUserNotFound):
		// 用户名尚未注册，可以继续创建用户。
	default:
		return nil, err
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Username: username,
		Password: string(passwordHash),
	}
	if err := s.users.Create(ctx, user); err != nil {
		if errors.Is(err, repository.ErrUsernameAlreadyExists) {
			return nil, ErrUsernameAlreadyExists
		}
		return nil, err
	}

	accessToken, err := s.tokens.GenerateToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &RegisterResult{AccessToken: accessToken}, nil
}
