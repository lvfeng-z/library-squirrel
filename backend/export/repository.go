package export

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// 后代/祖先递归查询的深度上限（对齐 reWorkSetWorkSet 模块的防御性兜底）。
// 无环 DAG 自然终止；此上限为异常数据（误成环）下的防御性兜底。
const maxTraversalDepth = 100

// Repository 导出数据收集仓储接口（由 service 定义需要的数据库操作方法）。
type Repository interface {
	// ListWorkByIds 批量查询作品（软删行自动排除）
	ListWorkByIds(ctx context.Context, ids []int64) ([]*entity.Work, error)
	// ListResourcesByWorkIds 批量查询作品关联的资源
	ListResourcesByWorkIds(ctx context.Context, workIds []int64) ([]*entity.Resource, error)
	// ListLiveResourceStoresByResourceIds 批量查询 resource_store 关联（仅指向活行 store——
	// 软删行关联保留形态下，导出只取活行，按 STORE_ASSOCIATION_LIVENESS_FILTER）
	ListLiveResourceStoresByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error)
	// ListPersistentStoresByIds 批量查询 persistent_store（软删行自动排除）
	ListPersistentStoresByIds(ctx context.Context, ids []int64) ([]*entity.PersistentStore, error)

	// ListReWorkTagsByWorkIds 批量查询作品-标签关联（完整行，含关联级 namespace）
	ListReWorkTagsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkTag, error)
	// ListLocalTagsByIds 批量查询本地标签
	ListLocalTagsByIds(ctx context.Context, ids []int64) ([]*entity.LocalTag, error)
	// ListSiteTagsByIds 批量查询站点标签
	ListSiteTagsByIds(ctx context.Context, ids []int64) ([]*entity.SiteTag, error)

	// ListReWorkAuthorsByWorkIds 批量查询作品-作者关联
	ListReWorkAuthorsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkAuthor, error)
	// ListLocalAuthorsByIds 批量查询本地作者
	ListLocalAuthorsByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error)
	// ListSiteAuthorsByIds 批量查询站点作者
	ListSiteAuthorsByIds(ctx context.Context, ids []int64) ([]*entity.SiteAuthor, error)

	// ListSitesByIds 批量查询站点
	ListSitesByIds(ctx context.Context, ids []int64) ([]*entity.Site, error)
	// ListWorkSetsByIds 批量查询作品集（软删行自动排除）
	ListWorkSetsByIds(ctx context.Context, ids []int64) ([]*entity.WorkSet, error)
	// ListReWorkWorkSetsByWorkIds 批量查询作品的-作品集关联（完整行，含双轨排序）
	ListReWorkWorkSetsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkWorkSet, error)
	// ListWorkIdsByWorkSetIds 批量查询多个作品集关联的去重作品 ID
	ListWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) ([]int64, error)
	// ListReWorkSetWorkSetsBetween 批量查询作品集间父子关联（父与子均在给定集合内，多父 DAG 边）
	ListReWorkSetWorkSetsBetween(ctx context.Context, workSetIds []int64) ([]*entity.ReWorkSetWorkSet, error)
	// CollectDescendantWorkSetIds 递归查询作品集的所有后代作品集 ID（沿 parent→child 边向下，不含 root 自身；
	// 传递包含用，递归每步 JOIN work_set 过滤已删子集）
	CollectDescendantWorkSetIds(ctx context.Context, rootWorkSetId int64) ([]int64, error)
}

// repository 导出数据收集仓储实现
type repository struct {
	db *gorm.DB
}

// NewRepository 创建导出数据收集仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *repository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.db)
}

func (r *repository) ListWorkByIds(ctx context.Context, ids []int64) ([]*entity.Work, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.Work
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListResourcesByWorkIds(ctx context.Context, workIds []int64) ([]*entity.Resource, error) {
	if len(workIds) == 0 {
		return nil, nil
	}
	var rows []*entity.Resource
	if err := r.dbFromCtx(ctx).Where("work_id IN ?", workIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListLiveResourceStoresByResourceIds(ctx context.Context, resourceIds []int64) ([]*entity.ResourceStore, error) {
	if len(resourceIds) == 0 {
		return nil, nil
	}
	var rows []*entity.ResourceStore
	err := r.dbFromCtx(ctx).
		Where("resource_id IN ?", resourceIds).
		Where("store_id IN (SELECT id FROM persistent_store WHERE deleted_at = 0)").
		Find(&rows).Error
	return rows, err
}

func (r *repository) ListPersistentStoresByIds(ctx context.Context, ids []int64) ([]*entity.PersistentStore, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.PersistentStore
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListReWorkTagsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkTag, error) {
	if len(workIds) == 0 {
		return nil, nil
	}
	var rows []*entity.ReWorkTag
	if err := r.dbFromCtx(ctx).Where("work_id IN ?", workIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListLocalTagsByIds(ctx context.Context, ids []int64) ([]*entity.LocalTag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.LocalTag
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListSiteTagsByIds(ctx context.Context, ids []int64) ([]*entity.SiteTag, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.SiteTag
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListReWorkAuthorsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkAuthor, error) {
	if len(workIds) == 0 {
		return nil, nil
	}
	var rows []*entity.ReWorkAuthor
	if err := r.dbFromCtx(ctx).Where("work_id IN ?", workIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListLocalAuthorsByIds(ctx context.Context, ids []int64) ([]*entity.LocalAuthor, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.LocalAuthor
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListSiteAuthorsByIds(ctx context.Context, ids []int64) ([]*entity.SiteAuthor, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.SiteAuthor
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListSitesByIds(ctx context.Context, ids []int64) ([]*entity.Site, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.Site
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListWorkSetsByIds(ctx context.Context, ids []int64) ([]*entity.WorkSet, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var rows []*entity.WorkSet
	if err := r.dbFromCtx(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListReWorkWorkSetsByWorkIds(ctx context.Context, workIds []int64) ([]*entity.ReWorkWorkSet, error) {
	if len(workIds) == 0 {
		return nil, nil
	}
	var rows []*entity.ReWorkWorkSet
	if err := r.dbFromCtx(ctx).Where("work_id IN ?", workIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) ([]int64, error) {
	if len(workSetIds) == 0 {
		return []int64{}, nil
	}
	var ids []int64
	err := r.dbFromCtx(ctx).
		Model(new(entity.ReWorkWorkSet)).
		Where("work_set_id IN ?", workSetIds).
		Pluck("DISTINCT work_id", &ids).Error
	return ids, err
}

func (r *repository) ListReWorkSetWorkSetsBetween(ctx context.Context, workSetIds []int64) ([]*entity.ReWorkSetWorkSet, error) {
	if len(workSetIds) == 0 {
		return nil, nil
	}
	var rows []*entity.ReWorkSetWorkSet
	err := r.dbFromCtx(ctx).
		Where("parent_work_set_id IN ?", workSetIds).
		Where("child_work_set_id IN ?", workSetIds).
		Find(&rows).Error
	return rows, err
}

// CollectDescendantWorkSetIds 递归查询作品集的所有后代作品集 ID（沿 parent→child 边向下，不含 root 自身）。
// 多父 DAG 用 UNION（非 UNION ALL）去重——菱形依赖下同一后代会经多条路径到达。
func (r *repository) CollectDescendantWorkSetIds(ctx context.Context, rootWorkSetId int64) ([]int64, error) {
	query := `
		WITH RECURSIVE descendants(id, level) AS (
			SELECT rw.child_work_set_id, 1
			FROM re_work_set_work_set rw
			JOIN work_set c ON c.id = rw.child_work_set_id AND c.deleted_at = 0
			WHERE rw.parent_work_set_id = ?
			UNION
			SELECT rw.child_work_set_id, descendants.level + 1
			FROM re_work_set_work_set rw
			JOIN descendants ON rw.parent_work_set_id = descendants.id
			JOIN work_set c2 ON c2.id = rw.child_work_set_id AND c2.deleted_at = 0
			WHERE descendants.level < ?
		)
		SELECT id FROM descendants
	`
	var ids []int64
	err := r.dbFromCtx(ctx).Raw(query, rootWorkSetId, maxTraversalDepth).Scan(&ids).Error
	return ids, err
}
