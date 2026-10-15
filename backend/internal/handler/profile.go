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

type ProfileHandler struct {
	profile *service.ProfileService
	logger  *logger.Logger
}

func NewProfileHandler(profile *service.ProfileService, baseLogger *logger.Logger) *ProfileHandler {
	return &ProfileHandler{
		profile: profile,
		logger:  baseLogger.WithFields(logger.Fields{"module": "Profile_Handler"}),
	}
}

type ProfileResponse struct {
	ID         uint64    `json:"id" example:"42"`
	Username   string    `json:"username" example:"alice"`
	CreateTime time.Time `json:"create_time" example:"2026-09-13T10:00:00+08:00"`
	UsedQuota  int64     `json:"used_quota" example:"1048576"`
	QuotaLimit int64     `json:"quota_limit" example:"10737418240"`
}

// Profile 获取用户基本信息
// @Summary 获取用户基本信息
// @Description 根据已通过 JWT 鉴权的用户 ID 获取当前用户的基本信息和已用存储配额
// @Tags 认证
// @Produce json
// @Security BearerAuth
// @Success 200 {object} ProfileResponse "获取成功"
// @Failure 401 {object} app.ErrorResponse "身份信息不存在或无效"
// @Failure 404 {object} app.ErrorResponse "用户不存在"
// @Failure 500 {object} app.ErrorResponse "服务内部错误"
// @Router /api/profile [get]
func (p *ProfileHandler) Profile(c *gin.Context) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint64)
	if !exists || !valid || userID == 0 {
		p.logger.WithFields(logger.Fields{
			"exists": exists,
			"type":   fmt.Sprintf("%T", value),
		}).Warn("profile request has no valid user ID")
		app.NewResponse(c).ToErrorResponse(errcode.AuthenticationRequired)
		return
	}

	result, err := p.profile.Profile(c.Request.Context(), userID)
	switch {
	case errors.Is(err, service.ErrProfileNotFound):
		p.logger.WithFields(logger.Fields{"user_id": userID}).Warn("profile user not found")
		app.NewResponse(c).ToErrorResponse(errcode.ErrNotFoundUser)
		return
	case err != nil:
		p.logger.WithFields(logger.Fields{
			"error":   err.Error(),
			"user_id": userID,
		}).Error("get profile failed")
		app.NewResponse(c).ToErrorResponse(errcode.ServerError)
		return
	}

	p.logger.WithFields(logger.Fields{"user_id": userID}).Info("profile retrieved")
	app.NewResponse(c).ToResponse(ProfileResponse{
		ID:         result.ID,
		Username:   result.Username,
		CreateTime: result.CreateTime,
		UsedQuota:  result.UsedQuota,
		QuotaLimit: result.QuotaLimit,
	})
}
