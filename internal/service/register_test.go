package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"disk/internal/model"
	"disk/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

type fakeUserRegister struct {
	findUser    *model.User
	findErr     error
	createErr   error
	createdUser *model.User
	createdID   uint64
	findCalls   int
	createCalls int
}

func (f *fakeUserRegister) FindByUsername(context.Context, string) (*model.User, error) {
	f.findCalls++
	return f.findUser, f.findErr
}

func (f *fakeUserRegister) CreateRoot(_ context.Context, user *model.User) error {
	f.createCalls++
	f.createdUser = user
	if f.createErr == nil {
		user.ID = f.createdID
	}
	return f.createErr
}

func TestRegisterSuccess(t *testing.T) {
	users := &fakeUserRegister{
		findErr:   repository.ErrUserNotFound,
		createdID: 42,
	}
	tokens := &fakeTokenGenerator{token: "access-token"}
	register := NewRegisterService(users, tokens)

	result, err := register.Register(context.Background(), RegisterInput{
		Username: " alice ",
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("注册失败: %v", err)
	}
	if result.AccessToken != "access-token" {
		t.Fatalf("访问令牌不一致: got %q", result.AccessToken)
	}
	if users.createCalls != 1 {
		t.Fatalf("期望创建一次用户，实际创建 %d 次", users.createCalls)
	}
	if users.createdUser.Username != "alice" {
		t.Fatalf("用户名未正确去除空格: got %q", users.createdUser.Username)
	}
	if users.createdUser.Password == "correct-password" {
		t.Fatal("数据库中不应保存明文密码")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(users.createdUser.Password), []byte("correct-password")); err != nil {
		t.Fatalf("保存的密码哈希无效: %v", err)
	}
	if tokens.receivedID != 42 || tokens.generateCall != 1 {
		t.Fatalf("签发令牌参数错误: userID=%d, calls=%d", tokens.receivedID, tokens.generateCall)
	}
}

func TestRegisterRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input RegisterInput
	}{
		{name: "用户名为空", input: RegisterInput{Password: "password"}},
		{name: "密码为空", input: RegisterInput{Username: "alice"}},
		{name: "用户名过长", input: RegisterInput{Username: strings.Repeat("a", 101), Password: "password"}},
		{name: "密码超过 bcrypt 上限", input: RegisterInput{Username: "alice", Password: strings.Repeat("p", 73)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRegister{}
			tokens := &fakeTokenGenerator{}
			register := NewRegisterService(users, tokens)

			_, err := register.Register(context.Background(), tt.input)
			if !errors.Is(err, ErrInvalidRegisterInput) {
				t.Fatalf("期望 ErrInvalidRegisterInput，实际错误: %v", err)
			}
			if users.findCalls != 0 || users.createCalls != 0 || tokens.generateCall != 0 {
				t.Fatal("参数非法时不应访问数据库或签发令牌")
			}
		})
	}
}

func TestRegisterRejectsExistingUsername(t *testing.T) {
	users := &fakeUserRegister{findUser: &model.User{ID: 1, Username: "alice"}}
	tokens := &fakeTokenGenerator{}
	register := NewRegisterService(users, tokens)

	_, err := register.Register(context.Background(), RegisterInput{Username: "alice", Password: "password"})
	if !errors.Is(err, ErrUsernameAlreadyExists) {
		t.Fatalf("期望 ErrUsernameAlreadyExists，实际错误: %v", err)
	}
	if users.createCalls != 0 || tokens.generateCall != 0 {
		t.Fatal("用户名已存在时不应创建用户或签发令牌")
	}
}

func TestRegisterReturnsFindError(t *testing.T) {
	databaseErr := errors.New("database unavailable")
	users := &fakeUserRegister{findErr: databaseErr}
	register := NewRegisterService(users, &fakeTokenGenerator{})

	_, err := register.Register(context.Background(), RegisterInput{Username: "alice", Password: "password"})
	if !errors.Is(err, databaseErr) {
		t.Fatalf("期望数据库查询错误，实际错误: %v", err)
	}
	if users.createCalls != 0 {
		t.Fatal("查询失败时不应创建用户")
	}
}

func TestRegisterMapsDuplicateCreateError(t *testing.T) {
	users := &fakeUserRegister{
		findErr:   repository.ErrUserNotFound,
		createErr: repository.ErrUsernameAlreadyExists,
	}
	register := NewRegisterService(users, &fakeTokenGenerator{})

	_, err := register.Register(context.Background(), RegisterInput{Username: "alice", Password: "password"})
	if !errors.Is(err, ErrUsernameAlreadyExists) {
		t.Fatalf("期望 ErrUsernameAlreadyExists，实际错误: %v", err)
	}
}

func TestRegisterReturnsTokenGenerationError(t *testing.T) {
	tokenErr := errors.New("token generation failed")
	users := &fakeUserRegister{findErr: repository.ErrUserNotFound, createdID: 7}
	register := NewRegisterService(users, &fakeTokenGenerator{err: tokenErr})

	_, err := register.Register(context.Background(), RegisterInput{Username: "alice", Password: "password"})
	if !errors.Is(err, tokenErr) {
		t.Fatalf("期望令牌生成错误，实际错误: %v", err)
	}
}
