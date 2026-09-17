package handler

import (
	"bytes"
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

type handlerRootChildrenRepository struct {
	root *model.Folder
	err  error
}

type handlerRenameFolderRepository struct {
	err              error
	receivedUserID   uint64
	receivedFolderID uint64
	receivedName     string
	called           bool
}

func (f *handlerRenameFolderRepository) RenameFolder(_ context.Context, userID uint64, folderID uint64, name string) error {
	f.called = true
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	f.receivedName = name
	return f.err
}

func (f *handlerRootChildrenRepository) ListRootChildren(context.Context, uint64) (*model.Folder, error) {
	return f.root, f.err
}

func TestListRootChildrenHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 15, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	parentID := uint64(1)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		repository *handlerRootChildrenRepository
		wantStatus int
		wantCode   int
		wantChild  bool
	}{
		{
			name:   "success",
			userID: uint64(42),
			repository: &handlerRootChildrenRepository{root: &model.Folder{
				ID: 1, UserID: 42, Name: "/", CreatedAt: createdAt,
				Children: []model.Folder{{
					ID: 2, UserID: 42, ParentID: &parentID, Name: "旅行照片", CreatedAt: createdAt,
				}},
			}},
			wantStatus: http.StatusOK,
			wantChild:  true,
		},
		{
			name:       "empty children",
			userID:     uint64(42),
			repository: &handlerRootChildrenRepository{root: &model.Folder{ID: 1, UserID: 42, Name: "/"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			repository: &handlerRootChildrenRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid user ID type",
			userID:     "42",
			repository: &handlerRootChildrenRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "root folder not found",
			userID:     uint64(42),
			repository: &handlerRootChildrenRepository{err: repository.ErrRootFolderNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   30005,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			repository: &handlerRootChildrenRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   30006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foldersService := service.NewListFoldersService(tt.repository)
			h := NewListFoldersHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/folders/root/children", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.ListFolders(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/folders/root/children", nil)
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

			var payload ListFoldersResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode list response: %v", err)
			}
			if payload.Root.ID != 1 || payload.Root.Name != "/" {
				t.Fatalf("unexpected root response: %+v", payload.Root)
			}
			if payload.Children == nil {
				t.Fatal("children must be an empty array instead of null")
			}
			if tt.wantChild {
				if len(payload.Children) != 1 || payload.Children[0].ID != 2 || payload.Children[0].Name != "旅行照片" {
					t.Fatalf("unexpected children response: %+v", payload.Children)
				}
			} else if len(payload.Children) != 0 {
				t.Fatalf("children length = %d, want 0", len(payload.Children))
			}
		})
	}
}

func TestRenameFolderHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		userID        any
		folderID      string
		body          string
		repositoryErr error
		wantStatus    int
		wantCode      int
		wantName      string
		wantRepoCall  bool
	}{
		{
			name:         "success trims name",
			userID:       uint64(42),
			folderID:     "7",
			body:         `{"name":" 旅行照片 "}`,
			wantStatus:   http.StatusOK,
			wantName:     "旅行照片",
			wantRepoCall: true,
		},
		{
			name:       "missing user ID",
			folderID:   "7",
			body:       `{"name":"旅行照片"}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid folder ID",
			userID:     uint64(42),
			folderID:   "invalid",
			body:       `{"name":"旅行照片"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "invalid folder name",
			userID:     uint64(42),
			folderID:   "7",
			body:       `{"name":"../照片"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   30001,
		},
		{
			name:          "folder not found",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"name":"旅行照片"}`,
			repositoryErr: repository.ErrFolderNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      30007,
			wantRepoCall:  true,
		},
		{
			name:          "duplicate folder name",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"name":"旅行照片"}`,
			repositoryErr: repository.ErrFolderAlreadyExists,
			wantStatus:    http.StatusConflict,
			wantCode:      30003,
			wantRepoCall:  true,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"name":"旅行照片"}`,
			repositoryErr: databaseErr,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      30008,
			wantRepoCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerRenameFolderRepository{err: tt.repositoryErr}
			foldersService := service.NewRenameFolderService(repo)
			h := NewRenameFolderHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.PATCH("/api/folders/:id", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.RenameFolder(c)
			})

			request := httptest.NewRequest(
				http.MethodPatch,
				"/api/folders/"+tt.folderID,
				bytes.NewBufferString(tt.body),
			)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("HTTP status = %d, want %d", response.Code, tt.wantStatus)
			}
			if repo.called != tt.wantRepoCall {
				t.Fatalf("repository called = %v, want %v", repo.called, tt.wantRepoCall)
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

			var payload RenameFolderResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode rename response: %v", err)
			}
			if payload.ID != 7 || payload.Name != tt.wantName {
				t.Fatalf("unexpected rename response: %+v", payload)
			}
			if repo.receivedUserID != 42 || repo.receivedFolderID != 7 || repo.receivedName != tt.wantName {
				t.Fatalf("unexpected repository input: user=%d folder=%d name=%q", repo.receivedUserID, repo.receivedFolderID, repo.receivedName)
			}
		})
	}
}
