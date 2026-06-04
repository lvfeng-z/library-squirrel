package persistentStore

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PersistentStoreRepository 文件持久存储仓储实现
type PersistentStoreRepository struct {
	*database.BaseRepository[domain.PersistentStore]
}

// NewRepository 创建文件持久存储仓储
func NewRepository(db *gorm.DB) *PersistentStoreRepository {
	return &PersistentStoreRepository{
		BaseRepository: database.NewBaseRepository[domain.PersistentStore](db),
	}
}

// GetByFilePath 根据文件路径获取记录
func (r *PersistentStoreRepository) GetByFilePath(ctx context.Context, filePath string) (*domain.PersistentStore, error) {
	opt := &database.QueryOption{
		Conditions: []clause.Expression{
			clause.Eq{Column: "file_path", Value: filePath},
		},
		Limit: 1,
	}
	list, err := r.BaseRepository.List(ctx, opt)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}

// ExistsByFilePath 检查文件路径是否已存在记录
func (r *PersistentStoreRepository) ExistsByFilePath(ctx context.Context, filePath string) bool {
	record, err := r.GetByFilePath(ctx, filePath)
	if err != nil {
		return false
	}
	return record != nil
}
