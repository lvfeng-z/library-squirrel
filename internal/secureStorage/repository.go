package secureStorage

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"

	"gorm.io/gorm"
)

// Repository 安全存储仓储接口
type Repository interface {
	// Save 保存
	Save(ctx context.Context, entity *domain.SecureStorage) error
	// Update 更新
	Update(ctx context.Context, entity *domain.SecureStorage) error
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// DeleteByKey 根据存储键删除
	DeleteByKey(ctx context.Context, storageKey string) (int64, error)
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.SecureStorage, error)
	// GetByKey 根据存储键获取
	GetByKey(ctx context.Context, storageKey string) (*domain.SecureStorage, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.SecureStorage, error)
}

// secureStorageRepository 安全存储仓储实现
type secureStorageRepository struct {
	*database.BaseRepository[domain.SecureStorage]
}

// NewRepository 创建安全存储仓储
func NewRepository(db *gorm.DB) Repository {
	return &secureStorageRepository{
		BaseRepository: database.NewBaseRepository[domain.SecureStorage](db),
	}
}

// GORM 返回底层 GORM DB 实例
func (r *secureStorageRepository) GORM() *gorm.DB {
	return r.BaseRepository.GORM()
}

// DeleteByKey 根据存储键删除
func (r *secureStorageRepository) DeleteByKey(ctx context.Context, storageKey string) (int64, error) {
	result := r.GORM().WithContext(ctx).Where("storage_key = ?", storageKey).Delete(&domain.SecureStorage{})
	return result.RowsAffected, result.Error
}

// GetByKey 根据存储键获取
func (r *secureStorageRepository) GetByKey(ctx context.Context, storageKey string) (*domain.SecureStorage, error) {
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
func (r *secureStorageRepository) List(ctx context.Context, opt *database.QueryOption) ([]*domain.SecureStorage, error) {
	return r.BaseRepository.List(ctx, opt)
}
