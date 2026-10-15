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

type handlerTrashRepository struct {
	entries       []model.TrashEntry
	total         int64
	listErr       error
	restoreErr    error
	deleteErr     error
	storageKeys   []string
	receivedUser  uint64
	receivedTrash uint64
}

func (f *handlerTrashRepository) ListTrash(_ context.Context, userID uint64, _ int, _ int) ([]model.TrashEntry, int64, error) {
	f.receivedUser = userID
	return f.entries, f.total, f.listErr
}

func (f *handlerTrashRepository) ListExpiredTrashEntries(context.Context, time.Time, int) ([]model.TrashEntry, error) {
	return []model.TrashEntry{}, nil
}

func (f *handlerTrashRepository) RestoreTrash(_ context.Context, userID uint64, trashID uint64) error {
	f.receivedUser = userID
	f.receivedTrash = trashID
	return f.restoreErr
}

func (f *handlerTrashRepository) PermanentlyDeleteTrash(_ context.Context, userID uint64, trashID uint64) ([]string, error) {
	f.receivedUser = userID
	f.receivedTrash = trashID
	return f.storageKeys, f.deleteErr
}

func TestListTrashHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	databaseErr := errors.New("database unavailable")
	tests := []struct {
		name       string
		userID     any
		query      string
		repository *handlerTrashRepository
		wantStatus int
		wantCode   int
		wantItems  int
	}{
		{
			name:   "success",
			userID: uint64(42),
			query:  "?page=1&page_size=10",
			repository: &handlerTrashRepository{
				entries: []model.TrashEntry{{
					ID: 1, UserID: 42, ResourceType: model.TrashResourceFile,
					ResourceID: 2, Name: "photo.jpg", TrashedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour),
				}},
				total: 1,
			},
			wantStatus: http.StatusOK,
			wantItems:  1,
		},
		{
			name:       "missing user ID",
			repository: &handlerTrashRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			repository: &handlerTrashRepository{listErr: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   50003,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trashService := service.NewTrashService(tt.repository, &handlerObjectStorage{})
			h := NewTrashHandler(trashService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/trash", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.ListTrash(c)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/trash"+tt.query, nil))
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
					t.Fatalf("code = %d, want %d", payload.Code, tt.wantCode)
				}
				return
			}
			var payload ListTrashResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if payload.Items == nil || len(payload.Items) != tt.wantItems {
				t.Fatalf("unexpected response: %+v", payload)
			}
		})
	}
}

func TestRestoreTrashHandler(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   int
	}{
		{name: "success", wantStatus: http.StatusNoContent},
		{name: "not found", err: repository.ErrTrashNotFound, wantStatus: http.StatusNotFound, wantCode: 50001},
		{name: "name conflict", err: repository.ErrTrashRestoreConflict, wantStatus: http.StatusConflict, wantCode: 50002},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerTrashRepository{restoreErr: tt.err}
			h := NewTrashHandler(service.NewTrashService(repo, &handlerObjectStorage{}), logger.NewLogger(io.Discard))
			router := gin.New()
			router.POST("/api/trash/:id/restore", func(c *gin.Context) {
				c.Set("user_id", uint64(42))
				h.RestoreTrash(c)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/trash/7/restore", nil))
			if response.Code != tt.wantStatus {
				t.Fatalf("HTTP status = %d, want %d", response.Code, tt.wantStatus)
			}
			if tt.wantCode != 0 {
				var payload struct {
					Code int `json:"code"`
				}
				_ = json.Unmarshal(response.Body.Bytes(), &payload)
				if payload.Code != tt.wantCode {
					t.Fatalf("code = %d, want %d", payload.Code, tt.wantCode)
				}
			}
		})
	}
}

func TestPermanentlyDeleteTrashHandler(t *testing.T) {
	repo := &handlerTrashRepository{storageKeys: []string{"objects/aa/value"}}
	objects := &handlerObjectStorage{}
	h := NewTrashHandler(service.NewTrashService(repo, objects), logger.NewLogger(io.Discard))
	router := gin.New()
	router.DELETE("/api/trash/:id", func(c *gin.Context) {
		c.Set("user_id", uint64(42))
		h.PermanentlyDeleteTrash(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/trash/7", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("HTTP status = %d, want 204", response.Code)
	}
	if repo.receivedUser != 42 || repo.receivedTrash != 7 || len(objects.deletedKeys) != 1 {
		t.Fatalf("unexpected permanent delete state: repo=%+v deleted=%v", repo, objects.deletedKeys)
	}
}
