package localAuthor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Repository 本地作者仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, author *domain.LocalAuthor) error
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, authors []*domain.LocalAuthor) error
	// Updates 更新
	Updates(ctx context.Context, author *domain.LocalAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.LocalAuthor], error)
	// ListReWorkAuthor 批量获取作品与作者的关联
	ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedLocalAuthor, error)
	// ListByWorkId 查询作品的本地作者
	ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error)
	// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
	ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error)
	// ListSelectItems 查询选择项列表
	ListSelectItems(ctx context.Context, where clause.Expression, order clause.Expression) ([]*dto.SelectItem, error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.SelectItem], error)
	// UpdateAvatarStoreId 更新头像引用列（可入头像导入编排事务；NULL=清除引用）
	UpdateAvatarStoreId(ctx context.Context, localAuthorId int64, storeId sql.NullInt64) error
}

// Transactor 数据库事务执行器（删除编排用）
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// SiteAuthorBindingClearer 站点绑定清理接口（siteAuthor 仓储实现）
type SiteAuthorBindingClearer interface {
	// ClearLocalAuthorBinding 清除指向本地作者的站点绑定列（置 NULL）
	ClearLocalAuthorBinding(ctx context.Context, localAuthorId int64) error
}

// ReWorkAuthorDeleter 作品-作者关联删除接口（reWorkAuthor 服务实现）
type ReWorkAuthorDeleter interface {
	// DeleteByLocalAuthorId 删除本地作者的全部作品关联
	DeleteByLocalAuthorId(ctx context.Context, localAuthorId int64) error
}

// WorkAuthorMirrorClearer 作品镜像作者列清理接口（work 仓储实现）
type WorkAuthorMirrorClearer interface {
	// ClearLocalAuthorOnWorks 清作品的本地作者镜像列（置 NULL，覆盖含软删行）
	ClearLocalAuthorOnWorks(ctx context.Context, localAuthorId int64) error
}

// AvatarFileCleaner 作者头像清理接口（authorInfo 服务实现）：删除联动的头像行面进本模块
// 删除事务、文件面在事务提交后清理
type AvatarFileCleaner interface {
	// DeleteAvatarStoreRows 事务内物理删头像 persistent_store 行（dbFromCtx 入本删除事务；
	// 须在作者行删除之后调用——FK NO ACTION，引用未释放时删行被拒）。返回被删行的文件
	// 相对路径清单（供事务提交后删文件）
	DeleteAvatarStoreRows(ctx context.Context, storeIds []int64) ([]string, error)
	// RemoveAvatarFiles 事务提交后物理删头像文件（含 fsmonitor 操作抑制登记；尽力而为，
	// 失败记日志留对账裁决）
	RemoveAvatarFiles(relPaths []string)
}

// AvatarStoreReader 头像 persistent_store 行批量读取（persistentStore.Service 实现）：GetByIds
// 经 GORM 软删 scope 自动排除死行，落盘完成判定由 dto.AvatarFilePathByStoreID 统一承担
type AvatarStoreReader interface {
	// GetByIds 根据 ID 列表批量查询记录
	GetByIds(ctx context.Context, ids []int64) ([]*domain.PersistentStore, error)
}

// Service 本地作者服务
type Service struct {
	repo Repository
	// 事务执行器（删除编排）
	transactor Transactor
	// 删除编排的引用清理提供方（窄接口注入）
	siteAuthorBindingClearer SiteAuthorBindingClearer
	reWorkAuthorDeleter      ReWorkAuthorDeleter
	workAuthorMirrorClearer  WorkAuthorMirrorClearer
	// 删除编排的头像清理提供方（authorInfo 在本服务之后创建，经 setter 注入；nil=跳过头像清理）
	avatarFileCleaner AvatarFileCleaner
	// 头像 store 行读取（persistentStore 在本服务之后创建，经 setter 注入；nil=跳过头像 enrich）
	avatarStoreReader AvatarStoreReader
}

// NewService 创建本地作者服务
func NewService(
	repo Repository,
	transactor Transactor,
	siteAuthorBindingClearer SiteAuthorBindingClearer,
	reWorkAuthorDeleter ReWorkAuthorDeleter,
	workAuthorMirrorClearer WorkAuthorMirrorClearer,
) *Service {
	return &Service{
		repo:                     repo,
		transactor:               transactor,
		siteAuthorBindingClearer: siteAuthorBindingClearer,
		reWorkAuthorDeleter:      reWorkAuthorDeleter,
		workAuthorMirrorClearer:  workAuthorMirrorClearer,
	}
}

// Save 保存作者
func (s *Service) Save(ctx context.Context, author *domain.LocalAuthor) error {
	return s.repo.Create(ctx, author)
}

// SaveBatch 批量保存本地作者
func (s *Service) SaveBatch(ctx context.Context, authors []*domain.LocalAuthor) error {
	return s.repo.CreateBatch(ctx, authors)
}

// UpdateById 更新作者
func (s *Service) UpdateById(ctx context.Context, author *domain.LocalAuthor) error {
	if author.ID == 0 {
		return ErrAuthorIdRequired
	}
	return s.repo.Updates(ctx, author)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		// Updates 仅写非零字段，建只含 ID+LastUse 的对象即可，无需读回完整记录
		author := domain.NewLocalAuthor()
		author.SetID(id)
		author.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Updates(ctx, author); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.LocalAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.LocalAuthor, error) {
	return s.repo.List(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*domain.LocalAuthor, error) {
	if len(ids) == 0 {
		return make([]*domain.LocalAuthor, 0), nil
	}
	return s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: util.ToAnySlice(ids)}},
	})
}

// GetByName 根据作者名称查询本地作者
func (s *Service) GetByName(ctx context.Context, name string) (*domain.LocalAuthor, error) {
	authors, err := s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.Eq{Column: "author_name", Value: name}},
		Limit:      1,
	})
	if err != nil {
		return nil, err
	}
	if len(authors) == 0 {
		return nil, fmt.Errorf("local author not found: %s", name)
	}
	return authors[0], nil
}

// GetByNames 根据作者名称列表批量查询本地作者
func (s *Service) GetByNames(ctx context.Context, names []string) ([]*domain.LocalAuthor, error) {
	if len(names) == 0 {
		return make([]*domain.LocalAuthor, 0), nil
	}
	return s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "author_name", Values: util.ToAnySlice(names)}},
	})
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除本地作者。三类指向引用在同一事务内先行清理，再删作者行，最后物理删头像
// persistent_store 行（文件面在事务提交后清理）：
// ①站点绑定列（site_author.local_author_id 无外键防线，不清则留静默悬空引用）
// ②作品-作者关联行（re_work_author，有外键防线，不清则删作者被拒）
// ③作品镜像列（work.local_author_id，有外键防线且拦截不分行态——软删作品行的引用同样须清）
// 头像行删除排在作者行之后：local_author.avatar_store_id 有外键（NO ACTION），作者行未删
// 即删 store 行会被外键拒绝，先删父引用方为强制顺序
func (s *Service) Delete(ctx context.Context, id int64) error {
	// 事务前读作者行头像引用（作者行删除后不可再读）
	var avatarStoreIds []int64
	if s.avatarFileCleaner != nil {
		author, err := s.repo.GetById(ctx, id)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			author = nil
		}
		if author != nil && author.AvatarStoreID.Valid {
			avatarStoreIds = append(avatarStoreIds, author.AvatarStoreID.Int64)
		}
	}
	var avatarRelPaths []string
	if err := s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		if err := s.siteAuthorBindingClearer.ClearLocalAuthorBinding(txCtx, id); err != nil {
			return err
		}
		if err := s.reWorkAuthorDeleter.DeleteByLocalAuthorId(txCtx, id); err != nil {
			return err
		}
		if err := s.workAuthorMirrorClearer.ClearLocalAuthorOnWorks(txCtx, id); err != nil {
			return err
		}
		// 引用已清空，外键放行
		if err := s.repo.Delete(txCtx, id); err != nil {
			return err
		}
		if len(avatarStoreIds) == 0 {
			return nil
		}
		paths, err := s.avatarFileCleaner.DeleteAvatarStoreRows(txCtx, avatarStoreIds)
		if err != nil {
			return err
		}
		avatarRelPaths = paths
		return nil
	}); err != nil {
		return err
	}
	// 事务提交后清理头像文件（行面已消亡，文件面尽力而为）
	if len(avatarRelPaths) > 0 {
		s.avatarFileCleaner.RemoveAvatarFiles(avatarRelPaths)
	}
	return nil
}

// SetAvatarFileCleaner 注入头像清理提供方（authorInfo 服务在本服务之后创建，装配处接线）
func (s *Service) SetAvatarFileCleaner(cleaner AvatarFileCleaner) {
	s.avatarFileCleaner = cleaner
}

// SetAvatarStoreReader 注入头像 store 行读取（persistentStore 服务在本服务之后创建，装配处接线）
func (s *Service) SetAvatarStoreReader(reader AvatarStoreReader) {
	s.avatarStoreReader = reader
}

// AvatarFilePathsByAuthorIds 批量解析本地作者头像展示路径（作者 DB id → workDir 相对路径；
// 无可展示头像的 id 不出现在返回 map，展示侧占位图兜底）。供本模块展示链组装与 reWorkAuthor
// 的 Ranked* 产出后置 enrich 共用
func (s *Service) AvatarFilePathsByAuthorIds(ctx context.Context, authorIds []int64) (map[int64]*string, error) {
	authorIds = util.UniqueInt64(authorIds)
	if len(authorIds) == 0 || s.avatarStoreReader == nil {
		return nil, nil
	}
	authors, err := s.ListByIds(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	storeIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
			storeIds = append(storeIds, author.AvatarStoreID.Int64)
		}
	}
	if len(storeIds) == 0 {
		return nil, nil
	}
	stores, err := s.avatarStoreReader.GetByIds(ctx, storeIds)
	if err != nil {
		return nil, err
	}
	avatarPaths := dto.AvatarFilePathByStoreID(stores)
	result := make(map[int64]*string, len(authors))
	for _, author := range authors {
		if author.AvatarStoreID.Valid && author.AvatarStoreID.Int64 > 0 {
			if path, ok := avatarPaths[author.AvatarStoreID.Int64]; ok {
				result[author.GetID()] = path
			}
		}
	}
	return result, nil
}

// UpdateAvatarStoreId 更新本地作者头像引用列（可入头像导入编排事务；NULL=清除引用）
func (s *Service) UpdateAvatarStoreId(ctx context.Context, localAuthorId int64, storeId sql.NullInt64) error {
	return s.repo.UpdateAvatarStoreId(ctx, localAuthorId, storeId)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[domain.LocalAuthor], queryDTO LocalAuthorQueryDTO) (*model.Page[domain.LocalAuthor], error) {
	conv := query.NewConverter(domain.LocalAuthor{})
	opt, err := conv.ToPageOption(queryDTO, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// GetFullById 根据 ID 获取宿主侧展示 DTO（SDK 实体 DTO + 头像展示路径后置 enrich），
// 管理页详情数据源
func (s *Service) GetFullById(ctx context.Context, id int64) (*dto.LocalAuthorFullDTO, error) {
	author, err := s.repo.GetById(ctx, id)
	if err != nil {
		return nil, err
	}
	result := dto.NewLocalAuthorFullDTO(author)
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, []int64{id})
	if err != nil {
		return nil, err
	}
	result.AvatarFilePath = avatarPaths[id]
	return result, nil
}

// QueryFullPage 分页查询宿主侧展示 DTO（SDK 实体 DTO + 头像展示路径批量后置 enrich），
// 管理页列表数据源
func (s *Service) QueryFullPage(ctx context.Context, page *model.Page[dto.LocalAuthorFullDTO], queryDTO LocalAuthorQueryDTO) (*model.Page[dto.LocalAuthorFullDTO], error) {
	rawPage, err := s.Page(ctx, &model.Page[domain.LocalAuthor]{PageNumber: page.PageNumber, PageSize: page.PageSize}, queryDTO)
	if err != nil {
		return nil, err
	}
	if len(rawPage.Data) == 0 {
		return model.NewPage[dto.LocalAuthorFullDTO](nil, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
	}
	authorIds := make([]int64, 0, len(rawPage.Data))
	for _, author := range rawPage.Data {
		authorIds = append(authorIds, author.GetID())
	}
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	data := make([]*dto.LocalAuthorFullDTO, 0, len(rawPage.Data))
	for _, author := range rawPage.Data {
		full := dto.NewLocalAuthorFullDTO(author)
		full.AvatarFilePath = avatarPaths[author.GetID()]
		data = append(data, full)
	}
	return model.NewPage[dto.LocalAuthorFullDTO](data, rawPage.DataCount, rawPage.PageNumber, rawPage.PageSize), nil
}

// ListSelectItems 查询选择项列表
func (s *Service) ListSelectItems(ctx context.Context, queryDTO LocalAuthorQueryDTO) ([]*dto.SelectItem, error) {
	conv := query.NewConverter(domain.LocalAuthor{})
	queryOpt, err := conv.ToQueryOption(queryDTO, nil)
	if err != nil {
		return nil, err
	}
	var where clause.Expression
	if len(queryOpt.Conditions) > 0 {
		where = queryOpt.Conditions[0]
	}
	var order clause.Expression
	if len(queryOpt.OrderBy) > 0 {
		order = queryOpt.OrderBy[0]
	}
	return s.repo.ListSelectItems(ctx, where, order)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem], queryDTO LocalAuthorQueryDTO) (*model.Page[dto.SelectItem], error) {
	conv := query.NewConverter(domain.LocalAuthor{})
	opt, err := conv.ToPageOption(queryDTO, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QuerySelectItemPage(ctx, opt)
}

// ListReWorkAuthor 批量获取作品与作者的关联
func (s *Service) ListReWorkAuthor(ctx context.Context, workIds []int64) (map[int64][]*dto.RankedLocalAuthor, error) {
	resultMap, err := s.repo.ListReWorkAuthor(ctx, workIds)
	if err != nil {
		return nil, err
	}
	if len(resultMap) == 0 {
		return resultMap, nil
	}
	ranked := make([]*dto.RankedLocalAuthor, 0)
	for _, list := range resultMap {
		ranked = append(ranked, list...)
	}
	if err := s.fillRankedAvatarFilePaths(ctx, ranked); err != nil {
		return nil, err
	}
	return resultMap, nil
}

// ListByWorkId 查询作品的本地作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
	results, err := s.repo.ListByWorkId(ctx, workId)
	if err != nil {
		return nil, err
	}
	if err := s.fillRankedAvatarFilePaths(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
	results, err := s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return nil, err
	}
	authorIds := make([]int64, 0, len(results))
	for _, author := range results {
		if author.Author.Id > 0 {
			authorIds = append(authorIds, author.Author.Id)
		}
	}
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return nil, err
	}
	for _, author := range results {
		author.AvatarFilePath = avatarPaths[author.Author.Id]
	}
	return results, nil
}

// fillRankedAvatarFilePaths 批量填充带排序本地作者 DTO 的头像字段（后置 enrich）
func (s *Service) fillRankedAvatarFilePaths(ctx context.Context, authors []*dto.RankedLocalAuthor) error {
	authorIds := make([]int64, 0, len(authors))
	for _, author := range authors {
		if author.Author.Id > 0 {
			authorIds = append(authorIds, author.Author.Id)
		}
	}
	avatarPaths, err := s.AvatarFilePathsByAuthorIds(ctx, authorIds)
	if err != nil {
		return err
	}
	for _, author := range authors {
		author.AvatarFilePath = avatarPaths[author.Author.Id]
	}
	return nil
}

// 错误定义
var (
	ErrAuthorIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新本地作者失败，id不能为空"}
)
