package handler

import (
	"crypto/sha256"
	"encoding/hex"
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
	"disk/pkg/setting"

	"github.com/gin-gonic/gin"
)

type UploadFileHandler struct {
	files           *service.UploadFileService
	quota           *service.QuotaService
	maxUploadBytes  int64
	maxRequestBytes int64
	baselogger      *logger.Logger
}

func NewUploadFileHandler(files *service.UploadFileService, quota *service.QuotaService, upload setting.UploadConfig, baselogger *logger.Logger) *UploadFileHandler {
	return &UploadFileHandler{
		files: files, quota: quota, maxUploadBytes: upload.MaxFileBytes,
		maxRequestBytes: upload.MaxRequestBytes(), baselogger: baselogger,
	}
}

type UploadFileResponse struct {
	ID        uint64    `json:"id" example:"1"`
	FolderID  uint64    `json:"folder_id" example:"10"`
	Name      string    `json:"name" example:"photo.jpg"`
	Size      int64     `json:"size" example:"1048576"`
	SHA256    string    `json:"sha256" example:"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
	CreatedAt time.Time `json:"created_at" example:"2026-09-17T10:30:00+08:00"`
}

// UploadFile 上传文件到当前用户拥有的目录。
// @Summary 上传文件
// @Description 上传文件并计算 SHA-256；相同内容复用物理文件，同一目录下不能存在同名文件。
// @Tags 文件
// @Accept multipart/form-data
// @Produce json
// @Security BearerAuth
// @Param X-Upload-Size header int false "文件字节数；提供后会在读取正文前检查大小和剩余额度"
// @Param folder_id formData int true "目标目录 ID"
// @Param file formData file true "待上传文件"
// @Success 200 {object} UploadFileResponse "上传成功"
// @Failure 400 {object} app.ErrorResponse "请求参数或文件名不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "目标目录不存在或无权访问"
// @Failure 409 {object} app.ErrorResponse "目标目录下已存在同名文件"
// @Failure 413 {object} app.ErrorResponse "文件超过上传大小限制"
// @Failure 429 {object} app.ErrorResponse "上传过于频繁或并发上传数超限"
// @Failure 507 {object} app.ErrorResponse "存储额度不足"
// @Failure 500 {object} app.ErrorResponse "上传文件失败"
// @Router /api/files [post]
func (h *UploadFileHandler) UploadFile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("UploadFile invalid user_id")
		return
	}

	declaredSize, hasDeclaredSize, err := uploadSizeHeader(c.Request)
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	if (hasDeclaredSize && declaredSize > h.maxUploadBytes) || c.Request.ContentLength > h.maxRequestBytes {
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileTooLarge)
		return
	}
	// 必须在任何 multipart 解析之前安装硬上限。MultipartReader 采用流式读取，
	// 不调用 ParseMultipartForm/FormFile，因此不会向系统 multipart 临时目录落盘。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxRequestBytes)

	if hasDeclaredSize && h.quota != nil {
		quota, quotaErr := h.quota.GetQuota(c.Request.Context(), userID)
		switch {
		case errors.Is(quotaErr, service.ErrQuotaUserNotFound):
			app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
			return
		case quotaErr != nil:
			h.baselogger.WithContext(c.Request.Context()).Errorf("preflight upload quota failed: %v", quotaErr)
			app.NewResponse(c).ToErrorResponse(errcode.ErrUploadFileFailed)
			return
		case declaredSize > quota.Remaining:
			app.NewResponse(c).ToErrorResponse(errcode.ErrQuotaExceeded)
			return
		}
	}

	reader, err := c.Request.MultipartReader()
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	var folderID uint64
	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
			return
		}
		if partErr != nil {
			if requestTooLarge(partErr) {
				app.NewResponse(c).ToErrorResponse(errcode.ErrFileTooLarge)
			} else {
				app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
			}
			return
		}

		switch part.FormName() {
		case "folder_id":
			value, readErr := io.ReadAll(io.LimitReader(part, 21))
			if readErr != nil || len(value) > 20 || folderID != 0 {
				app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
				return
			}
			_ = part.Close()
			folderID, err = strconv.ParseUint(string(value), 10, 64)
			if err != nil || folderID == 0 {
				app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
				return
			}
		case "file":
			if folderID == 0 || part.FileName() == "" {
				app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
				return
			}
			result, uploadErr := h.files.UploadFile(c.Request.Context(), service.UploadFileInput{
				UserID: userID, FolderID: folderID, Name: part.FileName(), Content: part,
				DeclaredSize: declaredSize, HasDeclaredSize: hasDeclaredSize,
			})
			if uploadErr == nil {
				_ = part.Close()
			}
			h.writeUploadResult(c, result, uploadErr)
			return
		default:
			app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
			return
		}
	}
}

func (h *UploadFileHandler) writeUploadResult(c *gin.Context, result *service.UploadFileOutput, err error) {
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrInvalidFileName):
		app.NewResponse(c).ToErrorResponse(errcode.ErrInvalidFileName)
		return
	case errors.Is(err, service.ErrUploadFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrUploadFolderNotFound)
		return
	case errors.Is(err, service.ErrFileAlreadyExists):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileExists)
		return
	case errors.Is(err, service.ErrFileTooLarge):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileTooLarge)
		return
	case errors.Is(err, service.ErrUploadSizeMismatch):
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	case errors.Is(err, service.ErrQuotaExceeded):
		app.NewResponse(c).ToErrorResponse(errcode.ErrQuotaExceeded)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("upload file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrUploadFileFailed)
		return
	}

	app.NewResponse(c).ToResponse(UploadFileResponse{
		ID:        result.ID,
		FolderID:  result.FolderID,
		Name:      result.Name,
		Size:      result.Size,
		SHA256:    result.SHA256,
		CreatedAt: result.CreatedAt,
	})
}

func uploadSizeHeader(request *http.Request) (int64, bool, error) {
	value := strings.TrimSpace(request.Header.Get("X-Upload-Size"))
	if value == "" {
		return 0, false, nil
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil || size < 0 {
		return 0, false, errors.New("invalid X-Upload-Size")
	}
	return size, true, nil
}

func requestTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError)
}

type ListFilesHandler struct {
	files      *service.ListFilesService
	baselogger *logger.Logger
}

func NewListFilesHandler(files *service.ListFilesService, baselogger *logger.Logger) *ListFilesHandler {
	return &ListFilesHandler{files: files, baselogger: baselogger}
}

type FileListItemResponse struct {
	ID        uint64    `json:"id"`
	FolderID  uint64    `json:"folder_id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type ListFilesResponse struct {
	FolderID uint64                 `json:"folder_id"`
	Files    []FileListItemResponse `json:"files"`
}

// ListFiles 返回指定目录中的直接文件。
// @Summary 获取目录中的文件
// @Description 返回当前用户指定目录中的直接文件，不包含子目录中的文件，也不暴露底层存储 Key。
// @Tags 文件
// @Produce json
// @Security BearerAuth
// @Param id path int true "目录 ID"
// @Success 200 {object} ListFilesResponse "查询成功"
// @Failure 400 {object} app.ErrorResponse "目录 ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "目录不存在或无权访问"
// @Failure 500 {object} app.ErrorResponse "获取文件列表失败"
// @Router /api/folders/{id}/files [get]
func (h *ListFilesHandler) ListFiles(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("ListFiles invalid user_id")
		return
	}

	folderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || folderID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.files.ListFiles(c.Request.Context(), userID, folderID)
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFileFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotFound)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("list files failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrListFilesFailed)
		return
	}

	files := make([]FileListItemResponse, 0, len(result.Files))
	for _, file := range result.Files {
		files = append(files, FileListItemResponse{
			ID:        file.ID,
			FolderID:  file.FolderID,
			Name:      file.Name,
			Size:      file.Size,
			SHA256:    file.SHA256,
			CreatedAt: file.CreatedAt,
		})
	}

	app.NewResponse(c).ToResponse(ListFilesResponse{FolderID: result.FolderID, Files: files})
}

type DownloadFileHandler struct {
	files      *service.DownloadFileService
	baselogger *logger.Logger
}

func NewDownloadFileHandler(files *service.DownloadFileService, baselogger *logger.Logger) *DownloadFileHandler {
	return &DownloadFileHandler{files: files, baselogger: baselogger}
}

// DownloadFile 下载当前用户拥有的文件。
// @Summary 下载文件
// @Description 根据逻辑文件 ID 下载当前用户拥有的文件，客户端不能指定底层存储 Key。
// @Tags 文件
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Success 200 {file} binary "文件内容"
// @Failure 400 {object} app.ErrorResponse "文件 ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件不存在或无权访问"
// @Failure 500 {object} app.ErrorResponse "下载文件失败"
// @Router /api/files/{id}/download [get]
func (h *DownloadFileHandler) DownloadFile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("DownloadFile invalid user_id")
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.files.DownloadFile(c.Request.Context(), userID, fileID)
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("download file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrDownloadFileFailed)
		return
	}
	defer func() {
		if err := result.Content.Close(); err != nil {
			h.baselogger.WithContext(c.Request.Context()).Errorf("close downloaded file failed: %v", err)
		}
	}()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": result.Name})
	if disposition == "" {
		disposition = "attachment"
	}
	c.Header("Content-Disposition", disposition)
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(result.Size, 10))
	if result.SHA256 != "" {
		c.Header("ETag", strconv.Quote(result.SHA256))
		c.Header("X-File-SHA256", result.SHA256)
	}
	if !result.UpdatedAt.IsZero() {
		c.Header("Last-Modified", result.UpdatedAt.UTC().Format(http.TimeFormat))
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, result.Content); err != nil {
		// 响应头可能已经发送，此时不能再改写为 JSON 错误响应。
		h.baselogger.WithContext(c.Request.Context()).Errorf("stream downloaded file failed: %v", err)
	}
}

type UpdateFileContentHandler struct {
	files      *service.UpdateFileContentService
	baselogger *logger.Logger
}

func NewUpdateFileContentHandler(files *service.UpdateFileContentService, baselogger *logger.Logger) *UpdateFileContentHandler {
	return &UpdateFileContentHandler{files: files, baselogger: baselogger}
}

type UpdateFileContentResponse struct {
	ID        uint64    `json:"id" example:"1"`
	Name      string    `json:"name" example:"note.md"`
	Size      int64     `json:"size" example:"1024"`
	SHA256    string    `json:"sha256" example:"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpdateFileContent 使用当前内容版本作为前置条件，替换文本文件正文。
// @Summary 更新文件正文
// @Description 使用 If-Match 中的 SHA-256 强 ETag 防止覆盖并发修改；正文必须是 UTF-8 文本。
// @Tags 文件
// @Accept text/plain
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Param If-Match header string true "当前文件 SHA-256 强 ETag"
// @Param create_version query bool false "仅手动保存时传 true，创建历史快照；默认只更新当前内容"
// @Param content body string true "新的 UTF-8 文本正文"
// @Success 200 {object} UpdateFileContentResponse "保存成功"
// @Failure 400 {object} app.ErrorResponse "请求、版本或文本编码不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件不存在或无权访问"
// @Failure 412 {object} app.ErrorResponse "文件已被其他操作修改"
// @Failure 413 {object} app.ErrorResponse "正文超过大小限制"
// @Failure 428 {object} app.ErrorResponse "缺少 If-Match 前置条件"
// @Failure 507 {object} app.ErrorResponse "存储额度不足"
// @Failure 500 {object} app.ErrorResponse "更新文件内容失败"
// @Router /api/files/{id}/content [put]
func (h *UpdateFileContentHandler) UpdateFileContent(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	ifMatch := strings.TrimSpace(c.GetHeader("If-Match"))
	if ifMatch == "" {
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileVersionRequired)
		return
	}
	expectedSHA256, ok := strongSHA256FromETag(ifMatch)
	if !ok {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	createVersion := false
	if raw, present := c.GetQuery("create_version"); present {
		createVersion, err = strconv.ParseBool(raw)
		if err != nil {
			app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
			return
		}
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.DefaultMaxEditableTextSize+1)
	result, err := h.files.UpdateFileContent(c.Request.Context(), service.UpdateFileContentInput{
		CreateVersion:  createVersion,
		UserID:         userID,
		FileID:         fileID,
		ExpectedSHA256: expectedSHA256,
		Content:        c.Request.Body,
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
		return
	case errors.Is(err, service.ErrInvalidFileVersion):
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	case errors.Is(err, service.ErrInvalidTextContent):
		app.NewResponse(c).ToErrorResponse(errcode.ErrInvalidTextContent)
		return
	case errors.Is(err, service.ErrFileVersionConflict):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileVersionConflict)
		return
	case errors.Is(err, service.ErrFileContentTooLarge):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileContentTooLarge)
		return
	case errors.Is(err, service.ErrQuotaExceeded):
		app.NewResponse(c).ToErrorResponse(errcode.ErrQuotaExceeded)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("update file content failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrUpdateFileContentFailed)
		return
	}

	c.Header("ETag", strconv.Quote(result.SHA256))
	c.Header("Cache-Control", "no-store")
	app.NewResponse(c).ToResponse(UpdateFileContentResponse{
		ID:        result.ID,
		Name:      result.Name,
		Size:      result.Size,
		SHA256:    result.SHA256,
		UpdatedAt: result.UpdatedAt,
	})
}

func strongSHA256FromETag(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "W/") || strings.Contains(trimmed, ",") || trimmed == "*" {
		return "", false
	}
	unquoted, err := strconv.Unquote(trimmed)
	if err != nil || len(unquoted) != sha256.Size*2 {
		return "", false
	}
	if _, err := hex.DecodeString(unquoted); err != nil {
		return "", false
	}
	return strings.ToLower(unquoted), true
}

type DeleteFileHandler struct {
	files      *service.DeleteFileService
	baselogger *logger.Logger
}

func NewDeleteFileHandler(files *service.DeleteFileService, baselogger *logger.Logger) *DeleteFileHandler {
	return &DeleteFileHandler{files: files, baselogger: baselogger}
}

// DeleteFile 将当前用户拥有的逻辑文件移入回收站。
// @Summary 删除文件到回收站
// @Description 将逻辑文件移入回收站；恢复期限内保留用户额度和物理文件。
// @Tags 文件
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Success 204 "删除成功"
// @Failure 400 {object} app.ErrorResponse "文件 ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件不存在或无权访问"
// @Failure 500 {object} app.ErrorResponse "删除文件失败"
// @Router /api/files/{id} [delete]
func (h *DeleteFileHandler) DeleteFile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("DeleteFile invalid user_id")
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	err = h.files.DeleteFile(c.Request.Context(), userID, fileID)
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("delete file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrDeleteFileFailed)
		return
	}
	c.Status(http.StatusNoContent)
}

type RenameFileHandler struct {
	files      *service.RenameFileService
	baselogger *logger.Logger
}

func NewRenameFileHandler(files *service.RenameFileService, baselogger *logger.Logger) *RenameFileHandler {
	return &RenameFileHandler{files: files, baselogger: baselogger}
}

type RenameFileRequest struct {
	Name string `json:"name" binding:"required" example:"新的文件名.pdf"`
}

type RenameFileResponse struct {
	ID   uint64 `json:"id" example:"1"`
	Name string `json:"name" example:"新的文件名.pdf"`
}

// RenameFile 重命名当前用户拥有的逻辑文件。
// @Summary 重命名文件
// @Description 重命名当前用户拥有的逻辑文件，同一目录下不能存在同名文件。
// @Tags 文件
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Param request body RenameFileRequest true "新的文件名"
// @Success 200 {object} RenameFileResponse "重命名成功"
// @Failure 400 {object} app.ErrorResponse "请求参数或文件名不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件不存在或无权访问"
// @Failure 409 {object} app.ErrorResponse "同一目录下已存在同名文件"
// @Failure 500 {object} app.ErrorResponse "重命名文件失败"
// @Router /api/files/{id} [patch]
func (h *RenameFileHandler) RenameFile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("RenameFile invalid user_id")
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	var req RenameFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.files.RenameFile(c.Request.Context(), service.RenameFileInput{
		UserID: userID,
		FileID: fileID,
		Name:   req.Name,
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrInvalidFileName):
		app.NewResponse(c).ToErrorResponse(errcode.ErrInvalidFileName)
		return
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
		return
	case errors.Is(err, service.ErrFileAlreadyExists):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileExists)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("rename file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrRenameFileFailed)
		return
	}

	app.NewResponse(c).ToResponse(RenameFileResponse{ID: result.ID, Name: result.Name})
}

type MoveFileHandler struct {
	files      *service.MoveFileService
	baselogger *logger.Logger
}

func NewMoveFileHandler(files *service.MoveFileService, baselogger *logger.Logger) *MoveFileHandler {
	return &MoveFileHandler{files: files, baselogger: baselogger}
}

type MoveFileRequest struct {
	FolderID uint64 `json:"folder_id" binding:"required" example:"20"`
}

type MoveFileResponse struct {
	ID       uint64 `json:"id" example:"1"`
	FolderID uint64 `json:"folder_id" example:"20"`
}

// MoveFile 将当前用户拥有的逻辑文件移动到新的目录。
// @Summary 移动文件
// @Description 将当前用户拥有的逻辑文件移动到新的目录，目标目录下不能存在同名文件。
// @Tags 文件
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件 ID"
// @Param request body MoveFileRequest true "目标目录"
// @Success 200 {object} MoveFileResponse "移动成功"
// @Failure 400 {object} app.ErrorResponse "请求参数不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件或目标目录不存在或无权访问"
// @Failure 409 {object} app.ErrorResponse "目标目录下已存在同名文件"
// @Failure 500 {object} app.ErrorResponse "移动文件失败"
// @Router /api/files/{id}/move [patch]
func (h *MoveFileHandler) MoveFile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("MoveFile invalid user_id")
		return
	}

	fileID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || fileID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	var req MoveFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.files.MoveFile(c.Request.Context(), service.MoveFileInput{
		UserID:   userID,
		FileID:   fileID,
		FolderID: req.FolderID,
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFileNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileNotFound)
		return
	case errors.Is(err, service.ErrFileFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotFound)
		return
	case errors.Is(err, service.ErrFileAlreadyExists):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFileExists)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("move file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrMoveFileFailed)
		return
	}

	app.NewResponse(c).ToResponse(MoveFileResponse{ID: result.ID, FolderID: result.FolderID})
}
