package model

import "time"

// ContentItem 是目录内容查询使用的只读投影，不对应独立数据表。
type ContentItem struct {
	ID        uint64
	Kind      string
	ParentID  *uint64
	Name      string
	Size      *int64
	CreatedAt time.Time
	UpdatedAt time.Time
}
