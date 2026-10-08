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
	"disk/internal/storage"
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
	deleteErr   error
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

type handlerDeleteFileRepository struct {
	err            error
	receivedUserID uint64
	receivedFileID uint64
}

type handlerRenameFileRepository struct {
	err            error
	called         bool
	receivedUserID uint64
	receivedFileID uint64
	receivedName   string
}

type handlerMoveFileRepository struct {
	err              error
	called           bool
	receivedUserID   uint64
	receivedFileID   uint64
	receivedFolderID uint64
}

func (f *handlerMoveFileRepository) MoveFile(_ context.Context, userID uint64, fileID uint64, folderID uint64) error {
	f.called = true
	f.receivedUserID = userID
	f.receivedFileID = fileID
	f.receivedFolderID = folderID
	return f.err
}

func (f *handlerRenameFileRepository) RenameFile(_ context.Context, userID uint64, fileID uint64, name string) error {
	f.called = true
	f.receivedUserID = userID
	f.receivedFileID = fileID
	f.receivedName = name
	return f.err
}

func (f *handlerDeleteFileRepository) TrashFile(_ context.Context, userID uint64, fileID uint64, _ time.Time, _ time.Time) error {
	f.receivedUserID = userID
	f.receivedFileID = fileID
	return f.err
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
	return s.deleteErr
}

func (s *handlerObjectStorage) List(context.Context, string) ([]storage.ObjectInfo, error) {
	return []storage.ObjectInfo{}, nil
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
		{
			name:          "quota exceeded",
			userID:        uint64(42),
			folderID:      "7",
			filename:      "photo.jpg",
			content:       []byte("hello"),
			includeFile:   true,
			repositoryErr: repository.ErrQuotaExceeded,
			maxUploadSize: 1024,
			wantStatus:    http.StatusInsufficientStorage,
			wantCode:      40012,
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
	updatedAt := time.Date(2026, time.October, 8, 14, 0, 0, 0, time.UTC)
	const contentSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
				ID: 2, UserID: 42, FolderID: 7, PhysicalFileID: 1, Name: "photo.txt", UpdatedAt: updatedAt,
				PhysicalFile: model.PhysicalFile{ID: 1, Size: 5, SHA256: contentSHA, StorageKey: "objects/aa/value"},
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
			if response.Header().Get("ETag") != `"`+contentSHA+`"` || response.Header().Get("X-File-SHA256") != contentSHA {
				t.Fatalf("missing version headers: %+v", response.Header())
			}
			if objects.openedKey != "objects/aa/value" || tt.repository.receivedUserID != 42 || tt.repository.receivedFileID != 2 {
				t.Fatalf("unexpected download inputs: key=%q user=%d file=%d", objects.openedKey, tt.repository.receivedUserID, tt.repository.receivedFileID)
			}
		})
	}
}

func TestDeleteFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")
	tests := []struct {
		name       string
		userID     any
		fileID     string
		repository *handlerDeleteFileRepository
		wantStatus int
		wantCode   int
	}{
		{
			name:       "success keeps shared physical file",
			userID:     uint64(42),
			fileID:     "2",
			repository: &handlerDeleteFileRepository{},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing user ID",
			fileID:     "2",
			repository: &handlerDeleteFileRepository{},
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid file ID",
			userID:     uint64(42),
			fileID:     "invalid",
			repository: &handlerDeleteFileRepository{},
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "file not found",
			userID:     uint64(42),
			fileID:     "2",
			repository: &handlerDeleteFileRepository{err: repository.ErrFileNotFound},
			wantStatus: http.StatusNotFound,
			wantCode:   40007,
		},
		{
			name:       "database error",
			userID:     uint64(42),
			fileID:     "2",
			repository: &handlerDeleteFileRepository{err: databaseErr},
			wantStatus: http.StatusInternalServerError,
			wantCode:   40009,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filesService := service.NewDeleteFileService(tt.repository)
			h := NewDeleteFileHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.DELETE("/api/files/:id", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.DeleteFile(c)
			})

			request := httptest.NewRequest(http.MethodDelete, "/api/files/"+tt.fileID, nil)
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

			if response.Body.Len() != 0 {
				t.Fatalf("204 response body must be empty, got %q", response.Body.String())
			}
			if tt.repository.receivedUserID != 42 || tt.repository.receivedFileID != 2 {
				t.Fatalf("unexpected repository input: user=%d file=%d", tt.repository.receivedUserID, tt.repository.receivedFileID)
			}
		})
	}
}

func TestRenameFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		userID        any
		fileID        string
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
			fileID:       "2",
			body:         `{"name":" report.pdf "}`,
			wantStatus:   http.StatusOK,
			wantName:     "report.pdf",
			wantRepoCall: true,
		},
		{
			name:       "missing user ID",
			fileID:     "2",
			body:       `{"name":"report.pdf"}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid file ID",
			userID:     uint64(42),
			fileID:     "invalid",
			body:       `{"name":"report.pdf"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "invalid file name",
			userID:     uint64(42),
			fileID:     "2",
			body:       `{"name":"../report.pdf"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   40001,
		},
		{
			name:          "file not found",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"name":"report.pdf"}`,
			repositoryErr: repository.ErrFileNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      40007,
			wantRepoCall:  true,
		},
		{
			name:          "duplicate file name",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"name":"report.pdf"}`,
			repositoryErr: repository.ErrFileAlreadyExists,
			wantStatus:    http.StatusConflict,
			wantCode:      40003,
			wantRepoCall:  true,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"name":"report.pdf"}`,
			repositoryErr: databaseErr,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      40010,
			wantRepoCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerRenameFileRepository{err: tt.repositoryErr}
			filesService := service.NewRenameFileService(repo)
			h := NewRenameFileHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.PATCH("/api/files/:id", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.RenameFile(c)
			})

			request := httptest.NewRequest(http.MethodPatch, "/api/files/"+tt.fileID, bytes.NewBufferString(tt.body))
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

			var payload RenameFileResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode rename response: %v", err)
			}
			if payload.ID != 2 || payload.Name != tt.wantName {
				t.Fatalf("unexpected rename response: %+v", payload)
			}
			if repo.receivedUserID != 42 || repo.receivedFileID != 2 || repo.receivedName != tt.wantName {
				t.Fatalf("unexpected repository input: user=%d file=%d name=%q", repo.receivedUserID, repo.receivedFileID, repo.receivedName)
			}
		})
	}
}

func TestMoveFileHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	databaseErr := errors.New("database unavailable")

	tests := []struct {
		name          string
		userID        any
		fileID        string
		body          string
		repositoryErr error
		wantStatus    int
		wantCode      int
		wantRepoCall  bool
	}{
		{
			name:         "success",
			userID:       uint64(42),
			fileID:       "2",
			body:         `{"folder_id":20}`,
			wantStatus:   http.StatusOK,
			wantRepoCall: true,
		},
		{
			name:       "missing user ID",
			fileID:     "2",
			body:       `{"folder_id":20}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   20004,
		},
		{
			name:       "invalid file ID",
			userID:     uint64(42),
			fileID:     "invalid",
			body:       `{"folder_id":20}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:       "missing folder ID",
			userID:     uint64(42),
			fileID:     "2",
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   10001,
		},
		{
			name:          "file not found",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"folder_id":20}`,
			repositoryErr: repository.ErrFileNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      40007,
			wantRepoCall:  true,
		},
		{
			name:          "folder not found",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"folder_id":20}`,
			repositoryErr: repository.ErrFileFolderNotFound,
			wantStatus:    http.StatusNotFound,
			wantCode:      30007,
			wantRepoCall:  true,
		},
		{
			name:          "duplicate file name",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"folder_id":20}`,
			repositoryErr: repository.ErrFileAlreadyExists,
			wantStatus:    http.StatusConflict,
			wantCode:      40003,
			wantRepoCall:  true,
		},
		{
			name:          "database error",
			userID:        uint64(42),
			fileID:        "2",
			body:          `{"folder_id":20}`,
			repositoryErr: databaseErr,
			wantStatus:    http.StatusInternalServerError,
			wantCode:      40011,
			wantRepoCall:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &handlerMoveFileRepository{err: tt.repositoryErr}
			filesService := service.NewMoveFileService(repo)
			h := NewMoveFileHandler(filesService, logger.NewLogger(io.Discard))
			router := gin.New()
			router.PATCH("/api/files/:id/move", func(c *gin.Context) {
				if tt.userID != nil {
					c.Set("user_id", tt.userID)
				}
				h.MoveFile(c)
			})

			request := httptest.NewRequest(http.MethodPatch, "/api/files/"+tt.fileID+"/move", bytes.NewBufferString(tt.body))
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

			var payload MoveFileResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode move response: %v", err)
			}
			if payload.ID != 2 || payload.FolderID != 20 {
				t.Fatalf("unexpected move response: %+v", payload)
			}
			if repo.receivedUserID != 42 || repo.receivedFileID != 2 || repo.receivedFolderID != 20 {
				t.Fatalf("unexpected repository input: user=%d file=%d folder=%d", repo.receivedUserID, repo.receivedFileID, repo.receivedFolderID)
			}
		})
	}
}
