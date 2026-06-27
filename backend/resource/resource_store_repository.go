package resource

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceStoreRepository resource_store 仓储实现
type ResourceStoreRepository struct {
	*database.BaseRepository[domain.ResourceStore]
}

// NewResourceStoreRepository 创建 resource_store 仓储
func NewResourceStoreRepository(db *gorm.DB) *ResourceStoreRepository {
	return &ResourceStoreRepository{
		BaseRepository: database.NewBaseRepository[domain.ResourceStore](db),
	}
}

// ListByResourceId 查询 Resource 关联的全部 store(按 order_idx 排序)
func (r *ResourceStoreRepository) ListByResourceId(ctx context.Context, resourceId int64) ([]*domain.ResourceStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "resource_id", Value: resourceId},
		},
		OrderBy: []clause.Expression{
			clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "order_idx"}},
			}},
		},
	}
	return r.BaseRepository.List(ctx, opt)
}

// ListByResourceIds 批量查询多个 Resource 关联的 store(避免 N+1)
func (r *ResourceStoreRepository) ListByResourceIds(ctx context.Context, resourceIds []int64) ([]*domain.ResourceStore, error) {
	if len(resourceIds) == 0 {
		return []*domain.ResourceStore{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.IN{Column: "resource_id", Values: toInterfaceSlice(resourceIds)},
		},
	}
	return r.BaseRepository.List(ctx, opt)
}

// GetByType 按 store_type 查询单个 store
func (r *ResourceStoreRepository) GetByType(ctx context.Context, resourceId int64, storeType string) (*domain.ResourceStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "resource_id", Value: resourceId},
			clause.Eq{Column: "store_type", Value: storeType},
		},
	}
	return r.BaseRepository.Get(ctx, opt)
}

// DeleteByResourceIdAndTypes 删除指定 Resource 下、store_type 属于给定集合的 store 关联(替换/续传场景清理旧关联)
func (r *ResourceStoreRepository) DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error {
	if len(storeTypes) == 0 {
		return nil
	}
	values := make([]any, 0, len(storeTypes))
	for _, t := range storeTypes {
		values = append(values, t)
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("resource_id = ? AND store_type IN ?", resourceId, storeTypes).
		Delete(new(domain.ResourceStore)).Error
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ResourceStoreRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}
