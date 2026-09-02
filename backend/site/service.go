package site

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	"github.com/lvfeng-z/library-squirrel-sdk/identity"
	"gorm.io/gorm/clause"
)

// Repository 站点仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// CreateBatch 批量新建
	CreateBatch(ctx context.Context, sites []*entity.Site) error
	// Updates 更新
	Updates(ctx context.Context, site *entity.Site) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*entity.Site, error)
	// Get 根据条件获取单个
	Get(ctx context.Context, opt *database.QueryOption) (*entity.Site, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*entity.Site, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
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
	return &Service{repo: repo}
}

// SyncFromRegistry 注册表投影同步：SDK identity 注册表全量条目中，本库缺失的键按注册表
// 权威值（键/站名/主页；无主页条目落 NULL）新建站点行，已有行一律不动（insert-only）——
// 幂等，且用户对展示名（site_name 可改语义）的编辑持久；注册表只增不改，同步无删除分支。
// 调用点为应用启动装配期，失败即启动失败（fail-fast）：站点表缺行会让任务创建/导入报
// 「站点未找到」等衍生错误掩盖根因；insert-only 幂等保证重启重试安全。
func (s *Service) SyncFromRegistry(ctx context.Context) error {
	entries := identity.All()
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, entry.Key)
	}
	existing, err := s.repo.List(ctx, &database.QueryOption{
		Conditions: []clause.Expression{clause.IN{Column: "site_key", Values: util.ToAnySlice(keys)}},
	})
	if err != nil {
		return fmt.Errorf("注册表投影查询既有站点行失败: %w", err)
	}
	existingKeys := make(map[string]struct{}, len(existing))
	for _, row := range existing {
		existingKeys[row.SiteKey] = struct{}{}
	}
	creates := make([]*entity.Site, 0)
	for _, entry := range entries {
		if _, ok := existingKeys[entry.Key]; ok {
			continue
		}
		e := entity.NewSite()
		e.SiteKey = entry.Key
		e.SiteName = sql.NullString{String: entry.Name, Valid: true}
		if entry.Homepage != "" {
			e.Homepage = sql.NullString{String: entry.Homepage, Valid: true}
		}
		creates = append(creates, e)
	}
	if err := s.repo.CreateBatch(ctx, creates); err != nil {
		return fmt.Errorf("注册表投影新建站点行失败: %w", err)
	}
	return nil
}

// UpdateById 更新站点
func (s *Service) UpdateById(ctx context.Context, site *entity.Site) error {
	if site.GetID() == 0 {
		return ErrSiteIdRequired
	}
	return s.repo.Updates(ctx, site)
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

// GetByKey 根据站点键获取——站点身份查询的规范入口（site_key 为站点唯一身份，
// 名称仅展示、同名可共存，身份匹配一律走键）
func (s *Service) GetByKey(ctx context.Context, siteKey string) (*entity.Site, error) {
	where := clause.Eq{Column: "site_key", Value: siteKey}
	opt := &database.QueryOption{
		Conditions: []clause.Expression{where},
	}
	return s.repo.Get(ctx, opt)
}

// 错误定义
var (
	// ErrSiteIdRequired 更新站点缺少 id
	ErrSiteIdRequired = &pkgerr.BusinessError{Code: 400, Message: "更新站点失败，id不能为空"}
)
