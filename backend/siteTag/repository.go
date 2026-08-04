package siteTag

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SiteTagRepository 站点标签仓储实现
type SiteTagRepository struct {
	*database.BaseRepository[entity2.SiteTag]
}

// NewRepository 创建站点标签仓储
func NewRepository(db *gorm.DB) *SiteTagRepository {
	return &SiteTagRepository{
		BaseRepository: database.NewBaseRepository[entity2.SiteTag](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SiteTagRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *SiteTagRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// BatchUpsert 批量插入或更新（基于 site_id + site_tag_id 唯一约束）
func (r *SiteTagRepository) BatchUpsert(ctx context.Context, tags []*entity2.SiteTag) error {
	if len(tags) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, t := range tags {
		if t.GetID() == 0 {
			t.SetCreateTime(now)
		}
		t.SetUpdateTime(now)
	}
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_tag_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"site_tag_name", "base_site_tag_id", "description",
			"local_tag_id", "last_use", "update_time",
		}),
	}).Create(tags).Error
}

// ListBySiteAndSiteTagIDs 根据站点ID和站点标签ID列表批量查询
func (r *SiteTagRepository) ListBySiteAndSiteTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity2.SiteTag, error) {
	if len(siteTagIds) == 0 {
		return nil, nil
	}
	var result []*entity2.SiteTag
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where("site_id = ? AND site_tag_id IN ?", siteId, siteTagIds).
		Find(&result).Error
	return result, err
}

// Upsert 原子插入或更新（基于 site_id + site_tag_id 唯一约束）
func (r *SiteTagRepository) Upsert(ctx context.Context, tag *entity2.SiteTag) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_tag_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"site_tag_name", "base_site_tag_id", "description",
			"local_tag_id", "last_use", "update_time",
		}),
	}).Create(tag).Error
}

// GetBySiteAndSiteTagID 根据站点ID和站点标签ID查询
func (r *SiteTagRepository) GetBySiteAndSiteTagID(ctx context.Context, siteId int64, siteTagId string) (*entity2.SiteTag, error) {
	var tag entity2.SiteTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Where("site_id = ? AND site_tag_id = ?", siteId, siteTagId).First(&tag).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}

// ListByWorkId 查询作品的站点标签
func (r *SiteTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*entity2.SiteTag, error) {
	query := `
		SELECT t1.*
		FROM site_tag t1
		INNER JOIN re_work_tag t2 ON t1.id = t2.site_tag_id
		WHERE t2.work_id = ?
	`

	var results []*entity2.SiteTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListBySiteTagIds 根据站点标签ID列表查询
func (r *SiteTagRepository) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*entity2.SiteTag, error) {
	if len(siteTagIds) == 0 {
		return make([]*entity2.SiteTag, 0), nil
	}

	placeholders := make([]string, len(siteTagIds))
	args := make([]interface{}, len(siteTagIds))
	for i, id := range siteTagIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT * FROM site_tag WHERE id IN (%s)`, strings.Join(placeholders, ","))

	var results []*entity2.SiteTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// UpdateBindLocalTag 绑定本地标签
func (r *SiteTagRepository) UpdateBindLocalTag(ctx context.Context, localTagId *int64, siteTagIds []int64) (int64, error) {
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

	result := r.dbFromCtx(ctx).WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (r *SiteTagRepository) QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[dto2.SiteTagFullDTO], error) {
	var results []*dto2.SiteTagFullDTO
	var total int64

	db := r.dbFromCtx(ctx).WithContext(ctx).Model(&entity2.SiteTag{})

	// 构建 EXISTS 子查询
	if boundOnWorkId != nil && *boundOnWorkId {
		db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		db = db.Where(" NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	}

	// 应用查询条件
	for _, cond := range opt.Conditions {
		if cond != nil {
			db = db.Clauses(cond)
		}
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (opt.Page - 1) * opt.PageSize
	db = db.Offset(offset).Limit(opt.PageSize)

	// 应用排序
	for _, order := range opt.OrderBy {
		if order != nil {
			db = db.Clauses(order)
		}
	}

	// 执行查询
	var siteTags []*entity2.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, tag := range siteTags {
		dto := dto2.NewSiteTagFullDTO(tag)
		// 查询关联的本地标签
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			var localTag entity2.LocalTag
			if err := r.dbFromCtx(ctx).WithContext(ctx).First(&localTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			dto.LocalTag = dto2.NewLocalTagDTO(&localTag)
		}
		// 查询关联的站点
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			var site entity2.Site
			if err := r.dbFromCtx(ctx).WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			dto.Site = dto2.NewSiteDTO(&site)
		}
		results = append(results, dto)
	}

	return model.NewPage[dto2.SiteTagFullDTO](results, total, opt.Page, opt.PageSize), nil
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (r *SiteTagRepository) QueryLocalRelateDTOPage(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[dto2.SiteTagLocalRelateDTO], error) {
	var results []*dto2.SiteTagLocalRelateDTO
	var total int64

	db := r.dbFromCtx(ctx).WithContext(ctx).Model(&entity2.SiteTag{})

	// 构建基础查询
	if boundOnWorkId != nil && *boundOnWorkId {
		db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		db = db.Where(" NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	}

	// 应用查询条件
	for _, cond := range opt.Conditions {
		if cond != nil {
			db = db.Clauses(cond)
		}
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (opt.Page - 1) * opt.PageSize
	db = db.Offset(offset).Limit(opt.PageSize)

	// 应用排序
	for _, order := range opt.OrderBy {
		if order != nil {
			db = db.Clauses(order)
		}
	}

	// 执行查询
	var siteTags []*entity2.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, tag := range siteTags {
		dto := dto2.NewSiteTagLocalRelateDTO(tag)
		// 查询关联的本地标签
		if tag.LocalTagID.Valid && tag.LocalTagID.Int64 > 0 {
			localTag := entity2.NewLocalTag()
			if err := r.dbFromCtx(ctx).WithContext(ctx).First(localTag, tag.LocalTagID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			dto.LocalTag = dto2.NewLocalTagDTO(localTag)
		}
		// 查询关联的站点
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			var site entity2.Site
			if err := r.dbFromCtx(ctx).WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			dto.Site = dto2.NewSiteDTO(&site)
		}
		// 检查是否有同名本地标签
		var count int64
		r.dbFromCtx(ctx).WithContext(ctx).Model(&entity2.LocalTag{}).Where("local_tag_name = ?", tag.SiteTagName).Count(&count)
		dto.HasSameNameLocalTag = count > 0

		results = append(results, dto)
	}

	return model.NewPage[dto2.SiteTagLocalRelateDTO](results, total, opt.Page, opt.PageSize), nil
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (r *SiteTagRepository) QuerySelectItemPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[dto2.SelectItem], error) {
	var results []*dto2.SelectItem
	var total int64

	db := r.dbFromCtx(ctx).WithContext(ctx).Model(&entity2.SiteTag{})

	if boundOnWorkId != nil && *boundOnWorkId {
		db = db.Where(" EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		db = db.Where(" NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND site_tag_id = site_tag.id)", workId)
	}

	// 应用查询条件
	for _, cond := range opt.Conditions {
		if cond != nil {
			db = db.Clauses(cond)
		}
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (opt.Page - 1) * opt.PageSize
	db = db.Offset(offset).Limit(opt.PageSize)

	// 应用排序
	for _, order := range opt.OrderBy {
		if order != nil {
			db = db.Clauses(order)
		}
	}

	// 执行查询
	var siteTags []*entity2.SiteTag
	if err := db.Find(&siteTags).Error; err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	for _, tag := range siteTags {
		label := ""
		if tag.SiteTagName.Valid {
			label = tag.SiteTagName.String
		}
		item := &dto2.SelectItem{
			Value: tag.ID,
			Label: label,
		}
		// 查询站点名称作为副标题
		if tag.SiteID.Valid && tag.SiteID.Int64 > 0 {
			var site entity2.Site
			if err := r.dbFromCtx(ctx).WithContext(ctx).First(&site, tag.SiteID.Int64).Error; err == nil {
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

	return model.NewPage[dto2.SelectItem](results, total, opt.Page, opt.PageSize), nil
}
