package resource

import (
	"context"

	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
)

// Repository 资源仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, resource *domain.Resource) error
	// Updates 更新
	Updates(ctx context.Context, resource *domain.Resource) error
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
	return s.repo.Create(ctx, resource)
}

// Update 更新资源
func (s *Service) Updates(ctx context.Context, resource *domain.Resource) error {
	return s.repo.Updates(ctx, resource)
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

// ListStoreTypeSetsByWorkIds 批量查询多个作品的 resource_store 行 store_type 集合(覆盖确认判定用:任务板块选择与已有行求交)
func (s *Service) ListStoreTypeSetsByWorkIds(ctx context.Context, workIds []int64) (map[int64]map[string]struct{}, error) {
	result := make(map[int64]map[string]struct{})
	if len(workIds) == 0 {
		return result, nil
	}
	resources, err := s.repo.ListByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if len(resources) == 0 {
		return result, nil
	}
	resourceIds := make([]int64, 0, len(resources))
	for _, r := range resources {
		resourceIds = append(resourceIds, r.ID)
		result[r.WorkID] = make(map[string]struct{})
	}
	stores, err := s.resourceStoreRepo.ListByResourceIds(ctx, resourceIds)
	if err != nil {
		return nil, err
	}
	storeByResourceId := make(map[int64][]*domain.ResourceStore, len(resources))
	for _, rs := range stores {
		storeByResourceId[rs.ResourceID] = append(storeByResourceId[rs.ResourceID], rs)
	}
	resourceIdToWorkId := make(map[int64]int64, len(resources))
	for _, r := range resources {
		resourceIdToWorkId[r.ID] = r.WorkID
	}
	for resourceId, rsList := range storeByResourceId {
		workId := resourceIdToWorkId[resourceId]
		for _, rs := range rsList {
			result[workId][rs.StoreType] = struct{}{}
		}
	}
	return result, nil
}
