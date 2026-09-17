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
	"strings"
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
	openData    []byte
	openErr     error
	openedKey   string
}

type handlerListFilesRepository struct {
	files            []model.File
	err              error
	receivedUserID   uint64
	receivedFolderID uint64
}

func (f *handlerListFilesRepository) ListFiles(_ context.Context, userID uint64, folderID uint64) ([]model.File, error) {
	f.receivedUserID = userID
	f.receivedFolderID = folderID
	return f.files, f.err
}

type handlerDownloadFileRepository struct {
	file           *model.File
	err            error
	receivedUserID uint64
	receivedFileID uint64
}

func (f *handlerDownloadFileRepository) FindFileForDownload(_ context.Context, userID uint64, fileID uint64) (*model.File, error) {
	f.receivedUserID = userID
	f.receivedFileID = fileID
	return f.file, f.err
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

func (s *handlerObjectStorage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	s.openedKey = key
	return io.NopCloser(bytes.NewReader(s.openData)), nil
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

func TestListFilesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	createdAt := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		userID     any
		folderID   string
		repository *handlerListFilesRepository
		wantStatus int
		wantCode   int
		wantFile   bool
	}{
		{
			name:     "success",
			userID:   uint64(42),
			folderID: "7",
			repository: &handlerListFilesRepository{files: []model.File{{
				ID: 2, UserID: 42, FolderID: 7, Name: "photo.jpg", CreatedAt: createdAt,
				PhysicalFile: model.PhysicalFile{ID: 1, Size: 5, SHA256: "abc"},
			}}},
			wantStatus: http.StatusOK,
			wantFile:   true,
		},
		{
			name:       "empty files",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerListFilesRepository{},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			folderID:   "7",
			repository: &handlerListFilesRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid folder ID",
			userID:     uint64(42),
			folderID:   "invalid",
			repository: &handlerListFilesRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "folder not found",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerListFilesRepository{err: repository.ErrFileFolderNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   30007,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			folderID:   "7",
			repository: &handlerListFilesRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   40006,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesService := service.NewListFilesService(tt.repository)
			h := NewListFilesHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/folders/:id/files", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.ListFiles(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/folders/"+tt.folderID+"/files", nil)
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

			var payload ListFilesResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode list response: %v", err)
			}
			if payload.FolderID != 7 || payload.Files == nil {
				t.Fatalf("unexpected list response: %+v", payload)
			}
			if tt.repository.receivedUserID != 42 || tt.repository.receivedFolderID != 7 {
				t.Fatalf("unexpected repository input: user=%d folder=%d", tt.repository.receivedUserID, tt.repository.receivedFolderID)
			}
			if tt.wantFile {
				if len(payload.Files) != 1 || payload.Files[0].ID != 2 || payload.Files[0].Size != 5 || payload.Files[0].SHA256 != "abc" {
					t.Fatalf("unexpected files: %+v", payload.Files)
				}
			} else if len(payload.Files) != 0 {
				t.Fatalf("files length = %d, want 0", len(payload.Files))
			}
		})
	}
}

func TestDownloadFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")
	storageErr := errors.New("storage unavailable")

	tests := []struct {
		name       string
		userID     any
		fileID     string
		repository *handlerDownloadFileRepository
		storageErr error
		wantStatus int
		wantCode   int
	}{
		{
			name:   "success",
			userID: uint64(42),
			fileID: "2",
			repository: &handlerDownloadFileRepository{file: &model.File{
				ID: 2, UserID: 42, FolderID: 7, PhysicalFileID: 1, Name: "photo.txt",
				PhysicalFile: model.PhysicalFile{ID: 1, Size: 5, StorageKey: "objects/aa/value"},
			}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing user ID",
			fileID:     "2",
			repository: &handlerDownloadFileRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid file ID",
			userID:     uint64(42),
			fileID:     "invalid",
			repository: &handlerDownloadFileRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "file not found",
			userID:     uint64(42),
			fileID:     "2",
			repository: &handlerDownloadFileRepository{err: repository.ErrFileNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   40007,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			fileID:     "2",
			repository: &handlerDownloadFileRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   40008,
		},
		{
			name:   "storage error",
			userID: uint64(42),
			fileID: "2",
			repository: &handlerDownloadFileRepository{file: &model.File{
				ID: 2, UserID: 42, FolderID: 7, PhysicalFileID: 1, Name: "photo.txt",
				PhysicalFile: model.PhysicalFile{ID: 1, Size: 5, StorageKey: "objects/aa/value"},
			}},
			storageErr: storageErr,
			wantStatus: http.StatusInternalServerError,
			wantCode:   40008,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := &handlerObjectStorage{openData: []byte("hello"), openErr: tt.storageErr}
			filesService := service.NewDownloadFileService(tt.repository, objects)
			h := NewDownloadFileHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.GET("/api/files/:id/download", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.DownloadFile(c)
			})

			request := httptest.NewRequest(http.MethodGet, "/api/files/"+tt.fileID+"/download", nil)
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

			if response.Body.String() != "hello" {
				t.Fatalf("download body = %q, want hello", response.Body.String())
			}
			if response.Header().Get("Content-Type") != "application/octet-stream" || response.Header().Get("Content-Length") != "5" {
				t.Fatalf("unexpected download headers: %+v", response.Header())
			}
			if !strings.Contains(response.Header().Get("Content-Disposition"), "photo.txt") {
				t.Fatalf("unexpected content disposition: %q", response.Header().Get("Content-Disposition"))
			}
			if objects.openedKey != "objects/aa/value" || tt.repository.receivedUserID != 42 || tt.repository.receivedFileID != 2 {
				t.Fatalf("unexpected download inputs: key=%q user=%d file=%d", objects.openedKey, tt.repository.receivedUserID, tt.repository.receivedFileID)
			}
		})
	}
}
