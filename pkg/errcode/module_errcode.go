package errcode

var (
	ErrNotFoundUser    = NewError(20001, "用户不存在")
	InvalidCredentials = NewError(20002, "用户名或密码错误")

	UsernameAlreadyExists = NewError(20003, "用户名已存在")

	AuthenticationRequired = NewError(20004, "未提供有效的身份凭证")
)

var (
	ErrInvalidFileName      = NewError(40001, "文件名不合法")
	ErrUploadFolderNotFound = NewError(40002, "上传目录不存在")
	ErrFileExists           = NewError(40003, "同名文件已存在")
	ErrFileTooLarge         = NewError(40004, "文件超过上传大小限制")
	ErrUploadFileFailed     = NewError(40005, "上传文件失败")
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
	ErrCannotMoveRoot     = NewError(30012, "根目录不能移动")
	ErrInvalidFolderMove  = NewError(30013, "不能将文件夹移动到自身或其子目录")
	ErrMoveFolderFailed   = NewError(30014, "移动文件夹失败")
)
