package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/service"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type handlerProfileUserFinder struct {
	user *model.User
	err  error
}

func (f *handlerProfileUserFinder) FindByID(context.Context, uint64) (*model.User, error) {
	return f.user, f.err
}

func TestProfileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	databaseError := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		finder     *handlerProfileUserFinder
		wantStatus int
		wantCode   int
	}{
		{
			name:   "success",
			userID: uint64(42),
			finder: &handlerProfileUserFinder{user: &model.User{
				ID: 42, Username: "alice", CreateTime: createdAt, UsedQuota: 1048576, QuotaLimit: 10737418240,
			}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			finder:     &handlerProfileUserFinder{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid user ID type",
			userID:     "42",
			finder:     &handlerProfileUserFinder{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "user not found",
			userID:     uint64(42),
			finder:     &handlerProfileUserFinder{err: repository.ErrUserNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   20001,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			finder:     &handlerProfileUserFinder{err: databaseError},
			wantStatus: http.StatusInternalServerError,
			wantCode:   1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profileService := service.NewProfileService(tt.finder)
			h := NewProfileHandler(profileService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/profile", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.Profile(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
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

			var payload ProfileResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode profile response: %v", err)
			}
			if payload.ID != 42 || payload.Username != "alice" || payload.UsedQuota != 1048576 || payload.QuotaLimit != 10737418240 || !payload.CreateTime.Equal(createdAt) {
				t.Fatalf("unexpected profile response: %+v", payload)
			}
		})
	}
}
