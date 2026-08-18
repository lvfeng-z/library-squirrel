package recycleBin

import (
	"context"
	"sort"

	"github.com/library-squirrel/backend/base/logger"
	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 排序列常量
const (
	orderFieldDeleteTime     = "delete_time"
	orderFieldWorkCreateTime = "work_create_time"
)

// Repository 回收站仓储接口
type Repository interface {
	// Create 保存回收站条目
	Create(ctx context.Context, item *domain.RecycleItem) error
	// GetById 根据 ID 获取
	GetById(ctx context.Context, id int64) (*domain.RecycleItem, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.RecycleItem, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Page 分页查询：opt 携带 SQL 条件（时间范围/站点）与分页参数，filter/order 见 RecycleSnapshotFilter/RecycleOrder
	Page(ctx context.Context, opt *database.PageOption, filter *RecycleSnapshotFilter, order *RecycleOrder) (*model.Page[domain.RecycleItem], error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// ListExpired 查询删除时间早于 expireBefore（毫秒时间戳）的条目，供 TTL 自动清理
	ListExpired(ctx context.Context, expireBefore int64) ([]*domain.RecycleItem, error)
}

// RecycleSnapshotFilter 作者/标签快照过滤条件
// 对应关联只存在于 snapshot JSON（非数据库列），命中任一作者/标签关联即保留
type RecycleSnapshotFilter struct {
	LocalAuthorID *int64
	LocalTagID    *int64
}

// HasCondition 是否携带快照过滤条件（决定 Page 走 SQL 路径还是快照过滤路径）
func (f *RecycleSnapshotFilter) HasCondition() bool {
	return f != nil && (f.LocalAuthorID != nil || f.LocalTagID != nil)
}

// RecycleOrder 显式排序（Field ∈ delete_time | work_create_time；零值时由 Page 取默认 delete_time DESC）
type RecycleOrder struct {
	Field string
	Desc  bool
}

// RecycleItemRepository 回收站仓储实现
type RecycleItemRepository struct {
	*database.BaseRepository[domain.RecycleItem]
}

// NewRepository 创建回收站仓储
func NewRepository(db *gorm.DB) *RecycleItemRepository {
	return &RecycleItemRepository{
		BaseRepository: database.NewBaseRepository[domain.RecycleItem](db),
	}
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *RecycleItemRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// Page 分页查询
// SQL 路径（filter 无条件）：条件/排序/分页全在 SQL 层；
// 快照路径（filter 携带作者/标签条件）：SQL 层先按 opt 条件取全量，逐条解析快照过滤后内存排序分页
// 固定排序列表为回收站的固有展示需求，BaseRepository.Page 的 OrderBy 机制无法表达动态排序，故自定义
func (r *RecycleItemRepository) Page(ctx context.Context, opt *database.PageOption, filter *RecycleSnapshotFilter, order *RecycleOrder) (*model.Page[domain.RecycleItem], error) {
	page := opt.Page
	pageSize := opt.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if order == nil || order.Field == "" {
		order = &RecycleOrder{Field: orderFieldDeleteTime, Desc: true}
	}

	if !filter.HasCondition() {
		listOpt := opt.QueryOption
		listOpt.Limit = pageSize
		listOpt.Offset = (page - 1) * pageSize
		listOpt.OrderBy = append(listOpt.OrderBy, clause.OrderBy{Columns: []clause.OrderByColumn{{
			Column: clause.Column{Name: order.Field}, Desc: order.Desc,
		}}})
		items, err := r.List(ctx, &listOpt)
		if err != nil {
			return nil, err
		}
		countOpt := opt.QueryOption
		countOpt.Limit = 0
		countOpt.Offset = 0
		total, err := r.Count(ctx, &countOpt)
		if err != nil {
			return nil, err
		}
		return model.NewPage(items, total, page, pageSize), nil
	}

	// 快照路径：全量查询（保留 SQL 条件）→ 快照过滤 → 内存排序分页
	listOpt := opt.QueryOption
	listOpt.Limit = 0
	listOpt.Offset = 0
	items, err := r.List(ctx, &listOpt)
	if err != nil {
		return nil, err
	}
	filtered := make([]*domain.RecycleItem, 0, len(items))
	for _, item := range items {
		snap, err := UnmarshalSnapshot(item.Snapshot)
		if err != nil {
			logger.Log.Warnf("回收站条目 %d 快照解析失败，无法按作者/标签判定归属，快照过滤时剔除: %v", item.GetID(), err)
			continue
		}
		if matchSnapshotFilter(snap, filter) {
			filtered = append(filtered, item)
		}
	}
	sortRecycleItems(filtered, order)
	total := int64(len(filtered))
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(filtered) {
		start = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return model.NewPage(filtered[start:end], total, page, pageSize), nil
}

// matchSnapshotFilter 判断快照是否命中作者/标签过滤条件
func matchSnapshotFilter(snap *WorkRecycleSnapshot, filter *RecycleSnapshotFilter) bool {
	if filter.LocalAuthorID != nil {
		hit := false
		for _, a := range snap.Authors {
			if a.LocalAuthorID.Valid && a.LocalAuthorID.Int64 == *filter.LocalAuthorID {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if filter.LocalTagID != nil {
		hit := false
		for _, t := range snap.Tags {
			if t.LocalTagID.Valid && t.LocalTagID.Int64 == *filter.LocalTagID {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return true
}

// sortRecycleItems 内存排序，NULL 当最小值（与 SQLite ORDER BY 语义一致：asc 在前、desc 在后）
func sortRecycleItems(items []*domain.RecycleItem, order *RecycleOrder) {
	sort.SliceStable(items, func(i, j int) bool {
		vi, vj := items[i].DeleteTime, items[j].DeleteTime
		ni, nj := false, false
		if order.Field == orderFieldWorkCreateTime {
			vi, ni = items[i].WorkCreateTime.Int64, !items[i].WorkCreateTime.Valid
			vj, nj = items[j].WorkCreateTime.Int64, !items[j].WorkCreateTime.Valid
		}
		if ni != nj {
			if order.Desc {
				return nj
			}
			return ni
		}
		if vi == vj {
			return false
		}
		if order.Desc {
			return vi > vj
		}
		return vi < vj
	})
}

// ListExpired 查询删除时间早于 expireBefore（毫秒时间戳）的条目
func (r *RecycleItemRepository) ListExpired(ctx context.Context, expireBefore int64) ([]*domain.RecycleItem, error) {
	var items []*domain.RecycleItem
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where("delete_time < ?", expireBefore).
		Find(&items).Error
	return items, err
}
