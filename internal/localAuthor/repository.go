package localAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// localAuthorRepository 本地作者仓储实现
// 不嵌入 database.BaseRepository 以避免 Page 返回类型的泛型限制问题
type localAuthorRepository struct {
	db *gorm.DB
}

// NewRepository 创建本地作者仓储
func NewRepository(db *gorm.DB) Repository {
	return &localAuthorRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *localAuthorRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *localAuthorRepository) Save(ctx context.Context, author *domain.LocalAuthor) error {
	return r.db.WithContext(ctx).Create(author).Error
}

// Update 更新
func (r *localAuthorRepository) Update(ctx context.Context, author *domain.LocalAuthor) error {
	return r.db.WithContext(ctx).Save(author).Error
}

// GetById 根据ID获取
func (r *localAuthorRepository) GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error) {
	var author domain.LocalAuthor
	err := r.db.WithContext(ctx).First(&author, id).Error
	if err != nil {
		return nil, err
	}
	return &author, nil
}

// List 查询列表
func (r *localAuthorRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalAuthor, error) {
	var authors []*domain.LocalAuthor
	db := r.db.WithContext(ctx).Model(new(domain.LocalAuthor))
	db = applyQueryOption(db, opt)
	err := db.Find(&authors).Error
	if err != nil {
		return nil, err
	}
	return authors, nil
}

// Count 统计数量
func (r *localAuthorRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(domain.LocalAuthor))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *localAuthorRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(domain.LocalAuthor), id).Error
}

// Page 分页查询
func (r *localAuthorRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalAuthor, LocalAuthorQueryDTO], error) {
	page := opt.Page
	pageSize := opt.PageSize

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	// 构建查询选项（设置 Limit 和 Offset）
	queryOpt := opt.QueryOption
	queryOpt.Limit = pageSize
	queryOpt.Offset = offset

	// 查询列表
	list, err := r.List(ctx, &queryOpt)
	if err != nil {
		return nil, err
	}

	// 统计总数（不需要 Limit 和 Offset）
	countOpt := opt.QueryOption
	countOpt.Limit = 0
	countOpt.Offset = 0
	total, err := r.Count(ctx, &countOpt)
	if err != nil {
		return nil, err
	}

	return model.NewPage[domain.LocalAuthor, LocalAuthorQueryDTO](list, total, page, pageSize), nil
}

// ListReWorkAuthor 批量获取作品与作者的关联
func (r *localAuthorRepository) ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*model.RankedLocalAuthor, error) {
	if len(workIds) == 0 {
		return make(map[int64][]*model.RankedLocalAuthor), nil
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
		model.RankedLocalAuthor
	}

	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	// 转换为 map
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

// ListByWorkId 查询作品的本地作者
func (r *localAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error) {
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

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (r *localAuthorRepository) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error) {
	if len(workIds) == 0 {
		return make([]*model.RankedLocalAuthorWithWorkId, 0), nil
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

	var results []*model.RankedLocalAuthorWithWorkId
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListSelectItems 查询选择项列表
func (r *localAuthorRepository) ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*domain.SelectItem, error) {
	var results []*domain.SelectItem

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
		results = append(results, &domain.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return results, nil
}

// QuerySelectItemPage 分页查询选择项
func (r *localAuthorRepository) QuerySelectItemPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SelectItem, LocalAuthorQueryDTO], error) {
	var results []*domain.SelectItem

	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: []clause.Expression{where},
			OrderBy:    []clause.Expression{order},
		},
		Page:     page,
		PageSize: pageSize,
	}
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
		results = append(results, &domain.SelectItem{
			Value: author.ID,
			Label: label,
		})
	}

	return model.NewPage[domain.SelectItem, LocalAuthorQueryDTO](results, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// applyQueryOption 将 QueryOption 应用到 db 实例
func applyQueryOption(db *gorm.DB, opt *database.QueryOption) *gorm.DB {
	// 1. Select（覆盖型）
	if opt.Select != nil {
		db = db.Select(opt.Select)
	}

	// 2. Joins（叠加型）
	for _, join := range opt.Joins {
		db = db.Clauses(join)
	}

	// 3. Conditions（叠加型）
	for _, cond := range opt.Conditions {
		db = db.Where(cond)
	}

	// 4. OrderBy（叠加型）
	if len(opt.OrderBy) > 0 {
		db = db.Order(opt.OrderBy)
	}

	// 5. GroupBy（覆盖型）
	if opt.GroupBy != nil {
		db = db.Clauses(opt.GroupBy)
	}

	// 6. Having（覆盖型）
	if opt.Having != nil {
		db = db.Having(opt.Having)
	}

	// 7. Limit & Offset（覆盖型）
	if opt.Limit > 0 {
		db = db.Limit(opt.Limit)
	}
	if opt.Offset > 0 {
		db = db.Offset(opt.Offset)
	}

	return db
}