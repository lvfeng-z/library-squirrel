package siteTag

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// siteTagRepository 站点标签仓储实现
// 不嵌入 database.BaseRepository 以避免 Page 返回类型的泛型限制问题
type siteTagRepository struct {
	*database.BaseRepository[domain.SiteTag]
}

// NewRepository 创建站点标签仓储
func NewRepository(db *gorm.DB) Repository {
	return &siteTagRepository{
		BaseRepository: database.NewBaseRepository[domain.SiteTag](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *siteTagRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// ListByWorkId 查询作品的站点标签
func (r *siteTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.SiteTag, error) {
	query := `
		SELECT t1.*
		FROM site_tag t1
		INNER JOIN re_work_tag t2 ON t1.id = t2.site_tag_id
		WHERE t2.work_id = ?
	`

	var results []*domain.SiteTag
	err := r.GORM().WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListBySiteTagIds 根据站点标签ID列表查询
func (r *siteTagRepository) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*domain.SiteTag, error) {
	if len(siteTagIds) == 0 {
		return make([]*domain.SiteTag, 0), nil
	}

	placeholders := make([]string, len(siteTagIds))
	args := make([]interface{}, len(siteTagIds))
	for i, id := range siteTagIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT * FROM site_tag WHERE id IN (%s)`, strings.Join(placeholders, ","))

	var results []*domain.SiteTag
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// UpdateBindLocalTag 绑定本地标签
func (r *siteTagRepository) UpdateBindLocalTag(ctx context.Context, localTagId int64, siteTagIds []int64) (int64, error) {
	if len(siteTagIds) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(siteTagIds))
	args := make([]interface{}, len(siteTagIds))
	for i, id := range siteTagIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`UPDATE site_tag SET local_tag_id = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	args = append([]interface{}{localTagId}, args...)

	result := r.GORM().WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (r *siteTagRepository) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalTagId *bool, localTagId *int64) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error) {
	var results []*domain.SiteTagFullDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteTag{})

	// 根据 boundOnLocalTagId 添加 localTagId 的过滤条件
	if localTagId != nil {
		if boundOnLocalTagId != nil && *boundOnLocalTagId {
			// 绑定到指定本地标签
			db = db.Where("local_tag_id = ?", *localTagId)
		} else if boundOnLocalTagId != nil && !*boundOnLocalTagId {
			// 未绑定到指定本地标签（包括绑定到其他本地标签或从未绑定过本地标签的）
			db = db.Where("(local_tag_id != ? OR local_tag_id IS NULL)", *localTagId)
		}
	}

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	queryCondition := database.QueryOption{Conditions: []clause.Expression{where}, OrderBy: []clause.Expression{order}}
	pageCondition := database.PageOption{QueryOption: queryCondition, Page: page, PageSize: pageSize}

	resPage, err := r.Page(ctx, &pageCondition)

	if err != nil {
		return nil, err
	}
	siteTags := resPage.Data

	// 转换为 DTO
	for _, tag := range siteTags {
		dto := domain.NewSiteTagFullDTO(tag)
		// 查询关联的本地标签
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			dto.LocalTag = &domain.LocalTag{}
			if err := r.GORM().WithContext(ctx).First(dto.LocalTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 查询关联的站点
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			dto.Site = &domain.Site{}
			if err := r.GORM().WithContext(ctx).First(dto.Site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		results = append(results, dto)
	}

	return model.NewPage[domain.SiteTagFullDTO, SiteTagQueryDTO](results, total, page, pageSize), nil
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (r *siteTagRepository) QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error) {
	var results []*domain.SiteTagFullDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteTag{})

	// 构建 EXISTS 子查询
	if boundOnWorkId != nil && *boundOnWorkId {
		db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		db = db.Where(" NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	}

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if order != nil {
		db = db.Clauses(order)
	}

	// 执行查询
	var siteTags []*domain.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, tag := range siteTags {
		dto := domain.NewSiteTagFullDTO(tag)
		// 查询关联的本地标签
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			dto.LocalTag = &domain.LocalTag{}
			if err := r.GORM().WithContext(ctx).First(dto.LocalTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 查询关联的站点
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			dto.Site = &domain.Site{}
			if err := r.GORM().WithContext(ctx).First(dto.Site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		results = append(results, dto)
	}

	return model.NewPage[domain.SiteTagFullDTO, SiteTagQueryDTO](results, total, page, pageSize), nil
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (r *siteTagRepository) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO, SiteTagQueryDTO], error) {
	var results []*domain.SiteTagLocalRelateDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteTag{})

	// 构建基础查询
	if boundOnWorkId != nil && *boundOnWorkId {
		db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		db = db.Where(" NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	}

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if order != nil {
		db = db.Clauses(order)
	}

	// 执行查询
	var siteTags []*domain.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, tag := range siteTags {
		dto := domain.NewSiteTagLocalRelateDTO(tag)
		// 查询关联的本地标签
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			dto.LocalTag = &domain.LocalTag{}
			if err := r.GORM().WithContext(ctx).First(dto.LocalTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 查询关联的站点
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			dto.Site = &domain.Site{}
			if err := r.GORM().WithContext(ctx).First(dto.Site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 检查是否有同名本地标签
		var count int64
		r.GORM().WithContext(ctx).Model(&domain.LocalTag{}).Where("local_tag_name = ?", tag.SiteTagName).Count(&count)
		dto.HasSameNameLocalTag = count > 0

		results = append(results, dto)
	}

	return model.NewPage[domain.SiteTagLocalRelateDTO, SiteTagQueryDTO](results, total, page, pageSize), nil
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (r *siteTagRepository) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem, SiteTagQueryDTO], error) {
	var results []*domain.SelectItem
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteTag{})

	// 查询已绑定到该作品的
	db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if order != nil {
		db = db.Clauses(order)
	}

	// 执行查询
	var siteTags []*domain.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	for _, tag := range siteTags {
		label := ""
		if tag.SiteTagName.Valid {
			label = tag.SiteTagName.String
		}
		item := &domain.SelectItem{
			Value: tag.ID,
			Label: label,
		}
		// 查询站点名称作为副标题
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			var site domain.Site
			if err := r.GORM().WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err == nil {
				subLabel := "?"
				if site.SiteName.Valid {
					subLabel = site.SiteName.String
				}
				item.SubLabels = []string{subLabel}
			} else {
				item.SubLabels = []string{"?"}
			}
		} else {
			item.SubLabels = []string{"?"}
		}
		results = append(results, item)
	}

	return model.NewPage[domain.SelectItem, SiteTagQueryDTO](results, total, page, pageSize), nil
}
