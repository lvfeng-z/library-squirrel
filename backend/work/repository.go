package work

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkRepository 作品仓储实现
type WorkRepository struct {
	*database.BaseRepository[domain.Work]
}

// NewRepository 创建作品仓储
func NewRepository(db *gorm.DB) *WorkRepository {
	return &WorkRepository{
		BaseRepository: database.NewBaseRepository[domain.Work](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *WorkRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (r *WorkRepository) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_id", Value: siteWorkId},
			),
		},
	}
	return r.Get(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (r *WorkRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error) {
	if len(ids) == 0 {
		return []*domain.Work{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: toInterfaceSlice(ids)}},
	}
	return r.List(ctx, opt)
}

// UpdateLastViewBatch 批量更新最后查看时间
func (r *WorkRepository) UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().
		WithContext(ctx).
		Model(new(domain.Work)).
		Where("id IN ?", ids).
		Update("last_view", lastView).Error
}

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
