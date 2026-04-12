package plugin

import (
	"context"
	"fmt"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// pluginRepository 插件仓储实现
// 嵌入 database.BaseRepository[domain.Plugin] 获得基础 CRUD 实现
type pluginRepository struct {
	*database.BaseRepository[domain.Plugin]
}

// NewRepository 创建插件仓储
func NewRepository(db *gorm.DB) Repository {
	return &pluginRepository{
		BaseRepository: database.NewBaseRepository[domain.Plugin](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *pluginRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// Page 分页查询
func (r *pluginRepository) Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, order clause.Expression) (*model.Page[domain.Plugin], error) {
	data, total, err := r.BaseRepository.Page(ctx, page, pageSize, conditions, order)
	if err != nil {
		return nil, err
	}
	return model.NewPage(data, total, page, pageSize), nil
}

// CheckInstalled 检查插件是否已安装
func (r *pluginRepository) CheckInstalled(ctx context.Context, publicId string) (bool, error) {
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
func (r *pluginRepository) GetByPublicId(ctx context.Context, publicId string) (*domain.Plugin, error) {
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
