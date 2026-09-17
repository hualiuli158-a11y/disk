package errcode

import (
	"fmt"
	"net/http"
)

type Error struct {
	code    int
	msg     string
	details []string
}

var codes = map[int]string{}

func NewError(code int, msg string) *Error {
	if _, ok := codes[code]; ok {
		panic(fmt.Sprintf("错误码%d已经存在，请更换一个", code))
	}
	codes[code] = msg
	return &Error{code: code, msg: msg}
}

func (e *Error) Error() string {
	return fmt.Sprintf("错误码：%d,错误信息::%s", e.Code(), e.Msg())
}

func (e *Error) Code() int {
	return e.code
}

func (e *Error) Msg() string {
	return e.msg
}

func (e *Error) Msgf(args ...any) string {
	return fmt.Sprintf(e.msg, args...)
}

func (e *Error) Details() []string {
	return e.details
}

func (e *Error) WithDetails(details ...string) *Error {
	newError := *e
	newError.details = []string{}
	for _, d := range details {
		newError.details = append(newError.details, d)
	}
	return &newError
}

func (e *Error) StatusCode() int {
	switch e.Code() {
	case Success.Code():
		return http.StatusOK
	case ServerError.Code():
		return http.StatusInternalServerError
	case InvalidParams.Code():
		return http.StatusBadRequest
	case NotFound.Code():
		return http.StatusNotFound
	case ErrNotFoundUser.Code():
		return http.StatusNotFound
	case InvalidCredentials.Code():
		return http.StatusUnauthorized
	case AuthenticationRequired.Code():
		return http.StatusUnauthorized
	case UsernameAlreadyExists.Code():
		return http.StatusConflict
	case ErrInvalidFolderName.Code():
		return http.StatusBadRequest
	case ErrParentNotFound.Code():
		return http.StatusNotFound
	case ErrFolderExists.Code():
		return http.StatusConflict
	case ErrCreateFolderFailed.Code():
		return http.StatusInternalServerError
	case ErrRootFolderNotFound.Code():
		return http.StatusNotFound
	case ErrListFoldersFailed.Code():
		return http.StatusInternalServerError
	case ErrFolderNotFound.Code():
		return http.StatusNotFound
	case ErrRenameFolderFailed.Code():
		return http.StatusInternalServerError
	case ErrCannotDeleteRoot.Code():
		return http.StatusConflict
	case ErrFolderNotEmpty.Code():
		return http.StatusConflict
	case ErrDeleteFolderFailed.Code():
		return http.StatusInternalServerError
	case ErrCannotMoveRoot.Code():
		return http.StatusConflict
	case ErrInvalidFolderMove.Code():
		return http.StatusConflict
	case ErrMoveFolderFailed.Code():
		return http.StatusInternalServerError
	case ErrInvalidFileName.Code():
		return http.StatusBadRequest
	case ErrUploadFolderNotFound.Code():
		return http.StatusNotFound
	case ErrFileExists.Code():
		return http.StatusConflict
	case ErrFileTooLarge.Code():
		return http.StatusRequestEntityTooLarge
	case ErrUploadFileFailed.Code():
		return http.StatusInternalServerError
	case ErrListFilesFailed.Code():
		return http.StatusInternalServerError
	case ErrFileNotFound.Code():
		return http.StatusNotFound
	case ErrDownloadFileFailed.Code():
		return http.StatusInternalServerError
	case ErrDeleteFileFailed.Code():
		return http.StatusInternalServerError
	case ErrRenameFileFailed.Code():
		return http.StatusInternalServerError
	case UnauthorizedAuthNotExist.Code():
		fallthrough
	case UnauthorizedTokenError.Code():
		fallthrough
	case UnauthorizedTokenGenrate.Code():
		fallthrough
	case UnauthorizedTokenTimeout.Code():
		return http.StatusUnauthorized
	case TooManyRequests.Code():
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}
