package backup

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// idChunkSize ID 集分块的块大小：SQLite 绑定参数上限（旧默认 999）内的保守取值，
// 超 500 个 ID 的 IN/NOT IN 拆多块拼装
const idChunkSize = 500

// BackupRepository 备份仓储实现（纯保管清单 CRUD，按主键为唯一查询面）
type BackupRepository struct {
	*database.BaseRepository[entity.Backup]
}

// NewRepository 创建备份仓储
func NewRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{
		BaseRepository: database.NewBaseRepository[entity.Backup](db),
	}
}

// ListCreatedBefore 查询创建时间早于 beforeMs（毫秒）的清单行（backup 表无软删，普通查询；
// 供备份治理正向无主候选扫描）
func (r *BackupRepository) ListCreatedBefore(ctx context.Context, beforeMs int64) ([]*entity.Backup, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Lt{Column: "create_time", Value: beforeMs},
		},
	}
	return r.List(ctx, opt)
}

// ListAllIDs 全量投影清单行 ID（BaseRepository 无投影面故链式表达；供备份治理反向悬空判定的现存集）
func (r *BackupRepository) ListAllIDs(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).Pluck("id", &ids).Error
	return ids, err
}

// PageBackups 分页查询保管清单（create_time 倒序固定序）。
// includeIDs/excludeIDs 为 ID 集过滤（引用态过滤由调用方把引用集折算成 ID 集传入，仓储不感知引用语义）：
// nil=无该向过滤；非 nil 空集 include=恒空结果（引用态=有主但当前零引用）、空集 exclude 无作用。
// 大 ID 集分块拼 IN/NOT IN，避免触碰 SQLite 绑定参数上限
func (r *BackupRepository) PageBackups(ctx context.Context, pageNumber, pageSize int, includeIDs []int64, excludeIDs []int64) (*model.Page[entity.Backup], error) {
	if includeIDs != nil && len(includeIDs) == 0 {
		return model.NewPage[entity.Backup]([]*entity.Backup{}, 0, pageNumber, pageSize), nil
	}
	conds := make([]clause.Expression, 0, len(includeIDs)/idChunkSize+len(excludeIDs)/idChunkSize+2)
	if len(includeIDs) > 0 {
		chunks := make([]clause.Expression, 0, (len(includeIDs)+idChunkSize-1)/idChunkSize)
		for start := 0; start < len(includeIDs); start += idChunkSize {
			end := min(start+idChunkSize, len(includeIDs))
			chunks = append(chunks, clause.IN{Column: clause.PrimaryColumn, Values: toAnySlice(includeIDs[start:end])})
		}
		conds = append(conds, clause.Or(chunks...))
	}
	for start := 0; start < len(excludeIDs); start += idChunkSize {
		end := min(start+idChunkSize, len(excludeIDs))
		conds = append(conds, clause.Not(clause.IN{Column: clause.PrimaryColumn, Values: toAnySlice(excludeIDs[start:end])}))
	}
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conds,
			OrderBy: []clause.Expression{
				clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "create_time"}, Desc: true}}},
			},
		},
		Page:     pageNumber,
		PageSize: pageSize,
	}
	return r.Page(ctx, opt)
}

// toAnySlice int64 切片转 any 切片（GORM clause.Values 形参）
func toAnySlice(ids []int64) []any {
	values := make([]any, len(ids))
	for i, id := range ids {
		values[i] = id
	}
	return values
}
