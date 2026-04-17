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

// QuerySelectItemPage 分页查询选择项
func (r *siteRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.SelectItem], error) {
	var results []*domain.SelectItem

	rawPage, err := r.Page(ctx, page, pageSize, conditions, orderBy)
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

	return model.NewPage(results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
