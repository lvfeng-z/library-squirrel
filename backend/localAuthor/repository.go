package localAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
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
func (r *LocalAuthorRepository) ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*sdkdto.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*sdkdto.RankedLocalAuthor), nil
	}

	// 构建 IN 子句
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
		sdkdto.RankedLocalAuthor
	}

	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// 转换为 map
	resultMap := make(map[int64][]*sdkdto.RankedLocalAuthor)
	for _, res := range results {
		if _, ok := resultMap[res.WorkID]; !ok {
			resultMap[res.WorkID] = make([]*sdkdto.RankedLocalAuthor, 0)
		}
		resultMap[res.WorkID] = append(resultMap[res.WorkID], new(res.RankedLocalAuthor))
	}

	return resultMap, nil
}

// ListByWorkId 查询作品的本地作者
func (r *LocalAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*sdkdto.RankedLocalAuthor, error) {
	query := `
		SELECT t1.id, t1.author_name, t1.introduce, t1.last_use, t1.create_time, t1.update_time, t2.author_rank
		FROM local_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.local_author_id
		WHERE t2.work_id = ?
	`

	var results []*sdkdto.RankedLocalAuthor
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (r *LocalAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*sdkdto.RankedLocalAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*sdkdto.RankedLocalAuthorWithWorkId, 0), nil
	}

	// 构建 IN 子句
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

	var results []*sdkdto.RankedLocalAuthorWithWorkId
	err := r.dbFromCtx(ctx).WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListSelectItems 查询选择项列表
func (r *LocalAuthorRepository) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*sdkdto.SelectItem, error) {
	var results []*sdkdto.SelectItem

	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
		OrderBy:    []clause.Expression{order},
		Limit:      -1,
	}
	authors, err := r.List(ctx, opt)
	if err != nil {
		return nil, err
	}

	// 转换为 SelectItem
	for _, author := range authors {
		label := ""
		if author.AuthorName.Valid {
			label = author.AuthorName.String
		}
		results = append(results, &sdkdto.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return results, nil
}

// QuerySelectItemPage 分页查询选择项
func (r *LocalAuthorRepository) QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[sdkdto.SelectItem], error) {
	var results []*sdkdto.SelectItem

	rawPage, err := r.Page(ctx, opt)
	if err != nil {
		return nil, err
	}
	authors := rawPage.Data

	// 转换为 SelectItem
	for _, author := range authors {
		label := ""
		if author.AuthorName.Valid {
			label = author.AuthorName.String
		}
		results = append(results, &sdkdto.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return model.NewPage[sdkdto.SelectItem](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}
