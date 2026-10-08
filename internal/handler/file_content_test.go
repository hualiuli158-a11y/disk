package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"disk/internal/model"
	"disk/internal/repository"
	"disk/internal/service"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type handlerUpdateContentRepository struct {
	err             error
	physicalCreated bool
	called          bool
	expectedSHA256  string
	sha256Value     string
	size            int64
	updatedAt       time.Time
}

func (r *handlerUpdateContentRepository) ReplaceFileContent(
	_ context.Context,
	_ uint64,
	fileID uint64,
	expectedSHA256 string,
	sha256Value string,
	size int64,
	storageKey string,
) (*model.File, *model.PhysicalFile, bool, error) {
	r.called = true
	r.expectedSHA256 = expectedSHA256
	r.sha256Value = sha256Value
	r.size = size
	if r.err != nil {
		return nil, nil, false, r.err
	}
	physical := &model.PhysicalFile{ID: 3, SHA256: sha256Value, Size: size, StorageKey: storageKey}
	file := &model.File{ID: fileID, Name: "note.md", PhysicalFileID: physical.ID, UpdatedAt: r.updatedAt}
	return file, physical, r.physicalCreated, nil
}

func TestUpdateFileContentHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const expectedSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	updatedAt := time.Date(2026, time.October, 8, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		userID         any
		fileID         string
		ifMatch        string
		body           string
		repositoryErr  error
		wantStatus     int
		wantCode       int
		wantRepoCalled bool
	}{
		{name: "success", userID: uint64(42), fileID: "5", ifMatch: `"` + expectedSHA + `"`, body: "next", wantStatus: http.StatusOK, wantRepoCalled: true},
		{name: "missing precondition", userID: uint64(42), fileID: "5", body: "next", wantStatus: http.StatusPreconditionRequired, wantCode: 40017},
		{name: "stale version", userID: uint64(42), fileID: "5", ifMatch: `"` + expectedSHA + `"`, body: "next", repositoryErr: repository.ErrFileVersionConflict, wantStatus: http.StatusPreconditionFailed, wantCode: 40015, wantRepoCalled: true},
		{name: "invalid etag", userID: uint64(42), fileID: "5", ifMatch: `"short"`, body: "next", wantStatus: http.StatusBadRequest, wantCode: 10001},
		{name: "invalid utf8", userID: uint64(42), fileID: "5", ifMatch: `"` + expectedSHA + `"`, body: string([]byte{0xff}), wantStatus: http.StatusBadRequest, wantCode: 40014},
		{name: "missing user", fileID: "5", ifMatch: `"` + expectedSHA + `"`, body: "next", wantStatus: http.StatusUnauthorized, wantCode: 20004},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repositoryStub := &handlerUpdateContentRepository{err: tt.repositoryErr, physicalCreated: true, updatedAt: updatedAt}
			objects := &handlerObjectStorage{}
			files := service.NewUpdateFileContentService(repositoryStub, objects, service.DefaultMaxEditableTextSize)
			h := NewUpdateFileContentHandler(files, logger.NewLogger(io.Discard))
			router := gin.New()
			router.PUT("/api/files/:id/content", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.UpdateFileContent(c)
			})

			request := httptest.NewRequest(http.MethodPut, "/api/files/"+tt.fileID+"/content", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "text/plain; charset=utf-8")
			if tt.ifMatch != "" {
				request.Header.Set("If-Match", tt.ifMatch)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if repositoryStub.called != tt.wantRepoCalled {
				t.Fatalf("repository called = %v, want %v", repositoryStub.called, tt.wantRepoCalled)
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
			var payload UpdateFileContentResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode success response: %v", err)
			}
			if payload.ID != 5 || payload.Name != "note.md" || payload.Size != 4 || payload.SHA256 == "" || !payload.UpdatedAt.Equal(updatedAt) {
				t.Fatalf("unexpected response: %+v", payload)
			}
			if response.Header().Get("ETag") != `"`+payload.SHA256+`"` {
				t.Fatalf("etag = %q, sha=%q", response.Header().Get("ETag"), payload.SHA256)
			}
			if repositoryStub.expectedSHA256 != expectedSHA {
				t.Fatalf("expected sha = %q", repositoryStub.expectedSHA256)
			}
		})
	}
}
