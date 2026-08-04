package dto

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// ResourceDTO 资源数据传输对象
type ResourceDTO struct {
	ID               int64   `json:"id"`
	WorkID           int64   `json:"workId"`
	TaskID           int64   `json:"taskId"`
	Enabled          bool    `json:"enabled"`
	SuggestName      *string `json:"suggestName"`
	ResourceType     string  `json:"resourceType"`
	ResourceComplete int     `json:"resourceComplete"`
	CreateTime       int64   `json:"createTime"`
	UpdateTime       int64   `json:"updateTime"`
}

// NewResourceDTO 从 entity.Resource 创建 ResourceDTO
func NewResourceDTO(resource *entity.Resource) *ResourceDTO {
	if resource == nil {
		return nil
	}
	return &ResourceDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceType:     resource.ResourceType,
		ResourceComplete: int(resource.ResourceComplete.Int64),
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}
}

// ToResourceEntity 将 ResourceDTO 转换为 Resource 实体
func ToResourceEntity(dto *ResourceDTO) *entity.Resource {
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
	newResource.ResourceComplete = sql.NullInt64{Int64: int64(dto.ResourceComplete), Valid: true}

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

// ResourceStoreDTO Resource 关联的单个 typed store(包装 PersistentStoreDTO + store_type/generation)
type ResourceStoreDTO struct {
	StoreType  string              `json:"storeType"`       // image | document | thumbnail | videoTrack | audioTrack | videoMain
	Generation string              `json:"generation"`      // downloaded | derived
	Store      *PersistentStoreDTO `json:"store,omitempty"` // 对应的 PersistentStore 信息
}

// ResourceFullDTO 资源完整 DTO
// Stores 为 resource_store 关联表全部 store(多轨模型,主数据源);
// WorkStore 为展示主体,由后端按资源类型的 PrimaryRoles 优先级链派生(ResolvePrimaryStore),前端纯消费;
// ThumbnailStore 为从 Stores 按 storeType=thumbnail 派生的便捷访问器。
type ResourceFullDTO struct {
	ID               int64               `json:"id"`
	WorkID           int64               `json:"workId"`
	TaskID           int64               `json:"taskId"`
	Enabled          bool                `json:"enabled"`
	SuggestName      *string             `json:"suggestName"`
	ResourceType     string              `json:"resourceType"`
	ResourceComplete int                 `json:"resourceComplete"`
	Stores           []ResourceStoreDTO  `json:"stores,omitempty"`
	WorkStore        *PersistentStoreDTO `json:"workStore,omitempty"`
	ThumbnailStore   *PersistentStoreDTO `json:"thumbnailStore,omitempty"`
	CreateTime       int64               `json:"createTime"`
	UpdateTime       int64               `json:"updateTime"`
}

// NewResourceFullDTO 从 entity.Resource + resource_store 关联行 + PersistentStore 实体创建 ResourceFullDTO。
// resourceStores 为该 Resource 的全部 resource_store 关联行;
// storeMap 为 storeId → PersistentStore 的查找映射(由调用方批量查询构建,避免 N+1)。
// Stores 为全量多轨数据(主数据源);ThumbnailStore 从 Stores 按 storeType=thumbnail 派生;
// WorkStore 由 ResolvePrimaryStore 按资源类型的 PrimaryRoles 优先级链派生(未知/历史 resource_type 安全降级)。
func NewResourceFullDTO(resource *entity.Resource, resourceStores []*entity.ResourceStore, storeMap map[int64]*entity.PersistentStore) *ResourceFullDTO {
	if resource == nil {
		return nil
	}
	dto := &ResourceFullDTO{
		ID:               resource.GetID(),
		WorkID:           resource.WorkID,
		TaskID:           resource.TaskID,
		SuggestName:      util.NullStringToPointer(resource.SuggestName),
		ResourceType:     resource.ResourceType,
		ResourceComplete: int(resource.ResourceComplete.Int64),
		CreateTime:       resource.GetCreateTime(),
		UpdateTime:       resource.GetUpdateTime(),
	}

	// 遍历 resource_store 行构 Stores,同时派生 ThumbnailStore
	for _, rs := range resourceStores {
		if rs == nil {
			continue
		}
		store := storeMap[rs.StoreID]
		storeDTO := &ResourceStoreDTO{
			StoreType:  rs.StoreType,
			Generation: rs.Generation,
			Store:      NewPersistentStoreDTO(store),
		}
		dto.Stores = append(dto.Stores, *storeDTO)

		if rs.StoreType == entity.StoreTypeThumbnail {
			dto.ThumbnailStore = storeDTO.Store
		}
	}

	// WorkStore 按资源类型规约的 PrimaryRoles 优先级链派生展示主体;
	// 未知/历史 resource_type(LookupResourceTypeSpec 返回 nil)由 ResolvePrimaryStore 安全降级
	if primaryRS := entity.ResolvePrimaryStore(resourceStores, entity.LookupResourceTypeSpec(resource.ResourceType)); primaryRS != nil {
		if ps := storeMap[primaryRS.StoreID]; ps != nil {
			dto.WorkStore = NewPersistentStoreDTO(ps)
		}
	}

	return dto
}
