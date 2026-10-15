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

type TrashHandler struct {
	trash      *service.TrashService
	baselogger *logger.Logger
}

func NewTrashHandler(trash *service.TrashService, baselogger *logger.Logger) *TrashHandler {
	return &TrashHandler{trash: trash, baselogger: baselogger}
}

type TrashItemResponse struct {
	ID               uint64    `json:"id"`
	Type             string    `json:"type"`
	ResourceID       uint64    `json:"resource_id"`
	OriginalParentID *uint64   `json:"original_parent_id"`
	Name             string    `json:"name"`
	TrashedAt        time.Time `json:"trashed_at"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type ListTrashResponse struct {
	Items      []TrashItemResponse `json:"items"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	Total      int64               `json:"total"`
	TotalPages int64               `json:"total_pages"`
}

// ListTrash 分页返回当前用户的回收站条目。
// @Summary 获取回收站
// @Tags 回收站
// @Produce json
// @Security BearerAuth
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 50，最大 100"
// @Success 200 {object} ListTrashResponse "查询成功"
// @Failure 400 {object} app.ErrorResponse "分页参数不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 500 {object} app.ErrorResponse "回收站操作失败"
// @Router /api/trash [get]
func (h *TrashHandler) ListTrash(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	page, err := parseOptionalPositiveInt(c.Query("page"), 1)
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	pageSize, err := parseOptionalPositiveInt(c.Query("page_size"), service.DefaultContentPageSize)
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.trash.ListTrash(c.Request.Context(), userID, page, pageSize)
	if errors.Is(err, service.ErrInvalidContentQuery) {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	if err != nil {
		h.baselogger.WithContext(c.Request.Context()).Errorf("list trash failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashOperationFailed)
		return
	}
	items := make([]TrashItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, TrashItemResponse{
			ID: item.ID, Type: item.ResourceType, ResourceID: item.ResourceID,
			OriginalParentID: item.OriginalParentID, Name: item.Name,
			TrashedAt: item.TrashedAt, ExpiresAt: item.ExpiresAt,
		})
	}
	app.NewResponse(c).ToResponse(ListTrashResponse{
		Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total, TotalPages: result.TotalPages,
	})
}

// RestoreTrash 恢复回收站条目。
// @Summary 恢复回收站条目
// @Tags 回收站
// @Security BearerAuth
// @Param id path int true "回收站条目 ID"
// @Success 204 "恢复成功"
// @Failure 400 {object} app.ErrorResponse "ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "回收站条目不存在"
// @Failure 409 {object} app.ErrorResponse "恢复位置存在同名资源"
// @Failure 500 {object} app.ErrorResponse "回收站操作失败"
// @Router /api/trash/{id}/restore [post]
func (h *TrashHandler) RestoreTrash(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	trashID, ok := parsePathID(c)
	if !ok {
		return
	}
	err := h.trash.RestoreTrash(c.Request.Context(), userID, trashID)
	switch {
	case errors.Is(err, service.ErrTrashNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashNotFound)
	case errors.Is(err, service.ErrTrashRestoreConflict):
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashRestoreConflict)
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("restore trash failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashOperationFailed)
	default:
		c.Status(http.StatusNoContent)
	}
}

// PermanentlyDeleteTrash 永久删除回收站条目。
// @Summary 永久删除回收站条目
// @Tags 回收站
// @Security BearerAuth
// @Param id path int true "回收站条目 ID"
// @Success 204 "永久删除成功"
// @Failure 400 {object} app.ErrorResponse "ID 不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "回收站条目不存在"
// @Failure 500 {object} app.ErrorResponse "回收站操作失败"
// @Router /api/trash/{id} [delete]
func (h *TrashHandler) PermanentlyDeleteTrash(c *gin.Context) {
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	trashID, ok := parsePathID(c)
	if !ok {
		return
	}
	result, err := h.trash.PermanentlyDeleteTrash(c.Request.Context(), userID, trashID)
	if errors.Is(err, service.ErrTrashNotFound) {
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashNotFound)
		return
	}
	if err != nil {
		h.baselogger.WithContext(c.Request.Context()).Errorf("permanently delete trash failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrTrashOperationFailed)
		return
	}
	if result.CleanupFailures > 0 {
		h.baselogger.WithContext(c.Request.Context()).Errorf("trash cleanup left %d orphan objects", result.CleanupFailures)
	}
	c.Status(http.StatusNoContent)
}

func authenticatedUserID(c *gin.Context) (uint64, bool) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return 0, false
	}
	return userID, true
}

func parsePathID(c *gin.Context) (uint64, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return 0, false
	}
	return id, true
}
