package recycleBin

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// Repository 回收站仓储接口
type Repository interface {
	// Save 保存回收站条目
	Save(ctx context.Context, item *domain.RecycleItem) error
	// GetById 根据 ID 获取
	GetById(ctx context.Context, id int64) (*domain.RecycleItem, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.RecycleItem, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Page 分页查询（按删除时间倒序）
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.RecycleItem], error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// ListExpired 查询删除时间早于 expireBefore（毫秒时间戳）的条目，供 TTL 自动清理
	ListExpired(ctx context.Context, expireBefore int64) ([]*domain.RecycleItem, error)
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

// Page 分页查询（固定按删除时间倒序）
// 固定排序列表为回收站的固有展示需求，BaseRepository.Page 的 OrderBy 机制无法直接表达，故自定义
func (r *RecycleItemRepository) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.RecycleItem], error) {
	page := opt.Page
	pageSize := opt.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize

	var items []*domain.RecycleItem
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Order("delete_time DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&items).Error
	if err != nil {
		return nil, err
	}

	var total int64
	err = r.dbFromCtx(ctx).WithContext(ctx).
		Model(new(domain.RecycleItem)).
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	return model.NewPage(items, total, page, pageSize), nil
}

// ListExpired 查询删除时间早于 expireBefore（毫秒时间戳）的条目
func (r *RecycleItemRepository) ListExpired(ctx context.Context, expireBefore int64) ([]*domain.RecycleItem, error) {
	var items []*domain.RecycleItem
	err := r.dbFromCtx(ctx).WithContext(ctx).
		Where("delete_time < ?", expireBefore).
		Find(&items).Error
	return items, err
}
