package model

import "time"

type User struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement;not null" json:"id"`
	Username   string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	CreateTime time.Time `gorm:"autoCreateTime;not null" json:"create_time"`
	Password   string    `gorm:"type:varchar(255);not null" json:"-"`
	UsedQuota  int64     `gorm:"not null;default:0" json:"used_quota"`
}
