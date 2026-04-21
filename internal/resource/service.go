package resource

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
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

// Service 资源服务
type Service struct {
	repo Repository
}

// NewService 创建资源服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存资源
func (s *Service) Save(ctx context.Context, resource *domain.Resource) error {
	return s.repo.Save(ctx, resource)
}

// Update 更新资源
func (s *Service) Update(ctx context.Context, resource *domain.Resource) error {
	return s.repo.Update(ctx, resource)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.Resource, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Resource, error) {
	return s.repo.List(ctx, opt)
}

// Delete 删除资源
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// ListByWorkId 查询作品关联的资源
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// DeleteByWorkId 根据作品ID删除所有资源
func (s *Service) DeleteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteByWorkId(ctx, workId)
}
