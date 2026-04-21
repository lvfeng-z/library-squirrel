package resource

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ResourceRepository 资源仓储实现
type ResourceRepository struct {
	*database.BaseRepository[domain.Resource]
}

// NewRepository 创建资源仓储
func NewRepository(db *gorm.DB) *ResourceRepository {
	return &ResourceRepository{
		BaseRepository: database.NewBaseRepository[domain.Resource](db),
	}
}

// ListByWorkId 查询作品关联的资源
func (r *ResourceRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	where := clause.Eq{Column: "work_id", Value: workId}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return r.BaseRepository.List(ctx, opt)
}

// DeleteByWorkId 根据作品ID删除所有资源
func (r *ResourceRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.Resource)).Error
}
