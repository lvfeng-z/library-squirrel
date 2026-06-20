package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// RecycleItem 回收站条目（作品逻辑删除快照）
// 逻辑删除作品时，将作品的全部周边数据序列化为快照存入此处，资源文件另行移入 Backup 目录
// 冗余字段仅供回收站列表展示与 TTL 筛选，复原只依赖 Snapshot
type RecycleItem struct {
	*model.BaseEntity
	WorkID     sql.NullInt64  `gorm:"column:work_id" json:"workId"`               // 原作品 ID（溯源，不参与复原）
	SiteID     sql.NullInt64  `gorm:"column:site_id;index" json:"siteId"`         // 冗余，列表筛选
	SiteWorkID sql.NullString `gorm:"column:site_work_id" json:"siteWorkId"`      // 冗余，列表展示
	WorkName   sql.NullString `gorm:"column:work_name" json:"workName"`           // 冗余，列表展示
	Thumbnail  sql.NullString `gorm:"column:thumbnail" json:"thumbnail"`          // 冗余缩略图 backup 路径，列表展示
	DeleteTime int64          `gorm:"column:delete_time;index" json:"deleteTime"` // 逻辑删除时间（毫秒时间戳，TTL 依据）
	Snapshot   string         `gorm:"column:snapshot;type:text" json:"snapshot"`  // 完整快照 JSON
}

// NewRecycleItem 创建回收站条目
func NewRecycleItem() *RecycleItem {
	return &RecycleItem{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (RecycleItem) TableName() string {
	return "recycle_bin"
}
