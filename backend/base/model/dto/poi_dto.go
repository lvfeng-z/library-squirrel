package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// PoiDTO POI
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

// ToPoiEntity 将 PoiDTO 转换为 Poi 实体
func ToPoiEntity(dto *PoiDTO) *entity2.Poi {
	if dto == nil {
		return nil
	}

	entity := entity2.NewPoi()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.PoiName != nil {
		entity.PoiName.Valid = true
		entity.PoiName.String = *dto.PoiName
	} else {
		entity.PoiName.Valid = false
	}

	return entity
}
