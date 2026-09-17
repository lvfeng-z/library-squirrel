package tagNamespace

import (
	"context"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"github.com/library-squirrel/backend/util"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TagNamespaceRepository 标签 namespace 维度清单仓储实现
type TagNamespaceRepository struct {
	*database.BaseRepository[entity2.TagNamespace]
}

// NewRepository 创建标签 namespace 维度清单仓储
func NewRepository(db *gorm.DB) *TagNamespaceRepository {
	return &TagNamespaceRepository{
		BaseRepository: database.NewBaseRepository[entity2.TagNamespace](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *TagNamespaceRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// dbFromCtx 获取当前 context 对应的 GORM DB 实例，支持事务感知
func (r *TagNamespaceRepository) dbFromCtx(ctx context.Context) *gorm.DB {
	return database.DBFromContext(ctx, r.BaseRepository.GORM())
}

// ListByValues 按维度值列表批量查询（清单行存在性探测与读取）
func (r *TagNamespaceRepository) ListByValues(ctx context.Context, values []string) ([]*entity2.TagNamespace, error) {
	if len(values) == 0 {
		return nil, nil
	}
	return r.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "value", Values: util.ToAnySlice(values)}},
	})
}

// ListAllOrdered 全量清单行（value 升序稳定输出；分组与排序归前端）
func (r *TagNamespaceRepository) ListAllOrdered(ctx context.Context) ([]*entity2.TagNamespace, error) {
	return r.List(ctx, &database.QueryOption{
		OrderBy: []clause.Expression{clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "value"}}}}},
	})
}

// TouchBatch 刷新既有清单行的使用记录（单条 SQL 批量）：origin 取 MAX(现值, 来源)（数值即优先序，
// 升级不降级——内置行天然不被降级）、last_use 无条件刷新；label 不在更新列（显示名不被使用登记改写）
func (r *TagNamespaceRepository) TouchBatch(ctx context.Context, ids []int64, origin int64, lastUse int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.dbFromCtx(ctx).
		WithContext(ctx).
		Model(new(entity2.TagNamespace)).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"origin":      gorm.Expr("MAX(origin, ?)", origin),
			"last_use":    lastUse,
			"update_time": lastUse,
		}).Error
}
