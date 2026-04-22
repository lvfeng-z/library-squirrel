package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// PoiDTO POI数据传输对象（无 sql.Null* 版本）
type PoiDTO struct {
	ID         int64   `json:"id"`
	PoiName    *string `json:"poiName"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// NewPoiDTO 从 entity.Poi 创建 PoiDTO
func NewPoiDTO(poi *entity2.Poi) *PoiDTO {
	if poi == nil {
		return nil
	}
	return &PoiDTO{
		ID:         poi.GetID(),
		PoiName:    util.NullStringToPointer(poi.PoiName),
		CreateTime: poi.CreateTime,
		UpdateTime: poi.UpdateTime,
	}
}
