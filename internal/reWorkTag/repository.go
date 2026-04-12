package reWorkTag

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TagType 标签类型
const (
	TagTypeLocal = 1
	TagTypeSite  = 2
)

// Repository 作品-标签关联仓储接口
type Repository interface {
	// Save 保存关联
	Save(ctx context.Context, rel *domain.ReWorkTag) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndTag 根据作品ID和标签删除
	DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// ListByWorkId 查询作品关联的所有标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkTag, error)
	// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
	ListLocalTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
	ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// GetByWorkAndTag 根据作品ID和标签获取关联
	GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error)
	// CountByWorkId 统计作品关联的标签数量
	CountByWorkId(ctx context.Context, workId int64) (int64, error)
}

// reWorkTagRepository 作品-标签关联仓储实现
type reWorkTagRepository struct {
	*database.BaseRepository[domain.ReWorkTag]
}

// NewRepository 创建关联仓储
func NewRepository(db *gorm.DB) Repository {
	return &reWorkTagRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkTag](db),
	}
}

// DeleteByWorkAndTag 根据作品ID和标签删除
func (r *reWorkTagRepository) DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error {
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
func (r *reWorkTagRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.ReWorkTag)).Error
}

// ListByWorkId 查询作品关联的所有标签
func (r *reWorkTagRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkTag, error) {
	where := clause.Eq{Column: "work_id", Value: workId}
	return r.BaseRepository.List(ctx, []clause.Expression{where}, nil, 0, 0)
}

// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
func (r *reWorkTagRepository) ListLocalTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var tagIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND local_tag_id > 0", workId).
		Pluck("local_tag_id", &tagIds).Error
	return tagIds, err
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (r *reWorkTagRepository) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var tagIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ? AND site_tag_id > 0", workId).
		Pluck("site_tag_id", &tagIds).Error
	return tagIds, err
}

// GetByWorkAndTag 根据作品ID和标签获取关联
func (r *reWorkTagRepository) GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error) {
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
func (r *reWorkTagRepository) CountByWorkId(ctx context.Context, workId int64) (int64, error) {
	var count int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkTag)).
		Where("work_id = ?", workId).
		Count(&count).Error
	return count, err
}

// SaveBatch 批量保存
func (r *reWorkTagRepository) SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error {
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
