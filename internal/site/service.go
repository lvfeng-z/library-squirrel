package site

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
	"github.com/library-squirrel/wails/pkg/query"

	"gorm.io/gorm/clause"
)

// Repository 站点仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, site *domain.Site) error
	// Update 更新
	Update(ctx context.Context, site *domain.Site) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.Site, error)
	// Get 根据条件获取单个
	Get(ctx context.Context, opt *database.QueryOption) (*domain.Site, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.Site, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Site, any], error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[dto.SelectItem, SiteQueryDTO], error)
}

// Service 站点服务
type Service struct {
	repo Repository
}

// NewService 创建站点服务
func NewService(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}

// Save 保存站点
func (s *Service) Save(ctx context.Context, site *domain.Site) error {
	return s.repo.Save(ctx, site)
}

// UpdateById 更新站点
func (s *Service) UpdateById(ctx context.Context, site *domain.Site) error {
	if site.GetID() == 0 {
		return ErrSiteIdRequired
	}
	return s.repo.Update(ctx, site)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.Site, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.Site, error) {
	return s.repo.List(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*domain.Site, error) {
	if len(ids) == 0 {
		return make([]*domain.Site, 0), nil
	}
	return s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "id", Values: util.ToAnySlice(ids)}},
	})
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除站点
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Site, any], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteQueryDTO) (*model.Page[domain.Site, any], error) {
	conv := query.NewConverter(domain.Site{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QuerySelectItemPage 分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPage(ctx context.Context, page, pageSize int, queryDTO SiteQueryDTO) (*model.Page[dto.SelectItem, SiteQueryDTO], error) {
	conv := query.NewConverter(domain.Site{})
	queryOpt, err := conv.ToQueryOption(queryDTO)
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
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, []clause.Expression{where}, order)
}

// GetByName 根据站点名称获取
func (s *Service) GetByName(ctx context.Context, siteName string) (*domain.Site, error) {
	where := clause.Eq{Column: "site_name", Value: siteName}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return s.repo.Get(ctx, opt)
}

// 错误定义
var (
	ErrSiteIdRequired = &BusinessError{Code: 400, Message: "更新站点失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
