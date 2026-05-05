package plugin

import (
	"context"
	"fmt"

	domain "github.com/library-squirrel/wails/backend/base/model/entity"
	"github.com/library-squirrel/wails/backend/database"
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

// 辅助函数
func buildPublicIdCondition(publicId string) string {
	return fmt.Sprintf("public_id = '%s'", publicId)
}
