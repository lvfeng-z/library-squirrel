package reWorkAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
	"gorm.io/gorm"
)

// ReWorkAuthorRepository 作品-作者关联仓储实现
type ReWorkAuthorRepository struct {
	*database.BaseRepository[domain.ReWorkAuthor]
}

// NewRepository 创建作品-作者关联仓储
func NewRepository(db *gorm.DB) *ReWorkAuthorRepository {
	return &ReWorkAuthorRepository{
		BaseRepository: database.NewBaseRepository[domain.ReWorkAuthor](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *ReWorkAuthorRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *ReWorkAuthorRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// Save 保存
func (r *ReWorkAuthorRepository) Save(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return r.BaseRepository.Save(ctx, reWorkAuthor)
}

// SaveBatch 批量保存
func (r *ReWorkAuthorRepository) SaveBatch(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error {
	return r.BaseRepository.SaveBatch(ctx, reWorkAuthors)
}

// Update 更新
func (r *ReWorkAuthorRepository) Update(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return r.BaseRepository.Update(ctx, reWorkAuthor)
}

// Delete 删除
func (r *ReWorkAuthorRepository) Delete(ctx context.Context, id int64) error {
	return r.BaseRepository.Delete(ctx, id)
}

// GetById 根据ID获取
func (r *ReWorkAuthorRepository) GetById(ctx context.Context, id int64) (*domain.ReWorkAuthor, error) {
	return r.BaseRepository.GetById(ctx, id)
}

// List 查询列表
func (r *ReWorkAuthorRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.ReWorkAuthor, error) {
	return r.BaseRepository.List(ctx, opt)
}

// Count 统计数量
func (r *ReWorkAuthorRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return r.BaseRepository.Count(ctx, opt)
}

// DeleteByWorkId 根据作品ID删除所有关联
func (r *ReWorkAuthorRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Where("work_id = ?", workId).Delete(&domain.ReWorkAuthor{}).Error
}

// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
func (r *ReWorkAuthorRepository) DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Where("local_author_id = ?", localAuthorId).Delete(&domain.ReWorkAuthor{}).Error
}

// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
func (r *ReWorkAuthorRepository) DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Where("site_author_id = ?", siteAuthorId).Delete(&domain.ReWorkAuthor{}).Error
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (r *ReWorkAuthorRepository) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedLocalAuthor, error) {
	query := `
		SELECT t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time,
		       t2.role_name, t2.sort_order
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id = ?
	`

	var rows []*dto.LocalAuthorRankScanRow
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workId).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]*sdkdto.RankedLocalAuthor, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedLocalAuthor())
	}
	return results, nil
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (r *ReWorkAuthorRepository) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedSiteAuthor, error) {
	query := `
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.homepage, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.role_name, t2.sort_order
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id = ?
	`

	var rows []*dto.SiteAuthorRankScanRow
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workId).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]*sdkdto.RankedSiteAuthor, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedSiteAuthor())
	}
	return results, nil
}

// ListLocalAuthorsByWorkIds 批量查询作品的本地作者
func (r *ReWorkAuthorRepository) ListLocalAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*sdkdto.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*sdkdto.RankedLocalAuthor), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t2.work_id, t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time,
		       t2.role_name, t2.sort_order
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var rows []*struct {
		WorkId int64 `gorm:"column:work_id"`
		dto.LocalAuthorRankScanRow
	}

	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	resultMap := make(map[int64][]*sdkdto.RankedLocalAuthor)
	for _, row := range rows {
		if _, ok := resultMap[row.WorkId]; !ok {
			resultMap[row.WorkId] = make([]*sdkdto.RankedLocalAuthor, 0)
		}
		resultMap[row.WorkId] = append(resultMap[row.WorkId], row.ToRankedLocalAuthor())
	}

	return resultMap, nil
}

// ListSiteAuthorsByWorkIds 批量查询作品的站点作者
func (r *ReWorkAuthorRepository) ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*sdkdto.RankedSiteAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*sdkdto.RankedSiteAuthor), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t2.work_id, t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.homepage, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.role_name, t2.sort_order
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var rows []*struct {
		WorkId int64 `gorm:"column:work_id"`
		dto.SiteAuthorRankScanRow
	}

	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	resultMap := make(map[int64][]*sdkdto.RankedSiteAuthor)
	for _, row := range rows {
		if _, ok := resultMap[row.WorkId]; !ok {
			resultMap[row.WorkId] = make([]*sdkdto.RankedSiteAuthor, 0)
		}
		resultMap[row.WorkId] = append(resultMap[row.WorkId], row.ToRankedSiteAuthor())
	}

	return resultMap, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (r *ReWorkAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedLocalAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*sdkdto.RankedLocalAuthorWithWorkId, 0), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time,
		       t2.role_name, t2.sort_order, t2.work_id
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var rows []*dto.LocalAuthorRankWithWorkIdScanRow
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]*sdkdto.RankedLocalAuthorWithWorkId, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedLocalAuthorWithWorkId())
	}
	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (r *ReWorkAuthorRepository) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedSiteAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*sdkdto.RankedSiteAuthorWithWorkId, 0), nil
	}

	placeholders := make([]string, len(workIds))
	args := make([]interface{}, len(workIds))
	for i, id := range workIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.homepage, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.role_name, t2.sort_order, t2.work_id
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var rows []*dto.SiteAuthorRankWithWorkIdScanRow
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	results := make([]*sdkdto.RankedSiteAuthorWithWorkId, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedSiteAuthorWithWorkId())
	}
	return results, nil
}
