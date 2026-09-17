package reWorkAuthor

import (
	"context"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/constant"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"
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

// DeletePluginSiteByWorkId 删除作品插件来源的 SITE 作者关联（重拉窄域重建用）：
// 用户来源关联（source=MANUAL）与 LOCAL 关联均不在清理域
func (r *ReWorkAuthorRepository) DeletePluginSiteByWorkId(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).WithContext(ctx).
		Where("work_id = ? AND author_type = ? AND source = ?", workId, constant.SITE, constant.PLUGIN).
		Delete(&domain.ReWorkAuthor{}).Error
}

// SaveBatchOnConflict 批量保存，遇任何唯一约束冲突跳过该行（OnConflict DoNothing）。
// LOCAL 关联增量入库用：已存在的 (work_id, local_author_id, role_name) 跳过，不覆盖既有关联。
func (r *ReWorkAuthorRepository) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkAuthor) error {
	if len(rels) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, rel := range rels {
		if rel.GetID() == 0 {
			rel.SetCreateTime(now)
		}
		rel.SetUpdateTime(now)
	}
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rels).Error
}

// UpsertBatch 批量 upsert 关联：按 (work_id, author_id, role_name) 唯一约束冲突时更新用户可编辑字段（sort_order），否则插入。
// authorType 决定冲突列：local→(work_id, local_author_id, role_name)，site→(work_id, site_author_id, role_name)。
// role_name 属冲突键：命中冲突即三方同值（同作品同作者同 role），更新面收窄至 sort_order。
// 同作品同作者不同 role 不构成冲突，落独立关联行（身兼数职）。
// 冲突更新列不含 source：先建行者的来源（插件声明或用户手动）保持不变，插件再声明用户手动挂的
// 关联不翻转来源、不重复建行，仅刷新用户可编辑字段。
func (r *ReWorkAuthorRepository) UpsertBatch(ctx context.Context, rels []*domain.ReWorkAuthor, authorType int) error {
	if len(rels) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, rel := range rels {
		rel.SetUpdateTime(now)
		if rel.GetID() == 0 {
			rel.SetCreateTime(now)
		}
	}
	var conflictCols []clause.Column
	if authorType == constant.LOCAL {
		conflictCols = []clause.Column{{Name: "work_id"}, {Name: "local_author_id"}, {Name: "role_name"}}
	} else {
		conflictCols = []clause.Column{{Name: "work_id"}, {Name: "site_author_id"}, {Name: "role_name"}}
	}
	return r.dbFromCtx(ctx).WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   conflictCols,
			DoUpdates: clause.AssignmentColumns([]string{"sort_order", "update_time"}),
		}).Create(rels).Error
}

// DeleteByWorkAndAuthor 根据作品ID和作者删除关联（该作者的全部 role 关联行；authorType 非法时无操作）
func (r *ReWorkAuthorRepository) DeleteByWorkAndAuthor(ctx context.Context, workId int64, authorType int, authorId int64) error {
	query := r.dbFromCtx(ctx).WithContext(ctx).Where("work_id = ?", workId)

	switch authorType {
	case constant.LOCAL:
		query = query.Where("local_author_id = ?", authorId)
	case constant.SITE:
		query = query.Where("site_author_id = ?", authorId)
	default:
		return nil
	}

	return query.Delete(&domain.ReWorkAuthor{}).Error
}

// DeleteByWorkAuthorAndRole 按作品+作者+role 精确删除关联行（改 role 的旧值行摘除，
// 不波及同作者其他 role 行）；role 值归一化后比对（与关联写入同规则）
func (r *ReWorkAuthorRepository) DeleteByWorkAuthorAndRole(ctx context.Context, workId int64, authorType int, authorId int64, roleName string) error {
	query := r.dbFromCtx(ctx).WithContext(ctx).Where("work_id = ?", workId)

	switch authorType {
	case constant.LOCAL:
		query = query.Where("local_author_id = ?", authorId)
	case constant.SITE:
		query = query.Where("site_author_id = ?", authorId)
	default:
		return nil
	}

	return query.Where("role_name = ?", constant.NormalizeDimensionValue(roleName)).
		Delete(&domain.ReWorkAuthor{}).Error
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
