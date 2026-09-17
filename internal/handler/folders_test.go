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

type handlerFolderChildrenRepository struct {
	folder           *model.Folder
	err              error
	receivedUserID   uint64
	receivedFolderID uint64
}

type handlerDeleteFolderRepository struct {
	err              error
	called           bool
	receivedUserID   uint64
	receivedFolderID uint64
}

type handlerMoveFolderRepository struct {
	err                 error
	called              bool
	receivedUserID      uint64
	receivedFolderID    uint64
	receivedNewParentID uint64
}

func (f *handlerMoveFolderRepository) MoveFolder(_ context.Context, userID uint64, folderID uint64, newParentID uint64) error {
	f.called = true
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	f.receivedNewParentID = newParentID
	return f.err
}

func (f *handlerDeleteFolderRepository) DeleteEmptyFolder(_ context.Context, userID uint64, folderID uint64) error {
	f.called = true
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	return f.err
}

func (f *handlerFolderChildrenRepository) ListChildren(_ context.Context, userID uint64, folderID uint64) (*model.Folder, error) {
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	return f.folder, f.err
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

func TestListFolderChildrenHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	parentID := uint64(1)
	childParentID := uint64(7)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		folderID   string
		repository *handlerFolderChildrenRepository
		wantStatus int
		wantCode   int
		wantChild  bool
	}{
		{
			name:     "success",
			userID:   uint64(42),
			folderID: "7",
			repository: &handlerFolderChildrenRepository{folder: &model.Folder{
				ID: 7, UserID: 42, ParentID: &parentID, Name: "照片", CreatedAt: createdAt,
				Children: []model.Folder{{
					ID: 8, UserID: 42, ParentID: &childParentID, Name: "旅行", CreatedAt: createdAt,
				}},
			}},
			wantStatus: http.StatusOK,
			wantChild:  true,
		},
		{
			name:       "empty children",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerFolderChildrenRepository{folder: &model.Folder{ID: 7, UserID: 42, ParentID: &parentID, Name: "照片"}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			folderID:   "7",
			repository: &handlerFolderChildrenRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid folder ID",
			userID:     uint64(42),
			folderID:   "invalid",
			repository: &handlerFolderChildrenRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "folder not found",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerFolderChildrenRepository{err: repository.ErrFolderNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   30007,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerFolderChildrenRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   30006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			foldersService := service.NewListFolderChildrenService(tt.repository)
			h := NewListFolderChildrenHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/folders/:id/children", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.ListChildren(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/folders/"+tt.folderID+"/children", nil)
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

			var payload ListFolderChildrenResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode children response: %v", err)
			}
			if payload.Folder.ID != 7 || payload.Folder.Name != "照片" {
				t.Fatalf("unexpected folder response: %+v", payload.Folder)
			}
			if payload.Children == nil {
				t.Fatal("children must be an empty array instead of null")
			}
			if tt.repository.receivedUserID != 42 || tt.repository.receivedFolderID != 7 {
				t.Fatalf("unexpected repository input: user=%d folder=%d", tt.repository.receivedUserID, tt.repository.receivedFolderID)
			}
			if tt.wantChild {
				if len(payload.Children) != 1 || payload.Children[0].ID != 8 || payload.Children[0].Name != "旅行" {
					t.Fatalf("unexpected children response: %+v", payload.Children)
				}
			} else if len(payload.Children) != 0 {
				t.Fatalf("children length = %d, want 0", len(payload.Children))
			}
		})
	}
}

func TestDeleteFolderHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		userID        any
		folderID      string
		repositoryErr error
		wantStatus    int
		wantCode      int
		wantRepoCall  bool
	}{
		{
			name:         "success",
			userID:       uint64(42),
			folderID:     "7",
			wantStatus:   http.StatusNoContent,
			wantRepoCall: true,
		},
		{
			name:       "missing user ID",
			folderID:   "7",
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid folder ID",
			userID:     uint64(42),
			folderID:   "invalid",
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:          "folder not found",
			userID:        uint64(42),
			folderID:      "7",
			repositoryErr: repository.ErrFolderNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      30007,
			wantRepoCall:  true,
		},
		{
			name:          "cannot delete root",
			userID:        uint64(42),
			folderID:      "1",
			repositoryErr: repository.ErrCannotDeleteRoot,
			wantStatus:    http.StatusConflict,
			wantCode:      30009,
			wantRepoCall:  true,
		},
		{
			name:          "folder not empty",
			userID:        uint64(42),
			folderID:      "7",
			repositoryErr: repository.ErrFolderNotEmpty,
			wantStatus:    http.StatusConflict,
			wantCode:      30010,
			wantRepoCall:  true,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			folderID:      "7",
			repositoryErr: databaseErr,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      30011,
			wantRepoCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerDeleteFolderRepository{err: tt.repositoryErr}
			foldersService := service.NewDeleteFolderService(repo)
			h := NewDeleteFolderHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.DELETE("/api/folders/:id", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.DeleteFolder(c)
			})

			request := httptest.NewRequest(http.MethodDelete, "/api/folders/"+tt.folderID, nil)
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

			if response.Body.Len() != 0 {
				t.Fatalf("204 response body must be empty, got %q", response.Body.String())
			}
			if repo.receivedUserID != 42 || repo.receivedFolderID != 7 {
				t.Fatalf("unexpected repository input: user=%d folder=%d", repo.receivedUserID, repo.receivedFolderID)
			}
		})
	}
}

func TestMoveFolderHandler(t *testing.T) {
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
		wantRepoCall  bool
	}{
		{
			name:         "success",
			userID:       uint64(42),
			folderID:     "7",
			body:         `{"parent_id":10}`,
			wantStatus:   http.StatusOK,
			wantRepoCall: true,
		},
		{
			name:       "missing user ID",
			folderID:   "7",
			body:       `{"parent_id":10}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid folder ID",
			userID:     uint64(42),
			folderID:   "invalid",
			body:       `{"parent_id":10}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "missing parent ID",
			userID:     uint64(42),
			folderID:   "7",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:          "folder not found",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"parent_id":10}`,
			repositoryErr: repository.ErrFolderNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      30007,
			wantRepoCall:  true,
		},
		{
			name:          "cannot move root",
			userID:        uint64(42),
			folderID:      "1",
			body:          `{"parent_id":10}`,
			repositoryErr: repository.ErrCannotMoveRoot,
			wantStatus:    http.StatusConflict,
			wantCode:      30012,
			wantRepoCall:  true,
		},
		{
			name:          "invalid folder move",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"parent_id":8}`,
			repositoryErr: repository.ErrInvalidFolderMove,
			wantStatus:    http.StatusConflict,
			wantCode:      30013,
			wantRepoCall:  true,
		},
		{
			name:          "duplicate folder name",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"parent_id":10}`,
			repositoryErr: repository.ErrFolderAlreadyExists,
			wantStatus:    http.StatusConflict,
			wantCode:      30003,
			wantRepoCall:  true,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			folderID:      "7",
			body:          `{"parent_id":10}`,
			repositoryErr: databaseErr,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      30014,
			wantRepoCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerMoveFolderRepository{err: tt.repositoryErr}
			foldersService := service.NewMoveFolderService(repo)
			h := NewMoveFolderHandler(foldersService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.PATCH("/api/folders/:id/move", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.MoveFolder(c)
			})

			request := httptest.NewRequest(
				http.MethodPatch,
				"/api/folders/"+tt.folderID+"/move",
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

			var payload MoveFolderResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode move response: %v", err)
			}
			if payload.ID != 7 || payload.ParentID != 10 {
				t.Fatalf("unexpected move response: %+v", payload)
			}
			if repo.receivedUserID != 42 || repo.receivedFolderID != 7 || repo.receivedNewParentID != 10 {
				t.Fatalf(
					"unexpected repository input: user=%d folder=%d parent=%d",
					repo.receivedUserID,
					repo.receivedFolderID,
					repo.receivedNewParentID,
				)
			}
		})
	}
}
