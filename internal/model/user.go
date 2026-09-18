package model

import "time"

const DefaultQuotaLimit int64 = 10 * 1024 * 1024 * 1024 // 10 GiB

type User struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement;not null" json:"id"`
	Username   string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	CreateTime time.Time `gorm:"autoCreateTime;not null" json:"create_time"`
	Password   string    `gorm:"type:varchar(255);not null" json:"-"`
	UsedQuota  int64     `gorm:"not null;default:0" json:"used_quota"`
	QuotaLimit int64     `gorm:"not null;default:10737418240" json:"quota_limit"`
}
