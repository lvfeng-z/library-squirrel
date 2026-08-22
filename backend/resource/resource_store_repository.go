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

// ListByResourceId 查询 Resource 关联的全部 store(按 store_seq 排序)
func (r *ResourceStoreRepository) ListByResourceId(ctx context.Context, resourceId int64) ([]*domain.ResourceStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "resource_id", Value: resourceId},
		},
		OrderBy: []clause.Expression{
			clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "store_seq"}},
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

// ListAliveByResourceIds 批量查询多个 Resource 关联且指向活行 store 的关联（软删行关联保留形态下，
// 覆盖确认等「作品拥有该角色」判定只看活代——merge overwrite 轨道残留、替换残留不算）
func (r *ResourceStoreRepository) ListAliveByResourceIds(ctx context.Context, resourceIds []int64) ([]*domain.ResourceStore, error) {
	if len(resourceIds) == 0 {
		return []*domain.ResourceStore{}, nil
	}
	var stores []*domain.ResourceStore
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("resource_id IN ?", resourceIds).
		Where("store_id IN (SELECT id FROM persistent_store WHERE deleted_at = 0)").
		Find(&stores).Error
	return stores, err
}

// GetByType 按 store_type 查询单个关联行（仅指向活行 store——软删行关联保留形态下，
// 软删行不是可操作的 store，命中死行会令幂等/缺轨判定取错代）
func (r *ResourceStoreRepository) GetByType(ctx context.Context, resourceId int64, storeType string) (*domain.ResourceStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "resource_id", Value: resourceId},
			clause.Eq{Column: "store_type", Value: storeType},
			clause.Expr{SQL: "EXISTS (SELECT 1 FROM persistent_store ps WHERE ps.id = resource_store.store_id AND ps.deleted_at = 0)"},
		},
	}
	return r.BaseRepository.Get(ctx, opt)
}

// DeleteByResourceIdAndTypes 删除指定 Resource 下、store_type 属于给定集合且指向活行 store 的关联
// （挂载前置清理：软删行关联保留——残留行经挂载链可联作品、随作品级联净化，挂载不摘其关联）
func (r *ResourceStoreRepository) DeleteByResourceIdAndTypes(ctx context.Context, resourceId int64, storeTypes []string) error {
	if len(storeTypes) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("resource_id = ? AND store_type IN ?", resourceId, storeTypes).
		Where("store_id IN (SELECT id FROM persistent_store WHERE deleted_at = 0)").
		Delete(new(domain.ResourceStore)).Error
}

// DeleteByStoreIds 按 store ID 集合物理删除关联行（失败还原链清理本次新建 store 的关联：
// 新行已物理删，其关联若不摘即成断链孤儿，混入完整度计数与展示面）
func (r *ResourceStoreRepository) DeleteByStoreIds(ctx context.Context, storeIds []int64) error {
	if len(storeIds) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("store_id IN ?", storeIds).
		Delete(new(domain.ResourceStore)).Error
}

// CountAliveTypesByResourceId 统计 Resource 关联的活行 store 角色计数（JOIN persistent_store 过滤
// 软删行；关联保留形态下同 role 双关联只计活行）。资源完整度重算用
func (r *ResourceStoreRepository) CountAliveTypesByResourceId(ctx context.Context, resourceId int64) (map[string]int, error) {
	rows, err := r.dbFromCtx(ctx).WithContext(ctx).Raw(`
		SELECT rs.store_type, COUNT(*) FROM resource_store rs
		JOIN persistent_store ps ON rs.store_id = ps.id AND ps.deleted_at = 0
		WHERE rs.resource_id = ?
		GROUP BY rs.store_type`, resourceId).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]int)
	for rows.Next() {
		var storeType string
		var count int
		if err := rows.Scan(&storeType, &count); err != nil {
			return nil, err
		}
		result[storeType] = count
	}
	return result, rows.Err()
}

// DeleteByResourceIds 批量物理删除 resource 关联行（作品彻底删除链：级联清理 resource_store）
func (r *ResourceStoreRepository) DeleteByResourceIds(ctx context.Context, resourceIds []int64) error {
	if len(resourceIds) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("resource_id IN ?", resourceIds).
		Delete(new(domain.ResourceStore)).Error
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ResourceStoreRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}
