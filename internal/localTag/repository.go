package localTag

import (
	"context"
	"errors"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LocalTagRepository 本地标签仓储实现
type LocalTagRepository struct {
	*database.BaseRepository[domain.LocalTag]
}

// NewRepository 创建本地标签仓储
func NewRepository(db *gorm.DB) *LocalTagRepository {
	return &LocalTagRepository{
		BaseRepository: database.NewBaseRepository[domain.LocalTag](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *LocalTagRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// GetByName 根据名称获取
func (r *LocalTagRepository) GetByName(ctx context.Context, name string) (*domain.LocalTag, error) {
	var tag domain.LocalTag
	err := r.GORM().
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

// SelectTreeNode 递归查询子标签
func (r *LocalTagRepository) SelectTreeNode(ctx context.Context, rootId int64, depth int) ([]*domain.LocalTag, error) {
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

	var tags []*domain.LocalTag
	err := r.GORM().WithContext(ctx).Raw(query, rootId, depth).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// SelectParentNode 递归查询上级标签
func (r *LocalTagRepository) SelectParentNode(ctx context.Context, nodeId int64) ([]*domain.LocalTag, error) {
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

	var tags []*domain.LocalTag
	err := r.GORM().WithContext(ctx).Raw(query, nodeId).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// ListByWorkId 查询作品关联的本地标签
func (r *LocalTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.LocalTag, error) {
	query := `
		SELECT t1.id, t1.local_tag_name, t1.base_local_tag_id, t1.last_use, t1.create_time, t1.update_time
		FROM local_tag t1
		INNER JOIN re_work_tag t2 ON t1.id = t2.local_tag_id
		WHERE t2.work_id = ?
	`

	var tags []*domain.LocalTag
	err := r.GORM().WithContext(ctx).Raw(query, workId).Scan(&tags).Error
	if err != nil {
		return nil, err
	}
	return tags, nil
}

// QueryDTOPage DTO分页查询
func (r *LocalTagRepository) QueryDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error) {
	var tags []*domain.LocalTag
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.LocalTag{})

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

	if err := db.Scan(&tags).Error; err != nil {
		return nil, err
	}

	return model.NewPage[domain.LocalTag, LocalTagQueryDTO](tags, total, page, pageSize), nil
}

// ListSelectItems 查询选择项列表
func (r *LocalTagRepository) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error) {
	var results []*domain.SelectItem

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
		results = append(results, &domain.SelectItem{
			Value: tag.ID,
			Label: label,
		})
	}

	return results, nil
}

// QuerySelectItemPage 分页查询选择项
func (r *LocalTagRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, secondaryLabel string) (*model.Page[domain.SelectItem, LocalTagQueryDTO], error) {
	var results []*domain.SelectItem

	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: []clause.Expression{where},
			OrderBy:    []clause.Expression{order},
		},
		Page:     page,
		PageSize: pageSize,
	}
	rawPage, err := r.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	tags := rawPage.Data

	// 转换为 SelectItem
	for _, tag := range tags {
		label := ""
		if tag.LocalTagName.Valid {
			label = tag.LocalTagName.String
		}
		item := &domain.SelectItem{
			Value: tag.ID,
			Label: label,
		}
		if secondaryLabel != "" {
			item.SubLabels = []string{secondaryLabel}
		}
		results = append(results, item)
	}

	return model.NewPage[domain.SelectItem, LocalTagQueryDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// QueryPageByWorkId 根据作品ID分页查询
func (r *LocalTagRepository) QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.LocalTag, LocalTagQueryDTO], error) {
	var tags []*domain.LocalTag
	var total int64

	db := r.GORM().WithContext(ctx).
		Model(&domain.LocalTag{}).
		Joins("INNER JOIN re_work_tag ON local_tag.id = re_work_tag.local_tag_id").
		Where("re_work_tag.work_id = ?", workId)

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

	if err := db.Scan(&tags).Error; err != nil {
		return nil, err
	}

	return model.NewPage[domain.LocalTag, LocalTagQueryDTO](tags, total, page, pageSize), nil
}
