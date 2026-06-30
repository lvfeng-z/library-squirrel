package resource

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
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
	// GetEnabledByWorkId 查询作品关联的启用资源
	GetEnabledByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error)
	// ListByWorkIds 批量查询多个作品关联的资源
	ListByWorkIds(ctx context.Context, workIds []int64) ([]*domain.Resource, error)
}

// Service 资源服务
type Service struct {
	repo              Repository
	resourceStoreRepo *ResourceStoreRepository
}

// NewService 创建资源服务
func NewService(repo Repository, resourceStoreRepo *ResourceStoreRepository) *Service {
	return &Service{
		repo:              repo,
		resourceStoreRepo: resourceStoreRepo,
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

// GetEnabledByWorkId 查询作品关联的启用资源
func (s *Service) GetEnabledByWorkId(ctx context.Context, workId int64) ([]*domain.Resource, error) {
	return s.repo.GetEnabledByWorkId(ctx, workId)
}

// DeleteByWorkId 根据作品ID删除所有资源
func (s *Service) DeleteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteByWorkId(ctx, workId)
}

// ListByWorkIds 批量查询多个作品关联的资源，按 work_id 分组
func (s *Service) ListByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*domain.Resource, error) {
	resources, err := s.repo.ListByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]*domain.Resource)
	for _, r := range resources {
		result[r.WorkID] = append(result[r.WorkID], r)
	}
	return result, nil
}

// ListStoresByResourceIds 批量查询多个 Resource 关联的 resource_store 行(按 resourceId 分组,避免 N+1)
func (s *Service) ListStoresByResourceIds(ctx context.Context, resourceIds []int64) (map[int64][]*domain.ResourceStore, error) {
	if len(resourceIds) == 0 {
		return make(map[int64][]*domain.ResourceStore), nil
	}
	stores, err := s.resourceStoreRepo.ListByResourceIds(ctx, resourceIds)
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]*domain.ResourceStore)
	for _, rs := range stores {
		result[rs.ResourceID] = append(result[rs.ResourceID], rs)
	}
	return result, nil
}
