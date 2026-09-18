package handler

import (
	"errors"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	search     *service.SearchService
	baselogger *logger.Logger
}

func NewSearchHandler(search *service.SearchService, baselogger *logger.Logger) *SearchHandler {
	return &SearchHandler{search: search, baselogger: baselogger}
}

type SearchResponse struct {
	Items      []ContentItemResponse `json:"items"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	Total      int64                 `json:"total"`
	TotalPages int64                 `json:"total_pages"`
	Query      string                `json:"query"`
	Type       string                `json:"type"`
	Sort       string                `json:"sort"`
	Order      string                `json:"order"`
}

// Search 搜索当前用户的文件和目录。
// @Summary 搜索文件和目录
// @Description 按名称搜索当前用户的文件和非根目录，支持类型筛选、分页和排序。
// @Tags 搜索
// @Produce json
// @Security BearerAuth
// @Param q query string true "搜索关键字"
// @Param type query string false "类型：all、folder、file，默认 all"
// @Param page query int false "页码，默认 1"
// @Param page_size query int false "每页数量，默认 50，最大 100"
// @Param sort query string false "排序字段：name、size、created_at、updated_at"
// @Param order query string false "排序方向：asc、desc"
// @Success 200 {object} SearchResponse "搜索成功"
// @Failure 400 {object} app.ErrorResponse "搜索参数不合法"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 500 {object} app.ErrorResponse "搜索失败"
// @Router /api/search [get]
func (h *SearchHandler) Search(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
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

	result, err := h.search.Search(c.Request.Context(), service.SearchInput{
		UserID: userID, Query: c.Query("q"), Kind: c.Query("type"), Page: page, PageSize: pageSize,
		SortField: c.Query("sort"), SortOrder: c.Query("order"),
	})
	switch {
	case errors.Is(err, service.ErrFolderUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	case errors.Is(err, service.ErrInvalidSearchQuery):
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("search failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ErrSearchFailed)
		return
	}

	items := make([]ContentItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, ContentItemResponse{
			ID: item.ID, Type: item.Kind, ParentID: item.ParentID, Name: item.Name,
			Size: item.Size, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	app.NewResponse(c).ToResponse(SearchResponse{
		Items: items, Page: result.Page, PageSize: result.PageSize, Total: result.Total,
		TotalPages: result.TotalPages, Query: result.Query, Type: result.Kind,
		Sort: result.SortField, Order: result.SortOrder,
	})
}
