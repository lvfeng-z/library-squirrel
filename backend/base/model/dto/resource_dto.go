package dto

import (
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewResourceDTO 从 entity.Resource 创建 ResourceDTO
func NewResourceDTO(resource *entity.Resource) *sdkdto.ResourceDTO {
	if resource == nil {
		return nil
	}
	return &sdkdto.ResourceDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		Enabled:          resource.Enabled,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceComplete: resource.ResourceComplete,
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}
}

// ToResourceEntity 将 ResourceDTO 转换为 Resource 实体
func ToResourceEntity(dto *sdkdto.ResourceDTO) *entity.Resource {
	if dto == nil {
		return nil
	}

	newResource := entity.NewResource()

	// 设置基础字段
	if dto.ID != 0 {
		newResource.SetID(dto.ID)
	}

	// 设置业务字段
	newResource.WorkID = dto.WorkID
	newResource.TaskID = dto.TaskID
	newResource.Enabled = dto.Enabled
	newResource.ResourceComplete = dto.ResourceComplete

	if dto.SuggestName != nil {
		newResource.SuggestName.Valid = true
		newResource.SuggestName.String = *dto.SuggestName
	} else {
		newResource.SuggestName.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		newResource.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		newResource.SetUpdateTime(dto.UpdateTime)
	}

	return newResource
}

// NewResourceFullDTO 从 entity.Resource + resource_store 关联行 + PersistentStore 实体创建 ResourceFullDTO。
// resourceStores 为该 Resource 的全部 resource_store 关联行;
// storeMap 为 storeId → PersistentStore 的查找映射(由调用方批量查询构建,避免 N+1)。
// Stores 为全量多轨数据(主数据源);WorkStore/ThumbnailStore 为从 Stores 按 storeType 派生的便捷访问器。
func NewResourceFullDTO(resource *entity.Resource, resourceStores []*entity.ResourceStore, storeMap map[int64]*entity.PersistentStore) *sdkdto.ResourceFullDTO {
	if resource == nil {
		return nil
	}
	dto := &sdkdto.ResourceFullDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		Enabled:          resource.Enabled,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceComplete: resource.ResourceComplete,
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}

	// 遍历 resource_store 行构 Stores,同时派生便捷访问器
	for _, rs := range resourceStores {
		if rs == nil {
			continue
		}
		store := storeMap[rs.StoreID]
		storeDTO := &sdkdto.ResourceStoreDTO{
			StoreType:  rs.StoreType,
			Generation: rs.Generation,
			Store:      NewPersistentStoreDTO(store),
		}
		dto.Stores = append(dto.Stores, *storeDTO)

		// 派生便捷访问器
		switch rs.StoreType {
		case entity.StoreTypeMain:
			dto.WorkStore = storeDTO.Store
		case entity.StoreTypeThumbnail:
			dto.ThumbnailStore = storeDTO.Store
		}
	}

	return dto
}
