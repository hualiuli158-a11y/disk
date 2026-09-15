package handler

import (
	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
)

type ChildFoldersCreateHandler struct {
	folders    *service.ChildFoldersCreateService
	baselogger *logger.Logger
}

func NewChildFoldersCreateHandler(c *service.ChildFoldersCreateService, baselogger *logger.Logger) *ChildFoldersCreateHandler {
	return &ChildFoldersCreateHandler{folders: c,
		baselogger: baselogger}
}

type CreateFolderRequest struct {
	ParentID uint64 `json:"parent_id" binding:"required" example:"1"`
	Name     string `json:"name" binding:"required" example:"旅行照片"`
}

type CreateFolderResponse struct {
	ID        uint64    `json:"id" example:"2"`
	ParentID  uint64    `json:"parent_id" example:"1"`
	Name      string    `json:"name" example:"旅行照片"`
	CreatedAt time.Time `json:"created_at" example:"2026-09-15T10:30:00+08:00"`
}

// CreateChildFolder 创建子目录。
// @Summary 创建子目录
// @Description 在当前登录用户拥有的父文件夹下创建子目录。文件夹名称不能为空，不能是“/”、“.”或“..”，也不能包含“/”或“\”；同一父目录下不能存在同名文件夹。
// @Tags 文件夹
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateFolderRequest true "子目录信息"
// @Success 200 {object} CreateFolderResponse "创建成功"
// @Failure 400 {object} app.ErrorResponse "请求参数或文件夹名称不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "父文件夹不存在"
// @Failure 409 {object} app.ErrorResponse "同一父目录下已存在同名文件夹"
// @Failure 500 {object} app.ErrorResponse "创建文件夹失败"
// @Router /api/folders [post]
func (ch *ChildFoldersCreateHandler) CreateChildFolder(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		ch.baselogger.WithContext(c.Request.Context()).Errorf("CreateChildFolder invalid user_id")
		return
	}
	var req CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		ch.baselogger.WithContext(c.Request.Context()).Errorf("CreateChildFolder invalid request body")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	result, err := ch.folders.CreateChildFolder(c.Request.Context(), service.CreateFolderInput{
		UserID:   userID,
		ParentID: req.ParentID,
		Name:     req.Name,
	})
	switch {
	case errors.Is(err, service.ErrInvalidFolderName):
		app.NewResponse(c).ToErrorResponse(errcode.ErrInvalidFolderName)
		return

	case errors.Is(err, service.ErrParentNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrParentNotFound)
		return

	case errors.Is(err, service.ErrFolderAlreadyExists):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderExists)
		return

	case err != nil:
		ch.baselogger.WithContext(c.Request.Context()).Errorf("create child folder failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrCreateFolderFailed)
		return
	}

	app.NewResponse(c).ToResponse(CreateFolderResponse{
		ID:        result.ID,
		ParentID:  result.ParentID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
	})
	return
}
