package importer

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// batchCreateChunk 批量插入分块行数。SQLite 单语句绑定变量有上限（mattn 构建为 32766），
// 万级作品导入的单批列数 × 行数会超限，按行分块兜底。
const batchCreateChunk = 500

// Repository 导入仓储接口（本模块定义所需数据操作，直查共享表——对齐 export 模块先例）。
// 查询方法走 dbFromCtx（事务感知）；批量写方法保留实体上已设置的时间戳——
// BaseRepository.CreateBatch 会把时间统一覆写为当前时刻，无法表达「保真源库时间戳」语义，故自定义。
type Repository interface {
	// ===== 查询（find-or-create 匹配 / 查重）=====

	// ListSitesByNames 按站点名批量查询站点（site_name 唯一索引；软删表无此列，全量口径）
	ListSitesByNames(ctx context.Context, names []string) ([]*entity.Site, error)
	// ListLocalTagsByNames 按名称批量查询本地标签
	ListLocalTagsByNames(ctx context.Context, names []string) ([]*entity.LocalTag, error)
	// ListLocalAuthorsByNames 按名称批量查询本地作者
	ListLocalAuthorsByNames(ctx context.Context, names []string) ([]*entity.LocalAuthor, error)
	// ListSiteTagsBySiteAndTagIDs 按站点 + 站点标签 ID 批量查询站点标签（站点侧稳定身份）
	ListSiteTagsBySiteAndTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity.SiteTag, error)
	// ListSiteAuthorsBySiteAndAuthorIDs 按站点 + 站点作者 ID 批量查询站点作者（站点侧稳定身份）
	ListSiteAuthorsBySiteAndAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity.SiteAuthor, error)
	// ListWorksBySiteAndWorkIDs 按站点 + 站点作品 ID 批量查询作品（查重口径=活行，软删行经 GORM scope 自动排除）
	ListWorksBySiteAndWorkIDs(ctx context.Context, siteId int64, siteWorkIds []string) ([]*entity.Work, error)
	// ListWorkSetsBySiteAndSetIDs 按站点 + 站点作品集 ID 批量查询作品集（查重口径=活行）
	ListWorkSetsBySiteAndSetIDs(ctx context.Context, siteId int64, siteWorkSetIds []string) ([]*entity.WorkSet, error)

	// ===== 批量写入（保留实体自带 create_time/update_time；ID 由数据库自增分配并回填）=====

	CreateSites(ctx context.Context, rows []*entity.Site) error
	CreateLocalTags(ctx context.Context, rows []*entity.LocalTag) error
	CreateLocalAuthors(ctx context.Context, rows []*entity.LocalAuthor) error
	CreateSiteTags(ctx context.Context, rows []*entity.SiteTag) error
	CreateSiteAuthors(ctx context.Context, rows []*entity.SiteAuthor) error
	CreateWorks(ctx context.Context, rows []*entity.Work) error
	CreateResources(ctx context.Context, rows []*entity.Resource) error
	CreateResourceStores(ctx context.Context, rows []*entity.ResourceStore) error
	CreateWorkSets(ctx context.Context, rows []*entity.WorkSet) error
	CreateReWorkTags(ctx context.Context, rows []*entity.ReWorkTag) error
	CreateReWorkAuthors(ctx context.Context, rows []*entity.ReWorkAuthor) error
	CreateReWorkWorkSets(ctx context.Context, rows []*entity.ReWorkWorkSet) error
	CreateReWorkSetWorkSets(ctx context.Context, rows []*entity.ReWorkSetWorkSet) error
}

// repository 导入仓储实现
type repository struct {
	db *gorm.DB
}

// NewRepository 创建导入仓储。
func NewRepository(db *gorm.DB) Repository {
	return &repository{db: db}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
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

func (r *repository) ListLocalTagsByNames(ctx context.Context, names []string) ([]*entity.LocalTag, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var rows []*entity.LocalTag
	if err := r.dbFromCtx(ctx).Where("local_tag_name IN ?", names).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListLocalAuthorsByNames(ctx context.Context, names []string) ([]*entity.LocalAuthor, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var rows []*entity.LocalAuthor
	if err := r.dbFromCtx(ctx).Where("author_name IN ?", names).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *repository) ListSiteTagsBySiteAndTagIDs(ctx context.Context, siteId int64, siteTagIds []string) ([]*entity.SiteTag, error) {
	if len(siteTagIds) == 0 {
		return nil, nil
	}
	var rows []*entity.SiteTag
	err := r.dbFromCtx(ctx).
		Where("site_id = ? AND site_tag_id IN ?", siteId, siteTagIds).
		Find(&rows).Error
	return rows, err
}

func (r *repository) ListSiteAuthorsBySiteAndAuthorIDs(ctx context.Context, siteId int64, siteAuthorIds []string) ([]*entity.SiteAuthor, error) {
	if len(siteAuthorIds) == 0 {
		return nil, nil
	}
	var rows []*entity.SiteAuthor
	err := r.dbFromCtx(ctx).
		Where("site_id = ? AND site_author_id IN ?", siteId, siteAuthorIds).
		Find(&rows).Error
	return rows, err
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

func (r *repository) ListWorkSetsBySiteAndSetIDs(ctx context.Context, siteId int64, siteWorkSetIds []string) ([]*entity.WorkSet, error) {
	if len(siteWorkSetIds) == 0 {
		return nil, nil
	}
	var rows []*entity.WorkSet
	err := r.dbFromCtx(ctx).
		Where("site_id = ? AND site_work_set_id IN ?", siteId, siteWorkSetIds).
		Find(&rows).Error
	return rows, err
}

func (r *repository) CreateSites(ctx context.Context, rows []*entity.Site) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateLocalTags(ctx context.Context, rows []*entity.LocalTag) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateLocalAuthors(ctx context.Context, rows []*entity.LocalAuthor) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateSiteTags(ctx context.Context, rows []*entity.SiteTag) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateSiteAuthors(ctx context.Context, rows []*entity.SiteAuthor) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateWorks(ctx context.Context, rows []*entity.Work) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateResources(ctx context.Context, rows []*entity.Resource) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateResourceStores(ctx context.Context, rows []*entity.ResourceStore) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateWorkSets(ctx context.Context, rows []*entity.WorkSet) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateReWorkTags(ctx context.Context, rows []*entity.ReWorkTag) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateReWorkAuthors(ctx context.Context, rows []*entity.ReWorkAuthor) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateReWorkWorkSets(ctx context.Context, rows []*entity.ReWorkWorkSet) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

func (r *repository) CreateReWorkSetWorkSets(ctx context.Context, rows []*entity.ReWorkSetWorkSet) error {
	return createBatchPreservingTimestamps(ctx, r.dbFromCtx(ctx), rows)
}

// createBatchPreservingTimestamps 批量插入并分块（绑定变量上限兜底），
// 实体自带 create_time/update_time 原样落库（导入保真源库时间戳），ID 自增分配并回填实体。
func createBatchPreservingTimestamps[T model.Entity](ctx context.Context, db *gorm.DB, rows []*T) error {
	if len(rows) == 0 {
		return nil
	}
	for start := 0; start < len(rows); start += batchCreateChunk {
		end := start + batchCreateChunk
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[start:end]
		if err := db.WithContext(ctx).Create(&chunk).Error; err != nil {
			return err
		}
	}
	return nil
}
