package workSet

import (
	"context"

	domain "github.com/library-squirrel/wails/backend/base/model/entity"
	"github.com/library-squirrel/wails/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkSetRepository 作品集仓储实现
type WorkSetRepository struct {
	*database.BaseRepository[domain.WorkSet]
}

// NewRepository 创建作品集仓储
func NewRepository(db *gorm.DB) *WorkSetRepository {
	return &WorkSetRepository{
		BaseRepository: database.NewBaseRepository[domain.WorkSet](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *WorkSetRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (r *WorkSetRepository) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_set_id", Value: siteWorkSetId},
			),
		},
	}
	return r.Get(ctx, opt)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
// 注意：需要通过 site 表进行 JOIN 查询
func (r *WorkSetRepository) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error) {
	var result *domain.WorkSet
	err := r.GORM().
		WithContext(ctx).
		Joins("INNER JOIN site ON work_set.site_id = site.id").
		Where("work_set.site_work_set_id = ? AND site.name = ?", siteWorkSetId, siteName).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}
