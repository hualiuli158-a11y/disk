package handler

import (
	"errors"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type RegisterHandler struct {
	registerService *service.RegisterService
	logger          *logger.Logger
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required" example:"password123"`
}

type RegisterResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

func NewRegisterHandler(registerService *service.RegisterService, baseLogger *logger.Logger) *RegisterHandler {
	return &RegisterHandler{
		registerService: registerService,
		logger:          baseLogger.WithFields(logger.Fields{"module": "Register_Handler"}),
	}
}

// Register 用户注册。
// @Summary 用户注册
// @Description 使用用户名和密码创建用户,同时为用户创建文件根目录,成功后返回访问令牌。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "注册信息"
// @Success 200 {object} RegisterResponse "注册成功"
// @Failure 400 {object} app.ErrorResponse "请求参数错误"
// @Failure 409 {object} app.ErrorResponse "用户名已存在"
// @Failure 500 {object} app.ErrorResponse "服务内部错误"
// @Router /register [post]
func (h *RegisterHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithFields(logger.Fields{"error": err.Error()}).Warn("invalid register request")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}

	result, err := h.registerService.Register(c.Request.Context(), service.RegisterInput{
		Username: req.Username,
		Password: req.Password,
	})
	switch {
	case errors.Is(err, service.ErrInvalidRegisterInput):
		h.logger.WithFields(logger.Fields{"username": req.Username}).Warn("invalid register input")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	case errors.Is(err, service.ErrUsernameAlreadyExists):
		h.logger.WithFields(logger.Fields{"username": req.Username}).Warn("register rejected: username already exists")
		app.NewResponse(c).ToErrorResponse(errcode.UsernameAlreadyExists)
		return
	case err != nil:
		h.logger.WithFields(logger.Fields{
			"error":    err.Error(),
			"username": req.Username,
		}).Error("register failed")
		app.NewResponse(c).ToErrorResponse(errcode.ServerError)
		return
	}

	h.logger.WithFields(logger.Fields{"username": req.Username}).Info("register succeeded")
	app.NewResponse(c).ToResponse(RegisterResponse{AccessToken: result.AccessToken})
}
