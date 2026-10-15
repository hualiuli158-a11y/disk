package handler

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"
	"disk/pkg/logger"

	"github.com/gin-gonic/gin"
)

type LoginHandler struct {
	loginService *service.LoginService
	logger       *logger.Logger
	attempts     LoginAttemptProtector
}

type LoginAttemptProtector interface {
	Allow(key string) (bool, time.Duration)
	Failure(key string)
	Success(key string)
}

func NewLoginHandler(loginService *service.LoginService, baseLogger *logger.Logger, attempts LoginAttemptProtector) *LoginHandler {
	return &LoginHandler{loginService: loginService,
		logger: baseLogger.WithFields(logger.Fields{"module": "Login_Handler"}), attempts: attempts}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required" example:"alice"`
	Password string `json:"password" binding:"required" example:"password123"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
}

// Login 用户登录
// @Summary 用户登录
// @Description 使用用户名和密码完成身份认证，成功后返回访问令牌。用户名不存在和密码错误统一返回“用户名或密码错误”。
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body LoginRequest true "登录信息"
// @Success 200 {object} LoginResponse "登录成功"
// @Failure 400 {object} app.ErrorResponse "请求参数错误"
// @Failure 401 {object} app.ErrorResponse "用户名或密码错误"
// @Failure 429 {object} app.ErrorResponse "请求过多或登录暂时封禁"
// @Failure 500 {object} app.ErrorResponse "服务内部错误"
// @Router /login [post]
func (h *LoginHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.WithFields(logger.Fields{
			"error": err.Error(),
		}).Warn("invalid login request")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	attemptKey := c.ClientIP() + "\x00" + strings.ToLower(strings.TrimSpace(req.Username))
	if h.attempts != nil {
		if allowed, retryAfter := h.attempts.Allow(attemptKey); !allowed {
			seconds := int(math.Ceil(retryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			app.NewResponse(c).ToErrorResponse(errcode.TooManyRequests)
			return
		}
	}
	result, err := h.loginService.Login(c.Request.Context(), service.LoginInput{
		Username: req.Username,
		Password: req.Password,
	})
	if errors.Is(err, service.ErrInvalidCredentials) {
		if h.attempts != nil {
			h.attempts.Failure(attemptKey)
		}
		h.logger.WithFields(logger.Fields{
			"username": req.Username,
		}).Warn("login rejected")
		app.NewResponse(c).ToErrorResponse(errcode.InvalidCredentials)
		return
	}
	if err != nil {
		h.logger.WithFields(logger.Fields{
			"error":    err.Error(),
			"username": req.Username,
		}).Error("login failed")
		app.NewResponse(c).ToErrorResponse(errcode.ServerError)
		return
	}
	if h.attempts != nil {
		h.attempts.Success(attemptKey)
	}

	h.logger.WithFields(logger.Fields{
		"username": req.Username,
	}).Info("login succeeded")
	app.NewResponse(c).ToResponse(LoginResponse{
		AccessToken: result.AccessToken,
	})
}
