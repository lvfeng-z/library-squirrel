package duplicate

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// Repository 查重判定共享查询能力（自 import ingestor 迁出）：站点名→本库站点映射与
// 作品批量定位两条查询，由 duplicate.Service 与 import ingestor 共用同一实例。
type Repository interface {
	// ListSitesByNames 按站点名批量查询站点（site_name 唯一索引；软删表无此列，全量口径）
	ListSitesByNames(ctx context.Context, names []string) ([]*entity.Site, error)
	// ListWorksBySiteAndWorkIDs 按站点 + 站点作品 ID 批量查询作品（查重口径=活行，软删行经 GORM scope 自动排除）
	ListWorksBySiteAndWorkIDs(ctx context.Context, siteId int64, siteWorkIds []string) ([]*entity.Work, error)
}

// repository 查重仓储实现
type repository struct {
	db *gorm.DB
}

// NewRepository 创建查重仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知（import 的 find-or-create 在事务内复用同一条查询）
func (r *repository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.db)
}

func (r *repository) ListSitesByNames(ctx context.Context, names []string) ([]*entity.Site, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var rows []*entity.Site
	if err := r.dbFromCtx(ctx).Where("site_name IN ?", names).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListWorksBySiteAndWorkIDs(ctx context.Context, siteId int64, siteWorkIds []string) ([]*entity.Work, error) {
	if len(siteWorkIds) == 0 {
		return nil, nil
	}
	var rows []*entity.Work
	err := r.dbFromCtx(ctx).
		Where("site_id = ? AND site_work_id IN ?", siteId, siteWorkIds).
		Find(&rows).Error
	return rows, err
}
