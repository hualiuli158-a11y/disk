package service

import (
	"context"
	"errors"
	"testing"

	"disk/internal/model"
	"disk/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type fakeUserFinder struct {
	user *model.User
	err  error
}

func (f *fakeUserFinder) FindByUsername(context.Context, string) (*model.User, error) {
	return f.user, f.err
}

type fakeTokenGenerator struct {
	token        string
	err          error
	receivedID   uint64
	generateCall int
}

func (f *fakeTokenGenerator) GenerateToken(userID uint64) (string, error) {
	f.receivedID = userID
	f.generateCall++
	return f.token, f.err
}

func TestLoginSuccess(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}

	users := &fakeUserFinder{user: &model.User{
		ID:       42,
		Username: "alice",
		Password: string(passwordHash),
	}}
	tokens := &fakeTokenGenerator{token: "access-token"}
	login := NewLoginService(users, tokens)

	result, err := login.Login(context.Background(), LoginInput{
		Username: " alice ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("登录失败: %v", err)
	}
	if result.AccessToken != "access-token" {
		t.Fatalf("访问令牌不一致: got %q", result.AccessToken)
	}
	if tokens.receivedID != 42 || tokens.generateCall != 1 {
		t.Fatalf("签发令牌参数错误: userID=%d, calls=%d", tokens.receivedID, tokens.generateCall)
	}
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}

	tests := []struct {
		name  string
		users UserFinder
	}{
		{
			name:  "用户不存在",
			users: &fakeUserFinder{err: repository.ErrUserNotFound},
		},
		{
			name: "密码错误",
			users: &fakeUserFinder{user: &model.User{
				ID:       42,
				Username: "alice",
				Password: string(passwordHash),
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := &fakeTokenGenerator{token: "should-not-be-created"}
			login := NewLoginService(tt.users, tokens)

			_, err := login.Login(context.Background(), LoginInput{
				Username: "alice",
				Password: "wrong-password",
			})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("期望 ErrInvalidCredentials，实际错误: %v", err)
			}
			if tokens.generateCall != 0 {
				t.Fatalf("认证失败时不应签发令牌")
			}
		})
	}
}

func TestLoginReturnsTokenGenerationError(t *testing.T) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	tokenErr := errors.New("签发令牌失败")
	login := NewLoginService(
		&fakeUserFinder{user: &model.User{ID: 7, Password: string(passwordHash)}},
		&fakeTokenGenerator{err: tokenErr},
	)

	_, err = login.Login(context.Background(), LoginInput{
		Username: "alice",
		Password: "password",
	})
	if !errors.Is(err, tokenErr) {
		t.Fatalf("期望签发令牌错误，实际错误: %v", err)
	}
}
