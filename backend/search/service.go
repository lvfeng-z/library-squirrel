package search

import (
	"context"
	"fmt"
	"sync"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
)

// ========== 外部模块接口定义（由 search 模块定义自己需要的接口）==========

// Repository 搜索仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
	QuerySearchConditionPage(ctx context.Context, page, pageSize int, keyword string, types []sdkdto.SearchType) ([]*sdkdto.SelectItem, int64, error)
	// QueryWorkPage 查询作品分页
	QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) ([]*sdkdto.WorkFullDTO, int64, error)
	// QueryWorkSetPageByConditions 根据搜索条件查询作品集分页（EXISTS 子查询）
	QueryWorkSetPageByConditions(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) ([]*entity2.WorkSet, int64, error)
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

// Service 搜索服务
type Service struct {
	repo Repository

	// 作品集查询依赖
	coverResolver  CoverResolver
	workReader     WorkSetPageWorkReader
	resourceReader WorkSetPageResourceReader

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
	localTagUpdater LocalTagUpdater,
	siteTagUpdater SiteTagUpdater,
	localAuthorUpdater LocalTagUpdater,
	siteAuthorUpdater SiteAuthorUpdater,
) *Service {
	return &Service{
		repo:               repo,
		coverResolver:      coverResolver,
		workReader:         workReader,
		resourceReader:     resourceReader,
		localTagUpdater:    localTagUpdater,
		siteTagUpdater:     siteTagUpdater,
		localAuthorUpdater: localAuthorUpdater,
		siteAuthorUpdater:  siteAuthorUpdater,
	}
}

// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
func (s *Service) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *sdkdto.SearchConditionQuery) (*model.Page[sdkdto.SelectItem], error) {
	if query == nil {
		query = &sdkdto.SearchConditionQuery{}
	}
	items, total, err := s.repo.QuerySearchConditionPage(ctx, page, pageSize, query.Keyword, query.Types)
	if err != nil {
		return nil, fmt.Errorf("query search condition page error: %w", err)
	}
	return model.NewPage[sdkdto.SelectItem](items, total, page, pageSize), nil
}

// QueryWorkPage 查询作品分页
func (s *Service) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) (*model.Page[sdkdto.WorkFullDTO], error) {
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

	return model.NewPage[sdkdto.WorkFullDTO](items, total, page, pageSize), nil
}

// extractUsedConditions 从搜索条件中提取需要更新lastUse的ID
func extractUsedConditions(conditions []*sdkdto.SearchCondition) map[sdkdto.SearchType][]int64 {
	used := make(map[sdkdto.SearchType][]int64)
	for _, cond := range conditions {
		if cond == nil {
			continue
		}
		switch cond.Type {
		case sdkdto.SearchTypeLocalTag, sdkdto.SearchTypeSiteTag, sdkdto.SearchTypeLocalAuthor, sdkdto.SearchTypeSiteAuthor:
			if id, ok := cond.Value.(float64); ok {
				used[cond.Type] = append(used[cond.Type], int64(id))
			}
		}
	}
	return used
}

// QueryWorkSetPage 查询作品集分页（通过搜索条件筛选关联的作品，返回带封面的作品集）
func (s *Service) QueryWorkSetPage(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) (*model.Page[sdkdto.WorkSetWithCoverDTO], error) {
	if conditions == nil {
		conditions = []*sdkdto.SearchCondition{}
	}

	// Phase 1: 查询作品集分页（EXISTS 子查询）
	workSets, total, err := s.repo.QueryWorkSetPageByConditions(ctx, page, pageSize, conditions)
	if err != nil {
		return nil, fmt.Errorf("query work set page error: %w", err)
	}

	if len(workSets) == 0 {
		return model.NewPage[sdkdto.WorkSetWithCoverDTO]([]*sdkdto.WorkSetWithCoverDTO{}, 0, page, pageSize), nil
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

	// Phase 5: 组装结果
	results := make([]*sdkdto.WorkSetWithCoverDTO, 0, len(workSets))
	for _, ws := range workSets {
		item := &sdkdto.WorkSetWithCoverDTO{
			WorkSet: dto2.NewWorkSetDTO(ws),
		}
		if coverWorkId, ok := coverMap[ws.GetID()]; ok {
			if work, ok := worksMap[coverWorkId]; ok {
				item.CoverWork = dto2.NewWorkDTO(work)
				if resources, ok := resourcesMap[coverWorkId]; ok {
					for _, res := range resources {
						if res.Enabled {
							item.CoverResource = dto2.NewResourceDTO(res)
							break
						}
					}
					if item.CoverResource == nil && len(resources) > 0 {
						item.CoverResource = dto2.NewResourceDTO(resources[0])
					}
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

	return model.NewPage[sdkdto.WorkSetWithCoverDTO](results, total, page, pageSize), nil
}

// UpdateLastUsed 更新搜索条件最后使用时间
func (s *Service) UpdateLastUsed(ctx context.Context, used map[sdkdto.SearchType][]int64) error {
	var wg sync.WaitGroup
	var errs []error
	var mu sync.Mutex

	// 更新本地标签
	if ids, ok := used[sdkdto.SearchTypeLocalTag]; ok && len(ids) > 0 {
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
	if ids, ok := used[sdkdto.SearchTypeSiteTag]; ok && len(ids) > 0 {
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
	if ids, ok := used[sdkdto.SearchTypeLocalAuthor]; ok && len(ids) > 0 {
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
	if ids, ok := used[sdkdto.SearchTypeSiteAuthor]; ok && len(ids) > 0 {
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
