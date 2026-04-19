package site

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// siteRepository 站点仓储实现
// 不嵌入 database.BaseRepository 以避免 Page 返回类型的泛型限制问题
type siteRepository struct {
	db *gorm.DB
}

// NewRepository 创建站点仓储
func NewRepository(db *gorm.DB) Repository {
	return &siteRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例（供特殊查询使用）
func (r *siteRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *siteRepository) Save(ctx context.Context, site *domain.Site) error {
	return r.db.WithContext(ctx).Create(site).Error
}

// Update 更新
func (r *siteRepository) Update(ctx context.Context, site *domain.Site) error {
	return r.db.WithContext(ctx).Save(site).Error
}

// GetById 根据ID获取
func (r *siteRepository) GetById(ctx context.Context, id int64) (*domain.Site, error) {
	var site domain.Site
	err := r.db.WithContext(ctx).First(&site, id).Error
	if err != nil {
		return nil, err
	}
	return &site, nil
}

// Get 根据条件获取单个
func (r *siteRepository) Get(ctx context.Context, opt *database.QueryOption) (*domain.Site, error) {
	var site domain.Site
	db := r.db.WithContext(ctx).Model(new(domain.Site))
	db = applyQueryOption(db, opt)
	err := db.First(&site).Error
	if err != nil {
		return nil, err
	}
	return &site, nil
}

// List 查询列表
func (r *siteRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Site, error) {
	var sites []*domain.Site
	db := r.db.WithContext(ctx).Model(new(domain.Site))
	db = applyQueryOption(db, opt)
	err := db.Find(&sites).Error
	if err != nil {
		return nil, err
	}
	return sites, nil
}

// Count 统计数量
func (r *siteRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.Site))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *siteRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.Site), id).Error
}

// Page 分页查询
func (r *siteRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Site, SiteQueryDTO], error) {
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

	return model.NewPage[domain.Site, SiteQueryDTO](list, total, page, pageSize), nil
}

// QuerySelectItemPage 分页查询选择项
func (r *siteRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.SelectItem, SiteQueryDTO], error) {
	var results []*domain.SelectItem

	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conditions,
			OrderBy:    []clause.Expression{orderBy},
		},
		Page:     page,
		PageSize: pageSize,
	}
	rawPage, err := r.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	sites := rawPage.Data

	// 转换为 SelectItem
	for _, site := range sites {
		siteName := ""
		if site.SiteName.Valid {
			siteName = site.SiteName.String
		}
		results = append(results, &domain.SelectItem{
			Value: site.ID,
			Label: siteName,
		})
	}

	return model.NewPage[domain.SelectItem, SiteQueryDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
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