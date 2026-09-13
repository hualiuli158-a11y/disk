package errcode

var (
	ErrNotFoundUser    = NewError(20001, "用户不存在")
	InvalidCredentials = NewError(20002, "用户名或密码错误")

	UsernameAlreadyExists = NewError(20003, "用户名已存在")

	AuthenticationRequired = NewError(20004, "未提供有效的身份凭证")
)
