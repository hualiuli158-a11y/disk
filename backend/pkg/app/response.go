package app

import (
	"disk/pkg/errcode"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Response struct {
	Ctx *gin.Context
}

// ErrorResponse 是所有 HTTP 错误响应共用的数据结构。
type ErrorResponse struct {
	Code    int      `json:"code" example:"20002"`
	Msg     string   `json:"msg" example:"用户名或密码错误"`
	Details []string `json:"details,omitempty"`
}

func NewResponse(c *gin.Context) *Response {
	return &Response{Ctx: c}
}

func (r *Response) ToResponse(data any) {
	if data == nil {
		data = gin.H{}
	}
	r.Ctx.JSON(http.StatusOK, data)
}

//func (r *Response) ToResponseList(list any, totalRows int) {
//	r.Ctx.JSON(http.StatusOK, gin.H{})
//}

func (r *Response) ToErrorResponse(err *errcode.Error) {
	response := ErrorResponse{
		Code: err.Code(),
		Msg:  err.Msg(),
	}
	details := err.Details()
	if len(details) > 0 {
		response.Details = details
	}
	r.Ctx.JSON(err.StatusCode(), response)
}
