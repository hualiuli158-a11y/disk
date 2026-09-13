package handler

import (
	"errors"
	"fmt"
	"time"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type FoldersHandler struct {
	folders *service.FoldersCreateService
	logger  *logger.Logger
}

type CreateRootFolderResponse struct {
	ID        uint64    `json:"id" example:"1"`
	UserID    uint64    `json:"user_id" example:"42"`
	Name      string    `json:"name" example:"/"`
	CreatedAt time.Time `json:"created_at" example:"2026-09-13T10:00:00+08:00"`
}

func NewFoldersHandler(folders *service.FoldersCreateService, baseLogger *logger.Logger) *FoldersHandler {
	return &FoldersHandler{
		folders: folders,
		logger:  baseLogger.WithFields(logger.Fields{"module": "Folders_Handler"}),
	}
}

// CreateFolder 为当前登录用户创建根目录。
// @Summary 创建用户根目录
// @Description 根据 JWT 中的用户 ID 创建根目录；每个用户只能创建一个根目录。
// @Tags 文件夹
// @Produce json
// @Security BearerAuth
// @Success 200 {object} CreateRootFolderResponse "创建成功"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "用户不存在"
// @Failure 409 {object} app.ErrorResponse "根目录已存在"
// @Failure 500 {object} app.ErrorResponse "创建文件夹失败"
// @Router /api/folders/root [post]
func (h *FoldersHandler) CreateFolder(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		h.logger.WithFields(logger.Fields{
			"exists": exists,
			"type":   fmt.Sprintf("%T", value),
		}).Warn("folder request has no valid user ID")
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}

	result, err := h.folders.CreateFolder(c.Request.Context(), userID)
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		h.logger.WithFields(logger.Fields{"user_id": userID}).Warn("folder user not found")
		app.NewResponse(c).ToErrorResponse(errcode.ErrNotFoundUser)
		return
	case errors.Is(err, service.ErrRootFolderAlreadyExists):
		h.logger.WithFields(logger.Fields{"user_id": userID}).Warn("root folder already exists")
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderExists)
		return
	case err != nil:
		h.logger.WithFields(logger.Fields{
			"error":   err.Error(),
			"user_id": userID,
		}).Error("create root folder failed")
		app.NewResponse(c).ToErrorResponse(errcode.ErrCreateFolderFailed)
		return
	}

	h.logger.WithFields(logger.Fields{
		"folder_id": result.ID,
		"user_id":   userID,
	}).Info("create root folder succeeded")
	app.NewResponse(c).ToResponse(CreateRootFolderResponse{
		ID:        result.ID,
		UserID:    result.UserID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt,
	})
}
