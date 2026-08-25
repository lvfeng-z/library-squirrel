package reWorkAuthor

import (
	"context"

	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/database"
)

// Repository 作品-作者关联仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error
	// Updates 更新
	Updates(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.ReWorkAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.ReWorkAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// DeleteSiteByWorkId 删除作品的 SITE 作者关联（保留 LOCAL）
	DeleteSiteByWorkId(ctx context.Context, workId int64) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（SITE 删后重建批内重复元数据折叠 + LOCAL 关联增量入库用）
	SaveBatchOnConflict(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error
	// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
	DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error
	// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
	DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error
	// ListRelationsByWorkId 查询作品关联的所有作者关联记录（原始实体，含 role_name/sort_order）
	ListRelationsByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkAuthor, error)

	// ========== 批量查询作者信息 ==========

	// ListLocalAuthorsByWorkId 查询作品关联的本地作者
	ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error)
	// ListSiteAuthorsByWorkId 查询作品关联的站点作者
	ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error)
	// ListLocalAuthorsByWorkIds 批量查询作品的本地作者
	ListLocalAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedLocalAuthor, error)
	// ListSiteAuthorsByWorkIds 批量查询作品的站点作者
	ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedSiteAuthor, error)
	// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
	ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error)
}

// Service 作品-作者关联服务
type Service struct {
	repo Repository
}

// NewService 创建作品-作者关联服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// ========== 基础 CRUD 操作 ==========

// Save 保存关联
func (s *Service) Save(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return s.repo.Create(ctx, reWorkAuthor)
}

// Update 更新关联
func (s *Service) Update(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return s.repo.Updates(ctx, reWorkAuthor)
}

// Delete 删除关联
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// GetById 根据ID获取关联
func (s *Service) GetById(ctx context.Context, id int64) (*domain.ReWorkAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.ReWorkAuthor, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// DeleteByWorkId 根据作品ID删除所有关联
func (s *Service) DeleteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteByWorkId(ctx, workId)
}

func (s *Service) DeleteSiteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteSiteByWorkId(ctx, workId)
}

func (s *Service) SaveBatchOnConflict(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error {
	return s.repo.SaveBatchOnConflict(ctx, reWorkAuthors)
}

// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
func (s *Service) DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error {
	return s.repo.DeleteByLocalAuthorId(ctx, localAuthorId)
}

// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
func (s *Service) DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error {
	return s.repo.DeleteBySiteAuthorId(ctx, siteAuthorId)
}

// ListRelationsByWorkId 查询作品关联的所有作者关联记录（原始实体）
func (s *Service) ListRelationsByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkAuthor, error) {
	return s.repo.ListRelationsByWorkId(ctx, workId)
}

// ========== 查询操作 ==========

// ListByWorkId 获取单个作品的作者关联信息
func (s *Service) ListByWorkId(ctx context.Context, workId int64) (*dto.WorkAuthorDTO, error) {
	result := &dto.WorkAuthorDTO{}

	// 查询本地作者
	localAuthors, err := s.repo.ListLocalAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	result.LocalAuthors = localAuthors

	// 查询站点作者
	siteAuthors, err := s.repo.ListSiteAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	result.SiteAuthors = siteAuthors

	return result, nil
}

// ListByWorkIds 批量获取多个作品的作者关联信息
func (s *Service) ListByWorkIds(ctx context.Context, workIds []int64) ([]*dto.WorkAuthorsResultDTO, error) {
	if len(workIds) == 0 {
		return make([]*dto.WorkAuthorsResultDTO, 0), nil
	}

	// 批量查询本地作者
	localAuthorMap, err := s.repo.ListLocalAuthorsByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 批量查询站点作者
	siteAuthorMap, err := s.repo.ListSiteAuthorsByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}

	// 组装结果
	results := make([]*dto.WorkAuthorsResultDTO, 0, len(workIds))
	for _, workId := range workIds {
		result := &dto.WorkAuthorsResultDTO{
			WorkId:       workId,
			LocalAuthors: localAuthorMap[workId],
			SiteAuthors:  siteAuthorMap[workId],
		}
		// 确保空切片而不是nil
		if result.LocalAuthors == nil {
			result.LocalAuthors = make([]*dto.RankedLocalAuthor, 0)
		}
		if result.SiteAuthors == nil {
			result.SiteAuthors = make([]*dto.RankedSiteAuthor, 0)
		}
		results = append(results, result)
	}

	return results, nil
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (s *Service) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
	return s.repo.ListLocalAuthorsByWorkId(ctx, workId)
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (s *Service) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	return s.repo.ListSiteAuthorsByWorkId(ctx, workId)
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
	return s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
	return s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// ListSiteAuthorsByWorkIds 批量查询作品的站点作者，按 workId 分组
func (s *Service) ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedSiteAuthor, error) {
	return s.repo.ListSiteAuthorsByWorkIds(ctx, workIds)
}
