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
