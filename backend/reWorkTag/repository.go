package reWorkTag

import (
	"context"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReWorkTagRepository 作品-标签关联仓储实现
type ReWorkTagRepository struct {
	*database.BaseRepository[domain.ReWorkTag]
}

// NewRepository 创建关联仓储
func NewRepository(db *gorm.DB) *ReWorkTagRepository {
	return &ReWorkTagRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkTag](db),
	}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ReWorkTagRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// DeleteByWorkAndTag 根据作品ID和标签删除
func (r *ReWorkTagRepository) DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error {
	query := r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("work_id = ?", workId)

	switch tagType {
	case constant.LOCAL:
		query = query.Where("local_tag_id = ?", tagId)
	case constant.SITE:
		query = query.Where("site_tag_id = ?", tagId)
	default:
		return nil
	}

	return query.Delete(new(domain.ReWorkTag)).Error
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *ReWorkTagRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.ReWorkTag)).Error
}

// DeleteSiteByWorkId 删除作品的全部 SITE 标签关联（保留 LOCAL 关联）
func (r *ReWorkTagRepository) DeleteSiteByWorkId(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("work_id = ? AND tag_type = ?", workId, constant.SITE).
		Delete(new(domain.ReWorkTag)).Error
}

// SaveBatchOnConflict 批量保存，遇任何唯一约束冲突跳过该行（OnConflict DoNothing）。
// LOCAL 关联增量入库用：已存在的 (work_id, local_tag_id) 跳过，保留用户手动设的 namespace 等字段不被新行零值覆盖。
func (r *ReWorkTagRepository) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkTag) error {
	if len(rels) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, rel := range rels {
		if rel.GetID() == 0 {
			rel.SetCreateTime(now)
		}
		rel.SetUpdateTime(now)
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(rels).Error
}

// UpsertBatch 批量 upsert 关联：按 (work_id, tag_id) 唯一约束冲突时更新 namespace，否则插入。
// tagType 决定冲突列：local→(work_id, local_tag_id)，site→(work_id, site_tag_id)。
// 已存在的关联（如已绑定 tag 改了 namespace 重新确认）走 UPDATE namespace；新关联走 INSERT。
func (r *ReWorkTagRepository) UpsertBatch(ctx context.Context, rels []*domain.ReWorkTag, tagType int) error {
	if len(rels) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, rel := range rels {
		rel.SetUpdateTime(now)
		if rel.GetID() == 0 {
			rel.SetCreateTime(now)
		}
	}
	var conflictCols []clause.Column
	if tagType == constant.LOCAL {
		conflictCols = []clause.Column{{Name: "work_id"}, {Name: "local_tag_id"}}
	} else {
		conflictCols = []clause.Column{{Name: "work_id"}, {Name: "site_tag_id"}}
	}
	return r.dbFromCtx(ctx).WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   conflictCols,
			DoUpdates: clause.AssignmentColumns([]string{"namespace", "update_time"}),
		}).Create(rels).Error
}

// ListByWorkId 查询作品关联的所有标签
func (r *ReWorkTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkTag, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "work_id", Value: workId}},
	}
	return r.BaseRepository.List(ctx, opt)
}

// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
func (r *ReWorkTagRepository) ListLocalTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var tagIds []int64
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND local_tag_id > 0", workId).
		Pluck("local_tag_id", &tagIds).Error
	return tagIds, err
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (r *ReWorkTagRepository) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var tagIds []int64
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND site_tag_id > 0", workId).
		Pluck("site_tag_id", &tagIds).Error
	return tagIds, err
}

// GetByWorkAndTag 根据作品ID和标签获取关联
func (r *ReWorkTagRepository) GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error) {
	query := r.dbFromCtx(ctx).
		WithContext(ctx).
		Where("work_id = ?", workId)

	switch tagType {
	case constant.LOCAL:
		query = query.Where("local_tag_id = ?", tagId)
	case constant.SITE:
		query = query.Where("site_tag_id = ?", tagId)
	default:
		return nil, nil
	}

	var result domain.ReWorkTag
	if err := query.First(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

// CountByWorkId 统计作品关联的标签数量
func (r *ReWorkTagRepository) CountByWorkId(ctx context.Context, workId int64) (int64, error) {
	var count int64
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ?", workId).
		Count(&count).Error
	return count, err
}

// ListLocalTagIdsByWorkIds 批量查询多个作品关联的本地标签ID
func (r *ReWorkTagRepository) ListLocalTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error) {
	type row struct {
		WorkID     int64 `gorm:"column:work_id"`
		LocalTagID int64 `gorm:"column:local_tag_id"`
	}
	var rows []row
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Select("work_id, local_tag_id").
		Where("work_id IN ? AND local_tag_id > 0", workIds).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]int64, len(workIds))
	for _, r := range rows {
		result[r.WorkID] = append(result[r.WorkID], r.LocalTagID)
	}
	return result, nil
}

// ListSiteTagIdsByWorkIds 批量查询多个作品关联的站点标签ID
func (r *ReWorkTagRepository) ListSiteTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error) {
	type row struct {
		WorkID    int64 `gorm:"column:work_id"`
		SiteTagID int64 `gorm:"column:site_tag_id"`
	}
	var rows []row
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Select("work_id, site_tag_id").
		Where("work_id IN ? AND site_tag_id > 0", workIds).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]int64, len(workIds))
	for _, r := range rows {
		result[r.WorkID] = append(result[r.WorkID], r.SiteTagID)
	}
	return result, nil
}
