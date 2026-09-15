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
	ParentID uint64 `json:"parent_id" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

type CreateFolderResponse struct {
	ID        uint64    `json:"id"`
	ParentID  uint64    `json:"parent_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// @Description 为一个用户创建子目录
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
