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
	"disk/internal/service"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type handlerSearchRepository struct {
	items             []model.ContentItem
	total             int64
	err               error
	receivedUserID    uint64
	receivedQuery     string
	receivedKind      string
	receivedOffset    int
	receivedLimit     int
	receivedSortField string
	receivedSortOrder string
}

func (f *handlerSearchRepository) Search(
	_ context.Context,
	userID uint64,
	query string,
	kind string,
	offset int,
	limit int,
	sortField string,
	sortOrder string,
) ([]model.ContentItem, int64, error) {
	f.receivedUserID = userID
	f.receivedQuery = query
	f.receivedKind = kind
	f.receivedOffset = offset
	f.receivedLimit = limit
	f.receivedSortField = sortField
	f.receivedSortOrder = sortOrder
	return f.items, f.total, f.err
}

func TestSearchHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 18, 12, 0, 0, 0, time.UTC)
	parentID := uint64(7)
	size := int64(5)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		query      string
		repository *handlerSearchRepository
		wantStatus int
		wantCode   int
		wantItems  int
	}{
		{
			name:   "success",
			userID: uint64(42),
			query:  "?q=photo&type=file&page=2&page_size=2&sort=created_at&order=desc",
			repository: &handlerSearchRepository{
				items: []model.ContentItem{{
					ID: 2, Kind: "file", ParentID: &parentID, Name: "photo.jpg", Size: &size,
					CreatedAt: createdAt, UpdatedAt: createdAt,
				}},
				total: 3,
			},
			wantStatus: http.StatusOK,
			wantItems:  1,
		},
		{
			name:       "empty results",
			userID:     uint64(42),
			query:      "?q=missing",
			repository: &handlerSearchRepository{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			query:      "?q=photo",
			repository: &handlerSearchRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "missing query",
			userID:     uint64(42),
			repository: &handlerSearchRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "invalid type",
			userID:     uint64(42),
			query:      "?q=photo&type=physical",
			repository: &handlerSearchRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			query:      "?q=photo",
			repository: &handlerSearchRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   40013,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchService := service.NewSearchService(tt.repository)
			h := NewSearchHandler(searchService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/search", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.Search(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/search"+tt.query, nil)
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

			var payload SearchResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode search response: %v", err)
			}
			if payload.Items == nil || len(payload.Items) != tt.wantItems {
				t.Fatalf("unexpected search response: %+v", payload)
			}
			if tt.name == "success" {
				if payload.Page != 2 || payload.PageSize != 2 || payload.Total != 3 || payload.TotalPages != 2 || payload.Type != "file" {
					t.Fatalf("unexpected pagination: %+v", payload)
				}
				if tt.repository.receivedUserID != 42 || tt.repository.receivedQuery != "photo" || tt.repository.receivedKind != "file" || tt.repository.receivedOffset != 2 {
					t.Fatalf("unexpected repository query: %+v", tt.repository)
				}
			}
		})
	}
}
