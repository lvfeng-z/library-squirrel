package resource

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/backend/base/logger"
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

// RecomputeResourceComplete 重算并持久化资源完整度（活行 store 角色计数 → ComputeResourceComplete 三态）。
// 下载完成、合并、回收站复原置换共用，保证各路径判定一致；关联保留形态下软删行关联不计入。
// 读路径不抛错：查询/更新异常降级为保持原值，不阻断调用方主流程
func (s *Service) RecomputeResourceComplete(ctx context.Context, resourceId int64) {
	if resourceId == 0 {
		return
	}
	resource, err := s.repo.GetById(ctx, resourceId)
	if err != nil || resource == nil {
		logger.Log.Warnf("[Resource] 重算完整度失败(查 resource): resourceId=%d err=%v", resourceId, err)
		return
	}
	counts, err := s.resourceStoreRepo.CountAliveTypesByResourceId(ctx, resourceId)
	if err != nil {
		logger.Log.Warnf("[Resource] 重算完整度失败(统计 store 角色): resourceId=%d err=%v", resourceId, err)
		return
	}
	complete, missing, excess := domain.ComputeResourceComplete(resource.ResourceType, counts)
	if complete == 2 {
		logger.Log.Infof("[Resource] 资源结构不完整: resourceId=%d type=%s missing=%v excess=%v", resourceId, resource.ResourceType, missing, excess)
	}
	if resource.ResourceComplete.Valid && resource.ResourceComplete.Int64 == int64(complete) {
		return // 值未变，跳过写库
	}
	resource.ResourceComplete = sql.NullInt64{Int64: int64(complete), Valid: true}
	if err := s.repo.Updates(ctx, resource); err != nil {
		logger.Log.Warnf("[Resource] 更新完整度失败: resourceId=%d err=%v", resourceId, err)
	}
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

// ListStoreTypeSetsByWorkIds 批量查询多个作品的活行 store 角色集合(覆盖确认判定用:任务板块选择与已有行求交)。
// 只计指向活行 store 的关联——软删残留代（merge overwrite 轨道、替换残留）不算「作品拥有该角色」，
// 全量计入会令仅剩死行的角色永远弹覆盖确认
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
	stores, err := s.resourceStoreRepo.ListAliveByResourceIds(ctx, resourceIds)
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
