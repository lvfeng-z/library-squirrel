package resource

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceRepository 资源仓储实现
type ResourceRepository struct {
	*database.BaseRepository[domain.Resource]
}

// NewRepository 创建资源仓储
func NewRepository(db *gorm.DB) *ResourceRepository {
	return &ResourceRepository{
		BaseRepository: database.NewBaseRepository[domain.Resource](db),
	}
}

// ListByWorkId 查询作品关联的资源
func (r *ResourceRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	where := clause.Eq{Column: "work_id", Value: workId}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return r.BaseRepository.List(ctx, opt)
}

// GetEnabledByWorkId 查询作品关联的启用资源
func (r *ResourceRepository) GetEnabledByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "work_id", Value: workId},
			clause.Eq{Column: "enabled", Value: true},
		},
	}
	return r.BaseRepository.List(ctx, opt)
}

// ListByWorkIds 批量查询多个作品关联的资源
func (r *ResourceRepository) ListByWorkIds(ctx context.Context, workIds []int64) ([]*domain.Resource, error) {
	if len(workIds) == 0 {
		return []*domain.Resource{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.IN{Column: "work_id", Values: toInterfaceSlice(workIds)},
		},
	}
	return r.BaseRepository.List(ctx, opt)
}

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ResourceRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// DeleteByWorkId 根据作品ID删除所有资源
func (r *ResourceRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.Resource)).Error
}
