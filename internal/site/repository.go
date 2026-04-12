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
// 嵌入 database.BaseRepository[domain.Site] 获得基础 CRUD 实现
type siteRepository struct {
	*database.BaseRepository[domain.Site]
}

// NewRepository 创建站点仓储
func NewRepository(db *gorm.DB) Repository {
	return &siteRepository{
		BaseRepository: database.NewBaseRepository[domain.Site](db),
	}
}

// GORM 返回底层 GORM DB 实例（供特殊查询使用）
func (r *siteRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// Page 分页查询
func (r *siteRepository) Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, order clause.Expression) (*model.Page[domain.Site], error) {
	data, total, err := r.BaseRepository.Page(ctx, page, pageSize, conditions, order)
	if err != nil {
		return nil, err
	}
	return model.NewPage(data, total, page, pageSize), nil
}

// QuerySelectItemPage 分页查询选择项
func (r *siteRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.SelectItem], error) {
	var results []*domain.SelectItem
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.Site{})

	// 应用查询条件
	if len(conditions) > 0 {
		for _, cond := range conditions {
			db = db.Clauses(cond)
		}
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if orderBy != nil {
		db = db.Clauses(orderBy)
	}

	var sites []*domain.Site
	if err := db.Find(&sites).Error; err != nil {
		return nil, err
	}

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

	return model.NewPage(results, total, page, pageSize), nil
}
