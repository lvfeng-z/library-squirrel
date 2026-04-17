package reWorkAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
)

// Repository 作品-作者关联仓储接口
type Repository interface {
	// Save 保存
	Save(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error
	// Update 更新
	Update(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.ReWorkAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.ReWorkAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
	DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error
	// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
	DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error

	// ========== 批量查询作者信息 ==========

	// ListLocalAuthorsByWorkId 查询作品关联的本地作者
	ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error)
	// ListSiteAuthorsByWorkId 查询作品关联的站点作者
	ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error)
	// ListLocalAuthorsByWorkIds 批量查询作品的本地作者
	ListLocalAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*model.RankedLocalAuthor, error)
	// ListSiteAuthorsByWorkIds 批量查询作品的站点作者
	ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*model.RankedSiteAuthor, error)
	// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
	ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error)
}

// reWorkAuthorRepository 作品-作者关联仓储实现
type reWorkAuthorRepository struct {
	*database.BaseRepository[domain.ReWorkAuthor]
}

// NewRepository 创建作品-作者关联仓储
func NewRepository(db *gorm.DB) Repository {
	return &reWorkAuthorRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkAuthor](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *reWorkAuthorRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// Save 保存
func (r *reWorkAuthorRepository) Save(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return r.BaseRepository.Save(ctx, reWorkAuthor)
}

// SaveBatch 批量保存
func (r *reWorkAuthorRepository) SaveBatch(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error {
	return r.BaseRepository.SaveBatch(ctx, reWorkAuthors)
}

// Update 更新
func (r *reWorkAuthorRepository) Update(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return r.BaseRepository.Update(ctx, reWorkAuthor)
}

// Delete 删除
func (r *reWorkAuthorRepository) Delete(ctx context.Context, id int64) error {
	return r.BaseRepository.Delete(ctx, id)
}

// GetById 根据ID获取
func (r *reWorkAuthorRepository) GetById(ctx context.Context, id int64) (*domain.ReWorkAuthor, error) {
	return r.BaseRepository.GetById(ctx, id)
}

// List 查询列表
func (r *reWorkAuthorRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.ReWorkAuthor, error) {
	return r.BaseRepository.List(ctx, opt)
}

// Count 统计数量
func (r *reWorkAuthorRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return r.BaseRepository.Count(ctx, opt)
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *reWorkAuthorRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.GORM().WithContext(ctx).Where("work_id = ?", workId).Delete(&domain.ReWorkAuthor{}).Error
}

// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
func (r *reWorkAuthorRepository) DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error {
	return r.GORM().WithContext(ctx).Where("local_author_id = ?", localAuthorId).Delete(&domain.ReWorkAuthor{}).Error
}

// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
func (r *reWorkAuthorRepository) DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error {
	return r.GORM().WithContext(ctx).Where("site_author_id = ?", siteAuthorId).Delete(&domain.ReWorkAuthor{}).Error
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (r *reWorkAuthorRepository) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error) {
	query := `
		SELECT t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time, t2.author_rank
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id = ?
	`

	var results []*model.RankedLocalAuthor
	err := r.GORM().WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (r *reWorkAuthorRepository) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error) {
	query := `
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.author_rank
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id = ?
	`

	var results []*model.RankedSiteAuthor
	err := r.GORM().WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListLocalAuthorsByWorkIds 批量查询作品的本地作者
func (r *reWorkAuthorRepository) ListLocalAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*model.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*model.RankedLocalAuthor), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t2.work_id, t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time, t2.author_rank
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var results []*struct {
		WorkID int64 `gorm:"column:work_id"`
		model.RankedLocalAuthor
	}

	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	resultMap := make(map[int64][]*model.RankedLocalAuthor)
	for _, res := range results {
		if _, ok := resultMap[res.WorkID]; !ok {
			resultMap[res.WorkID] = make([]*model.RankedLocalAuthor, 0)
		}
		ranked := res.RankedLocalAuthor
		resultMap[res.WorkID] = append(resultMap[res.WorkID], &ranked)
	}

	return resultMap, nil
}

// ListSiteAuthorsByWorkIds 批量查询作品的站点作者
func (r *reWorkAuthorRepository) ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*model.RankedSiteAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*model.RankedSiteAuthor), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t2.work_id, t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.author_rank
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var results []*struct {
		WorkID int64 `gorm:"column:work_id"`
		model.RankedSiteAuthor
	}

	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	resultMap := make(map[int64][]*model.RankedSiteAuthor)
	for _, res := range results {
		if _, ok := resultMap[res.WorkID]; !ok {
			resultMap[res.WorkID] = make([]*model.RankedSiteAuthor, 0)
		}
		siteAuthor := res.RankedSiteAuthor
		resultMap[res.WorkID] = append(resultMap[res.WorkID], &siteAuthor)
	}

	return resultMap, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (r *reWorkAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*model.RankedLocalAuthorWithWorkId, 0), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time, t2.author_rank, t2.work_id
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var results []*model.RankedLocalAuthorWithWorkId
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (r *reWorkAuthorRepository) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*model.RankedSiteAuthorWithWorkId, 0), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.author_rank, t2.work_id
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var results []*model.RankedSiteAuthorWithWorkId
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}
