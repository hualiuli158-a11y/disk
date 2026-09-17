package errcode

var (
	ErrNotFoundUser    = NewError(20001, "用户不存在")
	InvalidCredentials = NewError(20002, "用户名或密码错误")

	UsernameAlreadyExists = NewError(20003, "用户名已存在")

	AuthenticationRequired = NewError(20004, "未提供有效的身份凭证")
)

var (
	ErrInvalidFolderName  = NewError(30001, "文件夹名称不合法")
	ErrParentNotFound     = NewError(30002, "父文件夹不存在")
	ErrFolderExists       = NewError(30003, "文件夹已存在")
	ErrCreateFolderFailed = NewError(30004, "创建文件夹失败")
	ErrRootFolderNotFound = NewError(30005, "根目录不存在")
	ErrListFoldersFailed  = NewError(30006, "获取目录列表失败")
	ErrFolderNotFound     = NewError(30007, "文件夹不存在")
	ErrRenameFolderFailed = NewError(30008, "重命名文件夹失败")
	ErrCannotDeleteRoot   = NewError(30009, "根目录不能删除")
	ErrFolderNotEmpty     = NewError(30010, "文件夹不为空")
	ErrDeleteFolderFailed = NewError(30011, "删除文件夹失败")
)
