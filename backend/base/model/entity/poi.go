package entity

import (
	"database/sql"

	"github.com/library-squirrel/wails/backend/base/model"
)

// Poi POI
type Poi struct {
	*model.BaseEntity                // 嵌入基础实体
	PoiName           sql.NullString `gorm:"column:poi_name" json:"poiName"`
}

func NewPoi() *Poi {
	return &Poi{
		BaseEntity: &model.BaseEntity{},
	}
}

func (Poi) TableName() string {
	return "poi"
}
