package siteAuthor

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

// siteAuthorRepository 站点作者仓储实现
type siteAuthorRepository struct {
	*database.BaseRepository[domain.SiteAuthor]
}

// NewRepository 创建站点作者仓储
func NewRepository(db *gorm.DB) Repository {
	return &siteAuthorRepository{
		BaseRepository: database.NewBaseRepository[domain.SiteAuthor](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *siteAuthorRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// ListByWorkId 查询作品的站点作者
func (r *siteAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error) {
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

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (r *siteAuthorRepository) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*domain.SiteAuthor, error) {
	if len(siteAuthorIds) == 0 {
		return make([]*domain.SiteAuthor, 0), nil
	}

	placeholders := make([]string, len(siteAuthorIds))
	args := make([]interface{}, len(siteAuthorIds))
	for i, id := range siteAuthorIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`SELECT * FROM site_author WHERE id IN (%s)`, strings.Join(placeholders, ","))

	var results []*domain.SiteAuthor
	err := r.GORM().WithContext(ctx).Raw(query, args...).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (r *siteAuthorRepository) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error) {
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

// UpdateBindLocalAuthor 绑定本地作者
func (r *siteAuthorRepository) UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) (int64, error) {
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
func (r *siteAuthorRepository) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalAuthorId *bool, localAuthorId *int64) (*model.Page[domain.SiteAuthorFullDTO], error) {
	var results []*domain.SiteAuthorFullDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteAuthor{})

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
	var siteAuthors []*domain.SiteAuthor
	if err := db.Find(&siteAuthors).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, author := range siteAuthors {
		dto := domain.NewSiteAuthorFullDTO(author)
		// 查询关联的本地作者
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			dto.LocalAuthor = &domain.LocalAuthor{}
			if err := r.GORM().WithContext(ctx).First(dto.LocalAuthor, author.LocalAuthorID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 查询关联的站点
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			dto.Site = &domain.Site{}
			if err := r.GORM().WithContext(ctx).First(dto.Site, author.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		results = append(results, dto)
	}

	return model.NewPage(results, total, page, pageSize), nil
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (r *siteAuthorRepository) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SiteAuthorLocalRelateDTO], error) {
	var results []*domain.SiteAuthorLocalRelateDTO
	var total int64

	db := r.GORM().WithContext(ctx).Model(&domain.SiteAuthor{})

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
	var siteAuthors []*domain.SiteAuthor
	if err := db.Find(&siteAuthors).Error; err != nil {
		return nil, err
	}

	// 转换为 DTO
	for _, author := range siteAuthors {
		dto := domain.NewSiteAuthorLocalRelateDTO(author)
		// 查询关联的本地作者
		if author.LocalAuthorID.Valid && author.LocalAuthorID.Int64 > 0 {
			dto.LocalAuthor = &domain.LocalAuthor{}
			if err := r.GORM().WithContext(ctx).First(dto.LocalAuthor, author.LocalAuthorID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 查询关联的站点
		if author.SiteID.Valid && author.SiteID.Int64 > 0 {
			dto.Site = &domain.Site{}
			if err := r.GORM().WithContext(ctx).First(dto.Site, author.SiteID.Int64).Error; err != nil && err != gorm.ErrRecordNotFound {
				return nil, err
			}
		}
		// 检查是否有同名本地作者
		var count int64
		r.GORM().WithContext(ctx).Model(&domain.LocalAuthor{}).Where("author_name = ?", author.AuthorName).Count(&count)
		dto.HasSameNameLocalAuthor = count > 0

		results = append(results, dto)
	}

	return model.NewPage(results, total, page, pageSize), nil
}

// GetLocalAuthorByName 根据作者名称查询本地作者
func (r *siteAuthorRepository) GetLocalAuthorByName(ctx context.Context, authorName string) (*domain.LocalAuthor, error) {
	var author domain.LocalAuthor
	err := r.GORM().WithContext(ctx).Where("author_name = ?", authorName).First(&author).Error
	if err != nil {
		return nil, err
	}
	return &author, nil
}

// SaveLocalAuthor 保存本地作者
func (r *siteAuthorRepository) SaveLocalAuthor(ctx context.Context, author *domain.LocalAuthor) error {
	return r.GORM().WithContext(ctx).Save(author).Error
}
