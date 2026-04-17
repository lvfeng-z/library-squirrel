package site

import (
	"context"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// SiteQueryDTO 站点查询条件
type SiteQueryDTO struct {
	// 精确查询
	ID           *int64  `json:"-"`            // 站点ID（程序设置，不从JSON解析）
	SiteName     *string `json:"siteName"`     // 站点名称（精确匹配）
	Homepage     *string `json:"homepage"`     // 主页地址（精确匹配）
	SiteNameLike *string `json:"siteNameLike"` // 站点名称（模糊匹配）
	// 排序字段：create_time, update_time, site_name
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *SiteQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time": "create_time",
			"update_time": "update_time",
			"site_name":   "site_name",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

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
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Site], error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.SelectItem], error)
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

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除站点
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.Site], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteQueryDTO) (*model.Page[domain.Site], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	opt := &database.PageOption{
		QueryOption: database.QueryOption{
			Conditions: conditions,
			OrderBy:    []clause.Expression{orderBy},
		},
		Page:     page,
		PageSize: pageSize,
	}
	return s.repo.Page(ctx, opt)
}

// QuerySelectItemPage 分页查询选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPage(ctx context.Context, page, pageSize int, queryDTO SiteQueryDTO) (*model.Page[domain.SelectItem], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QuerySelectItemPage(ctx, page, pageSize, conditions, orderBy)
}

// GetByName 根据站点名称获取
func (s *Service) GetByName(ctx context.Context, siteName string) (*domain.Site, error) {
	where := clause.Eq{Column: "site_name", Value: siteName}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return s.repo.Get(ctx, opt)
}

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *SiteQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.SiteName != nil {
		conditions = append(conditions, clause.Eq{Column: "site_name", Value: *dto.SiteName})
	}
	if dto.Homepage != nil {
		conditions = append(conditions, clause.Eq{Column: "homepage", Value: *dto.Homepage})
	}
	if dto.SiteNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_name", Value: *dto.SiteNameLike})
	}

	return conditions
}

// combineConditions 将多个条件组合成单个表达式
func combineConditions(conditions []clause.Expression) clause.Expression {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return conditions[0]
	}
	result := clause.AndConditions{}
	for _, cond := range conditions {
		result.Exprs = append(result.Exprs, cond)
	}
	return result
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
