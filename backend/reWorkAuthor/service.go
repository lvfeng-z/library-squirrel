package reWorkAuthor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/library-squirrel/backend/base/constant"
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
	// DeleteByWorkAndAuthor 根据作品ID和作者删除关联（该作者的全部 role 关联行；authorType 非法时无操作）
	DeleteByWorkAndAuthor(ctx context.Context, workId int64, authorType int, authorId int64) error
	// DeleteByWorkAuthorAndRole 按作品+作者+role 精确删除关联行（不波及同作者其他 role 行）
	DeleteByWorkAuthorAndRole(ctx context.Context, workId int64, authorType int, authorId int64, roleName string) error
	// DeletePluginSiteByWorkId 删除作品插件来源的 SITE 作者关联（保留 LOCAL 与用户来源关联）
	DeletePluginSiteByWorkId(ctx context.Context, workId int64) error
	// SaveBatchOnConflict 批量保存，唯一冲突跳过（SITE 删后重建批内重复元数据折叠 + LOCAL 关联增量入库用）
	SaveBatchOnConflict(ctx context.Context, reWorkAuthors []*domain.ReWorkAuthor) error
	// UpsertBatch 批量 upsert：按 (work_id, author_id, role_name) 冲突更新 sort_order（不含 source），否则插入；
	// 同作品同作者不同 role 不构成冲突，落独立关联行
	UpsertBatch(ctx context.Context, rels []*domain.ReWorkAuthor, authorType int) error
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
	repo          Repository
	transactor    Transactor
	roleInventory RoleInventoryWriter
	// 头像路径解析提供方（siteAuthor/localAuthor 服务在本服务之后创建，经 setter 注入；
	// 各自 nil=跳过对应侧的头像 enrich）
	siteAvatarResolver  SiteAuthorAvatarPathResolver
	localAvatarResolver LocalAuthorAvatarPathResolver
}

// Transactor 事务执行器（手动挂联链的关联写入与 role 清单登记同事务，事务连接经 ctx 传递）
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// RoleInventoryWriter 作者 role 维度清单写入（authorRole.Service 实现）：
// 挂联写 role 时同事务登记清单行（find-or-create + origin 升级 + last_use 刷新）
type RoleInventoryWriter interface {
	// EnsureUsedBatch 批量登记维度值使用（空串不产生清单行）
	EnsureUsedBatch(ctx context.Context, values []string, origin int64) error
}

// SiteAuthorAvatarPathResolver 站点作者头像路径解析（siteAuthor.Service 实现）：本模块的 Ranked*
// 站点作者产出自关联行 JOIN 作者表而来，不含头像引用列，经此反查补齐
type SiteAuthorAvatarPathResolver interface {
	// AvatarFilePathsByAuthorIds 批量解析站点作者头像展示路径（无可展示头像的 id 不在返回 map）
	AvatarFilePathsByAuthorIds(ctx context.Context, authorIds []int64) (map[int64]*string, error)
}

// LocalAuthorAvatarPathResolver 本地作者头像路径解析（localAuthor.Service 实现，语义同站点侧）
type LocalAuthorAvatarPathResolver interface {
	// AvatarFilePathsByAuthorIds 批量解析本地作者头像展示路径（无可展示头像的 id 不在返回 map）
	AvatarFilePathsByAuthorIds(ctx context.Context, authorIds []int64) (map[int64]*string, error)
}

// NewService 创建作品-作者关联服务
func NewService(repo Repository, transactor Transactor, roleInventory RoleInventoryWriter) *Service {
	return &Service{
		repo:          repo,
		transactor:    transactor,
		roleInventory: roleInventory,
	}
}

// SetAvatarPathResolvers 注入作者头像路径解析提供方（siteAuthor/localAuthor 服务在本服务之后创建，
// 装配处接线；各自 nil=跳过对应侧的头像 enrich）
func (s *Service) SetAvatarPathResolvers(site SiteAuthorAvatarPathResolver, local LocalAuthorAvatarPathResolver) {
	s.siteAvatarResolver = site
	s.localAvatarResolver = local
}

// fillSiteAuthorAvatars 批量填充站点作者 Ranked DTO 的头像字段（后置 enrich）
func (s *Service) fillSiteAuthorAvatars(ctx context.Context, authors []*dto.RankedSiteAuthor) error {
	if s.siteAvatarResolver == nil || len(authors) == 0 {
		return nil
	}
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.ID > 0 {
			authorIds = append(authorIds, author.Author.ID)
		}
	}
	avatarPaths, err := s.siteAvatarResolver.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.ID]
	}
	return nil
}

// fillLocalAuthorAvatars 批量填充本地作者 Ranked DTO 的头像字段（后置 enrich）
func (s *Service) fillLocalAuthorAvatars(ctx context.Context, authors []*dto.RankedLocalAuthor) error {
	if s.localAvatarResolver == nil || len(authors) == 0 {
		return nil
	}
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.Id > 0 {
			authorIds = append(authorIds, author.Author.Id)
		}
	}
	avatarPaths, err := s.localAvatarResolver.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.Id]
	}
	return nil
}

// fillSiteAuthorAvatarsWithWorkId 批量填充带作品ID站点作者 DTO 的头像字段（后置 enrich）
func (s *Service) fillSiteAuthorAvatarsWithWorkId(ctx context.Context, authors []*dto.RankedSiteAuthorWithWorkId) error {
	if s.siteAvatarResolver == nil || len(authors) == 0 {
		return nil
	}
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.ID > 0 {
			authorIds = append(authorIds, author.Author.ID)
		}
	}
	avatarPaths, err := s.siteAvatarResolver.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.ID]
	}
	return nil
}

// fillLocalAuthorAvatarsWithWorkId 批量填充带作品ID本地作者 DTO 的头像字段（后置 enrich）
func (s *Service) fillLocalAuthorAvatarsWithWorkId(ctx context.Context, authors []*dto.RankedLocalAuthorWithWorkId) error {
	if s.localAvatarResolver == nil || len(authors) == 0 {
		return nil
	}
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.Id > 0 {
			authorIds = append(authorIds, author.Author.Id)
		}
	}
	avatarPaths, err := s.localAvatarResolver.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.Id]
	}
	return nil
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

func (s *Service) DeletePluginSiteByWorkId(ctx context.Context, workId int64) error {
	return s.repo.DeletePluginSiteByWorkId(ctx, workId)
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
	if err := s.fillLocalAuthorAvatars(ctx, localAuthors); err != nil {
		return nil, err
	}
	result.LocalAuthors = localAuthors

	// 查询站点作者
	siteAuthors, err := s.repo.ListSiteAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if err := s.fillSiteAuthorAvatars(ctx, siteAuthors); err != nil {
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
	if err := s.fillLocalAuthorAvatarMap(ctx, localAuthorMap); err != nil {
		return nil, err
	}

	// 批量查询站点作者
	siteAuthorMap, err := s.repo.ListSiteAuthorsByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if err := s.fillSiteAuthorAvatarMap(ctx, siteAuthorMap); err != nil {
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

// fillLocalAuthorAvatarMap 批量填充按作品分组的本地作者 Ranked DTO 的头像字段（聚合为一次批量解析）
func (s *Service) fillLocalAuthorAvatarMap(ctx context.Context, authorMap map[int64][]*dto.RankedLocalAuthor) error {
	if len(authorMap) == 0 {
		return nil
	}
	ranked := make([]*dto.RankedLocalAuthor, 0)
	for _, list := range authorMap {
		ranked = append(ranked, list...)
	}
	return s.fillLocalAuthorAvatars(ctx, ranked)
}

// fillSiteAuthorAvatarMap 批量填充按作品分组的站点作者 Ranked DTO 的头像字段（聚合为一次批量解析）
func (s *Service) fillSiteAuthorAvatarMap(ctx context.Context, authorMap map[int64][]*dto.RankedSiteAuthor) error {
	if len(authorMap) == 0 {
		return nil
	}
	ranked := make([]*dto.RankedSiteAuthor, 0)
	for _, list := range authorMap {
		ranked = append(ranked, list...)
	}
	return s.fillSiteAuthorAvatars(ctx, ranked)
}

// ListLocalAuthorsByWorkId 查询作品关联的本地作者
func (s *Service) ListLocalAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
	results, err := s.repo.ListLocalAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if err := s.fillLocalAuthorAvatars(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListSiteAuthorsByWorkId 查询作品关联的站点作者
func (s *Service) ListSiteAuthorsByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	results, err := s.repo.ListSiteAuthorsByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if err := s.fillSiteAuthorAvatars(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表（带作品ID）
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
	results, err := s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if err := s.fillLocalAuthorAvatarsWithWorkId(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表（带作品ID）
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
	results, err := s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if err := s.fillSiteAuthorAvatarsWithWorkId(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListSiteAuthorsByWorkIds 批量查询作品的站点作者，按 workId 分组
func (s *Service) ListSiteAuthorsByWorkIds(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedSiteAuthor, error) {
	resultMap, err := s.repo.ListSiteAuthorsByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if err := s.fillSiteAuthorAvatarMap(ctx, resultMap); err != nil {
		return nil, err
	}
	return resultMap, nil
}

// ========== 用户手动挂联 ==========

// UnlinkAuthorFromWork 从作品移除作者
func (s *Service) UnlinkAuthorFromWork(ctx context.Context, workId int64, authorType int, authorId int64) error {
	return s.repo.DeleteByWorkAndAuthor(ctx, workId, authorType, authorId)
}

// ErrRoleNameCountMismatch roleNames 与 authorIds 长度不匹配（须等长配对或全空）
var ErrRoleNameCountMismatch = errors.New("roleNames 与 authorIds 长度不匹配")

// LinkBatchToWork 批量链接作者到作品（upsert：同 work_id+author_id+role_name 已存在则更新 sort_order，否则新增）。
// roleNames 与 authorIds 等长配对（local/site 关联均用调用方值），空数组=全无角色。
// 维度值归一化（去空白+小写折叠）后写入；同作品同作者不同 role 不构成冲突，落独立关联行（身兼数职）。
// 冲突不翻转来源：已存在的关联（插件声明或用户手动先建）source 保持不变，仅刷新用户可编辑字段。
// role 清单登记与关联写入同事务（失败整体回滚，不留孤儿候选行），来源 origin=user
func (s *Service) LinkBatchToWork(ctx context.Context, workId int64, authorType int, authorIds []int64, roleNames []string) error {
	if len(authorIds) == 0 {
		return nil
	}
	if len(roleNames) != 0 && len(roleNames) != len(authorIds) {
		return ErrRoleNameCountMismatch
	}

	rels := make([]*domain.ReWorkAuthor, len(authorIds))
	for i, authorId := range authorIds {
		rel := domain.NewReWorkAuthor()
		rel.WorkID = sql.NullInt64{Int64: workId, Valid: true}
		rel.AuthorType = sql.NullInt64{Int64: int64(authorType), Valid: true}
		rel.Source = constant.MANUAL

		// role 取调用方传值并归一化（越界守卫：配对数组短于 authorIds 时余下关联按无 role 处理）
		if i < len(roleNames) {
			rel.RoleName = constant.NormalizeDimensionValue(roleNames[i])
		}

		if authorType == constant.LOCAL {
			rel.LocalAuthorID = sql.NullInt64{Int64: authorId, Valid: true}
			rel.SiteAuthorID = sql.NullInt64{Valid: false}
		} else {
			rel.LocalAuthorID = sql.NullInt64{Valid: false}
			rel.SiteAuthorID = sql.NullInt64{Int64: authorId, Valid: true}
		}
		rels[i] = rel
	}
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.roleInventory.EnsureUsedBatch(txCtx, roleNames, constant.ORIGIN_USER); err != nil {
			return fmt.Errorf("登记 role 维度清单失败: %w", err)
		}
		return s.repo.UpsertBatch(txCtx, rels, authorType)
	})
}

// RemoveBatchFromWork 批量从作品移除作者（该作者的全部 role 关联行）
func (s *Service) RemoveBatchFromWork(ctx context.Context, workId int64, authorType int, authorIds []int64) error {
	if len(authorIds) == 0 {
		return nil
	}
	for _, authorId := range authorIds {
		if err := s.repo.DeleteByWorkAndAuthor(ctx, workId, authorType, authorId); err != nil {
			return err
		}
	}
	return nil
}

// RemoveDimensionFromWork 批量精确摘除维度关联行（改 role 的旧值行删除：与 RemoveBatchFromWork 的
// 区别是只删 (work, author, role) 命中行，不波及同作者其他 role 行）。roleNames 与 authorIds 等长
// 配对（空数组=全无 role 行），值归一化后比对。清单行不随摘除清理（清单=见过的值，删后复活语义）
func (s *Service) RemoveDimensionFromWork(ctx context.Context, workId int64, authorType int, authorIds []int64, roleNames []string) error {
	if len(authorIds) == 0 {
		return nil
	}
	if len(roleNames) != 0 && len(roleNames) != len(authorIds) {
		return ErrRoleNameCountMismatch
	}
	for i, authorId := range authorIds {
		role := ""
		if i < len(roleNames) {
			role = roleNames[i]
		}
		if err := s.repo.DeleteByWorkAuthorAndRole(ctx, workId, authorType, authorId, role); err != nil {
			return err
		}
	}
	return nil
}
