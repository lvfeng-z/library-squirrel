package search

import (
	"context"
	"fmt"
	"sync"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
)

// ========== 外部模块接口定义（由 search 模块定义自己需要的接口）==========

// Repository 搜索仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
	QuerySearchConditionPage(ctx context.Context, page, pageSize int, keyword string, types []dto2.SearchType) ([]*dto2.SelectItem, int64, error)
	// QueryWorkPage 查询作品分页
	QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) ([]*dto2.WorkFullDTO, int64, error)
	// QueryRecycleWorkPage 查询回收站作品分页（work 已删行）
	QueryRecycleWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition, sortField string, sortDesc bool) ([]*dto2.RecycleWorkDTO, int64, error)
	// QueryRecycleStorePage 查询回收站文件条目分页（persistent_store 已删行，非「作品已删」聚合形态）
	QueryRecycleStorePage(ctx context.Context, page, pageSize int, query *dto2.RecycleStorePageQuery) ([]*dto2.RecycleStoreDTO, int64, error)
	// ListRecycleStoreIdsDeletedBefore 圈定删除时间早于 expireBefore 的文件条目 ID（回收站 TTL 清理）
	ListRecycleStoreIdsDeletedBefore(ctx context.Context, expireBefore int64) ([]int64, error)
	// GetRecycleStoreMount 查询单个 store 行的挂载身份与作品活性（回收站复原置换链）
	GetRecycleStoreMount(ctx context.Context, storeId int64) (*dto2.StoreMountDTO, error)
	// GetAliveStoreIdByKey 查挂载键 (resource_id, store_type, store_seq) 下的活行 store ID（无则 0）
	GetAliveStoreIdByKey(ctx context.Context, resourceId int64, storeType string, storeSeq int) (int64, error)
	// QueryWorkSetPageByConditions 根据搜索条件查询作品集分页（EXISTS 子查询）
	QueryWorkSetPageByConditions(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) ([]*entity2.WorkSet, int64, error)
	// QueryRecycleWorkSetPage 查询回收站作品集条目分页（work_set 已删行；作品集域平铺条件体系）
	QueryRecycleWorkSetPage(ctx context.Context, page, pageSize int, query *dto2.RecycleWorkSetPageQuery) ([]*dto2.RecycleWorkSetDTO, int64, error)
}

// CoverResolver 批量封面解析接口
type CoverResolver interface {
	ListCoverWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) (map[int64]int64, error)
	ListMinSortOrderWorkIdsByWorkSetIds(ctx context.Context, workSetIds []int64) (map[int64]int64, error)
}

// WorkSetPageWorkReader 作品读取接口
type WorkSetPageWorkReader interface {
	ListByIds(ctx context.Context, ids []int64) ([]*entity2.Work, error)
}

// WorkSetPageResourceReader 资源读取接口
type WorkSetPageResourceReader interface {
	ListByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*entity2.Resource, error)
}

// StoreBatchReader PersistentStore 批量读取接口
type StoreBatchReader interface {
	GetByIds(ctx context.Context, ids []int64) ([]*entity2.PersistentStore, error)
}

// LocalTagUpdater 本地标签更新接口
type LocalTagUpdater interface {
	UpdateLastUse(ctx context.Context, ids []int64) error
}

// SiteTagUpdater 站点标签更新接口
type SiteTagUpdater interface {
	UpdateLastUse(ctx context.Context, ids []int64) error
}

// LocalAuthorUpdater 本地作者更新接口
type LocalAuthorUpdater interface {
	UpdateLastUse(ctx context.Context, ids []int64) error
}

// SiteAuthorUpdater 站点作者更新接口
type SiteAuthorUpdater interface {
	UpdateLastUse(ctx context.Context, ids []int64) error
}

// ========== Service ==========

// ResourceStoreBatchReader resource_store 批量读取接口(按 resourceId 分组)
type ResourceStoreBatchReader interface {
	ListStoresByResourceIds(ctx context.Context, resourceIds []int64) (map[int64][]*entity2.ResourceStore, error)
}

// Service 搜索服务
type Service struct {
	repo Repository

	// 作品集查询依赖
	coverResolver            CoverResolver
	workReader               WorkSetPageWorkReader
	resourceReader           WorkSetPageResourceReader
	storeBatchReader         StoreBatchReader
	resourceStoreBatchReader ResourceStoreBatchReader

	// 外部模块依赖（通过构造函数注入）
	localTagUpdater    LocalTagUpdater
	siteTagUpdater     SiteTagUpdater
	localAuthorUpdater LocalAuthorUpdater
	siteAuthorUpdater  SiteAuthorUpdater
}

// NewService 创建搜索服务
func NewService(
	repo Repository,
	coverResolver CoverResolver,
	workReader WorkSetPageWorkReader,
	resourceReader WorkSetPageResourceReader,
	storeBatchReader StoreBatchReader,
	resourceStoreBatchReader ResourceStoreBatchReader,
	localTagUpdater LocalTagUpdater,
	siteTagUpdater SiteTagUpdater,
	localAuthorUpdater LocalTagUpdater,
	siteAuthorUpdater SiteAuthorUpdater,
) *Service {
	return &Service{
		repo:                     repo,
		coverResolver:            coverResolver,
		workReader:               workReader,
		resourceReader:           resourceReader,
		storeBatchReader:         storeBatchReader,
		resourceStoreBatchReader: resourceStoreBatchReader,
		localTagUpdater:          localTagUpdater,
		siteTagUpdater:           siteTagUpdater,
		localAuthorUpdater:       localAuthorUpdater,
		siteAuthorUpdater:        siteAuthorUpdater,
	}
}

// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
func (s *Service) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *dto2.SearchConditionQuery) (*model.Page[dto2.SelectItem], error) {
	if query == nil {
		query = &dto2.SearchConditionQuery{}
	}
	items, total, err := s.repo.QuerySearchConditionPage(ctx, page, pageSize, query.Keyword, query.Types)
	if err != nil {
		return nil, fmt.Errorf("query search condition page error: %w", err)
	}
	return model.NewPage[dto2.SelectItem](items, total, page, pageSize), nil
}

// QueryWorkPage 查询作品分页
func (s *Service) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) (*model.Page[dto2.WorkFullDTO], error) {
	items, total, err := s.repo.QueryWorkPage(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, err
	}

	// 更新搜索条件的最后使用时间
	used := extractUsedConditions(conditions)
	if len(used) > 0 {
		go func() {
			_ = s.UpdateLastUsed(context.Background(), used)
		}()
	}

	return model.NewPage[dto2.WorkFullDTO](items, total, page, pageSize), nil
}

// QueryRecycleWorkPage 查询回收站作品分页（work 已删行；条件体系与正常作品搜索一致）
// sortField: "deleted_at"（默认）| "create_time"；sortDesc: 降序
func (s *Service) QueryRecycleWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition, sortField string, sortDesc bool) (*model.Page[dto2.RecycleWorkDTO], error) {
	items, total, err := s.repo.QueryRecycleWorkPage(ctx, page, pageSize, conditions, sortField, sortDesc)
	if err != nil {
		return nil, err
	}
	return model.NewPage[dto2.RecycleWorkDTO](items, total, page, pageSize), nil
}

// QueryRecycleStorePage 查询回收站文件条目分页（persistent_store 已删行，非「作品已删」聚合形态；
// 文件域条件体系见 RecycleStorePageQuery，与作品条目的 SearchCondition 体系分轨）
func (s *Service) QueryRecycleStorePage(ctx context.Context, page, pageSize int, query *dto2.RecycleStorePageQuery) (*model.Page[dto2.RecycleStoreDTO], error) {
	items, total, err := s.repo.QueryRecycleStorePage(ctx, page, pageSize, query)
	if err != nil {
		return nil, err
	}
	return model.NewPage[dto2.RecycleStoreDTO](items, total, page, pageSize), nil
}

// ListRecycleStoreIdsDeletedBefore 圈定删除时间早于 expireBefore 的文件条目 ID（回收站 TTL 清理；
// 与列表查询同谓词——「作品已删」聚合行不被圈定）
func (s *Service) ListRecycleStoreIdsDeletedBefore(ctx context.Context, expireBefore int64) ([]int64, error) {
	return s.repo.ListRecycleStoreIdsDeletedBefore(ctx, expireBefore)
}

// GetRecycleStoreMount 查询单个 store 行的挂载身份与作品活性（回收站复原置换链）
func (s *Service) GetRecycleStoreMount(ctx context.Context, storeId int64) (*dto2.StoreMountDTO, error) {
	return s.repo.GetRecycleStoreMount(ctx, storeId)
}

// GetAliveStoreIdByKey 查挂载键 (resource_id, store_type, store_seq) 下的活行 store ID（无则 0）
func (s *Service) GetAliveStoreIdByKey(ctx context.Context, resourceId int64, storeType string, storeSeq int) (int64, error) {
	return s.repo.GetAliveStoreIdByKey(ctx, resourceId, storeType, storeSeq)
}

// QueryRecycleWorkSetPage 查询回收站作品集条目分页（work_set 已删行；作品集域平铺条件体系）
func (s *Service) QueryRecycleWorkSetPage(ctx context.Context, page, pageSize int, query *dto2.RecycleWorkSetPageQuery) (*model.Page[dto2.RecycleWorkSetDTO], error) {
	items, total, err := s.repo.QueryRecycleWorkSetPage(ctx, page, pageSize, query)
	if err != nil {
		return nil, err
	}
	return model.NewPage[dto2.RecycleWorkSetDTO](items, total, page, pageSize), nil
}

// extractUsedConditions 从搜索条件中提取需要更新lastUse的ID
func extractUsedConditions(conditions []*dto2.SearchCondition) map[dto2.SearchType][]int64 {
	used := make(map[dto2.SearchType][]int64)
	for _, cond := range conditions {
		if cond == nil {
			continue
		}
		switch cond.Type {
		case dto2.LocalTag, dto2.SiteTag, dto2.LocalAuthor, dto2.SiteAuthor:
			if id, ok := cond.Value.(float64); ok {
				used[cond.Type] = append(used[cond.Type], int64(id))
			}
		}
	}
	return used
}

// QueryWorkSetPage 查询作品集分页（通过搜索条件筛选关联的作品，返回带封面的作品集）
func (s *Service) QueryWorkSetPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) (*model.Page[dto2.WorkSetWithCoverDTO], error) {
	if conditions == nil {
		conditions = []*dto2.SearchCondition{}
	}

	// Phase 1: 查询作品集分页（EXISTS 子查询）
	workSets, total, err := s.repo.QueryWorkSetPageByConditions(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, fmt.Errorf("query work set page error: %w", err)
	}

	if len(workSets) == 0 {
		return model.NewPage[dto2.WorkSetWithCoverDTO]([]*dto2.WorkSetWithCoverDTO{}, 0, page, pageSize), nil
	}

	// 收集作品集 ID
	workSetIds := make([]int64, 0, len(workSets))
	for _, ws := range workSets {
		workSetIds = append(workSetIds, ws.GetID())
	}

	// Phase 2a: 批量查 is_cover=1 的封面
	coverMap, err := s.coverResolver.ListCoverWorkIdsByWorkSetIds(ctx, workSetIds)
	if err != nil {
		return nil, fmt.Errorf("resolve covers pass 1 error: %w", err)
	}

	// Phase 2b: 对未找到封面的作品集，用 MIN(sort_order) 兜底
	var uncoveredWorkSetIds []int64
	for _, id := range workSetIds {
		if _, ok := coverMap[id]; !ok {
			uncoveredWorkSetIds = append(uncoveredWorkSetIds, id)
		}
	}
	if len(uncoveredWorkSetIds) > 0 {
		fallbackMap, err := s.coverResolver.ListMinSortOrderWorkIdsByWorkSetIds(ctx, uncoveredWorkSetIds)
		if err != nil {
			return nil, fmt.Errorf("resolve covers pass 2 error: %w", err)
		}
		for k, v := range fallbackMap {
			coverMap[k] = v
		}
	}

	// Phase 3: 批量查封面作品
	var coverWorkIds []int64
	for _, workId := range coverMap {
		coverWorkIds = append(coverWorkIds, workId)
	}
	worksMap := make(map[int64]*entity2.Work)
	if len(coverWorkIds) > 0 {
		works, err := s.workReader.ListByIds(ctx, coverWorkIds)
		if err != nil {
			return nil, fmt.Errorf("batch fetch cover works error: %w", err)
		}
		for _, w := range works {
			worksMap[w.GetID()] = w
		}
	}

	// Phase 4: 批量查封面资源
	resourcesMap := make(map[int64][]*entity2.Resource)
	if len(coverWorkIds) > 0 {
		resourcesMap, err = s.resourceReader.ListByWorkIds(ctx, coverWorkIds)
		if err != nil {
			return nil, fmt.Errorf("batch fetch cover resources error: %w", err)
		}
	}

	// Phase 4.5: 批量查 resource_store + PersistentStore(从 resource_store 收集 storeId,不读旧列)
	var allResourceIds []int64
	for _, resources := range resourcesMap {
		for _, res := range resources {
			allResourceIds = append(allResourceIds, res.GetID())
		}
	}
	resourceStoreMap, err := s.resourceStoreBatchReader.ListStoresByResourceIds(ctx, allResourceIds)
	if err != nil {
		return nil, fmt.Errorf("batch fetch resource_stores error: %w", err)
	}
	var allStoreIds []int64
	storeIdSet := make(map[int64]bool)
	for _, rsList := range resourceStoreMap {
		for _, rs := range rsList {
			if rs.StoreID > 0 && !storeIdSet[rs.StoreID] {
				storeIdSet[rs.StoreID] = true
				allStoreIds = append(allStoreIds, rs.StoreID)
			}
		}
	}
	storeMap := make(map[int64]*entity2.PersistentStore)
	if len(allStoreIds) > 0 {
		stores, err := s.storeBatchReader.GetByIds(ctx, allStoreIds)
		if err != nil {
			return nil, fmt.Errorf("batch fetch persistent stores error: %w", err)
		}
		for _, st := range stores {
			storeMap[st.GetID()] = st
		}
	}

	// Phase 5: 组装结果
	results := make([]*dto2.WorkSetWithCoverDTO, 0, len(workSets))
	for _, ws := range workSets {
		item := &dto2.WorkSetWithCoverDTO{
			WorkSet: dto2.NewWorkSetDTO(ws),
		}
		if coverWorkId, ok := coverMap[ws.GetID()]; ok {
			if work, ok := worksMap[coverWorkId]; ok {
				item.CoverWork = dto2.NewWorkDTO(work)
				if resources, ok := resourcesMap[coverWorkId]; ok && len(resources) > 0 {
					res := resources[0]
					rsList := resourceStoreMap[res.GetID()]
					item.CoverResource = dto2.NewResourceFullDTO(res, rsList, storeMap)
				}
			}
		}
		results = append(results, item)
	}

	// Phase 6: 异步更新 lastUse
	used := extractUsedConditions(conditions)
	if len(used) > 0 {
		go func() {
			_ = s.UpdateLastUsed(context.Background(), used)
		}()
	}

	return model.NewPage[dto2.WorkSetWithCoverDTO](results, total, page, pageSize), nil
}

// UpdateLastUsed 更新搜索条件最后使用时间
func (s *Service) UpdateLastUsed(ctx context.Context, used map[dto2.SearchType][]int64) error {
	var wg sync.WaitGroup
	var errs []error
	var mu sync.Mutex

	// 更新本地标签
	if ids, ok := used[dto2.LocalTag]; ok && len(ids) > 0 {
		wg.Add(1)
		go func(tagIds []int64) {
			defer wg.Done()
			if err := s.localTagUpdater.UpdateLastUse(ctx, tagIds); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ids)
	}

	// 更新站点标签
	if ids, ok := used[dto2.SiteTag]; ok && len(ids) > 0 {
		wg.Add(1)
		go func(tagIds []int64) {
			defer wg.Done()
			if err := s.siteTagUpdater.UpdateLastUse(ctx, tagIds); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ids)
	}

	// 更新本地作者
	if ids, ok := used[dto2.LocalAuthor]; ok && len(ids) > 0 {
		wg.Add(1)
		go func(authorIds []int64) {
			defer wg.Done()
			if err := s.localAuthorUpdater.UpdateLastUse(ctx, authorIds); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ids)
	}

	// 更新站点作者
	if ids, ok := used[dto2.SiteAuthor]; ok && len(ids) > 0 {
		wg.Add(1)
		go func(authorIds []int64) {
			defer wg.Done()
			if err := s.siteAuthorUpdater.UpdateLastUse(ctx, authorIds); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(ids)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}
