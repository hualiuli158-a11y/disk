package handler

import (
	"errors"

	"disk/internal/service"
	"disk/pkg/app"
	"disk/pkg/errcode"

	"github.com/gin-gonic/gin"
)

type LoginHandler struct {
	loginService *service.LoginService
}

func NewLoginHandler(loginService *service.LoginService) *LoginHandler {
	return &LoginHandler{loginService: loginService}
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
// @Failure 500 {object} app.ErrorResponse "服务内部错误"
// @Router /login [post]
func (h *LoginHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidParams)
		return
	}
	result, err := h.loginService.Login(c.Request.Context(), service.LoginInput{
		Username: req.Username,
		Password: req.Password,
	})
	if errors.Is(err, service.ErrInvalidCredentials) {
		app.NewResponse(c).ToErrorResponse(errcode.InvalidCredentials)
		return
	}
	if err != nil {
		app.NewResponse(c).ToErrorResponse(errcode.ServerError)
		return
	}

	app.NewResponse(c).ToResponse(LoginResponse{
		AccessToken: result.AccessToken,
	})
}
