package model

import "time"

// Folder 表示一个真实存在的目录。
// 创建用户时应同时为其创建一个根目录，根目录的 ParentID 为 nil。
type Folder struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement"`
	UserID    uint64    `gorm:"not null;uniqueIndex:idx_user_parent_folder_name,priority:1"`
	User      User      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ParentID  *uint64   `gorm:"uniqueIndex:idx_user_parent_folder_name,priority:2;index"`
	Parent    *Folder   `gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Name      string    `gorm:"type:varchar(255);not null;uniqueIndex:idx_user_parent_folder_name,priority:3"`
	CreatedAt time.Time `gorm:"autoCreateTime;not null"`
	UpdatedAt time.Time `gorm:"autoUpdateTime;not null"`
	Children  []Folder  `gorm:"foreignKey:ParentID"`
	Files     []File    `gorm:"foreignKey:FolderID"`
}

//// Folder 表示一个文件夹，同时也是目录树中的一个节点。
//type Folder struct {
//	// 文件夹主键，由数据库自动递增生成。
//	ID uint64 `gorm:"primaryKey;autoIncrement"`
//
//	// 文件夹所属用户的 ID。
//	// 用于隔离不同用户的文件夹数据。
//	UserID uint64 `gorm:"not null;uniqueIndex:idx_user_parent_folder_name,priority:1"`
//
//	// UserID 对应的用户对象。
//	// 删除用户时，该用户的文件夹也会被级联删除。
//	User User `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
//
//	// 父文件夹 ID。
//	// 普通文件夹指向其父目录；根目录没有父目录，因此为 nil。
//	ParentID *uint64 `gorm:"uniqueIndex:idx_user_parent_folder_name,priority:2;index"`
//
//	// ParentID 对应的父文件夹对象。
//	// 如果当前文件夹还有子目录，数据库会限制父文件夹被直接删除。
//	Parent *Folder `gorm:"foreignKey:ParentID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
//
//	// 文件夹名称，最长 255 个字符，不能为空。
//	// UserID、ParentID、Name 组成联合唯一索引：
//	// 同一用户的同一父目录下不能出现两个同名文件夹。
//	Name string `gorm:"type:varchar(255);not null;uniqueIndex:idx_user_parent_folder_name,priority:3"`
//
//	// 文件夹创建时间，新增记录时由 GORM 自动填写。
//	CreatedAt time.Time `gorm:"autoCreateTime;not null"`
//
//	// 文件夹最后更新时间，更新记录时由 GORM 自动刷新。
//	UpdatedAt time.Time `gorm:"autoUpdateTime;not null"`
//
//	// 当前文件夹下面的直接子文件夹。
//	// 通过子文件夹的 ParentID 关联，不是 folders 表中的独立字段。
//	Children []Folder `gorm:"foreignKey:ParentID"`
//
//	// 当前文件夹中的文件。
//	// 通过 File 模型的 FolderID 关联，不是 folders 表中的独立字段。
//	Files []File `gorm:"foreignKey:FolderID"`
//}
