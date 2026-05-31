package backup

import (
	"context"

	"github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
)

// BackupRepository 备份仓储实现
type BackupRepository struct {
	*database.BaseRepository[entity.Backup]
}

// NewRepository 创建备份仓储
func NewRepository(db *gorm.DB) *BackupRepository {
	return &BackupRepository{
		BaseRepository: database.NewBaseRepository[entity.Backup](db),
	}
}

// GetBySourceTypeAndSourceId 根据来源类型和来源ID获取最新备份
func (r *BackupRepository) GetBySourceTypeAndSourceId(ctx context.Context, sourceType int, sourceId int64) (*entity.Backup, error) {
	var backup entity.Backup
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).
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

// GetBySourceTypeAndSourceIds 批量获取备份记录（每个 sourceId 取最新一条）
func (r *BackupRepository) GetBySourceTypeAndSourceIds(ctx context.Context, sourceType int, sourceIds []int64) ([]*entity.Backup, error) {
	if len(sourceIds) == 0 {
		return []*entity.Backup{}, nil
	}
	var backups []*entity.Backup
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).
		Where("source_type = ? AND source_id IN ?", sourceType, sourceIds).
		Order("id DESC").
		Find(&backups).Error
	if err != nil {
		return nil, err
	}
	return backups, nil
}
