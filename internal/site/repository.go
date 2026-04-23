package site

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	domain "github.com/library-squirrel/wails/pkg/model/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SiteRepository 站点仓储实现
type SiteRepository struct {
	*database.BaseRepository[domain.Site]
}

// NewRepository 创建站点仓储
func NewRepository(db *gorm.DB) *SiteRepository {
	return &SiteRepository{
		BaseRepository: database.NewBaseRepository[domain.Site](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SiteRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// QuerySelectItemPage 分页查询选择项
func (r *SiteRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[dto.SelectItem, SiteQueryDTO], error) {
	var results []*dto.SelectItem

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
		results = append(results, &dto.SelectItem{
			Value: site.ID,
			Label: siteName,
		})
	}

	return model.NewPage[dto.SelectItem, SiteQueryDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
