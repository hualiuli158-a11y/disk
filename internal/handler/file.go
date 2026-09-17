package handler

import (
	"errors"
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
