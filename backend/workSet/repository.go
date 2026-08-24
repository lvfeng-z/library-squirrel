package workSet

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkSetRepository 作品集仓储实现
type WorkSetRepository struct {
	*database.BaseRepository[domain.WorkSet]
}

// NewRepository 创建作品集仓储
func NewRepository(db *gorm.DB) *WorkSetRepository {
	return &WorkSetRepository{
		BaseRepository: database.NewBaseRepository[domain.WorkSet](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *WorkSetRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *WorkSetRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListByIds 根据 ID 列表批量查询作品集（复原时引用校验用）
func (r *WorkSetRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.WorkSet, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var items []*domain.WorkSet
	err := r.dbFromCtx(ctx).WithContext(ctx).Where("id IN ?", ids).Find(&items).Error
	return items, err
}

// BatchUpsert 批量插入或更新（基于 site_id + site_work_set_id + deleted_at 三列唯一约束：
// 插入行 deleted_at=0（soft_delete CreateClause 显式写值），与既有活行冲突→更新元数据；
// 已删行（deleted_at=删除时刻≠0）不冲突→同键新建放行、不复活已删行）
func (r *WorkSetRepository) BatchUpsert(ctx context.Context, workSets []*domain.WorkSet) error {
	if len(workSets) == 0 {
		return nil
	}
	now := util.GetCurrentTimestamp()
	for _, ws := range workSets {
		if ws.GetID() == 0 {
			ws.SetCreateTime(now)
		}
		ws.SetUpdateTime(now)
	}
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_work_set_id"}, {Name: "deleted_at"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"site_work_set_name", "site_author_id", "site_work_set_description",
			"site_upload_time", "site_update_time", "nick_name", "last_view", "update_time",
		}),
	}).Create(workSets).Error
}

// ListBySiteAndSiteWorkSetIDs 根据站点ID和站点作品集ID列表批量查询
func (r *WorkSetRepository) ListBySiteAndSiteWorkSetIDs(ctx context.Context, siteId int64, siteWorkSetIds []string) ([]*domain.WorkSet, error) {
	if len(siteWorkSetIds) == 0 {
		return nil, nil
	}
	var result []*domain.WorkSet
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where("site_id = ? AND site_work_set_id IN ?", siteId, siteWorkSetIds).
		Find(&result).Error
	return result, err
}

// Upsert 原子插入或更新（冲突目标与语义同 BatchUpsert：三列唯一约束，活行更新、已删行不复活）
func (r *WorkSetRepository) Upsert(ctx context.Context, ws *domain.WorkSet) error {
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "site_id"}, {Name: "site_work_set_id"}, {Name: "deleted_at"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"site_work_set_name", "site_author_id", "site_work_set_description",
			"site_upload_time", "site_update_time", "nick_name", "last_view", "update_time",
		}),
	}).Create(ws).Error
}

// GetBySiteAndSiteWorkSetID 根据站点和站点作品集ID查询
func (r *WorkSetRepository) GetBySiteAndSiteWorkSetID(ctx context.Context, siteId int64, siteWorkSetId string) (*domain.WorkSet, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_set_id", Value: siteWorkSetId},
			),
		},
	}
	return r.Get(ctx, opt)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称查询
// 注意：需要通过 site 表进行 JOIN 查询
func (r *WorkSetRepository) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) (*domain.WorkSet, error) {
	var result *domain.WorkSet
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Joins("INNER JOIN site ON work_set.site_id = site.id").
		Where("work_set.site_work_set_id = ? AND site.name = ?", siteWorkSetId, siteName).
		First(&result).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetDeletedById 按ID获取已软删作品集（复原链入口校验；nil = 非已删条目）
func (r *WorkSetRepository) GetDeletedById(ctx context.Context, id int64) (*domain.WorkSet, error) {
	ws, err := r.GetByIdUnscoped(ctx, id)
	if err != nil {
		return nil, err
	}
	if ws == nil || ws.DeletedAt == 0 {
		return nil, nil
	}
	return ws, nil
}

// ClearDeletedFlag 清软删标志（复原核心：一行 UPDATE；Unscoped 逃逸 Update 的软删过滤）
func (r *WorkSetRepository) ClearDeletedFlag(ctx context.Context, id int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Unscoped().
		Model(new(domain.WorkSet)).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
}

// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删行，供 TTL 清理
func (r *WorkSetRepository) ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.WorkSet, error) {
	return r.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Expr{SQL: "deleted_at > 0 AND deleted_at < ?", Vars: []interface{}{expireBefore}},
		},
		IncludeDeleted: true,
	})
}

// UpdateCoverWorkId 更新作品集的封面作品引用（集级单值，一条 UPDATE；workId nil=清除封面）
func (r *WorkSetRepository) UpdateCoverWorkId(ctx context.Context, workSetId int64, workId *int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.WorkSet)).
		Where("id = ?", workSetId).
		Update("cover_work_id", workId).Error
}

// ClearCoverReferences 清空指向指定作品的封面引用（作品彻底删除链首步，外键删除防线前置）。
// 原生 UPDATE 覆盖含软删集行——GORM 软删 scope 会排除已删集，而外键不分行态
func (r *WorkSetRepository) ClearCoverReferences(ctx context.Context, workId int64) error {
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Exec("UPDATE work_set SET cover_work_id = NULL WHERE cover_work_id = ?", workId).Error
}

// ListCoverWorkIdsByWorkSetIds 批量查询多个作品集的封面作品ID（work_set.cover_work_id 直读；
// 封面生效性由消费方判活——指向软删/已彻底删除作品时不命中，无兜底转投）
func (r *WorkSetRepository) ListCoverWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) (map[int64]int64, error) {
	if len(workSetIds) == 0 {
		return map[int64]int64{}, nil
	}
	type result struct {
		ID          int64
		CoverWorkID sql.NullInt64
	}
	var results []result
	err := r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(domain.WorkSet)).
		Where("id IN ?", workSetIds).
		Select("id, cover_work_id").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	coverMap := make(map[int64]int64, len(results))
	for _, r := range results {
		if r.CoverWorkID.Valid {
			coverMap[r.ID] = r.CoverWorkID.Int64
		}
	}
	return coverMap, nil
}
