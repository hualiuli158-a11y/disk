package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
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

type handlerUploadRepository struct {
	err              error
	physicalCreated  bool
	called           bool
	receivedUserID   uint64
	receivedFolderID uint64
	receivedName     string
	receivedSHA256   string
	receivedSize     int64
	receivedKey      string
}

func (f *handlerUploadRepository) CreateUploadedFile(
	_ context.Context,
	userID uint64,
	folderID uint64,
	name string,
	sha256Value string,
	size int64,
	storageKey string,
) (*model.File, *model.PhysicalFile, bool, error) {
	f.called = true
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	f.receivedName = name
	f.receivedSHA256 = sha256Value
	f.receivedSize = size
	f.receivedKey = storageKey
	if f.err != nil {
		return nil, nil, false, f.err
	}

	createdAt := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
	physical := &model.PhysicalFile{
		ID: 1, SHA256: sha256Value, Size: size, StorageKey: storageKey, CreatedAt: createdAt,
	}
	return &model.File{
		ID: 2, UserID: userID, FolderID: folderID, PhysicalFileID: physical.ID, Name: name, CreatedAt: createdAt,
	}, physical, f.physicalCreated, nil
}

type handlerObjectStorage struct {
	saveErr     error
	savedKey    string
	savedData   []byte
	deletedKeys []string
}

func (s *handlerObjectStorage) Save(_ context.Context, key string, reader io.Reader) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.savedKey = key
	s.savedData = data
	return nil
}

func (s *handlerObjectStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (s *handlerObjectStorage) Delete(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return nil
}

func TestUploadFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")
	storageErr := errors.New("storage unavailable")

	tests := []struct {
		name            string
		userID          any
		folderID        string
		filename        string
		content         []byte
		includeFile     bool
		repositoryErr   error
		physicalCreated bool
		storageErr      error
		maxUploadSize   int64
		wantStatus      int
		wantCode        int
		wantRepoCall    bool
		wantDelete      bool
	}{
		{
			name:            "success",
			userID:          uint64(42),
			folderID:        "7",
			filename:        "photo.jpg",
			content:         []byte("hello"),
			includeFile:     true,
			physicalCreated: true,
			maxUploadSize:   1024,
			wantStatus:      http.StatusOK,
			wantRepoCall:    true,
		},
		{
			name:          "deduplicated physical file cleans candidate",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "copy.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			maxUploadSize: 1024,
			wantStatus:    http.StatusOK,
			wantRepoCall:  true,
			wantDelete:    true,
		},
		{
			name:          "missing user ID",
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			maxUploadSize: 1024,
			wantStatus:    http.StatusUnauthorized,
			wantCode:      20004,
		},
		{
			name:          "invalid folder ID",
			userID:        uint64(42),
			folderID:      "invalid",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			maxUploadSize: 1024,
			wantStatus:    http.StatusBadRequest,
			wantCode:      10001,
		},
		{
			name:          "missing file",
			userID:        uint64(42),
			folderID:      "7",
			maxUploadSize: 1024,
			wantStatus:    http.StatusBadRequest,
			wantCode:      10001,
		},
		{
			name:          "folder not found",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			repositoryErr: repository.ErrFileFolderNotFound,
			maxUploadSize: 1024,
			wantStatus:    http.StatusNotFound,
			wantCode:      40002,
			wantRepoCall:  true,
			wantDelete:    true,
		},
		{
			name:          "duplicate logical file",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			repositoryErr: repository.ErrFileAlreadyExists,
			maxUploadSize: 1024,
			wantStatus:    http.StatusConflict,
			wantCode:      40003,
			wantRepoCall:  true,
			wantDelete:    true,
		},
		{
			name:          "file too large",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			maxUploadSize: 4,
			wantStatus:    http.StatusRequestEntityTooLarge,
			wantCode:      40004,
			wantDelete:    true,
		},
		{
			name:          "storage error",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			storageErr:    storageErr,
			maxUploadSize: 1024,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      40005,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			repositoryErr: databaseErr,
			maxUploadSize: 1024,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      40005,
			wantRepoCall:  true,
			wantDelete:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerUploadRepository{err: tt.repositoryErr, physicalCreated: tt.physicalCreated}
			objects := &handlerObjectStorage{saveErr: tt.storageErr}
			filesService := service.NewUploadFileService(repo, objects, tt.maxUploadSize)
			h := NewUploadFileHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.POST("/api/files", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.UploadFile(c)
			})

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			if err := writer.WriteField("folder_id", tt.folderID); err != nil {
				t.Fatalf("write folder_id: %v", err)
			}
			if tt.includeFile {
				part, err := writer.CreateFormFile("file", tt.filename)
				if err != nil {
					t.Fatalf("create multipart file: %v", err)
				}
				if _, err := part.Write(tt.content); err != nil {
					t.Fatalf("write multipart file: %v", err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("close multipart writer: %v", err)
			}

			request := httptest.NewRequest(http.MethodPost, "/api/files", body)
			request.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != tt.wantStatus {
				t.Fatalf("HTTP status = %d, want %d", response.Code, tt.wantStatus)
			}
			if repo.called != tt.wantRepoCall {
				t.Fatalf("repository called = %v, want %v", repo.called, tt.wantRepoCall)
			}
			if (len(objects.deletedKeys) > 0) != tt.wantDelete {
				t.Fatalf("candidate deleted = %v, want %v", len(objects.deletedKeys) > 0, tt.wantDelete)
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

			var payload UploadFileResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode upload response: %v", err)
			}
			if payload.ID != 2 || payload.FolderID != 7 || payload.Name != tt.filename || payload.Size != int64(len(tt.content)) {
				t.Fatalf("unexpected upload response: %+v", payload)
			}
			digest := sha256.Sum256(tt.content)
			wantSHA := hex.EncodeToString(digest[:])
			if payload.SHA256 != wantSHA || repo.receivedSHA256 != wantSHA {
				t.Fatalf("sha256 = %q, want %q", payload.SHA256, wantSHA)
			}
			if repo.receivedUserID != 42 || repo.receivedFolderID != 7 || repo.receivedSize != int64(len(tt.content)) {
				t.Fatalf("unexpected repository input: user=%d folder=%d size=%d", repo.receivedUserID, repo.receivedFolderID, repo.receivedSize)
			}
			if !bytes.Equal(objects.savedData, tt.content) {
				t.Fatalf("saved content = %q, want %q", objects.savedData, tt.content)
			}
		})
	}
}
