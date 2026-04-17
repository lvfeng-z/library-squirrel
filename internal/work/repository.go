package work

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// workRepository 作品仓储实现
type workRepository struct {
	*database.BaseRepository[domain.Work]
}

// NewRepository 创建作品仓储
func NewRepository(db *gorm.DB) Repository {
	return &workRepository{
		BaseRepository: database.NewBaseRepository[domain.Work](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *workRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (r *workRepository) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	where := clause.And(
		clause.Eq{Column: "site_id", Value: siteId},
		clause.Eq{Column: "site_work_id", Value: siteWorkId},
	)
	return r.BaseRepository.Get(ctx, []clause.Expression{where}, nil)
}

// ListByIds 根据ID列表批量查询
func (r *workRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error) {
	if len(ids) == 0 {
		return []*domain.Work{}, nil
	}
	where := clause.IN{Column: "id", Values: toInterfaceSlice(ids)}
	return r.BaseRepository.List(ctx, []clause.Expression{where}, nil, 0, 0)
}

// UpdateLastViewBatch 批量更新最后查看时间
func (r *workRepository) UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.BaseRepository.GORM().
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
