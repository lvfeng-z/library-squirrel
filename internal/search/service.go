package search

import (
	"context"
	"fmt"
	"sync"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// ========== 外部模块接口定义（由 search 模块定义自己需要的接口）==========

// Repository 搜索仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
	QuerySearchConditionPage(ctx context.Context, page, pageSize int, keyword string, types []domain.SearchType) ([]*domain.SelectItem, int64, error)
	// QueryWorkPage 查询作品分页
	QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*domain.SearchCondition) ([]*domain.WorkFullDTO, int64, error)
	// QueryWorkSetPage 查询作品集分页
	QueryWorkSetPage(ctx context.Context, page, pageSize int, keyword string, siteId int64) ([]*domain.SelectItem, int64, error)
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

	// 外部模块依赖（通过构造函数注入）
	localTagUpdater    LocalTagUpdater
	siteTagUpdater     SiteTagUpdater
	localAuthorUpdater LocalAuthorUpdater
	siteAuthorUpdater  SiteAuthorUpdater
}

// NewService 创建搜索服务
func NewService(
	repo Repository,
	localTagUpdater LocalTagUpdater,
	siteTagUpdater SiteTagUpdater,
	localAuthorUpdater LocalAuthorUpdater,
	siteAuthorUpdater SiteAuthorUpdater,
) *Service {
	return &Service{
		repo:               repo,
		localTagUpdater:    localTagUpdater,
		siteTagUpdater:     siteTagUpdater,
		localAuthorUpdater: localAuthorUpdater,
		siteAuthorUpdater:  siteAuthorUpdater,
	}
}

// QuerySearchConditionPage 查询搜索条件分页（localTag、siteTag、localAuthor、siteAuthor）
func (s *Service) QuerySearchConditionPage(ctx context.Context, page, pageSize int, query *domain.SearchConditionQuery) (*model.Page[domain.SelectItem, domain.SearchConditionQuery], error) {
	if query == nil {
		query = &domain.SearchConditionQuery{}
	}
	items, total, err := s.repo.QuerySearchConditionPage(ctx, page, pageSize, query.Keyword, query.Types)
	if err != nil {
		return nil, fmt.Errorf("query search condition page error: %w", err)
	}
	return model.NewPage[domain.SelectItem, domain.SearchConditionQuery](items, total, page, pageSize), nil
}

// QueryWorkPage 查询作品分页
func (s *Service) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*domain.SearchCondition) (*model.Page[domain.WorkFullDTO, domain.SearchCondition], error) {
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

	return model.NewPage[domain.WorkFullDTO, domain.SearchCondition](items, total, page, pageSize), nil
}

// extractUsedConditions 从搜索条件中提取需要更新lastUse的ID
func extractUsedConditions(conditions []*domain.SearchCondition) map[domain.SearchType][]int64 {
	used := make(map[domain.SearchType][]int64)
	for _, cond := range conditions {
		if cond == nil {
			continue
		}
		switch cond.Type {
		case domain.SearchTypeLocalTag, domain.SearchTypeSiteTag, domain.SearchTypeLocalAuthor, domain.SearchTypeSiteAuthor:
			if id, ok := cond.Value.(float64); ok {
				used[cond.Type] = append(used[cond.Type], int64(id))
			}
		}
	}
	return used
}

// QueryWorkSetPage 查询作品集分页
func (s *Service) QueryWorkSetPage(ctx context.Context, page, pageSize int, keyword string, siteId int64) (*model.Page[domain.SelectItem, WorkSetQueryDTO], error) {
	items, total, err := s.repo.QueryWorkSetPage(ctx, page, pageSize, keyword, siteId)
	if err != nil {
		return nil, err
	}
	return model.NewPage[domain.SelectItem, WorkSetQueryDTO](items, total, page, pageSize), nil
}

// UpdateLastUsed 更新搜索条件最后使用时间
func (s *Service) UpdateLastUsed(ctx context.Context, used map[domain.SearchType][]int64) error {
	var wg sync.WaitGroup
	var errs []error
	var mu sync.Mutex

	// 更新本地标签
	if ids, ok := used[domain.SearchTypeLocalTag]; ok && len(ids) > 0 {
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
	if ids, ok := used[domain.SearchTypeSiteTag]; ok && len(ids) > 0 {
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
	if ids, ok := used[domain.SearchTypeLocalAuthor]; ok && len(ids) > 0 {
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
	if ids, ok := used[domain.SearchTypeSiteAuthor]; ok && len(ids) > 0 {
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
