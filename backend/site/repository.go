package site

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// SiteRepository 站点仓储实现
type SiteRepository struct {
	*database.BaseRepository[entity.Site]
}

// NewRepository 创建站点仓储
func NewRepository(db *gorm.DB) *SiteRepository {
	return &SiteRepository{
		BaseRepository: database.NewBaseRepository[entity.Site](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SiteRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// QuerySelectItemPage 分页查询选择项
func (r *SiteRepository) QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.SelectItem], error) {
	var results []*dto.SelectItem

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

	return model.NewPage[dto.SelectItem](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
