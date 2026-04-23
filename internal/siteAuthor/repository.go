package siteAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SiteAuthorRepository 站点作者仓储实现
type SiteAuthorRepository struct {
	db *gorm.DB
}

// NewRepository 创建站点作者仓储
func NewRepository(db *gorm.DB) *SiteAuthorRepository {
	return &SiteAuthorRepository{
		db: db,
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SiteAuthorRepository) GORM() *gorm.DB {
	return r.db
}

// Save 保存
func (r *SiteAuthorRepository) Save(ctx context.Context, author *entity2.SiteAuthor) error {
	return r.db.WithContext(ctx).Create(author).Error
}

// SaveBatch 批量保存
func (r *SiteAuthorRepository) SaveBatch(ctx context.Context, authors []*entity2.SiteAuthor) error {
	return r.db.WithContext(ctx).Create(authors).Error
}

// Update 更新
func (r *SiteAuthorRepository) Update(ctx context.Context, author *entity2.SiteAuthor) error {
	return r.db.WithContext(ctx).Save(author).Error
}

// GetById 根据ID获取
func (r *SiteAuthorRepository) GetById(ctx context.Context, id int64) (*entity2.SiteAuthor, error) {
	var author entity2.SiteAuthor
	err := r.db.WithContext(ctx).First(&author, id).Error
	if err != nil {
		return nil, err
	}
	return &author, nil
}

// List 查询列表
func (r *SiteAuthorRepository) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.SiteAuthor, error) {
	var authors []*entity2.SiteAuthor
	db := r.db.WithContext(ctx).Model(new(entity2.SiteAuthor))
	db = applyQueryOption(db, opt)
	err := db.Find(&authors).Error
	if err != nil {
		return nil, err
	}
	return authors, nil
}

// Count 统计数量
func (r *SiteAuthorRepository) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	var count int64
	db := r.db.WithContext(ctx).Model(new(entity2.SiteAuthor))
	db = applyQueryOption(db, opt)
	err := db.Count(&count).Error
	return count, err
}

// Delete 删除
func (r *SiteAuthorRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(new(entity2.SiteAuthor), id).Error
}

// Page 分页查询
func (r *SiteAuthorRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.SiteAuthor, SiteAuthorQueryDTO], error) {
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

	return model.NewPage[entity2.SiteAuthor, SiteAuthorQueryDTO](list, total, page, pageSize), nil
}

// ListByWorkId 查询作品的站点作者
func (r *SiteAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	query := `
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.author_rank
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id = ?
	`

	var results []*dto.RankedSiteAuthor
	err := r.GORM().WithContext(ctx).Raw(query, workId).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (r *SiteAuthorRepository) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity2.SiteAuthor, error) {
	if len(siteAuthorIds) == 0 {
		return make([]*entity2.SiteAuthor, 0), nil
	}

	placeholders := make([]string, len(siteAuthorIds))
	args := make([]interface{}, len(siteAuthorIds))
	for i, id := range siteAuthorIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT * FROM site_author WHERE id IN (%s)`, strings.Join(placeholders, ","))

	var results []*entity2.SiteAuthor
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (r *SiteAuthorRepository) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
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
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
		       t1.create_time, t1.update_time, t2.author_rank, t2.work_id
		FROM site_author t1
		INNER JOIN re_work_author t2 ON t1.id = t2.site_author_id
		WHERE t2.work_id IN (%s)
	`, strings.Join(placeholders, ","))

	var results []*dto.RankedSiteAuthorWithWorkId
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// UpdateBindLocalAuthor 绑定本地作者
func (r *SiteAuthorRepository) UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) (int64, error) {
	if len(siteAuthorIds) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(siteAuthorIds))
	args := make([]interface{}, len(siteAuthorIds))
	for i, id := range siteAuthorIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`UPDATE site_author SET local_author_id = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	args = append([]interface{}{localAuthorId}, args...)

	result := r.GORM().WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (r *SiteAuthorRepository) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalAuthorId *bool, localAuthorId *int64) (*model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO], error) {
	var results []*dto.SiteAuthorFullDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&entity2.SiteAuthor{})

	// 根据 boundOnLocalAuthorId 添加 localAuthorId 的过滤条件
	if localAuthorId != nil {
		if boundOnLocalAuthorId != nil && *boundOnLocalAuthorId {
			// 绑定到指定本地作者
			db = db.Where("local_author_id = ?", *localAuthorId)
		} else if boundOnLocalAuthorId != nil && !*boundOnLocalAuthorId {
			// 未绑定到指定本地作者（包括绑定到其他本地作者或从未绑定过本地作者的）
			db = db.Where("(local_author_id != ? OR local_author_id IS NULL)", *localAuthorId)
		}
	}

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if order != nil {
		db = db.Clauses(order)
	}

	// 执行查询
	var siteAuthors []*entity2.SiteAuthor
	if err := db.Find(&siteAuthors).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, author := range siteAuthors {
		resultDTO := dto.NewSiteAuthorFullDTO(author)
		// 查询关联的本地作者
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			var localAuthor entity2.LocalAuthor
			if err := r.GORM().WithContext(ctx).First(&localAuthor, author.LocalAuthorID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			resultDTO.LocalAuthor = dto.NewLocalAuthorDTO(&localAuthor)
		}
		// 查询关联的站点
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			var site entity2.Site
			if err := r.GORM().WithContext(ctx).First(&site, author.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			resultDTO.Site = dto.NewSiteDTO(&site)
		}
		results = append(results, resultDTO)
	}

	return model.NewPage[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO](results, total, page, pageSize), nil
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (r *SiteAuthorRepository) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO], error) {
	var results []*dto.SiteAuthorLocalRelateDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&entity2.SiteAuthor{})

	// 应用查询条件
	if where != nil {
		db = db.Clauses(where)
	}

	// 统计总数
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}

	// 应用分页
	offset := (page - 1) * pageSize
	db = db.Offset(offset).Limit(pageSize)

	// 应用排序
	if order != nil {
		db = db.Clauses(order)
	}

	// 执行查询
	var siteAuthors []*entity2.SiteAuthor
	if err := db.Find(&siteAuthors).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, author := range siteAuthors {
		resultDTO := dto.NewSiteAuthorLocalRelateDTO(author)
		// 查询关联的本地作者
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			var localAuthor entity2.LocalAuthor
			if err := r.GORM().WithContext(ctx).First(&localAuthor, author.LocalAuthorID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			resultDTO.LocalAuthor = dto.NewLocalAuthorDTO(&localAuthor)
		}
		// 查询关联的站点
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			var site entity2.Site
			if err := r.GORM().WithContext(ctx).First(&site, author.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
			resultDTO.Site = dto.NewSiteDTO(&site)
		}
		// 检查是否有同名本地作者
		var count int64
		r.GORM().WithContext(ctx).Model(&entity2.LocalAuthor{}).Where("author_name = ?", author.AuthorName).Count(&count)
		resultDTO.HasSameNameLocalAuthor = count > 0

		results = append(results, resultDTO)
	}

	return model.NewPage[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO](results, total, page, pageSize), nil
}

// GetLocalAuthorByName 根据作者名称查询本地作者
func (r *SiteAuthorRepository) GetLocalAuthorByName(ctx context.Context, authorName string) (*entity2.LocalAuthor, error) {
	var author entity2.LocalAuthor
	err := r.GORM().WithContext(ctx).Where("author_name = ?", authorName).First(&author).Error
	if err != nil {
		return nil, err
	}
	return &author, nil
}

// SaveLocalAuthor 保存本地作者
func (r *SiteAuthorRepository) SaveLocalAuthor(ctx context.Context, author *entity2.LocalAuthor) error {
	return r.GORM().WithContext(ctx).Save(author).Error
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
