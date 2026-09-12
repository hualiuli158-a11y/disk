package model

import "time"

// PhysicalFile 表示存储后端中实际保存的一份物理文件。
// 这里只保存内部存储元数据，不应直接序列化后作为 API 响应返回。
type PhysicalFile struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement"`
	SHA256       string    `gorm:"type:char(64);uniqueIndex;not null"`
	Size         int64     `gorm:"not null"`
	StorageKey   string    `gorm:"type:varchar(500);uniqueIndex;not null"`
	ThumbnailKey *string   `gorm:"type:varchar(500)"`
	CreatedAt    time.Time `gorm:"autoCreateTime;not null"`
}

// File 表示用户能够看到的逻辑文件。
// 多个逻辑文件可以引用同一个 PhysicalFile，以支持文件去重和秒传。
//
// 当前第一版有意采用硬删除。以后增加 gorm.DeletedAt 和回收站功能时，
// 需要同时设计软删除文件与同名新文件之间的唯一索引策略。
type File struct {
	ID             uint64       `gorm:"primaryKey;autoIncrement"`
	UserID         uint64       `gorm:"not null;uniqueIndex:idx_user_folder_file_name,priority:1"`
	User           User         `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	FolderID       uint64       `gorm:"not null;uniqueIndex:idx_user_folder_file_name,priority:2;index"`
	Folder         Folder       `gorm:"foreignKey:FolderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	PhysicalFileID uint64       `gorm:"not null;index"`
	PhysicalFile   PhysicalFile `gorm:"foreignKey:PhysicalFileID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Name           string       `gorm:"type:varchar(255);not null;uniqueIndex:idx_user_folder_file_name,priority:3"`
	CreatedAt      time.Time    `gorm:"autoCreateTime;not null"`
	UpdatedAt      time.Time    `gorm:"autoUpdateTime;not null"`
}
