package localAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LocalAuthorRepository 本地作者仓储实现
type LocalAuthorRepository struct {
	*database.BaseRepository[entity.LocalAuthor]
}

// NewRepository 创建本地作者仓储
func NewRepository(db *gorm.DB) *LocalAuthorRepository {
	return &LocalAuthorRepository{
		BaseRepository: database.NewBaseRepository[entity.LocalAuthor](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *LocalAuthorRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *LocalAuthorRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListReWorkAuthor 批量获取作品与作者的关联
func (r *LocalAuthorRepository) ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedLocalAuthor, error) {
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

// ListByWorkId 查询作品的本地作者
func (r *LocalAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
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

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (r *LocalAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
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

// ListSelectItems 查询选择项列表
func (r *LocalAuthorRepository) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*dto.SelectItem, error) {
	var results []*dto.SelectItem

	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
		OrderBy:    []clause.Expression{order},
		Limit:      -1,
	}
	authors, err := r.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	for _, author := range authors {
		label := ""
		if author.AuthorName.Valid {
			label = author.AuthorName.String
		}
		results = append(results, &dto.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return results, nil
}

// QuerySelectItemPage 分页查询选择项
func (r *LocalAuthorRepository) QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.SelectItem], error) {
	var results []*dto.SelectItem

	rawPage, err := r.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	authors := rawPage.Data

	for _, author := range authors {
		label := ""
		if author.AuthorName.Valid {
			label = author.AuthorName.String
		}
		results = append(results, &dto.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return model.NewPage[dto.SelectItem](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
