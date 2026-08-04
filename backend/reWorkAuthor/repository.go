package reWorkAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// ListRelationsByWorkId 查询作品关联的所有作者关联记录（原始实体，含 role_name/sort_order）
func (r *ReWorkAuthorRepository) ListRelationsByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkAuthor, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "work_id", Value: workId}},
	}
	return r.BaseRepository.List(ctx, opt)
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (r *ReWorkAuthorRepository) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
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

	results := make([]*dto.RankedLocalAuthor, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedLocalAuthor())
	}
	return results, nil
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (r *ReWorkAuthorRepository) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
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

	results := make([]*dto.RankedSiteAuthor, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedSiteAuthor())
	}
	return results, nil
}

// ListLocalAuthorsByWorkIds 批量查询作品的本地作者
func (r *ReWorkAuthorRepository) ListLocalAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*dto.RankedLocalAuthor), nil
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

	resultMap := make(map[int64][]*dto.RankedLocalAuthor)
	for _, row := range rows {
		if _, ok := resultMap[row.WorkId]; !ok {
			resultMap[row.WorkId] = make([]*dto.RankedLocalAuthor, 0)
		}
		resultMap[row.WorkId] = append(resultMap[row.WorkId], row.ToRankedLocalAuthor())
	}

	return resultMap, nil
}

// ListSiteAuthorsByWorkIds 批量查询作品的站点作者
func (r *ReWorkAuthorRepository) ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedSiteAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*dto.RankedSiteAuthor), nil
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

	resultMap := make(map[int64][]*dto.RankedSiteAuthor)
	for _, row := range rows {
		if _, ok := resultMap[row.WorkId]; !ok {
			resultMap[row.WorkId] = make([]*dto.RankedSiteAuthor, 0)
		}
		resultMap[row.WorkId] = append(resultMap[row.WorkId], row.ToRankedSiteAuthor())
	}

	return resultMap, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (r *ReWorkAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*dto.RankedLocalAuthorWithWorkId, 0), nil
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

	results := make([]*dto.RankedLocalAuthorWithWorkId, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedLocalAuthorWithWorkId())
	}
	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (r *ReWorkAuthorRepository) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*dto.RankedSiteAuthorWithWorkId, 0), nil
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

	results := make([]*dto.RankedSiteAuthorWithWorkId, 0, len(rows))
	for _, row := range rows {
		results = append(results, row.ToRankedSiteAuthorWithWorkId())
	}
	return results, nil
}
