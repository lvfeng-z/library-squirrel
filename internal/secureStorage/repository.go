package secureStorage

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/pkg/model/entity"

	"gorm.io/gorm"
)

// SecureStorageRepository 安全存储仓储实现
type SecureStorageRepository struct {
	*database.BaseRepository[domain.SecureStorage]
}

// NewRepository 创建安全存储仓储
func NewRepository(db *gorm.DB) *SecureStorageRepository {
	return &SecureStorageRepository{
		BaseRepository: database.NewBaseRepository[domain.SecureStorage](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *SecureStorageRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// DeleteByKey 根据存储键删除
func (r *SecureStorageRepository) DeleteByKey(ctx context.Context, storageKey string) (int64, error) {
	result := r.GORM().WithContext(ctx).Where("storage_key = ?", storageKey).Delete(&domain.SecureStorage{})
	return result.RowsAffected, result.Error
}

// GetByKey 根据存储键获取
func (r *SecureStorageRepository) GetByKey(ctx context.Context, storageKey string) (*domain.SecureStorage, error) {
	var entity domain.SecureStorage
	err := r.GORM().WithContext(ctx).Where("storage_key = ?", storageKey).First(&entity).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &entity, nil
}

// List 查询列表
func (r *SecureStorageRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.SecureStorage, error) {
	return r.BaseRepository.List(ctx, opt)
}
