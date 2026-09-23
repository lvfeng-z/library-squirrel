package stickymemory

import (
	"context"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StickyMemoryRepository 粘性记忆仓储实现
type StickyMemoryRepository struct {
	*database.BaseRepository[entity2.StickyMemory]
}

// NewRepository 创建粘性记忆仓储
func NewRepository(db *gorm.DB) *StickyMemoryRepository {
	return &StickyMemoryRepository{
		BaseRepository: database.NewBaseRepository[entity2.StickyMemory](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *StickyMemoryRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *StickyMemoryRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// UpsertByKey 原子插入或更新（基于 (domain, context_key) 唯一约束）。冲突时仅重写
// value 与 update_time：行 id 与 create_time 保持首记时刻，幂等重记不换行
func (r *StickyMemoryRepository) UpsertByKey(ctx context.Context, domain, contextKey, value string, now int64) error {
	row := entity2.NewStickyMemory()
	row.Domain = domain
	row.ContextKey = contextKey
	row.Value = value
	row.SetCreateTime(now)
	row.SetUpdateTime(now)
	return r.dbFromCtx(ctx).WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "domain"}, {Name: "context_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"value", "update_time",
		}),
	}).Create(row).Error
}

// GetByKey 按 (domain, context_key) 查行（记忆读取点；未命中返回 gorm.ErrRecordNotFound）
func (r *StickyMemoryRepository) GetByKey(ctx context.Context, domain, contextKey string) (*entity2.StickyMemory, error) {
	return r.Get(ctx, &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "domain", Value: domain},
			clause.Eq{Column: "context_key", Value: contextKey},
		},
	})
}

// ListAllOrdered 全量记忆行（domain 升序、同域内 update_time 降序的稳定输出）
func (r *StickyMemoryRepository) ListAllOrdered(ctx context.Context) ([]*entity2.StickyMemory, error) {
	return r.List(ctx, &database.QueryOption{
		OrderBy: []clause.Expression{clause.OrderBy{Columns: []clause.OrderByColumn{
			{Column: clause.Column{Name: "domain"}},
			{Column: clause.Column{Name: "update_time"}, Desc: true},
		}}},
	})
}
