package model

import (
	"database/sql"
)

// Poi POI
type Poi struct {
	ID         int64          `gorm:"primaryKey;column:id" json:"id"`
	PoiName    sql.NullString `gorm:"column:poi_name" json:"poiName"`
	CreateTime int64          `gorm:"column:create_time" json:"createTime"`
	UpdateTime int64          `gorm:"column:update_time" json:"updateTime"`
}

func (Poi) TableName() string {
	return "poi"
}

func (e Poi) GetID() int64 {
	return e.ID
}
