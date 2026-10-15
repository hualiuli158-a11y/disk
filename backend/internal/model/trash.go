package model

import "time"

const (
	TrashResourceFile   = "file"
	TrashResourceFolder = "folder"
)

type TrashEntry struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement"`
	UserID           uint64    `gorm:"not null;index"`
	User             User      `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ResourceType     string    `gorm:"type:varchar(16);not null;index"`
	ResourceID       uint64    `gorm:"not null;index"`
	OriginalParentID *uint64   `gorm:"index"`
	Name             string    `gorm:"type:varchar(255);not null"`
	TrashedAt        time.Time `gorm:"not null;index"`
	ExpiresAt        time.Time `gorm:"not null;index"`
}
