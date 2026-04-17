package resource

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 资源仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, resource *domain.Resource) error
	// Update 更新
	Update(ctx context.Context, resource *domain.Resource) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Resource, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.Resource, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkId 根据作品ID删除所有资源
	DeleteByWorkId(ctx context.Context, workId int64) error
	// ListByWorkId 查询作品关联的资源
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error)
}

// resourceRepository 资源仓储实现
type resourceRepository struct {
	*database.BaseRepository[domain.Resource]
}

// NewRepository 创建资源仓储
func NewRepository(db *gorm.DB) Repository {
	return &resourceRepository{
		BaseRepository: database.NewBaseRepository[domain.Resource](db),
	}
}

// ListByWorkId 查询作品关联的资源
func (r *resourceRepository) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	where := clause.Eq{Column: "work_id", Value: workId}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return r.BaseRepository.List(ctx, opt)
}

// DeleteByWorkId 根据作品ID删除所有资源
func (r *resourceRepository) DeleteByWorkId(ctx context.Context, workId int64) error {
	return r.BaseRepository.GORM().
		WithContext(ctx).
		Where("work_id = ?", workId).
		Delete(new(domain.Resource)).Error
}
