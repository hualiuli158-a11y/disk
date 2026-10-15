package handler

import (
	"errors"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type QuotaHandler struct {
	quota      *service.QuotaService
	baselogger *logger.Logger
}

func NewQuotaHandler(quota *service.QuotaService, baselogger *logger.Logger) *QuotaHandler {
	return &QuotaHandler{quota: quota, baselogger: baselogger}
}

type QuotaResponse struct {
	Used      int64 `json:"used" example:"1048576"`
	Limit     int64 `json:"limit" example:"10737418240"`
	Remaining int64 `json:"remaining" example:"10736369664"`
}

// GetQuota 返回当前用户的存储额度。
// @Summary 获取存储额度
// @Description 返回当前用户的已用、总量和剩余额度。
// @Tags 用户
// @Produce json
// @Security BearerAuth
// @Success 200 {object} QuotaResponse "查询成功"
// @Failure 401 {object} app.ErrorResponse "未提供有效的身份凭证"
// @Failure 404 {object} app.ErrorResponse "用户不存在"
// @Failure 500 {object} app.ErrorResponse "服务内部错误"
// @Router /api/quota [get]
func (h *QuotaHandler) GetQuota(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}

	result, err := h.quota.GetQuota(c.Request.Context(), userID)
	switch {
	case errors.Is(err, service.ErrQuotaUserNotFound):
		app.NewResponse(c).ToErrorResponse(errcode.ErrNotFoundUser)
		return
	case err != nil:
		h.baselogger.WithContext(c.Request.Context()).Errorf("get quota failed: %v", err)
		app.NewResponse(c).ToErrorResponse(errcode.ServerError)
		return
	}

	app.NewResponse(c).ToResponse(QuotaResponse{
		Used: result.Used, Limit: result.Limit, Remaining: result.Remaining,
	})
}
