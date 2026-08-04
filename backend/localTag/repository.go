package localTag

import (
	"context"
	"errors"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LocalTagRepository 本地标签仓储实现
type LocalTagRepository struct {
	*database.BaseRepository[entity.LocalTag]
}

// NewRepository 创建本地标签仓储
func NewRepository(db *gorm.DB) *LocalTagRepository {
	return &LocalTagRepository{
		BaseRepository: database.NewBaseRepository[entity.LocalTag](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *LocalTagRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *LocalTagRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// GetByName 根据名称获取
func (r *LocalTagRepository) GetByName(ctx context.Context, name string) (*entity.LocalTag, error) {
	var tag entity.LocalTag
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("local_tag_name = ?", name).
		First(&tag).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tag, nil
}

// GetByNames 根据名称列表批量查询本地标签
func (r *LocalTagRepository) GetByNames(ctx context.Context, names []string) ([]*entity.LocalTag, error) {
	if len(names) == 0 {
		return make([]*entity.LocalTag, 0), nil
	}
	var tags []*entity.LocalTag
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where(clause.IN{Column: "local_tag_name", Values: util.ToAnySlice(names)}).
		Find(&tags).Error
	return tags, err
}

// SelectTreeNode 递归查询子标签
func (r *LocalTagRepository) SelectTreeNode(ctx context.Context, rootId int64, depth int) ([]*entity.LocalTag, error) {
	if depth <= 0 {
		depth = 10
	}

	// 使用 GORM Raw 执行递归 CTE 查询
	query := `
		WITH RECURSIVE treeNode AS
		(
			SELECT *, 1 AS level, NOT EXISTS(SELECT 1 FROM local_tag WHERE base_local_tag_id = t1.id) AS isLeaf
			FROM local_tag t1
			WHERE base_local_tag_id = ?
			UNION ALL
			SELECT t1.*, treeNode.level + 1 AS level, NOT EXISTS(SELECT 1 FROM local_tag WHERE base_local_tag_id = t1.id) AS isLeaf
			FROM local_tag t1
			JOIN treeNode ON t1.base_local_tag_id = treeNode.id
			WHERE treeNode.level < ?
		)
		SELECT id, local_tag_name, base_local_tag_id, last_use, create_time, update_time FROM treeNode
	`

	var tags []*entity.LocalTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, rootId, depth).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// SelectParentNode 递归查询上级标签
func (r *LocalTagRepository) SelectParentNode(ctx context.Context, nodeId int64) ([]*entity.LocalTag, error) {
	query := `
		WITH RECURSIVE parentNode AS
		(
			SELECT *
			FROM local_tag
			WHERE id = ?
			UNION ALL
			SELECT local_tag.*
			FROM local_tag
				JOIN parentNode ON local_tag.id = parentNode.base_local_tag_id
		)
		SELECT * FROM parentNode
	`

	var tags []*entity.LocalTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, nodeId).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// ListByWorkId 查询作品关联的本地标签
func (r *LocalTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*entity.LocalTag, error) {
	query := `
		SELECT t1.id, t1.local_tag_name, t1.base_local_tag_id, t1.last_use, t1.create_time, t1.update_time
		FROM local_tag t1
		INNER JOIN re_work_tag t2 ON t1.id = t2.local_tag_id
		WHERE t2.work_id = ?
	`

	var tags []*entity.LocalTag
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workId).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// ListSelectItems 查询选择项列表
func (r *LocalTagRepository) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*dto.SelectItem, error) {
	var results []*dto.SelectItem

	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
		OrderBy:    []clause.Expression{order},
		Limit:      -1,
	}
	tags, err := r.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	for _, tag := range tags {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		results = append(results, &dto.SelectItem{
			Value: tag.ID,
			Label: label,
		})
	}

	return results, nil
}

// QuerySelectItemPage 分页查询选择项
func (r *LocalTagRepository) QuerySelectItemPage(ctx context.Context, opt *database.PageOption, secondaryLabel string) (*model.Page[dto.SelectItem], error) {
	rawPage, err := r.BaseRepository.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	tags := rawPage.Data

	// 转换为 SelectItem
	var results []*dto.SelectItem
	for _, tag := range tags {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		item := &dto.SelectItem{
			Value: tag.ID,
			Label: label,
		}
		if secondaryLabel != "" {
			item.SubLabels = []string{secondaryLabel}
		}
		results = append(results, item)
	}

	return model.NewPage[dto.SelectItem](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// QueryPageByWorkId 根据作品ID分页查询
func (r *LocalTagRepository) QueryPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[entity.LocalTag], error) {
	// 构建 EXISTS / NOT EXISTS 子查询
	if boundOnWorkId != nil && *boundOnWorkId {
		opt.Conditions = append(opt.Conditions, clause.Expr{SQL: "EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND local_tag_id = local_tag.id)", Vars: []interface{}{workId}})
	} else if boundOnWorkId != nil && !*boundOnWorkId {
		opt.Conditions = append(opt.Conditions, clause.Expr{SQL: "NOT EXISTS (SELECT 1 FROM re_work_tag WHERE work_id = ? AND local_tag_id = local_tag.id)", Vars: []interface{}{workId}})
	}

	return r.BaseRepository.Page(ctx, opt)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (r *LocalTagRepository) QuerySelectItemPageByWorkId(ctx context.Context, opt *database.PageOption, workId int64, boundOnWorkId *bool) (*model.Page[dto.SelectItem], error) {
	pageResult, err := r.QueryPageByWorkId(ctx, opt, workId, boundOnWorkId)
	if err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	items := make([]*dto.SelectItem, len(pageResult.Data))
	for i, tag := range pageResult.Data {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		items[i] = &dto.SelectItem{
			Value: tag.ID,
			Label: label,
		}
	}
	return model.NewPage[dto.SelectItem](items, pageResult.DataCount, pageResult.PageNumber, pageResult.PageSize), nil
}

// QueryWithBaseTagPage 分页查询包含基础标签信息的本地标签
func (r *LocalTagRepository) QueryWithBaseTagPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.LocalTagWithBaseTagDTO], error) {
	var results []struct {
		TagID              int64   `gorm:"column:id"`
		TagName            *string `gorm:"column:local_tag_name"`
		TagBaseID          *int64  `gorm:"column:base_local_tag_id"`
		TagDescription     *string `gorm:"column:description"`
		TagLastUse         *int64  `gorm:"column:last_use"`
		TagCreateTime      int64   `gorm:"column:create_time"`
		TagUpdateTime      int64   `gorm:"column:update_time"`
		BaseTagID          *int64  `gorm:"column:base_tag__id"`
		BaseTagName        *string `gorm:"column:base_tag__local_tag_name"`
		BaseTagBaseID      *int64  `gorm:"column:base_tag__base_local_tag_id"`
		BaseTagDescription *string `gorm:"column:base_tag__description"`
		BaseTagLastUse     *int64  `gorm:"column:base_tag__last_use"`
		BaseTagCreateTime  *int64  `gorm:"column:base_tag__create_time"`
		BaseTagUpdateTime  *int64  `gorm:"column:base_tag__update_time"`
	}
	var total int64

	db := r.dbFromCtx(ctx).WithContext(ctx).
		Model(&entity.LocalTag{}).
		Select("local_tag.*, base_tag.id as base_tag__id, base_tag.local_tag_name as base_tag__local_tag_name, base_tag.base_local_tag_id as base_tag__base_local_tag_id, base_tag.description as base_tag__description, base_tag.last_use as base_tag__last_use, base_tag.create_time as base_tag__create_time, base_tag.update_time as base_tag__update_time").
		Joins("LEFT JOIN local_tag base_tag ON local_tag.base_local_tag_id = base_tag.id")

	// 应用查询条件
	for _, cond := range opt.Conditions {
		db = db.Where(cond)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用排序
	if len(opt.OrderBy) > 0 {
		db = db.Order(opt.OrderBy)
	}

	// 应用分页
	offset := (opt.Page - 1) * opt.PageSize
	db = db.Offset(offset).Limit(opt.PageSize)

	if err := db.Scan(&results).Error; err != nil {
		return nil, err
	}

	// 直接构造组合 DTO，无需中间 entity 转换
	dtoList := make([]*dto.LocalTagWithBaseTagDTO, len(results))
	for i, result := range results {
		localTag := &sdkdto.LocalTagDTO{
			ID:             result.TagID,
			LocalTagName:   result.TagName,
			BaseLocalTagID: result.TagBaseID,
			Description:    result.TagDescription,
			LastUse:        result.TagLastUse,
			CreateTime:     result.TagCreateTime,
			UpdateTime:     result.TagUpdateTime,
		}

		var baseTag *sdkdto.LocalTagDTO
		if result.BaseTagID != nil {
			baseTag = &sdkdto.LocalTagDTO{
				ID:             *result.BaseTagID,
				LocalTagName:   result.BaseTagName,
				BaseLocalTagID: result.BaseTagBaseID,
				Description:    result.BaseTagDescription,
				LastUse:        result.BaseTagLastUse,
				CreateTime:     *result.BaseTagCreateTime,
				UpdateTime:     *result.BaseTagUpdateTime,
			}
		}

		dtoList[i] = &dto.LocalTagWithBaseTagDTO{
			LocalTag: localTag,
			BaseTag:  baseTag,
		}
	}

	return model.NewPage[dto.LocalTagWithBaseTagDTO](dtoList, total, opt.Page, opt.PageSize), nil
}

// Page 分页查询
func (r *LocalTagRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.LocalTag], error) {
	return r.BaseRepository.Page(ctx, opt)
}
