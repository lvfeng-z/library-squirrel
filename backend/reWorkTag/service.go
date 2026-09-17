package reWorkTag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/library-squirrel/backend/base/constant"
	domain "github.com/library-squirrel/backend/base/model/entity"
)

// Repository 作品-标签关联仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建关联
	Create(ctx context.Context, rel *domain.ReWorkTag) error
	// UpsertBatch 批量 upsert：按 (work_id, tag_id, namespace) 冲突仅刷 update_time，否则插入
	UpsertBatch(ctx context.Context, rels []*domain.ReWorkTag, tagType int) error
	// Delete 删除关联
	Delete(ctx context.Context, id int64) error
	// DeleteByWorkAndTag 根据作品ID和标签删除（该标签的全部 ns 关联行）
	DeleteByWorkAndTag(ctx context.Context, workId int64, tagType int, tagId int64) error
	// DeleteByWorkTagAndNamespace 按作品+标签+namespace 精确删除关联行（不波及同标签其他 ns 行）
	DeleteByWorkTagAndNamespace(ctx context.Context, workId int64, tagType int, tagId int64, namespace string) error
	// DeleteByWorkId 根据作品ID删除所有关联
	DeleteByWorkId(ctx context.Context, workId int64) error
	// DeletePluginSiteByWorkId 删除作品插件来源的 SITE 标签关联（保留 LOCAL 与用户手动挂的 SITE 关联）
	DeletePluginSiteByWorkId(ctx context.Context, workId int64) error
	// DeleteByLocalTagId 根据本地标签ID删除所有关联
	DeleteByLocalTagId(ctx context.Context, localTagId int64) error
	// DeleteBySiteTagId 根据站点标签ID删除所有关联
	DeleteBySiteTagId(ctx context.Context, siteTagId int64) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（SITE 删后重建批内重复元数据折叠 + LOCAL 关联增量入库用）
	SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkTag) error
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

// Transactor 事务执行器（手动挂联链的关联写入与 ns 清单登记同事务，事务连接经 ctx 传递）
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// NamespaceInventoryWriter tag namespace 维度清单写入（tagNamespace.Service 实现）：
// 挂联写 ns 时同事务登记清单行（find-or-create + origin 升级 + last_use 刷新）
type NamespaceInventoryWriter interface {
	// EnsureUsedBatch 批量登记维度值使用（空串不产生清单行）
	EnsureUsedBatch(ctx context.Context, values []string, origin int64) error
}

// Service 作品-标签关联服务
type Service struct {
	repo        Repository
	transactor  Transactor
	nsInventory NamespaceInventoryWriter
}

// NewService 创建关联服务
func NewService(repo Repository, transactor Transactor, nsInventory NamespaceInventoryWriter) *Service {
	return &Service{
		repo:        repo,
		transactor:  transactor,
		nsInventory: nsInventory,
	}
}

// Save 保存关联
func (s *Service) Save(ctx context.Context, rel *domain.ReWorkTag) error {
	return s.repo.Create(ctx, rel)
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

func (s *Service) DeletePluginSiteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeletePluginSiteByWorkId(ctx, workId)
}

// UpsertBatch 批量 upsert 关联：按 (work_id, tag_id, namespace) 冲突仅刷 update_time（不翻转 source），否则插入。
// tagType 决定冲突列：local→(work_id, local_tag_id, namespace)，site→(work_id, site_tag_id, namespace)
func (s *Service) UpsertBatch(ctx context.Context, rels []*domain.ReWorkTag, tagType int) error {
	return s.repo.UpsertBatch(ctx, rels, tagType)
}

// DeleteByLocalTagId 根据本地标签ID删除所有关联（供 localTag 删除编排调用——
// re_work_tag.local_tag_id 有外键，未清关联即删标签行会被外键拒绝）
func (s *Service) DeleteByLocalTagId(ctx context.Context, localTagId int64) error {
	return s.repo.DeleteByLocalTagId(ctx, localTagId)
}

// DeleteBySiteTagId 根据站点标签ID删除所有关联（供 siteTag 删除编排调用——
// re_work_tag.site_tag_id 有外键，未清关联即删标签行会被外键拒绝）
func (s *Service) DeleteBySiteTagId(ctx context.Context, siteTagId int64) error {
	return s.repo.DeleteBySiteTagId(ctx, siteTagId)
}

func (s *Service) SaveBatchOnConflict(ctx context.Context, rels []*domain.ReWorkTag) error {
	return s.repo.SaveBatchOnConflict(ctx, rels)
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

// UnlinkTagFromWork 从作品移除标签
func (s *Service) UnlinkTagFromWork(ctx context.Context, workId int64, tagType int, tagId int64) error {
	return s.repo.DeleteByWorkAndTag(ctx, workId, tagType, tagId)
}

// ErrNamespaceCountMismatch namespaces 与 tagIds 长度不匹配（须等长配对或全空）
var ErrNamespaceCountMismatch = errors.New("namespaces 与 tagIds 长度不匹配")

// LinkBatchToWork 批量链接标签到作品（upsert：同 work_id+tag_id+namespace 已存在则冲突更新，否则新增）。
// namespaces：与 tagIds 等长配对（空数组=全无 namespace）。local 与 site 关联均用调用方传值——
// namespace 是关联级开放维度，site 关联同样开放用户自设。维度值归一化（去空白+小写折叠）后写入；
// ns 清单登记与关联写入同事务（失败整体回滚，不留孤儿候选行），来源 origin=user
func (s *Service) LinkBatchToWork(ctx context.Context, workId int64, tagType int, tagIds []int64, namespaces []string) error {
	if len(tagIds) == 0 {
		return nil
	}
	if len(namespaces) != 0 && len(namespaces) != len(tagIds) {
		return ErrNamespaceCountMismatch
	}

	rels := make([]*domain.ReWorkTag, len(tagIds))
	for i, tagId := range tagIds {
		rel := domain.NewReWorkTag()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.TagType = sql.NullInt64{Int64: int64(tagType), Valid: true}
		rel.Source = constant.MANUAL

		// namespace 取调用方传值并归一化（越界守卫：配对数组短于 tagIds 时余下关联按无 namespace 处理）
		if i < len(namespaces) {
			rel.Namespace = constant.NormalizeDimensionValue(namespaces[i])
		}

		if tagType == constant.LOCAL {
			rel.LocalTagID = sql.NullInt64{Int64: tagId, Valid: true}
			rel.SiteTagID = sql.NullInt64{Valid: false}
		} else {
			rel.LocalTagID = sql.NullInt64{Valid: false}
			rel.SiteTagID = sql.NullInt64{Int64: tagId, Valid: true}
		}
		rels[i] = rel
	}
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.nsInventory.EnsureUsedBatch(txCtx, namespaces, constant.ORIGIN_USER); err != nil {
			return fmt.Errorf("登记 ns 维度清单失败: %w", err)
		}
		return s.repo.UpsertBatch(txCtx, rels, tagType)
	})
}

// RemoveBatchFromWork 批量从作品移除标签（该标签的全部 ns 关联行）
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

// RemoveDimensionFromWork 批量精确摘除维度关联行（改 ns 的旧值行删除：与 RemoveBatchFromWork 的
// 区别是只删 (work, tag, ns) 命中行，不波及同标签其他 ns 行）。namespaces 与 tagIds 等长配对
// （空数组=全无 ns 行），值归一化后比对。清单行不随摘除清理（清单=见过的值，删后复活语义）
func (s *Service) RemoveDimensionFromWork(ctx context.Context, workId int64, tagType int, tagIds []int64, namespaces []string) error {
	if len(tagIds) == 0 {
		return nil
	}
	if len(namespaces) != 0 && len(namespaces) != len(tagIds) {
		return ErrNamespaceCountMismatch
	}
	for i, tagId := range tagIds {
		ns := ""
		if i < len(namespaces) {
			ns = namespaces[i]
		}
		if err := s.repo.DeleteByWorkTagAndNamespace(ctx, workId, tagType, tagId, ns); err != nil {
			return err
		}
	}
	return nil
}
