package siteAuthor

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/logger"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// SiteAuthorQueryDTO 站点作者查询条件
type SiteAuthorQueryDTO struct {
	// 精确查询
	ID              *int64  `json:"-"`               // 站点作者ID（程序设置，不从JSON解析）
	SiteID          *int64  `json:"siteId"`          // 站点ID
	SiteAuthorID    *string `json:"siteAuthorId"`    // 站点作者ID（外部）
	LocalAuthorID   *int64  `json:"localAuthorId"`   // 本地作者ID
	FixedAuthorName *string `json:"fixedAuthorName"` // 固定作者名称
	// 过滤绑定状态
	BoundOnLocalAuthorId *bool `json:"boundOnLocalAuthorId"` // 是否绑定到指定本地作者（true=绑定的，false=未绑定的）
	// 模糊查询
	AuthorNameLike *string `json:"authorNameLike"` // 作者名称（模糊匹配）
	IntroduceLike  *string `json:"introduceLike"`  // 介绍（模糊匹配）
	// 排序字段：create_time, update_time, author_name, last_use
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *SiteAuthorQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time": "create_time",
			"update_time": "update_time",
			"author_name": "author_name",
			"last_use":    "last_use",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// Repository 站点作者仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, author *domain.SiteAuthor) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, authors []*domain.SiteAuthor) error
	// Update 更新
	Update(ctx context.Context, author *domain.SiteAuthor) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.SiteAuthor, error)
	// List 查询列表
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.SiteAuthor, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.SiteAuthor], error)
	// ListByWorkId 查询作品的站点作者
	ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error)
	// ListBySiteAuthorIds 根据站点作者ID列表查询
	ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*domain.SiteAuthor, error)
	// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
	ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error)
	// UpdateBindLocalAuthor 绑定本地作者
	UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) (int64, error)
	// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
	QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalAuthorId *bool, localAuthorId *int64) (*model.Page[domain.SiteAuthorFullDTO], error)
	// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
	QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SiteAuthorLocalRelateDTO], error)
	// GetLocalAuthorByName 根据作者名称查询本地作者
	GetLocalAuthorByName(ctx context.Context, authorName string) (*domain.LocalAuthor, error)
	// SaveLocalAuthor 保存本地作者
	SaveLocalAuthor(ctx context.Context, author *domain.LocalAuthor) error
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
func (s *Service) Save(ctx context.Context, author *domain.SiteAuthor) error {
	return s.repo.Save(ctx, author)
}

// SaveBatch 批量保存站点作者
func (s *Service) SaveBatch(ctx context.Context, authors []*domain.SiteAuthor) error {
	return s.repo.SaveBatch(ctx, authors)
}

// UpdateById 更新站点作者
func (s *Service) UpdateById(ctx context.Context, author *domain.SiteAuthor) error {
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
func (s *Service) GetById(ctx context.Context, id int64) (*domain.SiteAuthor, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.SiteAuthor, error) {
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
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.SiteAuthor], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthor], error) {
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

// QueryBoundOrUnboundToLocalAuthorPageByDTO 查询绑定或未绑定到本地作者的站点作者分页（基于 QueryDTO）
func (s *Service) QueryBoundOrUnboundToLocalAuthorPageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthorFullDTO], error) {
	// 构建除了 LocalAuthorID 之外的其他条件（LocalAuthorID 的绑定逻辑由 repository 处理）
	var conditions []clause.Expression
	if dto := &queryDTO; dto != nil {
		if dto.ID != nil {
			conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
		}
		if dto.SiteID != nil {
			conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
		}
		if dto.SiteAuthorID != nil {
			conditions = append(conditions, clause.Eq{Column: "site_author_id", Value: *dto.SiteAuthorID})
		}
		if dto.FixedAuthorName != nil {
			conditions = append(conditions, clause.Eq{Column: "fixed_author_name", Value: *dto.FixedAuthorName})
		}
		if dto.AuthorNameLike != nil {
			conditions = append(conditions, clause.Like{Column: "author_name", Value: *dto.AuthorNameLike})
		}
		if dto.IntroduceLike != nil {
			conditions = append(conditions, clause.Like{Column: "introduce", Value: *dto.IntroduceLike})
		}
	}
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryBoundOrUnboundToLocalAuthorPage(ctx, page, pageSize, where, orderBy, queryDTO.BoundOnLocalAuthorId, queryDTO.LocalAuthorID)
}

// QueryLocalRelateDTOPageByDTO 查询站点作者与本地作者关联DTO分页（基于 QueryDTO）
func (s *Service) QueryLocalRelateDTOPageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteAuthorQueryDTO) (*model.Page[domain.SiteAuthorLocalRelateDTO], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, orderBy)
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

// buildConditionsFromDTO 根据查询DTO构建查询条件
func buildConditionsFromDTO(dto *SiteAuthorQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.SiteID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
	}
	if dto.SiteAuthorID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_author_id", Value: *dto.SiteAuthorID})
	}
	if dto.LocalAuthorID != nil {
		conditions = append(conditions, clause.Eq{Column: "local_author_id", Value: *dto.LocalAuthorID})
	}
	if dto.FixedAuthorName != nil {
		conditions = append(conditions, clause.Eq{Column: "fixed_author_name", Value: *dto.FixedAuthorName})
	}
	if dto.AuthorNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "author_name", Value: *dto.AuthorNameLike})
	}
	if dto.IntroduceLike != nil {
		conditions = append(conditions, clause.Like{Column: "introduce", Value: *dto.IntroduceLike})
	}

	return conditions
}

// ListByWorkId 查询作品的站点作者
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*model.RankedSiteAuthor, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListBySiteAuthorIds 根据站点作者ID列表查询
func (s *Service) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) ([]*domain.SiteAuthor, error) {
	return s.repo.ListBySiteAuthorIds(ctx, siteAuthorIds)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 查询多个作品的站点作者列表
func (s *Service) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) ([]*model.RankedSiteAuthorWithWorkId, error) {
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
func (s *Service) CreateSameNameLocalAuthor(ctx context.Context, siteAuthor *domain.SiteAuthor) (int64, error) {
	if !siteAuthor.AuthorName.Valid {
		return 0, nil
	}
	// 查询是否已有同名作者
	existing, err := s.repo.GetLocalAuthorByName(ctx, siteAuthor.AuthorName.String)
	if err == nil && existing != nil {
		return existing.ID, nil
	}

	// 新增同名作者
	newLocalAuthor := &domain.LocalAuthor{
		AuthorName: siteAuthor.AuthorName,
		Introduce:  siteAuthor.Introduce,
	}
	if err := s.repo.SaveLocalAuthor(ctx, newLocalAuthor); err != nil {
		return 0, err
	}
	return newLocalAuthor.ID, nil
}

// CreateAndBindSameNameLocalAuthor 创建并绑定同名的本地作者
func (s *Service) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *domain.SiteAuthor) (bool, error) {
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

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SiteAuthorLocalRelateDTO], error) {
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, order)
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
