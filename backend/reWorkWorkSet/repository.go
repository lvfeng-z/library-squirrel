package reWorkWorkSet

import (
	"context"
	"fmt"
	"strings"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
)

// ReWorkWorkSetRepository 作品-作品集关联仓储实现
type ReWorkWorkSetRepository struct {
	*database.BaseRepository[domain.ReWorkWorkSet]
}

// NewRepository 创建关联仓储
func NewRepository(db *gorm.DB) *ReWorkWorkSetRepository {
	return &ReWorkWorkSetRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkWorkSet](db),
	}
}

// DeleteByWorkAndWorkSet 根据作品ID和作品集ID删除
func (r *ReWorkWorkSetRepository) DeleteByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// DeleteByWorkSetId 根据作品集ID删除所有关联
func (r *ReWorkWorkSetRepository) DeleteByWorkSetId(ctx context.Context, workSetId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_set_id = ?", workSetId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *ReWorkWorkSetRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.ReWorkWorkSet)).Error
}

// ListByWorkSetId 查询作品集关联的所有作品ID
func (r *ReWorkWorkSetRepository) ListByWorkSetId(ctx context.Context, workSetId int64) ([]int64, error) {
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
func (r *ReWorkWorkSetRepository) ListByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	var workSetIds []int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ?", workId).
		Pluck("work_set_id", &workSetIds).Error
	return workSetIds, err
}

// GetByWorkAndWorkSet 根据作品ID和作品集ID获取关联
func (r *ReWorkWorkSetRepository) GetByWorkAndWorkSet(ctx context.Context, workId, workSetId int64) (*domain.ReWorkWorkSet, error) {
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
func (r *ReWorkWorkSetRepository) CountByWorkSetId(ctx context.Context, workSetId int64) (int64, error) {
	var count int64
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ?", workSetId).
		Count(&count).Error
	return count, err
}

// UpdateSortOrder 更新排序顺序
func (r *ReWorkWorkSetRepository) UpdateSortOrder(ctx context.Context, workId, workSetId int64, sortOrder int) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Update("sort_order", sortOrder).Error
}

// UpdateIsCover 更新封面标记
func (r *ReWorkWorkSetRepository) UpdateIsCover(ctx context.Context, workId, workSetId int64, isCover bool) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_id = ? AND work_set_id = ?", workId, workSetId).
		Update("is_cover", isCover).Error
}

// ClearOtherCovers 清除作品集的其他封面
func (r *ReWorkWorkSetRepository) ClearOtherCovers(ctx context.Context, workSetId int64, exceptWorkId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ? AND work_id != ?", workSetId, exceptWorkId).
		Update("is_cover", false).Error
}

// UpdateSortOrders 批量更新排序顺序
func (r *ReWorkWorkSetRepository) UpdateSortOrders(ctx context.Context, workSetId int64, sortOrders map[int64]int) error {
	if len(sortOrders) == 0 {
		return nil
	}
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("work_set_id = ?", workSetId).
		Where("work_id IN ?", getMapKeys(sortOrders)).
		Updates(map[string]interface{}{
			"sort_order":  gorm.Expr(buildCaseExpression(sortOrders)),
			"update_time": util.GetCurrentTimestamp(),
		}).Error
}

// GetCoverWorkId 获取封面作品ID
func (r *ReWorkWorkSetRepository) GetCoverWorkId(ctx context.Context, workSetId int64) (int64, error) {
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

// ListCoverWorkIdsByWorkSetIds 批量查询多个作品集的封面作品ID（is_cover = 1）
func (r *ReWorkWorkSetRepository) ListCoverWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) (map[int64]int64, error) {
	if len(workSetIds) == 0 {
		return map[int64]int64{}, nil
	}
	type result struct {
		WorkSetID int64
		WorkID    int64
	}
	var results []result
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Model(new(domain.ReWorkWorkSet)).
		Where("is_cover = 1 AND work_set_id IN ?", workSetIds).
		Select("work_set_id, work_id").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	coverMap := make(map[int64]int64, len(results))
	for _, r := range results {
		coverMap[r.WorkSetID] = r.WorkID
	}
	return coverMap, nil
}

// ListMinSortOrderWorkIdsByWorkSetIds 批量查询多个作品集中排序最小的作品ID（兜底封面）
func (r *ReWorkWorkSetRepository) ListMinSortOrderWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) (map[int64]int64, error) {
	if len(workSetIds) == 0 {
		return map[int64]int64{}, nil
	}
	type result struct {
		WorkSetID int64
		WorkID    int64
	}
	var results []result
	subQuery := r.BaseRepository.GORM().
		Model(new(domain.ReWorkWorkSet)).
		Select("work_set_id, MIN(sort_order) as sort_order").
		Where("work_set_id IN ?", workSetIds).
		Group("work_set_id")
	err := r.BaseRepository.GORM().
		WithContext(ctx).
		Table("re_work_work_set").
		Where("work_set_id IN ?", workSetIds).
		Where("(work_set_id, sort_order) IN (?)", subQuery).
		Select("work_set_id, work_id").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	fallbackMap := make(map[int64]int64, len(results))
	for _, r := range results {
		fallbackMap[r.WorkSetID] = r.WorkID
	}
	return fallbackMap, nil
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
func (r *ReWorkWorkSetRepository) SaveBatch(ctx context.Context, reWorkWorkSets []*domain.ReWorkWorkSet) error {
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
