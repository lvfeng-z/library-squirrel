package backup

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/model"

	"gorm.io/gorm"
)

// BackupRepository 备份仓储实现
type BackupRepository struct {
	*database.BaseRepository[model.Backup]
}

// NewRepository 创建备份仓储
func NewRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{
		BaseRepository: database.NewBaseRepository[model.Backup](db),
	}
}

// GetBySourceTypeAndSourceId 根据来源类型和来源ID获取最新备份
func (r *BackupRepository) GetBySourceTypeAndSourceId(ctx context.Context, sourceType int, sourceId int64) (*model.Backup, error) {
	var backup model.Backup
	err := r.GORM().WithContext(ctx).Model(&model.Backup{}).
		Where("source_type = ? AND source_id = ?", sourceType, sourceId).
		Order("id DESC").
		First(&backup).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &backup, nil
}
