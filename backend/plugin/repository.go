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
// 卸载链清空 backup_id，已卸载行不再持有备份引用——投影集即当前已安装版本的现役引用
func (r *PluginRepository) ListReferencedBackupIds(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("backup_id IS NOT NULL AND backup_id > 0").
		Distinct().
		Pluck("backup_id", &ids).Error
	return ids, err
}

// MarkUninstalledAndClearBackup 标记已卸载并清空备份引用（单条 UPDATE；map 形态零值安全——
// 结构体 Updates 跳过 NULL 列）。backup_id IS 条件为 null-safe 并发守卫：读取后引用被
// 重装/换版并发改写时 0 行受影响，返回 ErrPluginStateChanged 由调用方引导重试
func (r *PluginRepository) MarkUninstalledAndClearBackup(ctx context.Context, publicId string, expectedBackupId int64) error {
	var expected interface{}
	if expectedBackupId > 0 {
		expected = expectedBackupId
	}
	result := r.GORM().WithContext(ctx).Model(&domain.Plugin{}).
		Where("public_id = ? AND backup_id IS ?", publicId, expected).
		Updates(map[string]interface{}{
			"uninstalled": true,
			"backup_id":   nil,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPluginStateChanged
	}
	return nil
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
