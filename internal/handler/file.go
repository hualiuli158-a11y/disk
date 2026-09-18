package handler

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type UploadFileHandler struct {
	files      *service.UploadFileService
	baselogger *logger.Logger
}

func NewUploadFileHandler(files *service.UploadFileService, baselogger *logger.Logger) *UploadFileHandler {
	return &UploadFileHandler{files: files, baselogger: baselogger}
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
// @Param folder_id formData int true "目标目录 ID"
// @Param file formData file true "待上传文件"
// @Success 200 {object} UploadFileResponse "上传成功"
// @Failure 400 {object} app.ErrorResponse "请求参数或文件名不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "目标目录不存在或无权访问"
// @Failure 409 {object} app.ErrorResponse "目标目录下已存在同名文件"
// @Failure 413 {object} app.ErrorResponse "文件超过上传大小限制"
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

	folderID, err := strconv.ParseUint(c.PostForm("folder_id"), 10, 64)
	if err != nil || folderID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	header, err := c.FormFile("file")
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	content, err := header.Open()
	if err != nil {
		h.baselogger.WithContext(c.Request.Context()).Errorf("open multipart file failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrUploadFileFailed)
		return
	}
	defer func() {
		if err := content.Close(); err != nil {
			h.baselogger.WithContext(c.Request.Context()).Errorf("close multipart file failed: %v", err)
		}
	}()

	result, err := h.files.UploadFile(c.Request.Context(), service.UploadFileInput{
		UserID:   userID,
		FolderID: folderID,
		Name:     header.Filename,
		Content:  content,
	})
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
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, result.Content); err != nil {
		// 响应头可能已经发送，此时不能再改写为 JSON 错误响应。
		h.baselogger.WithContext(c.Request.Context()).Errorf("stream downloaded file failed: %v", err)
	}
}

type DeleteFileHandler struct {
	files      *service.DeleteFileService
	baselogger *logger.Logger
}

func NewDeleteFileHandler(files *service.DeleteFileService, baselogger *logger.Logger) *DeleteFileHandler {
	return &DeleteFileHandler{files: files, baselogger: baselogger}
}

// DeleteFile 删除当前用户拥有的逻辑文件。
// @Summary 删除文件
// @Description 删除逻辑文件并扣减用户额度；物理文件没有其他引用时同时清理底层存储对象。
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

	result, err := h.files.DeleteFile(c.Request.Context(), userID, fileID)
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
	if result.CleanupError != nil {
		h.baselogger.WithContext(c.Request.Context()).Errorf("cleanup orphan storage object failed: %v", result.CleanupError)
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
