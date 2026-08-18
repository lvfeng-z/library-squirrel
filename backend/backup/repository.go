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

// ListByWorkId 查询作品的全部备份记录（软删除链写入的归属关联）
func (r *BackupRepository) ListByWorkId(ctx context.Context, workId int64) ([]*entity.Backup, error) {
	var backups []*entity.Backup
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).
		Where("work_id = ?", workId).
		Find(&backups).Error
	if err != nil {
		return nil, err
	}
	return backups, nil
}

// GetLatestByOriginalPath 按原始 store 路径查最近一条备份（无则 nil）
// 供 /store/ 文件服务兜底：作品软删期间文件已移 backup/，原路径 404 时按 original_file_path 反查
func (r *BackupRepository) GetLatestByOriginalPath(ctx context.Context, originalRelPath string) (*entity.Backup, error) {
	var backup entity.Backup
	err := r.GORM().WithContext(ctx).Model(&entity.Backup{}).
		Where("original_file_path = ?", originalRelPath).
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
