package localAuthor

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
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

// Service 本地作者服务
type Service struct {
	repo Repository
	// 事务执行器（删除编排）
	transactor Transactor
	// 删除编排的引用清理提供方（窄接口注入）
	siteAuthorBindingClearer SiteAuthorBindingClearer
	reWorkAuthorDeleter      ReWorkAuthorDeleter
	workAuthorMirrorClearer  WorkAuthorMirrorClearer
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

// Delete 删除本地作者。三类指向引用在同一事务内先行清理，最后删作者行：
// ①站点绑定列（site_author.local_author_id 无外键防线，不清则留静默悬空引用）
// ②作品-作者关联行（re_work_author，有外键防线，不清则删作者被拒）
// ③作品镜像列（work.local_author_id，有外键防线且拦截不分行态——软删作品行的引用同样须清）
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
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
		return s.repo.Delete(txCtx, id)
	})
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
	return s.repo.ListReWorkAuthor(ctx, workIds)
}

// ListByWorkId 查询作品的本地作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedLocalAuthor, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 查询多个作品的本地作者列表
func (s *Service) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedLocalAuthorWithWorkId, error) {
	return s.repo.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// 错误定义
var (
	ErrAuthorIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新本地作者失败，id不能为空"}
)
