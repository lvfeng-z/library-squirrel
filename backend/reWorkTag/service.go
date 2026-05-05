package reWorkTag

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/backend/base/model/entity"
)

// Repository 作品-标签关联仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存关联
	Save(ctx context.Context, rel *domain.ReWorkTag) error
	// SaveBatch 批量保存关联
	SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndTag 根据作品ID和标签删除
	DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// ListByWorkId 查询作品关联的所有标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkTag, error)
	// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
	ListLocalTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
	ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error)
	// GetByWorkAndTag 根据作品ID和标签获取关联
	GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error)
	// CountByWorkId 统计作品关联的标签数量
	CountByWorkId(ctx context.Context, workId int64) (int64, error)
}

// Service 作品-标签关联服务
type Service struct {
	repo Repository
}

// NewService 创建关联服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存关联
func (s *Service) Save(ctx context.Context, rel *domain.ReWorkTag) error {
	return s.repo.Save(ctx, rel)
}

// SaveBatch 批量保存关联
func (s *Service) SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error {
	return s.repo.SaveBatch(ctx, rels)
}

// Delete 删除关联
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// DeleteByWorkAndTag 根据作品ID和标签删除
func (s *Service) DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error {
	return s.repo.DeleteByWorkAndTag(ctx, workId, tagType, tagId)
}

// DeleteByWorkId 根据作品ID删除所有关联
func (s *Service) DeleteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeleteByWorkId(ctx, workId)
}

// ListByWorkId 查询作品关联的所有标签
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*domain.ReWorkTag, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListLocalTagIdsByWorkId 查询作品关联的本地标签ID列表
func (s *Service) ListLocalTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	return s.repo.ListLocalTagIdsByWorkId(ctx, workId)
}

// ListSiteTagIdsByWorkId 查询作品关联的站点标签ID列表
func (s *Service) ListSiteTagIdsByWorkId(ctx context.Context, workId int64) ([]int64, error) {
	return s.repo.ListSiteTagIdsByWorkId(ctx, workId)
}

// GetByWorkAndTag 根据作品ID和标签获取关联
func (s *Service) GetByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) (*domain.ReWorkTag, error) {
	return s.repo.GetByWorkAndTag(ctx, workId, tagType, tagId)
}

// CountByWorkId 统计作品关联的标签数量
func (s *Service) CountByWorkId(ctx context.Context, workId int64) (int64, error) {
	return s.repo.CountByWorkId(ctx, workId)
}

// LinkTagToWork 链接标签到作品
func (s *Service) LinkTagToWork(ctx context.Context, workId int64, tagType int, tagId int64) error {
	rel := &domain.ReWorkTag{
		WorkID:     sql.NullInt64{Int64: workId, Valid: true},
		TagType:    sql.NullInt64{Int64: int64(tagType), Valid: true},
		LocalTagID: sql.NullInt64{Int64: 0, Valid: true},
		SiteTagID:  sql.NullInt64{Int64: 0, Valid: true},
	}
	if tagType == TagTypeLocal {
		rel.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
	} else {
		rel.SiteTagID = sql.NullInt64{Int64: tagId, Valid: true}
	}
	return s.repo.Save(ctx, rel)
}

// UnlinkTagFromWork 从作品移除标签
func (s *Service) UnlinkTagFromWork(ctx context.Context, workId int64, tagType int, tagId int64) error {
	return s.repo.DeleteByWorkAndTag(ctx, workId, tagType, tagId)
}

// LinkBatchToWork 批量链接标签到作品
func (s *Service) LinkBatchToWork(ctx context.Context, workId int64, tagType int, tagIds []int64) error {
	if len(tagIds) == 0 {
		return nil
	}
	rels := make([]*domain.ReWorkTag, len(tagIds))
	for i, tagId := range tagIds {
		rels[i] = &domain.ReWorkTag{
			WorkID:     sql.NullInt64{Int64: workId, Valid: true},
			TagType:    sql.NullInt64{Int64: int64(tagType), Valid: true},
			LocalTagID: sql.NullInt64{Int64: 0, Valid: true},
			SiteTagID:  sql.NullInt64{Int64: 0, Valid: true},
		}
		if tagType == TagTypeLocal {
			rels[i].LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
		} else {
			rels[i].SiteTagID = sql.NullInt64{Int64: tagId, Valid: true}
		}
	}
	return s.repo.SaveBatch(ctx, rels)
}

// RemoveBatchFromWork 批量从作品移除标签
func (s *Service) RemoveBatchFromWork(ctx context.Context, workId int64, tagType int, tagIds []int64) error {
	if len(tagIds) == 0 {
		return nil
	}
	for _, tagId := range tagIds {
		if err := s.repo.DeleteByWorkAndTag(ctx, workId, tagType, tagId); err != nil {
			return err
		}
	}
	return nil
}
