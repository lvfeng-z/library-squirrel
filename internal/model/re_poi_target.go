package model

import (
	"database/sql"
)

// RePoiTarget POI与目标关联
type RePoiTarget struct {
	ID         int64          `gorm:"primaryKey;column:id" json:"id"`
	PoiID      sql.NullInt64  `gorm:"column:poi_id" json:"poiId"`
	TargetID   sql.NullInt64  `gorm:"column:target_id" json:"targetId"`
	TargetType sql.NullString `gorm:"column:target_type" json:"targetType"`
	CreateTime int64          `gorm:"column:create_time" json:"createTime"`
	UpdateTime int64          `gorm:"column:update_time" json:"updateTime"`
}

func (RePoiTarget) TableName() string {
	return "re_poi_target"
}

func (e RePoiTarget) GetID() int64 {
	return e.ID
}
