package workSet

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 作品集仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, workSet *domain.WorkSet) error
	// Update 更新
	Update(ctx context.Context, workSet *domain.WorkSet) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.WorkSet, error)
	// List 查询列表
	List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.WorkSet, error)
	// Count 统计数量
	Count(ctx context.Context, conditions []clause.Expression) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.WorkSet], error)
	// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error)
	// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
	GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error)
}

// workSetRepository 作品集仓储实现
type workSetRepository struct {
	*database.BaseRepository[domain.WorkSet]
}

// NewRepository 创建作品集仓储
func NewRepository(db *gorm.DB) Repository {
	return &workSetRepository{
		BaseRepository: database.NewBaseRepository[domain.WorkSet](db),
	}
}

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (r *workSetRepository) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error) {
	where := clause.And(
		clause.Eq{Column: "site_id", Value: siteId},
		clause.Eq{Column: "site_work_set_id", Value: siteWorkSetId},
	)
	return r.BaseRepository.Get(ctx, []clause.Expression{where}, nil)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
// 注意：需要通过 site 表进行 JOIN 查询
func (r *workSetRepository) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error) {
	var result *domain.WorkSet
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Joins("INNER JOIN site ON work_set.site_id = site.id").
		Where("work_set.site_work_set_id = ? AND site.name = ?", siteWorkSetId, siteName).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}
