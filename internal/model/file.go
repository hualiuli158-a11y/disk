package model

import (
	"time"

	"gorm.io/gorm"
)

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
// 文件进入回收站时使用软删除；deletion_key 使软删除后可以创建同名文件。
type File struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement"`
	UserID         uint64         `gorm:"not null;uniqueIndex:idx_active_file_name,priority:1"`
	User           User           `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	FolderID       uint64         `gorm:"not null;uniqueIndex:idx_active_file_name,priority:2;index"`
	Folder         Folder         `gorm:"foreignKey:FolderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	PhysicalFileID uint64         `gorm:"not null;index"`
	PhysicalFile   PhysicalFile   `gorm:"foreignKey:PhysicalFileID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
	Name           string         `gorm:"type:varchar(255);not null;uniqueIndex:idx_active_file_name,priority:3"`
	DeletionKey    string         `gorm:"type:varchar(64);not null;default:'';uniqueIndex:idx_active_file_name,priority:4"`
	TrashID        *uint64        `gorm:"index"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
	CreatedAt      time.Time      `gorm:"autoCreateTime;not null"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime;not null"`
}
