package service

import (
	"context"
	"errors"
	"strings"

	"disk/internal/model"
	"disk/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("用户名或密码错误")

type UserFinder interface {
	FindByUsername(ctx context.Context, username string) (*model.User, error)
}

type TokenGenerator interface {
	GenerateToken(userID uint64) (string, error)
}

type LoginService struct {
	users  UserFinder
	tokens TokenGenerator
}

func NewLoginService(users UserFinder, tokens TokenGenerator) *LoginService {
	return &LoginService{
		users:  users,
		tokens: tokens,
	}
}

type LoginInput struct {
	Username string
	Password string
}

type LoginResult struct {
	AccessToken string
}

// Login 验证用户名和密码，并在认证成功后签发访问令牌。
func (s *LoginService) Login(ctx context.Context, input LoginInput) (*LoginResult, error) {
	username := strings.TrimSpace(input.Username)
	if username == "" || input.Password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.users.FindByUsername(ctx, username)
	if errors.Is(err, repository.ErrUserNotFound) {
		// 不向客户端区分“用户不存在”和“密码错误”，避免泄露已注册用户名。
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(input.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, err := s.tokens.GenerateToken(user.ID)
	if err != nil {
		return nil, err
	}

	return &LoginResult{AccessToken: accessToken}, nil
}
