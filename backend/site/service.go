package site

import (
	"context"

	"github.com/library-squirrel/wails/backend/base/model"
	"github.com/library-squirrel/wails/backend/base/model/dto"
	"github.com/library-squirrel/wails/backend/base/model/entity"
	querypkg "github.com/library-squirrel/wails/backend/base/query"
	"github.com/library-squirrel/wails/backend/database"
	pkgerr "github.com/library-squirrel/wails/backend/error"
	"github.com/library-squirrel/wails/backend/util"

	"gorm.io/gorm/clause"
)

// Repository 站点仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Save 保存
	Save(ctx context.Context, site *entity.Site) error
	// Update 更新
	Update(ctx context.Context, site *entity.Site) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Site, error)
	// Get 根据条件获取单个
	Get(ctx context.Context, opt *database.QueryOption) (*entity.Site, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.Site, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Site], error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.SelectItem], error)
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
func (s *Service) Save(ctx context.Context, site *entity.Site) error {
	return s.repo.Save(ctx, site)
}

// UpdateById 更新站点
func (s *Service) UpdateById(ctx context.Context, site *entity.Site) error {
	if site.GetID() == 0 {
		return ErrSiteIdRequired
	}
	return s.repo.Update(ctx, site)
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*entity.Site, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*entity.Site, error) {
	return s.repo.List(ctx, opt)
}

// ListByIds 根据ID列表批量查询
func (s *Service) ListByIds(ctx context.Context, ids []int64) ([]*entity.Site, error) {
	if len(ids) == 0 {
		return make([]*entity.Site, 0), nil
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
func (s *Service) Page(ctx context.Context, page *model.Page[entity.Site], query SiteQueryDTO) (*model.Page[entity.Site], error) {
	conv := querypkg.NewConverter(entity.Site{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QuerySelectItemPage 分页查询选择项
func (s *Service) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem], query SiteQueryDTO) (*model.Page[dto.SelectItem], error) {
	conv := querypkg.NewConverter(entity.Site{})
	opt, err := conv.ToPageOption(query, page.PageNumber, page.PageSize, nil)
	if err != nil {
		return nil, err
	}
	return s.repo.QuerySelectItemPage(ctx, opt)
}

// GetByName 根据站点名称获取
func (s *Service) GetByName(ctx context.Context, siteName string) (*entity.Site, error) {
	where := clause.Eq{Column: "site_name", Value: siteName}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return s.repo.Get(ctx, opt)
}

// 错误定义
var (
	ErrSiteIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新站点失败，id不能为空"}
)
