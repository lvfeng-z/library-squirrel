package site

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// GetByKey 按站点身份键查询（跨库匹配/查重/关联的权威寻址方式；未命中返回 gorm.ErrRecordNotFound）
func (r *SiteRepository) GetByKey(ctx context.Context, siteKey string) (*entity.Site, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "site_key", Value: siteKey},
		},
	}
	return r.Get(ctx, opt)
}

// ListAll 全量查询站点（注册表投影，站点键只增不改、个位数级，不分页），按 id 升序
func (r *SiteRepository) ListAll(ctx context.Context) ([]*entity.Site, error) {
	opt := &database.QueryOption{
		OrderBy: []clause.Expression{
			clause.OrderBy{Columns: []clause.OrderByColumn{
				{Column: clause.Column{Name: "id"}},
			}},
		},
	}
	return r.List(ctx, opt)
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
