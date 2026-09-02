package siteAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SiteAuthorRepository 站点作者仓储实现
type SiteAuthorRepository struct {
	*database.BaseRepository[entity.SiteAuthor]
}

// NewRepository 创建站点作者仓储
func NewRepository(db *gorm.DB) *SiteAuthorRepository {
	return &SiteAuthorRepository{
		BaseRepository: database.NewBaseRepository[entity.SiteAuthor](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SiteAuthorRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *SiteAuthorRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// BatchUpsert 批量插入或更新（基于 site_id + site_author_id 唯一约束）
func (r *SiteAuthorRepository) BatchUpsert(ctx context.Context, authors []*entity.SiteAuthor) error {
	if len(authors) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, a := range authors {
		if a.GetID() == 0 {
			a.SetCreateTime(now)
		}
		a.SetUpdateTime(now)
	}
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_author_id"}},
		// upsert 仅更新插件来源字段。local_author_id（用户手动 site→local 桥接）、last_use
		// 不归插件管：它们不在插件 DTO 中，excluded（待插入新行）为零值 NULL，列入 DoUpdates 会用 NULL 覆盖已有值。
		DoUpdates: clause.AssignmentColumns([]string{
			"author_name", "fixed_author_name", "site_author_name_before",
			"introduce", "homepage", "update_time",
		}),
	}).Create(authors).Error
}

// ListBySiteAndSiteAuthorIDs 根据站点ID和站点作者ID列表批量查询
func (r *SiteAuthorRepository) ListBySiteAndSiteAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity.SiteAuthor, error) {
	if len(siteAuthorIds) == 0 {
		return nil, nil
	}
	var result []*entity.SiteAuthor
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where("site_id = ? AND site_author_id IN ?", siteId, siteAuthorIds).
		Find(&result).Error
	return result, err
}

// Upsert 原子插入或更新（基于 site_id + site_author_id 唯一约束）
func (r *SiteAuthorRepository) Upsert(ctx context.Context, author *entity.SiteAuthor) error {
	now := util.GetCurrentTimestamp()
	if author.GetID() == 0 {
		author.SetCreateTime(now)
	}
	author.SetUpdateTime(now)
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_author_id"}},
		// upsert 仅更新插件来源字段。local_author_id（用户手动 site→local 桥接）、last_use
		// 不归插件管：它们不在插件 DTO 中，excluded（待插入新行）为零值 NULL，列入 DoUpdates 会用 NULL 覆盖已有值。
		DoUpdates: clause.AssignmentColumns([]string{
			"author_name", "fixed_author_name", "site_author_name_before",
			"introduce", "homepage", "update_time",
		}),
	}).Create(author).Error
}

// ListByWorkId 查询作品的站点作者
func (r *SiteAuthorRepository) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	query := `
		SELECT t1.id, t1.site_id, t1.site_author_id, t1.author_name, t1.fixed_author_name,
		       t1.site_author_name_before, t1.introduce, t1.local_author_id, t1.last_use,
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

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (r *SiteAuthorRepository) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity.SiteAuthor, error) {
	if len(siteAuthorIds) == 0 {
		return make([]*entity.SiteAuthor, 0), nil
	}
	var results []*entity.SiteAuthor
	err := r.dbFromCtx(ctx).WithContext(ctx).Where("id IN ?", siteAuthorIds).Find(&results).Error
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

// UpdateBindLocalAuthor 绑定本地作者
func (r *SiteAuthorRepository) UpdateBindLocalAuthor(ctx context.Context, localAuthorId *int64, siteAuthorIds []int64) (int64, error) {
	if len(siteAuthorIds) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(siteAuthorIds))
	args := make([]interface{}, len(siteAuthorIds))
	for i, id := range siteAuthorIds {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`UPDATE site_author SET local_author_id = ?, update_time = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	args = append([]interface{}{localAuthorId, util.GetCurrentTimestamp()}, args...)

	result := r.dbFromCtx(ctx).WithContext(ctx).Exec(query, args...)
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// ClearLocalAuthorBinding 清除指向指定本地作者的站点绑定（local_author_id 置 NULL；删除本地作者链调用，
// 由 localAuthor 经窄接口注入）。绑定列无外键防线，残留指向已删作者的值是静默悬空引用，删除链须显式清空
func (r *SiteAuthorRepository) ClearLocalAuthorBinding(ctx context.Context, localAuthorId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Exec("UPDATE site_author SET local_author_id = NULL WHERE local_author_id = ?", localAuthorId).Error
}

// UpdateLastUseByIds 批量更新最后使用时间
func (r *SiteAuthorRepository) UpdateLastUseByIds(ctx context.Context, ids []int64, lastUse int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).WithContext(ctx).Model(new(entity.SiteAuthor)).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{"last_use": lastUse, "update_time": util.GetCurrentTimestamp()}).Error
}

// GetBySiteAndSiteAuthorID 根据站点ID和站点作者ID查询
func (r *SiteAuthorRepository) GetBySiteAndSiteAuthorID(ctx context.Context, siteId int64, siteAuthorId string) (*entity.SiteAuthor, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_author_id", Value: siteAuthorId},
			),
		},
	}
	return r.Get(ctx, opt)
}

// Get 使用 QueryOption 查询单条记录
func (r *SiteAuthorRepository) Get(ctx context.Context, opt *database.QueryOption) (*entity.SiteAuthor, error) {
	var author entity.SiteAuthor
	db := r.dbFromCtx(ctx).WithContext(ctx).Model(new(entity.SiteAuthor))
	db = applyQueryOption(db, opt)
	err := db.First(&author).Error
	if err != nil {
		return nil, err
	}
	return &author, nil
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
