package reWorkTag

import (
	"context"
	"database/sql"
	"errors"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// Repository 作品-标签关联仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建关联
	Create(ctx context.Context, rel *domain.ReWorkTag) error
	// CreateBatch 批量新建关联
	CreateBatch(ctx context.Context, rels []*domain.ReWorkTag) error
	// UpsertBatch 批量 upsert：按 (work_id, tag_id) 冲突更新 namespace，否则插入
	UpsertBatch(ctx context.Context, rels []*domain.ReWorkTag, tagType int) error
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
	// ListLocalTagIdsByWorkIds 批量查询作品关联的本地标签ID，按 workId 分组
	ListLocalTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error)
	// ListSiteTagIdsByWorkIds 批量查询作品关联的站点标签ID，按 workId 分组
	ListSiteTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error)
}

// SiteTagNamespaceReader 提供 site_tag 查询，供 site 关联镜像 namespace。
// siteTag.Service 的 ListBySiteTagIds 结构化匹配此接口（无需 siteTag 反向依赖 reWorkTag）。
type SiteTagNamespaceReader interface {
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*domain.SiteTag, error)
}

// Service 作品-标签关联服务
type Service struct {
	repo          Repository
	siteTagReader SiteTagNamespaceReader
}

// NewService 创建关联服务。siteTagReader 用于 site 关联镜像 site_tag.namespace（可为 nil，仅 site 分支需要）。
func NewService(repo Repository, siteTagReader SiteTagNamespaceReader) *Service {
	return &Service{
		repo:          repo,
		siteTagReader: siteTagReader,
	}
}

// Save 保存关联
func (s *Service) Save(ctx context.Context, rel *domain.ReWorkTag) error {
	return s.repo.Create(ctx, rel)
}

// SaveBatch 批量保存关联
func (s *Service) SaveBatch(ctx context.Context, rels []*domain.ReWorkTag) error {
	return s.repo.CreateBatch(ctx, rels)
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

// ListLocalTagIdsByWorkIds 批量查询作品关联的本地标签ID，按 workId 分组
func (s *Service) ListLocalTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error) {
	return s.repo.ListLocalTagIdsByWorkIds(ctx, workIds)
}

// ListSiteTagIdsByWorkIds 批量查询作品关联的站点标签ID，按 workId 分组
func (s *Service) ListSiteTagIdsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]int64, error) {
	return s.repo.ListSiteTagIdsByWorkIds(ctx, workIds)
}

// LinkTagToWork 链接标签到作品
func (s *Service) LinkTagToWork(ctx context.Context, workId int64, tagType int, tagId int64) error {
	rel := &domain.ReWorkTag{
		WorkID:  sql.NullInt64{Int64: workId, Valid: true},
		TagType: sql.NullInt64{Int64: int64(tagType), Valid: true},
	}
	if tagType == constant.LOCAL {
		rel.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
	} else {
		rel.SiteTagID = sql.NullInt64{Int64: tagId, Valid: true}
	}
	return s.repo.Create(ctx, rel)
}

// UnlinkTagFromWork 从作品移除标签
func (s *Service) UnlinkTagFromWork(ctx context.Context, workId int64, tagType int, tagId int64) error {
	return s.repo.DeleteByWorkAndTag(ctx, workId, tagType, tagId)
}

// ErrNamespaceCountMismatch namespaces 与 tagIds 长度不匹配（须等长配对或全空）
var ErrNamespaceCountMismatch = errors.New("namespaces 与 tagIds 长度不匹配")

// LinkBatchToWork 批量链接标签到作品（upsert：同 work_id+tag_id 已存在则更新 namespace，否则新增）。
// namespaces：local 关联由前端传用户自设值（与 tagIds 等长配对，空数组=全无 namespace）；
// site 关联忽略前端传值，由后端按 site_tag.namespace 镜像（re_work_tag.namespace = 所指 site_tag.namespace）。
func (s *Service) LinkBatchToWork(ctx context.Context, workId int64, tagType int, tagIds []int64, namespaces []string) error {
	if len(tagIds) == 0 {
		return nil
	}
	if len(namespaces) != 0 && len(namespaces) != len(tagIds) {
		return ErrNamespaceCountMismatch
	}

	// site 关联的 namespace 按 site_tag.namespace 镜像（local 用前端传值，无需查 site_tag）
	nsByTagId := make(map[int64]string, len(tagIds))
	if tagType == constant.SITE && s.siteTagReader != nil {
		siteTags, err := s.siteTagReader.ListBySiteTagIds(ctx, tagIds)
		if err != nil {
			return err
		}
		for _, st := range siteTags {
			if st.Namespace.Valid {
				nsByTagId[st.ID] = st.Namespace.String
			}
		}
	}

	rels := make([]*domain.ReWorkTag, len(tagIds))
	for i, tagId := range tagIds {
		rel := domain.NewReWorkTag()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.TagType = sql.NullInt64{Int64: int64(tagType), Valid: true}

		// namespace 解析：local 用前端传值（越界守卫），site 用镜像 map
		ns := ""
		if tagType == constant.LOCAL {
			if i < len(namespaces) {
				ns = namespaces[i]
			}
		} else {
			ns = nsByTagId[tagId]
		}
		rel.Namespace = sql.NullString{String: ns, Valid: ns != ""}

		if tagType == constant.LOCAL {
			rel.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
			rel.SiteTagID = sql.NullInt64{Valid: false}
		} else {
			rel.LocalTagID = sql.NullInt64{Valid: false}
			rel.SiteTagID = sql.NullInt64{Int64: tagId, Valid: true}
		}
		rels[i] = rel
	}
	return s.repo.UpsertBatch(ctx, rels, tagType)
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
