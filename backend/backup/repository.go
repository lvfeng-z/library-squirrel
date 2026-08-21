package backup

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
