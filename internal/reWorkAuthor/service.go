package reWorkAuthor

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// WorkAuthorDTO 作品作者信息（包含本地作者和站点作者）
type WorkAuthorDTO struct {
	LocalAuthors []*model.RankedLocalAuthor `json:"localAuthors,omitempty"`
	SiteAuthors  []*model.RankedSiteAuthor  `json:"siteAuthors,omitempty"`
}

// WorkAuthorsResultDTO 批量作品作者信息返回结果
type WorkAuthorsResultDTO struct {
	WorkId       int64                      `json:"workId"`
	LocalAuthors []*model.RankedLocalAuthor `json:"localAuthors,omitempty"`
	SiteAuthors  []*model.RankedSiteAuthor  `json:"siteAuthors,omitempty"`
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
	return s.repo.Save(ctx, reWorkAuthor)
}

// SaveBatch 批量保存关联
func (s *Service) SaveBatch(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error {
	return s.repo.SaveBatch(ctx, reWorkAuthors)
}

// Update 更新关联
func (s *Service) Update(ctx context.Context, reWorkAuthor *domain.ReWorkAuthor) error {
	return s.repo.Update(ctx, reWorkAuthor)
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
func (s *Service) List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.ReWorkAuthor, error) {
	return s.repo.List(ctx, conditions, orderBy, limit, offset)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, conditions []clause.Expression) (int64, error) {
	return s.repo.Count(ctx, conditions)
}

// DeleteByWorkId 根据作品ID删除所有关联
func (s *Service) DeleteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteByWorkId(ctx, workId)
}

// DeleteByLocalAuthorId 根据本地作者ID删除所有关联
func (s *Service) DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error {
	return s.repo.DeleteByLocalAuthorId(ctx, localAuthorId)
}

// DeleteBySiteAuthorId 根据站点作者ID删除所有关联
func (s *Service) DeleteBySiteAuthorId(ctx context.Context, siteAuthorId int64) error {
	return s.repo.DeleteBySiteAuthorId(ctx, siteAuthorId)
}

// ========== 查询操作 ==========

// ListByWorkId 获取单个作品的作者关联信息
func (s *Service) ListByWorkId(ctx context.Context, workId int64) (*WorkAuthorDTO, error) {
	dto := &WorkAuthorDTO{}

	// 查询本地作者
	localAuthors, err := s.repo.ListLocalAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	dto.LocalAuthors = localAuthors

	// 查询站点作者
	siteAuthors, err := s.repo.ListSiteAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	dto.SiteAuthors = siteAuthors

	return dto, nil
}

// ListByWorkIds 批量获取多个作品的作者关联信息
func (s *Service) ListByWorkIds(ctx context.Context, workIds []int64) ([]*WorkAuthorsResultDTO, error) {
	if len(workIds) == 0 {
		return make([]*WorkAuthorsResultDTO, 0), nil
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
	results := make([]*WorkAuthorsResultDTO, 0, len(workIds))
	for _, workId := range workIds {
		result := &WorkAuthorsResultDTO{
			WorkId:       workId,
			LocalAuthors: localAuthorMap[workId],
			SiteAuthors:  siteAuthorMap[workId],
		}
		// 确保空切片而不是nil
		if result.LocalAuthors == nil {
			result.LocalAuthors = make([]*model.RankedLocalAuthor, 0)
		}
		if result.SiteAuthors == nil {
			result.SiteAuthors = make([]*model.RankedSiteAuthor, 0)
		}
		results = append(results, result)
	}

	return results, nil
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (s *Service) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedLocalAuthor, error) {
	return s.repo.ListLocalAuthorsByWorkId(ctx, workId)
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (s *Service) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error) {
	return s.repo.ListSiteAuthorsByWorkId(ctx, workId)
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedLocalAuthorWithWorkId, error) {
	return s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error) {
	return s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
}
