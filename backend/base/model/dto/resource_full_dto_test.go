package dto

import (
	"testing"

	"github.com/library-squirrel/backend/base/model/entity"
)

// TestNewResourceFullDTOFiltersDeadStoreAssociations 活行过滤：storeMap（活行批量查询构建）
// 未命中的死行关联不进 Stores、不参与 WorkStore/ThumbnailStore 派生——软删代属回收站文件条目域，
// 不加过滤时 ResolvePrimaryStore 可选中死行关联致封面/展示主体落空
func TestNewResourceFullDTOFiltersDeadStoreAssociations(t *testing.T) {
	resource := entity.NewResource()
	resource.ResourceType = "image"

	liveStore := entity.NewPersistentStore()
	liveStore.SetID(10)
	deadStore := entity.NewPersistentStore()
	deadStore.SetID(20)

	// 死行关联排在前面（解析顺序挑战位）：image 主代死 + image 新代活 + thumbnail 死
	resourceStores := []*entity.ResourceStore{
		{StoreType: entity.StoreTypeImage, StoreID: 20},
		{StoreType: entity.StoreTypeImage, StoreID: 10},
		{StoreType: entity.StoreTypeThumbnail, StoreID: 20},
	}
	storeMap := map[int64]*entity.PersistentStore{10: liveStore} // 活行查询只命中 10

	dto := NewResourceFullDTO(resource, resourceStores, storeMap)

	if len(dto.Stores) != 1 {
		t.Fatalf("Stores 应只含活行关联 1 条，实际 %d", len(dto.Stores))
	}
	if dto.Stores[0].Store.ID != 10 {
		t.Fatalf("Stores 应命中活行(10)，实际 %d", dto.Stores[0].Store.ID)
	}
	if dto.WorkStore == nil || dto.WorkStore.ID != 10 {
		t.Fatalf("WorkStore 应解析到活行(10)，实际 %v", dto.WorkStore)
	}
	if dto.ThumbnailStore != nil {
		t.Fatalf("thumbnail 死行关联不应派生 ThumbnailStore，实际 %v", dto.ThumbnailStore)
	}
}
