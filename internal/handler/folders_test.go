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

type handlerFoldersRepository struct {
	folder *model.Folder
	err    error
}

func (f *handlerFoldersRepository) CreateFoldersByID(context.Context, uint64) (*model.Folder, error) {
	return f.folder, f.err
}

func TestCreateRootFolderHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 13, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		repository *handlerFoldersRepository
		wantStatus int
		wantCode   int
	}{
		{
			name:   "success",
			userID: uint64(42),
			repository: &handlerFoldersRepository{folder: &model.Folder{
				ID: 1, UserID: 42, Name: "/", CreatedAt: createdAt,
			}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			repository: &handlerFoldersRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid user ID type",
			userID:     "42",
			repository: &handlerFoldersRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "user not found",
			userID:     uint64(42),
			repository: &handlerFoldersRepository{err: repository.ErrUserNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   20001,
		},
		{
			name:       "root folder already exists",
			userID:     uint64(42),
			repository: &handlerFoldersRepository{err: repository.ErrRootFolderAlreadyExists},
			wantStatus: http.StatusConflict,
			wantCode:   30003,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			repository: &handlerFoldersRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   30004,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foldersService := service.NewFoldersCreateService(tt.repository)
			h := NewFoldersHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.POST("/api/folders/root", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.CreateFolder(c)
			})

			request := httptest.NewRequest(http.MethodPost, "/api/folders/root", nil)
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

			var payload CreateRootFolderResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode folder response: %v", err)
			}
			if payload.ID != 1 || payload.UserID != 42 || payload.Name != "/" || !payload.CreatedAt.Equal(createdAt) {
				t.Fatalf("unexpected folder response: %+v", payload)
			}
		})
	}
}
