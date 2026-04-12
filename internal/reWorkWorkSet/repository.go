package reWorkWorkSet

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"

	"gorm.io/gorm"
)

// Repository 作品-作品集关联仓储接口
type Repository interface {
	// Save 保存关联
	Save(ctx context.Context, reWorkWorkSet *domain.ReWorkWorkSet) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, reWorkWorkSets []*domain.ReWorkWorkSet) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndWorkSet 根据作品ID和作品集ID删除
	DeleteByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) error
	// DeleteByWorkSetId 根据作品集ID删除所有关联
	DeleteByWorkSetId(ctx context.Context, workSetId int64) error
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// ListByWorkSetId 查询作品集关联的所有作品ID
	ListByWorkSetId(ctx context.Context, workSetId int64) ([]int64, error)
	// ListByWorkId 查询作品关联的所有作品集ID
	ListByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// GetByWorkAndWorkSet 根据作品ID和作品集ID获取关联
	GetByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) (*domain.ReWorkWorkSet, error)
	// CountByWorkSetId 统计作品集关联的作品数量
	CountByWorkSetId(ctx context.Context, workSetId int64) (int64, error)
	// UpdateSortOrder 更新排序顺序
	UpdateSortOrder(ctx context.Context, workId, workSetId int64, sortOrder int) error
	// UpdateSortOrders 批量更新排序顺序
	UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error
	// UpdateIsCover 更新封面标记
	UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover int) error
	// ClearOtherCovers 清除作品集的其他封面
	ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error
	// GetCoverWorkId 获取封面作品ID
	GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error)
}

// reWorkWorkSetRepository 作品-作品集关联仓储实现
type reWorkWorkSetRepository struct {
	*database.BaseRepository[domain.ReWorkWorkSet]
}

// NewRepository 创建关联仓储
func NewRepository(db *gorm.DB) Repository {
	return &reWorkWorkSetRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkWorkSet](db),
	}
}

// DeleteByWorkAndWorkSet 根据作品ID和作品集ID删除
func (r *reWorkWorkSetRepository) DeleteByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// DeleteByWorkSetId 根据作品集ID删除所有关联
func (r *reWorkWorkSetRepository) DeleteByWorkSetId(ctx context.Context, workSetId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_set_id = ?", workSetId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *reWorkWorkSetRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// ListByWorkSetId 查询作品集关联的所有作品ID
func (r *reWorkWorkSetRepository) ListByWorkSetId(ctx context.Context, workSetId int64) ([]int64, error) {
	var workIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ?", workSetId).
		Order("sort_order ASC").
		Pluck("work_id", &workIds).Error
	return workIds, err
}

// ListByWorkId 查询作品关联的所有作品集ID
func (r *reWorkWorkSetRepository) ListByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var workSetIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ?", workId).
		Pluck("work_set_id", &workSetIds).Error
	return workSetIds, err
}

// GetByWorkAndWorkSet 根据作品ID和作品集ID获取关联
func (r *reWorkWorkSetRepository) GetByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) (*domain.ReWorkWorkSet, error) {
	var result domain.ReWorkWorkSet
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// CountByWorkSetId 统计作品集关联的作品数量
func (r *reWorkWorkSetRepository) CountByWorkSetId(ctx context.Context, workSetId int64) (int64, error) {
	var count int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ?", workSetId).
		Count(&count).Error
	return count, err
}

// UpdateSortOrder 更新排序顺序
func (r *reWorkWorkSetRepository) UpdateSortOrder(ctx context.Context, workId, workSetId int64, sortOrder int) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Update("sort_order", sortOrder).Error
}

// UpdateIsCover 更新封面标记
func (r *reWorkWorkSetRepository) UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover int) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Update("is_cover", isCover).Error
}

// ClearOtherCovers 清除作品集的其他封面
func (r *reWorkWorkSetRepository) ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ? AND work_id != ?", workSetId, exceptWorkId).
		Update("is_cover", 0).Error
}

// UpdateSortOrders 批量更新排序顺序
func (r *reWorkWorkSetRepository) UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error {
	if len(sortOrders) == 0 {
		return nil
	}
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ?", workSetId).
		Where("work_id IN ?", getMapKeys(sortOrders)).
		Updates(map[string]interface{}{
			"sort_order":  gorm.Expr("CASE work_id %s END", buildCaseExpression(sortOrders)),
			"update_time": util.GetCurrentTimestamp(),
		}).Error
}

// GetCoverWorkId 获取封面作品ID
func (r *reWorkWorkSetRepository) GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error) {
	var workId int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ? AND is_cover = 1", workSetId).
		Pluck("work_id", &workId).Error
	if err != nil {
		return 0, err
	}
	return workId, nil
}

// getMapKeys 获取map的key列表
func getMapKeys(m map[int64]int) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// buildCaseExpression 构建CASE表达式
func buildCaseExpression(sortOrders map[int64]int) string {
	var cases []string
	for workId, order := range sortOrders {
		cases = append(cases, fmt.Sprintf("WHEN %d THEN %d", workId, order))
	}
	return "CASE work_id " + strings.Join(cases, " ") + " END"
}

// SaveBatch 批量保存
func (r *reWorkWorkSetRepository) SaveBatch(ctx context.Context, reWorkWorkSets []*domain.ReWorkWorkSet) error {
	if len(reWorkWorkSets) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, rel := range reWorkWorkSets {
		if rel.GetID() == 0 {
			rel.SetCreateTime(now)
		}
		rel.SetUpdateTime(now)
	}
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Create(reWorkWorkSets).Error
}
