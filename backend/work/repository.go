package work

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WorkRepository 作品仓储实现
type WorkRepository struct {
	*database.BaseRepository[domain.Work]
}

// NewRepository 创建作品仓储
func NewRepository(db *gorm.DB) *WorkRepository {
	return &WorkRepository{
		BaseRepository: database.NewBaseRepository[domain.Work](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *WorkRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// GetBySiteAndSiteWorkID 根据站点和站点作品ID查询
func (r *WorkRepository) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) (*domain.Work, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.And(
				clause.Eq{Column: "site_id", Value: siteId},
				clause.Eq{Column: "site_work_id", Value: siteWorkId},
			),
		},
	}
	return r.Get(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (r *WorkRepository) ListByIds(ctx context.Context, ids []int64) ([]*domain.Work, error) {
	if len(ids) == 0 {
		return []*domain.Work{}, nil
	}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: toInterfaceSlice(ids)}},
	}
	return r.List(ctx, opt)
}

// UpdateLastViewBatch 批量更新最后查看时间
func (r *WorkRepository) UpdateLastViewBatch(ctx context.Context, ids []int64, lastView int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().
		WithContext(ctx).
		Model(new(domain.Work)).
		Where("id IN ?", ids).
		Update("last_view", lastView).Error
}

// GetDeletedById 按ID查询已软删行（复原链入口校验；Unscoped 逃逸软删过滤）
func (r *WorkRepository) GetDeletedById(ctx context.Context, id int64) (*domain.Work, error) {
	work, err := r.GetByIdUnscoped(ctx, id)
	if err != nil {
		return nil, err
	}
	if work == nil || work.DeletedAt == 0 {
		return nil, nil
	}
	return work, nil
}

// ClearDeletedFlag 清软删标志（复原核心：一行 UPDATE；Unscoped 逃逸 Update 的软删过滤）
func (r *WorkRepository) ClearDeletedFlag(ctx context.Context, id int64) error {
	return r.GORM().
		WithContext(ctx).
		Unscoped().
		Model(new(domain.Work)).
		Where("id = ?", id).
		Update("deleted_at", 0).Error
}

// ListDeletedBefore 查询软删时间早于 expireBefore（毫秒时间戳）的已删行，供 TTL 清理
func (r *WorkRepository) ListDeletedBefore(ctx context.Context, expireBefore int64) ([]*domain.Work, error) {
	return r.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Expr{SQL: "deleted_at > 0 AND deleted_at < ?", Vars: []interface{}{expireBefore}},
		},
		IncludeDeleted: true,
	})
}

// batchSizeOfSiteWorkQuery 批量查重每批的 (site_id, site_work_id) 对数量上限
// 控制单条 SQL 的 OR 子句数与绑定参数数，避免 SQLite 规划器对超大 OR 查询的组合爆炸
const batchSizeOfSiteWorkQuery = 200

// ListBySiteAndSiteWorkIDs 批量根据站点和站点作品ID查询
// siteIds[i] 与 siteWorkIds[i] 一一对应
// 按 batchSizeOfSiteWorkQuery 分批查询并合并结果，避免单条超大 OR 查询导致 SQLite 规划器卡死
func (r *WorkRepository) ListBySiteAndSiteWorkIDs(ctx context.Context, siteIds []int64, siteWorkIds []string) ([]*domain.Work, error) {
	if len(siteIds) == 0 {
		return []*domain.Work{}, nil
	}
	var all []*domain.Work
	for start := 0; start < len(siteIds); start += batchSizeOfSiteWorkQuery {
		end := start + batchSizeOfSiteWorkQuery
		if end > len(siteIds) {
			end = len(siteIds)
		}
		conds := make([]clause.Expression, 0, end-start)
		for i := start; i < end; i++ {
			conds = append(conds, clause.And(
				clause.Eq{Column: "site_id", Value: siteIds[i]},
				clause.Eq{Column: "site_work_id", Value: siteWorkIds[i]},
			))
		}
		batch, err := r.List(ctx, &database.QueryOption{
			Conditions: []clause.Expression{clause.Or(conds...)},
		})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
	}
	return all, nil
}

// toInterfaceSlice converts int64 slice to interface{} slice
func toInterfaceSlice(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for i, id := range ids {
		result[i] = id
	}
	return result
}
