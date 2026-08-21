package plugin

import (
	"context"
	"fmt"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
)

// PluginRepository 插件仓储实现
type PluginRepository struct {
	*database.BaseRepository[domain.Plugin]
}

// NewRepository 创建插件仓储
func NewRepository(db *gorm.DB) *PluginRepository {
	return &PluginRepository{
		BaseRepository: database.NewBaseRepository[domain.Plugin](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *PluginRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// CheckInstalled 检查插件是否已安装
func (r *PluginRepository) CheckInstalled(ctx context.Context, publicId string) (bool, error) {
	var count int64
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("public_id = ? AND uninstalled = 0", publicId).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByPublicId 根据公开ID获取
func (r *PluginRepository) GetByPublicId(ctx context.Context, publicId string) (*domain.Plugin, error) {
	var plugin domain.Plugin
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("public_id = ?", publicId).
		First(&plugin).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &plugin, nil
}

// ListReferencedBackupIds 全量投影行内 BackupID（DISTINCT，非 NULL 且 >0；供备份治理引用集对账）。
// 表无软删，全量即含已卸载行——卸载行持有重装能力引用，合法有主
func (r *PluginRepository) ListReferencedBackupIds(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("backup_id IS NOT NULL AND backup_id > 0").
		Distinct().
		Pluck("backup_id", &ids).Error
	return ids, err
}

// ClearBackupRefsByBackupIds 按引用目标清列（悬空引用清列，BackupID 置 NULL——NullInt64 语义）
func (r *PluginRepository) ClearBackupRefsByBackupIds(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("backup_id IN ?", ids).
		Update("backup_id", nil).Error
}

// 辅助函数
func buildPublicIdCondition(publicId string) string {
	return fmt.Sprintf("public_id = '%s'", publicId)
}
