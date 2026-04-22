package reWorkTag

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/util"
	domain "github.com/library-squirrel/wails/pkg/model/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TagType 标签类型
const (
	TagTypeLocal = 1
	TagTypeSite  = 2
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

// DeleteByWorkAndTag 根据作品ID和标签删除
func (r *ReWorkTagRepository) DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error {
	query := r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId)

	switch tagType {
	case TagTypeLocal:
		query = query.Where("local_tag_id = ?", tagId)
	case TagTypeSite:
		query = query.Where("site_tag_id = ?", tagId)
	default:
		return nil
	}

	return query.Delete(new(domain.ReWorkTag)).Error
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *ReWorkTagRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.ReWorkTag)).Error
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
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND local_tag_id > 0", workId).
		Pluck("local_tag_id", &tagIds).Error
	return tagIds, err
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (r *ReWorkTagRepository) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var tagIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND site_tag_id > 0", workId).
		Pluck("site_tag_id", &tagIds).Error
	return tagIds, err
}

// GetByWorkAndTag 根据作品ID和标签获取关联
func (r *ReWorkTagRepository) GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error) {
	query := r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId)

	switch tagType {
	case TagTypeLocal:
		query = query.Where("local_tag_id = ?", tagId)
	case TagTypeSite:
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
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ?", workId).
		Count(&count).Error
	return count, err
}

// SaveBatch 批量保存
func (r *ReWorkTagRepository) SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error {
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
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Create(rels).Error
}
