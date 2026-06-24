package plugin

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
	"gorm.io/gorm"
)

// PluginStorageRepository 插件自存信息仓储实现
type PluginStorageRepository struct {
	*database.BaseRepository[domain.PluginStorage]
}

// NewStorageRepository 创建插件自存信息仓储
func NewStorageRepository(db *gorm.DB) *PluginStorageRepository {
	return &PluginStorageRepository{
		BaseRepository: database.NewBaseRepository[domain.PluginStorage](db),
	}
}

// GetByKey 根据 plugin_id 和 key 获取
func (r *PluginStorageRepository) GetByKey(ctx context.Context, pluginID int64, key string) (*domain.PluginStorage, error) {
	var entity domain.PluginStorage
	err := r.GORM().WithContext(ctx).
		Where("plugin_id = ? AND key = ?", pluginID, key).
		First(&entity).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &entity, nil
}

// ListByPlugin 根据 plugin_id 获取全部自存信息
func (r *PluginStorageRepository) ListByPlugin(ctx context.Context, pluginID int64) ([]*domain.PluginStorage, error) {
	var entities []*domain.PluginStorage
	err := r.GORM().WithContext(ctx).
		Where("plugin_id = ?", pluginID).
		Find(&entities).Error
	if err != nil {
		return nil, err
	}
	return entities, nil
}

// DeleteByKey 根据 plugin_id 和 key 删除
func (r *PluginStorageRepository) DeleteByKey(ctx context.Context, pluginID int64, key string) error {
	return r.GORM().WithContext(ctx).
		Where("plugin_id = ? AND key = ?", pluginID, key).
		Delete(&domain.PluginStorage{}).Error
}
