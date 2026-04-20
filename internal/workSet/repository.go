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
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.WorkSet, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.WorkSet, WorkSetQueryDTO], error)
	// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
	GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error)
	// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
	GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error)
}

// workSetRepository 作品集仓储实现
type workSetRepository struct {
	db *gorm.DB
}

// NewRepository 创建作品集仓储
func NewRepository(db *gorm.DB) Repository {
	return &workSetRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *workSetRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *workSetRepository) Save(ctx context.Context, workSet *domain.WorkSet) error {
	return r.db.WithContext(ctx).Create(workSet).Error
}

// Update 更新
func (r *workSetRepository) Update(ctx context.Context, workSet *domain.WorkSet) error {
	return r.db.WithContext(ctx).Save(workSet).Error
}

// GetById 根据ID获取
func (r *workSetRepository) GetById(ctx context.Context, id int64) (*domain.WorkSet, error) {
	var workSet domain.WorkSet
	err := r.db.WithContext(ctx).First(&workSet, id).Error
	if err != nil {
		return nil, err
	}
	return &workSet, nil
}

// List 查询列表
func (r *workSetRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.WorkSet, error) {
	var workSets []*domain.WorkSet
	db := r.db.WithContext(ctx).Model(new(domain.WorkSet))
	db = applyQueryOption(db, opt)
	err := db.Find(&workSets).Error
	if err != nil {
		return nil, err
	}
	return workSets, nil
}

// Count 统计数量
func (r *workSetRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.WorkSet))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *workSetRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.WorkSet), id).Error
}

// Page 分页查询
func (r *workSetRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.WorkSet, WorkSetQueryDTO], error) {
	page := opt.Page
	pageSize := opt.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 构建查询选项（设置 Limit 和 Offset）
	queryOpt := opt.QueryOption
	queryOpt.Limit = pageSize
	queryOpt.Offset = offset

	// 查询列表
	list, err := r.List(ctx, &queryOpt)
	if err != nil {
		return nil, err
	}

	// 统计总数（不需要 Limit 和 Offset）
	countOpt := opt.QueryOption
	countOpt.Limit = 0
	countOpt.Offset = 0
	total, err := r.Count(ctx, &countOpt)
	if err != nil {
		return nil, err
	}

	return model.NewPage[domain.WorkSet, WorkSetQueryDTO](list, total, page, pageSize), nil
}

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (r *workSetRepository) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_set_id", Value: siteWorkSetId},
			),
		},
	}
	var workSet domain.WorkSet
	db := r.db.WithContext(ctx).Model(new(domain.WorkSet))
	db = applyQueryOption(db, opt)
	err := db.First(&workSet).Error
	if err != nil {
		return nil, err
	}
	return &workSet, nil
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
// 注意：需要通过 site 表进行 JOIN 查询
func (r *workSetRepository) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error) {
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

// applyQueryOption 将 QueryOption 应用到 db 实例
func applyQueryOption(db *gorm.DB, opt *database.QueryOption) *gorm.DB {
	// 1. Select（覆盖型）
	if opt.Select != nil {
		db = db.Select(opt.Select)
	}

	// 2. Joins（叠加型）
	for _, join := range opt.Joins {
		db = db.Clauses(join)
	}

	// 3. Conditions（叠加型）
	for _, cond := range opt.Conditions {
		db = db.Where(cond)
	}

	// 4. OrderBy（叠加型）
	if len(opt.OrderBy) > 0 {
		db = db.Order(opt.OrderBy)
	}

	// 5. GroupBy（覆盖型）
	if opt.GroupBy != nil {
		db = db.Clauses(opt.GroupBy)
	}

	// 6. Having（覆盖型）
	if opt.Having != nil {
		db = db.Having(opt.Having)
	}

	// 7. Limit & Offset（覆盖型）
	if opt.Limit > 0 {
		db = db.Limit(opt.Limit)
	}
	if opt.Offset > 0 {
		db = db.Offset(opt.Offset)
	}

	return db
}
