package siteAuthor

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/logger"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
	"github.com/library-squirrel/wails/pkg/query"

	"gorm.io/gorm/clause"
)

// Repository 站点作者仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, author *entity2.SiteAuthor) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, authors []*entity2.SiteAuthor) error
	// Update 更新
	Update(ctx context.Context, author *entity2.SiteAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity2.SiteAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity2.SiteAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity2.SiteAuthor, SiteAuthorQueryDTO], error)
	// ListByWorkId 查询作品的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error)
	// ListBySiteAuthorIds 根据站点作者ID列表查询
	ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity2.SiteAuthor, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error)
	// UpdateBindLocalAuthor 绑定本地作者
	UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) (int64, error)
	// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
	QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalAuthorId *bool, localAuthorId *int64) (*model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO], error)
	// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
	QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO], error)
	// GetLocalAuthorByName 根据作者名称查询本地作者
	GetLocalAuthorByName(ctx context.Context, authorName string) (*entity2.LocalAuthor, error)
	// SaveLocalAuthor 保存本地作者
	SaveLocalAuthor(ctx context.Context, author *entity2.LocalAuthor) error
}

// Service 站点作者服务
type Service struct {
	repo Repository
}

// NewService 创建站点作者服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存站点作者
func (s *Service) Save(ctx context.Context, author *entity2.SiteAuthor) error {
	return s.repo.Save(ctx, author)
}

// SaveBatch 批量保存站点作者
func (s *Service) SaveBatch(ctx context.Context, authors []*entity2.SiteAuthor) error {
	return s.repo.SaveBatch(ctx, authors)
}

// UpdateById 更新站点作者
func (s *Service) UpdateById(ctx context.Context, author *entity2.SiteAuthor) error {
	if author.ID == 0 {
		return ErrAuthorIdRequired
	}
	return s.repo.Update(ctx, author)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		author, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		author.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Update(ctx, author); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity2.SiteAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity2.SiteAuthor, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除作者
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page *model.Page[entity2.SiteAuthor, SiteAuthorQueryDTO]) (*model.Page[entity2.SiteAuthor, SiteAuthorQueryDTO], error) {
	conv := query.NewConverter(entity2.SiteAuthor{})
	opt, err := conv.ToPageOption(page.Query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (s *Service) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO]) (*model.Page[dto.SiteAuthorFullDTO, SiteAuthorQueryDTO], error) {
	conv := query.NewConverter(entity2.SiteAuthor{})
	queryOpt, err := conv.ToQueryOption(page.Query, nil)
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
	// 类型断言获取 BoundOnLocalAuthorId 和 LocalAuthorID 的值
	var boundOnLocalAuthorId *bool
	if page.Query.BoundOnLocalAuthorId.Value != nil {
		boundOnLocalAuthorId = page.Query.BoundOnLocalAuthorId.Value
	}
	var localAuthorId *int64
	if page.Query.LocalAuthorID.Value != nil {
		localAuthorId = page.Query.LocalAuthorID.Value
	}
	return s.repo.QueryBoundOrUnboundToLocalAuthorPage(ctx, page.PageNumber, page.PageSize, where, order, boundOnLocalAuthorId, localAuthorId)
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]) (*model.Page[dto.SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO], error) {
	conv := query.NewConverter(entity2.SiteAuthor{})
	queryOpt, err := conv.ToQueryOption(page.Query, nil)
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
	return s.repo.QueryLocalRelateDTOPage(ctx, page.PageNumber, page.PageSize, where, order)
}

// ListByWorkId 查询作品的站点作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*dto.RankedSiteAuthor, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (s *Service) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*entity2.SiteAuthor, error) {
	return s.repo.ListBySiteAuthorIds(ctx, siteAuthorIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*dto.RankedSiteAuthorWithWorkId, error) {
	return s.repo.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
}

// UpdateBindLocalAuthor 绑定或解除本地作者绑定
func (s *Service) UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) (bool, error) {
	if len(siteAuthorIds) == 0 {
		return true, nil
	}
	affected, err := s.repo.UpdateBindLocalAuthor(ctx, localAuthorId, siteAuthorIds)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// CreateSameNameLocalAuthor 创建或获取同名的本地作者
func (s *Service) CreateSameNameLocalAuthor(ctx context.Context, siteAuthor *entity2.SiteAuthor) (int64, error) {
	if !siteAuthor.AuthorName.Valid {
		return 0, nil
	}
	// 查询是否已有同名作者
	existing, err := s.repo.GetLocalAuthorByName(ctx, siteAuthor.AuthorName.String)
	if err == nil && existing != nil {
		return existing.ID, nil
	}

	// 新增同名作者
	newLocalAuthor := &entity2.LocalAuthor{
		AuthorName: siteAuthor.AuthorName,
		Introduce:  siteAuthor.Introduce,
	}
	if err := s.repo.SaveLocalAuthor(ctx, newLocalAuthor); err != nil {
		return 0, err
	}
	return newLocalAuthor.ID, nil
}

// CreateAndBindSameNameLocalAuthor 创建并绑定同名的本地作者
func (s *Service) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *entity2.SiteAuthor) (bool, error) {
	if siteAuthor.ID == 0 {
		logger.Log.Error("创建同名本地作者失败，作者ID不能为空")
		return false, nil
	}
	if !siteAuthor.AuthorName.Valid || siteAuthor.AuthorName.String == "" {
		logger.Log.Error("创建同名本地作者失败，作者名称不能为空")
		return false, nil
	}

	localAuthorId, err := s.CreateSameNameLocalAuthor(ctx, siteAuthor)
	if err != nil {
		return false, err
	}

	return s.UpdateBindLocalAuthor(ctx, localAuthorId, []int64{siteAuthor.ID})
}

// 错误定义
var (
	ErrAuthorIdRequired = &BusinessError{Code: 400, Message: "更新站点作者失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
