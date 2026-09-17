package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

// 为父目录创建子目录
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

type ListFoldersHandler struct {
	folders    *service.ListFoldersService
	baselogger *logger.Logger
}

func NewListFoldersHandler(folders *service.ListFoldersService, baselogger *logger.Logger) *ListFoldersHandler {
	return &ListFoldersHandler{folders: folders, baselogger: baselogger}
}

type FolderListItemResponse struct {
	ID        uint64    `json:"id"`
	ParentID  *uint64   `json:"parent_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type ListFoldersResponse struct {
	Root     FolderListItemResponse   `json:"root"`
	Children []FolderListItemResponse `json:"children"`
}

// ListFolders 返回当前用户的根目录及其直接子目录。
// @Summary 获取根目录的子目录
// @Description 返回当前登录用户的根目录信息及其直接子目录，不递归加载更深层级。
// @Tags 文件夹
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ListFoldersResponse "查询成功"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "根目录不存在"
// @Failure 500 {object} app.ErrorResponse "获取目录列表失败"
// @Router /api/folders/root/children [get]
func (h *ListFoldersHandler) ListFolders(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("ListFolders invalid user_id")
		return
	}

	result, err := h.folders.ListFolders(c.Request.Context(), userID)
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrRootFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrRootFolderNotFound)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("ListFolder failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrListFoldersFailed)
		return
	}

	children := make([]FolderListItemResponse, 0, len(result.Children))
	for _, child := range result.Children {
		children = append(children, FolderListItemResponse{
			ID:        child.ID,
			ParentID:  child.ParentID,
			Name:      child.Name,
			CreatedAt: child.CreatedAt,
		})
	}

	app.NewResponse(c).ToResponse(ListFoldersResponse{
		Root: FolderListItemResponse{
			ID:        result.Root.ID,
			ParentID:  result.Root.ParentID,
			Name:      result.Root.Name,
			CreatedAt: result.Root.CreatedAt,
		},
		Children: children,
	})
}

type RenameFolderHandler struct {
	folders    *service.RenameFolderService
	baselogger *logger.Logger
}

func NewRenameFolderHandler(folders *service.RenameFolderService, baselogger *logger.Logger) *RenameFolderHandler {
	return &RenameFolderHandler{folders: folders, baselogger: baselogger}
}

type RenameFolderRequest struct {
	Name string `json:"name" binding:"required"`
}

type RenameFolderResponse struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// RenameFolder 重命名当前用户拥有的非根目录。
// @Summary 重命名文件夹
// @Description 重命名当前用户拥有的非根目录。文件夹名称遵循创建目录时的相同规则，同一父目录下不能存在同名文件夹。
// @Tags 文件夹
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件夹 ID"
// @Param request body RenameFolderRequest true "新的文件夹名称"
// @Success 200 {object} RenameFolderResponse "重命名成功"
// @Failure 400 {object} app.ErrorResponse "请求参数或文件夹名称不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件夹不存在、无权访问或为根目录"
// @Failure 409 {object} app.ErrorResponse "同一父目录下已存在同名文件夹"
// @Failure 500 {object} app.ErrorResponse "重命名文件夹失败"
// @Router /api/folders/{id} [patch]
func (r *RenameFolderHandler) RenameFolder(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		r.baselogger.WithContext(c.Request.Context()).Errorf("RenameFolder invalid User_id")
		return
	}

	folderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || folderID == 0 {
		r.baselogger.WithContext(c.Request.Context()).Errorf("parse folder id failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	var req RenameFolderRequest
	if err = c.ShouldBindJSON(&req); err != nil {
		r.baselogger.WithContext(c.Request.Context()).Errorf("RenameFolder invalid request body")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	result, err := r.folders.RenameFolder(c.Request.Context(), service.RenameFolderInput{
		UserID:   userID,
		FolderID: folderID,
		Name:     req.Name,
	})
	switch {
	case errors.Is(err, service.ErrInvalidFolderName):
		app.NewResponse(c).ToErrorResponse(errcode.ErrInvalidFolderName)
		return
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotFound)
		return
	case errors.Is(err, service.ErrFolderAlreadyExists):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderExists)
		return
	case err != nil:
		r.baselogger.WithContext(c.Request.Context()).Errorf("folder.RenameFolder failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrRenameFolderFailed)
		return
	}

	app.NewResponse(c).ToResponse(RenameFolderResponse{
		ID:   result.ID,
		Name: result.Name,
	})
}

type ListFolderChildrenHandler struct {
	folders    *service.ListFolderChildrenService
	baselogger *logger.Logger
}

func NewListFolderChildrenHandler(folders *service.ListFolderChildrenService, baselogger *logger.Logger) *ListFolderChildrenHandler {
	return &ListFolderChildrenHandler{folders: folders, baselogger: baselogger}
}

type ListFolderChildrenResponse struct {
	Folder   FolderListItemResponse   `json:"folder"`
	Children []FolderListItemResponse `json:"children"`
}

// ListChildren 返回指定目录及其直接子目录。
// @Summary 获取指定目录的子目录
// @Description 返回当前用户拥有的指定目录及其直接子目录，不递归加载更深层级。
// @Tags 文件夹
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件夹 ID"
// @Success 200 {object} ListFolderChildrenResponse "查询成功"
// @Failure 400 {object} app.ErrorResponse "文件夹 ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件夹不存在或无权访问"
// @Failure 500 {object} app.ErrorResponse "获取目录列表失败"
// @Router /api/folders/{id}/children [get]
func (h *ListFolderChildrenHandler) ListChildren(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("ListChildren invalid user_id")
		return
	}

	folderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || folderID == 0 {
		h.baselogger.WithContext(c.Request.Context()).Errorf("parse folder id failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.folders.ListChildren(c.Request.Context(), service.ListFolderChildrenInput{
		UserID:   userID,
		FolderID: folderID,
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotFound)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("list folder children failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrListFoldersFailed)
		return
	}

	children := make([]FolderListItemResponse, 0, len(result.Children))
	for _, child := range result.Children {
		children = append(children, FolderListItemResponse{
			ID:        child.ID,
			ParentID:  child.ParentID,
			Name:      child.Name,
			CreatedAt: child.CreatedAt,
		})
	}

	app.NewResponse(c).ToResponse(ListFolderChildrenResponse{
		Folder: FolderListItemResponse{
			ID:        result.Folder.ID,
			ParentID:  result.Folder.ParentID,
			Name:      result.Folder.Name,
			CreatedAt: result.Folder.CreatedAt,
		},
		Children: children,
	})
}

type DeleteFolderHandler struct {
	folders    *service.DeleteFolderService
	baselogger *logger.Logger
}

func NewDeleteFolderHandler(folders *service.DeleteFolderService, baselogger *logger.Logger) *DeleteFolderHandler {
	return &DeleteFolderHandler{folders: folders, baselogger: baselogger}
}

// DeleteFolder 删除当前用户拥有的空目录。
// @Summary 删除空目录
// @Description 删除当前用户拥有的非根目录。目录中存在子目录或文件时不能删除。
// @Tags 文件夹
// @Produce json
// @Security BearerAuth
// @Param id path int true "文件夹 ID"
// @Success 204 "删除成功"
// @Failure 400 {object} app.ErrorResponse "文件夹 ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "文件夹不存在或无权访问"
// @Failure 409 {object} app.ErrorResponse "根目录不能删除或文件夹不为空"
// @Failure 500 {object} app.ErrorResponse "删除文件夹失败"
// @Router /api/folders/{id} [delete]
func (h *DeleteFolderHandler) DeleteFolder(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		h.baselogger.WithContext(c.Request.Context()).Errorf("DeleteFolder invalid user_id")
		return
	}

	folderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || folderID == 0 {
		h.baselogger.WithContext(c.Request.Context()).Errorf("parse folder id failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	err = h.folders.DeleteFolder(c.Request.Context(), service.DeleteFolderInput{
		UserID:   userID,
		FolderID: folderID,
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrFolderNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotFound)
		return
	case errors.Is(err, service.ErrCannotDeleteRoot):
		app.NewResponse(c).ToErrorResponse(errcode.ErrCannotDeleteRoot)
		return
	case errors.Is(err, service.ErrFolderNotEmpty):
		app.NewResponse(c).ToErrorResponse(errcode.ErrFolderNotEmpty)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("delete folder failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrDeleteFolderFailed)
		return
	}

	c.Status(http.StatusNoContent)
}
