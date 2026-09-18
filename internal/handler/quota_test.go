package handler

import (
	"context"
	"encoding/json"
	"errors"
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

type handlerQuotaUserFinder struct {
	user *model.User
	err  error
}

func (f *handlerQuotaUserFinder) FindByID(context.Context, uint64) (*model.User, error) {
	return f.user, f.err
}

func TestQuotaHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")
	tests := []struct {
		name       string
		userID     any
		finder     *handlerQuotaUserFinder
		wantStatus int
		wantCode   int
	}{
		{
			name:       "success",
			userID:     uint64(42),
			finder:     &handlerQuotaUserFinder{user: &model.User{ID: 42, UsedQuota: 30, QuotaLimit: 100}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			finder:     &handlerQuotaUserFinder{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "user not found",
			userID:     uint64(42),
			finder:     &handlerQuotaUserFinder{err: repository.ErrUserNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   20001,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			finder:     &handlerQuotaUserFinder{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quotaService := service.NewQuotaService(tt.finder)
			h := NewQuotaHandler(quotaService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/quota", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.GetQuota(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/quota", nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("HTTP status = %d, want %d", response.Code, tt.wantStatus)
			}
			if tt.wantCode != 0 {
				var payload struct {
					Code int `json:"code"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode error response: %v", err)
				}
				if payload.Code != tt.wantCode {
					t.Fatalf("error code = %d, want %d", payload.Code, tt.wantCode)
				}
				return
			}

			var payload QuotaResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode quota response: %v", err)
			}
			if payload.Used != 30 || payload.Limit != 100 || payload.Remaining != 70 {
				t.Fatalf("unexpected quota response: %+v", payload)
			}
		})
	}
}
