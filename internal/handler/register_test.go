package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/service"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type handlerUserRegister struct {
	existing bool
}

func (f *handlerUserRegister) FindByUsername(context.Context, string) (*model.User, error) {
	if f.existing {
		return &model.User{ID: 1, Username: "alice"}, nil
	}
	return nil, repository.ErrUserNotFound
}

func (f *handlerUserRegister) CreateRoot(_ context.Context, user *model.User) error {
	user.ID = 42
	return nil
}

type handlerTokenGenerator struct{}

func (handlerTokenGenerator) GenerateToken(uint64) (string, error) {
	return "access-token", nil
}

func TestRegisterHandlerStatusCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		users      *handlerUserRegister
		body       map[string]string
		wantStatus int
		wantCode   int
	}{
		{
			name:       "参数非法返回 400",
			users:      &handlerUserRegister{},
			body:       map[string]string{"username": "   ", "password": "password"},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "用户名重复返回 409",
			users:      &handlerUserRegister{existing: true},
			body:       map[string]string{"username": "alice", "password": "password"},
			wantStatus: http.StatusConflict,
			wantCode:   20003,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerService := service.NewRegisterService(tt.users, handlerTokenGenerator{})
			h := NewRegisterHandler(registerService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.POST("/register", h.Register)

			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("编码请求失败: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, req)

			if response.Code != tt.wantStatus {
				t.Fatalf("HTTP 状态码错误: got %d, want %d", response.Code, tt.wantStatus)
			}
			var payload struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}
			if payload.Code != tt.wantCode {
				t.Fatalf("业务错误码错误: got %d, want %d", payload.Code, tt.wantCode)
			}
		})
	}
}
