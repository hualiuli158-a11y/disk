package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type FileVersionHandler struct {
	versions   *service.FileVersionService
	baselogger *logger.Logger
}

func NewFileVersionHandler(versions *service.FileVersionService, baselogger *logger.Logger) *FileVersionHandler {
	return &FileVersionHandler{versions: versions, baselogger: baselogger}
}

type FileVersionResponse struct {
	ID                    uint64    `json:"id"`
	Size                  int64     `json:"size"`
	SHA256                string    `json:"sha256"`
	Source                string    `json:"source"`
	RestoredFromVersionID *uint64   `json:"restored_from_version_id,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	Current               bool      `json:"current"`
}

type FileVersionListResponse struct {
	Items []FileVersionResponse `json:"items"`
}

func fileAndVersionIDs(c *gin.Context) (uint64, uint64, bool) {
	fileID, fileErr := strconv.ParseUint(c.Param("id"), 10, 64)
	versionID, versionErr := strconv.ParseUint(c.Param("version_id"), 10, 64)
	return fileID, versionID, fileErr == nil && versionErr == nil && fileID > 0 && versionID > 0
}

// ListFileVersions 返回文件版本历史。
func (h *FileVersionHandler) ListFileVersions(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}
	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	items, err := h.versions.ListFileVersions(c.Request.Context(), userID, fileID)
	if !h.handleError(c, err, "list file versions") {
		return
	}
	response := make([]FileVersionResponse, 0, len(items))
	for _, item := range items {
		response = append(response, FileVersionResponse{
			ID: item.ID, Size: item.Size, SHA256: item.SHA256, Source: item.Source,
			RestoredFromVersionID: item.RestoredFromVersionID, CreatedAt: item.CreatedAt, Current: item.Current,
		})
	}
	app.NewResponse(c).ToResponse(FileVersionListResponse{Items: response})
}

// DownloadFileVersion 下载指定历史版本。
func (h *FileVersionHandler) DownloadFileVersion(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}
	fileID, versionID, ok := fileAndVersionIDs(c)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	result, err := h.versions.DownloadFileVersion(c.Request.Context(), userID, fileID, versionID)
	if !h.handleError(c, err, "download file version") {
		return
	}
	defer func() {
		if err := result.Content.Close(); err != nil {
			h.baselogger.WithContext(c.Request.Context()).Errorf("close version content: %v", err)
		}
	}()
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": result.Name})
	if disposition == "" {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(result.Size, 10))
	c.Header("ETag", strconv.Quote(result.SHA256))
	c.Header("X-File-SHA256", result.SHA256)
	c.Header("Last-Modified", result.UpdatedAt.UTC().Format(http.TimeFormat))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, result.Content); err != nil {
		h.baselogger.WithContext(c.Request.Context()).Errorf("stream file version: %v", err)
	}
}

// RestoreFileVersion 将指定历史版本恢复为当前版本。
func (h *FileVersionHandler) RestoreFileVersion(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}
	fileID, versionID, ok := fileAndVersionIDs(c)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	ifMatch := strings.TrimSpace(c.GetHeader("If-Match"))
	if ifMatch == "" {
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileVersionRequired)
		return
	}
	expectedSHA256, valid := strongSHA256FromETag(ifMatch)
	if !valid {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	result, err := h.versions.RestoreFileVersion(c.Request.Context(), userID, fileID, versionID, expectedSHA256)
	if !h.handleError(c, err, "restore file version") {
		return
	}
	c.Header("ETag", strconv.Quote(result.SHA256))
	c.Header("Cache-Control", "no-store")
	app.NewResponse(c).ToResponse(UpdateFileContentResponse{
		ID: result.ID, Name: result.Name, Size: result.Size, SHA256: result.SHA256, UpdatedAt: result.UpdatedAt,
	})
}

func (h *FileVersionHandler) handleError(c *gin.Context, err error, operation string) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
	case errors.Is(err, service.ErrFileVersionNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileHistoryVersionNotFound)
	case errors.Is(err, service.ErrInvalidFileVersion):
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
	case errors.Is(err, service.ErrFileVersionConflict):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileVersionConflict)
	case errors.Is(err, service.ErrQuotaExceeded):
		app.NewResponse(c).ToErrorResponse(errcode.ErrQuotaExceeded)
	default:
		h.baselogger.WithContext(c.Request.Context()).Errorf("%s failed: %v", operation, err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileHistoryFailed)
	}
	return false
}
