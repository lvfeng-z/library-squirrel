package site

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	querypkg "github.com/library-squirrel/backend/base/query"
	"github.com/library-squirrel/backend/database"
	pkgerr "github.com/library-squirrel/backend/error"
	"github.com/library-squirrel/backend/util"
	"gorm.io/gorm/clause"
)

// Transactor 数据库事务执行器（删除守卫事务用）
type Transactor interface {
	// ExecInTransaction 在事务中执行 fn，事务 DB 实例通过 ctx 传递
	ExecInTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// WorkSiteRefCounter 站点删除守卫的作品引用计数提供方（work 仓储实现）：活行与软删行分别计数——
// 外键拦截不分行态，软删作品行同样令站点不可删；且两类行清理路径不同（活行经作品页删除、
// 软删行经回收站彻底删除），业务提示须分别给出计数与指引
type WorkSiteRefCounter interface {
	CountBySiteId(ctx context.Context, siteId int64) (alive int64, softDeleted int64, err error)
}

// TaskSiteRefCounter 站点删除守卫的任务引用计数提供方（task 仓储实现；task 无软删）
type TaskSiteRefCounter interface {
	CountBySiteId(ctx context.Context, siteId int64) (int64, error)
}

// WorkSetSiteRefCounter 站点删除守卫的作品集引用计数提供方（workSet 仓储实现）：活行与软删行
// 分别计数，语义同 WorkSiteRefCounter 的作品计数
type WorkSetSiteRefCounter interface {
	CountBySiteId(ctx context.Context, siteId int64) (alive int64, softDeleted int64, err error)
}

// SiteTagSiteRefCounter 站点删除守卫的站点标签引用计数提供方（siteTag 仓储实现）
type SiteTagSiteRefCounter interface {
	CountBySiteId(ctx context.Context, siteId int64) (int64, error)
}

// SiteAuthorSiteRefCounter 站点删除守卫的站点作者引用计数提供方（siteAuthor 仓储实现）
type SiteAuthorSiteRefCounter interface {
	CountBySiteId(ctx context.Context, siteId int64) (int64, error)
}

// Repository 站点仓储接口（由 service 定义需要的数据库操作方法）
type Repository interface {
	// Create 新建
	Create(ctx context.Context, site *entity.Site) error
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
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[entity.Site], error)
	// QuerySelectItemPage 分页查询选择项
	QuerySelectItemPage(ctx context.Context, opt *database.PageOption) (*model.Page[dto.SelectItem], error)
}

// Service 站点服务
type Service struct {
	repo                 Repository
	transactor           Transactor
	workRefCounter       WorkSiteRefCounter
	taskRefCounter       TaskSiteRefCounter
	workSetRefCounter    WorkSetSiteRefCounter
	siteTagRefCounter    SiteTagSiteRefCounter
	siteAuthorRefCounter SiteAuthorSiteRefCounter
}

// NewService 创建站点服务。transactor 承载删除守卫事务；
// 五类引用计数提供方（work/task/workSet/siteTag/siteAuthor 仓储）供删除守卫聚合查询
func NewService(
	repo Repository,
	transactor Transactor,
	workRefCounter WorkSiteRefCounter,
	taskRefCounter TaskSiteRefCounter,
	workSetRefCounter WorkSetSiteRefCounter,
	siteTagRefCounter SiteTagSiteRefCounter,
	siteAuthorRefCounter SiteAuthorSiteRefCounter,
) *Service {
	return &Service{
		repo:                 repo,
		transactor:           transactor,
		workRefCounter:       workRefCounter,
		taskRefCounter:       taskRefCounter,
		workSetRefCounter:    workSetRefCounter,
		siteTagRefCounter:    siteTagRefCounter,
		siteAuthorRefCounter: siteAuthorRefCounter,
	}
}

// Create 创建站点行——供插件 AddSite 与导入/分享回灌经 SiteSaveProvider.Create 消费
// （站点行的全部生产者；键来自 SDK identity 注册表，无 Handler 端点）
func (s *Service) Create(ctx context.Context, site *entity.Site) error {
	return s.repo.Create(ctx, site)
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

// Delete 删除站点：前置守卫，不做级联清理——事务内聚合查询该站点的五类引用
// （作品/任务/作品集/站点标签/站点作者；作品与作品集含软删行，外键拦截不分行态），
// 任一计数大于零即拒绝并返回带各项计数与清理指引的业务提示；全部为零才删站点行。
// 守卫查询与删除同事务包裹
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.transactor.ExecInTransaction(ctx, func(txCtx context.Context) error {
		workAlive, workDeleted, err := s.workRefCounter.CountBySiteId(txCtx, id)
		if err != nil {
			return err
		}
		taskCount, err := s.taskRefCounter.CountBySiteId(txCtx, id)
		if err != nil {
			return err
		}
		workSetAlive, workSetDeleted, err := s.workSetRefCounter.CountBySiteId(txCtx, id)
		if err != nil {
			return err
		}
		siteTagCount, err := s.siteTagRefCounter.CountBySiteId(txCtx, id)
		if err != nil {
			return err
		}
		siteAuthorCount, err := s.siteAuthorRefCounter.CountBySiteId(txCtx, id)
		if err != nil {
			return err
		}
		if workAlive > 0 || workDeleted > 0 || taskCount > 0 ||
			workSetAlive > 0 || workSetDeleted > 0 || siteTagCount > 0 || siteAuthorCount > 0 {
			return fmt.Errorf("%w：%s。%s",
				ErrSiteHasReferences,
				strings.Join(siteReferenceDetails(workAlive, workDeleted, taskCount, workSetAlive, workSetDeleted, siteTagCount, siteAuthorCount), "、"),
				siteReferenceGuidance(workAlive, workDeleted, taskCount, workSetAlive, workSetDeleted, siteTagCount, siteAuthorCount))
		}
		// 五类引用全空，外键放行
		return s.repo.Delete(txCtx, id)
	})
}

// siteReferenceDetails 构造守卫提示的计数明细：仅列出非零引用类别；作品/作品集软删行与活行
// 合并为总数并以「含回收站 N」标注软删部分（软删行未彻底删除前同样占用站点，不可删）
func siteReferenceDetails(workAlive, workDeleted, taskCount, workSetAlive, workSetDeleted, siteTagCount, siteAuthorCount int64) []string {
	details := make([]string, 0, 5)
	switch {
	case workAlive > 0 && workDeleted > 0:
		details = append(details, fmt.Sprintf("作品 %d（含回收站 %d）", workAlive+workDeleted, workDeleted))
	case workAlive > 0:
		details = append(details, fmt.Sprintf("作品 %d", workAlive))
	case workDeleted > 0:
		details = append(details, fmt.Sprintf("回收站中作品 %d", workDeleted))
	}
	if taskCount > 0 {
		details = append(details, fmt.Sprintf("任务 %d", taskCount))
	}
	switch {
	case workSetAlive > 0 && workSetDeleted > 0:
		details = append(details, fmt.Sprintf("作品集 %d（含回收站 %d）", workSetAlive+workSetDeleted, workSetDeleted))
	case workSetAlive > 0:
		details = append(details, fmt.Sprintf("作品集 %d", workSetAlive))
	case workSetDeleted > 0:
		details = append(details, fmt.Sprintf("回收站中作品集 %d", workSetDeleted))
	}
	if siteTagCount > 0 {
		details = append(details, fmt.Sprintf("站点标签 %d", siteTagCount))
	}
	if siteAuthorCount > 0 {
		details = append(details, fmt.Sprintf("站点作者 %d", siteAuthorCount))
	}
	return details
}

// siteReferenceGuidance 构造守卫提示的清理指引：按实际存在的引用类别给出可操作路径，
// 与计数明细一一对应（活行作品/作品集经对应页面删除，软删行须在回收站彻底删除）
func siteReferenceGuidance(workAlive, workDeleted, taskCount, workSetAlive, workSetDeleted, siteTagCount, siteAuthorCount int64) string {
	guides := make([]string, 0, 7)
	if workAlive > 0 {
		guides = append(guides, "在作品页删除相关作品")
	}
	if workDeleted > 0 {
		guides = append(guides, "在回收站彻底删除相关作品")
	}
	if taskCount > 0 {
		guides = append(guides, "在任务列表删除相关任务")
	}
	if workSetAlive > 0 {
		guides = append(guides, "在作品集页删除相关作品集")
	}
	if workSetDeleted > 0 {
		guides = append(guides, "在回收站彻底删除相关作品集")
	}
	if siteTagCount > 0 {
		guides = append(guides, "在站点标签页删除相关标签")
	}
	if siteAuthorCount > 0 {
		guides = append(guides, "在站点作者页删除相关作者")
	}
	return "请先" + strings.Join(guides, "、") + "，再重试删除"
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
	// ErrSiteHasReferences 站点仍被数据引用（作品/任务/作品集/站点标签/站点作者，含软删行），
	// 拒绝删除；包装后的完整消息含各项计数与清理指引，供前端直接展示
	ErrSiteHasReferences = errors.New("无法删除站点")
)
