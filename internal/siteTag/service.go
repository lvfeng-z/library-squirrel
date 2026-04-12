package siteTag

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// SiteTagQueryDTO 站点标签查询条件
type SiteTagQueryDTO struct {
	// 精确查询
	ID            *int64  `json:"-"`             // 站点标签ID（程序设置，不从JSON解析）
	SiteID        *int64  `json:"siteId"`        // 站点ID
	SiteTagID     *string `json:"siteTagId"`     // 站点标签ID（外部）
	BaseSiteTagID *string `json:"baseSiteTagId"` // 基础站点标签ID
	LocalTagID    *int64  `json:"localTagId"`    // 本地标签ID
	// 过滤绑定状态
	BoundOnLocalTagId *bool `json:"boundOnLocalTagId"` // 是否绑定到指定本地标签（true=绑定的，false=未绑定的）
	// 模糊查询
	SiteTagNameLike *string `json:"siteTagNameLike"` // 站点标签名称（模糊匹配）
	DescriptionLike *string `json:"descriptionLike"` // 描述（模糊匹配）
	// 排序字段：create_time, update_time, site_tag_name, last_use
	OrderBy   string `json:"orderBy"`   // 排序字段
	OrderDesc bool   `json:"orderDesc"` // 是否降序
}

// BuildOrderBy 根据查询DTO构建排序条件
func (dto *SiteTagQueryDTO) BuildOrderBy() clause.Expression {
	column := "id"
	if dto.OrderBy != "" {
		// 支持的排序字段映射
		orderByMap := map[string]string{
			"create_time":   "create_time",
			"update_time":   "update_time",
			"site_tag_name": "site_tag_name",
			"last_use":      "last_use",
		}
		if v, ok := orderByMap[dto.OrderBy]; ok {
			column = v
		}
	}
	return clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: column}, Desc: dto.OrderDesc}}}
}

// ========== 外部模块接口定义（由 siteTag 模块定义自己需要的接口）==========

// LocalTagOperator 本地标签操作接口
type LocalTagOperator interface {
	// Save 保存本地标签
	Save(ctx context.Context, tag *domain.LocalTag) error
	// GetByName 根据名称获取本地标签
	GetByName(ctx context.Context, name string) (*domain.LocalTag, error)
}

// Repository 站点标签仓储接口（由 service 定义需要的数据库操作方法）
// 注意：只定义 service 真正需要的方法，遵循最小依赖原则
type Repository interface {
	// Save 保存
	Save(ctx context.Context, tag *domain.SiteTag) error
	// SaveBatch 批量保存
	SaveBatch(ctx context.Context, tags []*domain.SiteTag) error
	// Update 更新
	Update(ctx context.Context, tag *domain.SiteTag) error
	// GetById 根据ID获取
	GetById(ctx context.Context, id int64) (*domain.SiteTag, error)
	// List 查询列表
	List(ctx context.Context, conditions []clause.Expression, orderBy clause.Expression, limit, offset int) ([]*domain.SiteTag, error)
	// Count 统计数量
	Count(ctx context.Context, conditions []clause.Expression) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, page, pageSize int, conditions []clause.Expression, orderBy clause.Expression) (*model.Page[domain.SiteTag], error)
	// ListByWorkId 查询作品的站点标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.SiteTag, error)
	// ListBySiteTagIds 根据站点标签ID列表查询
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*domain.SiteTag, error)
	// UpdateBindLocalTag 绑定本地标签
	UpdateBindLocalTag(ctx context.Context, localTagId int64, siteTagIds []int64) (int64, error)
	// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
	QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalTagId *bool, localTagId *int64) (*model.Page[domain.SiteTagFullDTO], error)
	// QueryPageByWorkId 根据作品ID分页查询站点标签
	QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO], error)
	// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
	QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO], error)
	// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
	QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem], error)
}

// Service 站点标签服务
type Service struct {
	repo             Repository
	localTagOperator LocalTagOperator
}

// NewService 创建站点标签服务
func NewService(repo Repository, localTagOperator LocalTagOperator) *Service {
	return &Service{
		repo:             repo,
		localTagOperator: localTagOperator,
	}
}

// Save 保存站点标签
func (s *Service) Save(ctx context.Context, tag *domain.SiteTag) error {
	return s.repo.Save(ctx, tag)
}

// SaveBatch 批量保存站点标签
func (s *Service) SaveBatch(ctx context.Context, tags []*domain.SiteTag) error {
	return s.repo.SaveBatch(ctx, tags)
}

// UpdateById 更新站点标签
func (s *Service) UpdateById(ctx context.Context, tag *domain.SiteTag) error {
	if tag.ID == 0 {
		return ErrTagIdRequired
	}
	return s.repo.Update(ctx, tag)
}

// UpdateLastUse 批量更新最后使用时间
func (s *Service) UpdateLastUse(ctx context.Context, ids []int64) error {
	now := util.GetCurrentTimestamp()
	for _, id := range ids {
		tag, err := s.repo.GetById(ctx, id)
		if err != nil {
			continue
		}
		tag.LastUse = sql.NullInt64{Int64: now, Valid: true}
		if err := s.repo.Update(ctx, tag); err != nil {
			return err
		}
	}
	return nil
}

// GetById 根据ID获取
func (s *Service) GetById(ctx context.Context, id int64) (*domain.SiteTag, error) {
	return s.repo.GetById(ctx, id)
}

// List 查询列表
func (s *Service) List(ctx context.Context, where clause.Expression, order clause.Expression, limit, offset int) ([]*domain.SiteTag, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.List(ctx, conditions, order, limit, offset)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, where clause.Expression) (int64, error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Count(ctx, conditions)
}

// Delete 删除标签
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression) (*model.Page[domain.SiteTag], error) {
	var conditions []clause.Expression
	if where != nil {
		conditions = []clause.Expression{where}
	}
	return s.repo.Page(ctx, page, pageSize, conditions, order)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO) (*model.Page[domain.SiteTag], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.Page(ctx, page, pageSize, conditions, orderBy)
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页（基于 QueryDTO）
func (s *Service) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO) (*model.Page[domain.SiteTagFullDTO], error) {
	// 构建除了 LocalTagID 之外的其他条件
	var conditions []clause.Expression
	if dto := &queryDTO; dto != nil {
		if dto.ID != nil {
			conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
		}
		if dto.SiteID != nil {
			conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
		}
		if dto.SiteTagID != nil {
			conditions = append(conditions, clause.Eq{Column: "site_tag_id", Value: *dto.SiteTagID})
		}
		if dto.BaseSiteTagID != nil {
			conditions = append(conditions, clause.Eq{Column: "base_site_tag_id", Value: *dto.BaseSiteTagID})
		}
		if dto.SiteTagNameLike != nil {
			conditions = append(conditions, clause.Like{Column: "site_tag_name", Value: *dto.SiteTagNameLike})
		}
		if dto.DescriptionLike != nil {
			conditions = append(conditions, clause.Like{Column: "description", Value: *dto.DescriptionLike})
		}
	}
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryBoundOrUnboundToLocalTagPage(ctx, page, pageSize, where, orderBy, queryDTO.BoundOnLocalTagId, queryDTO.LocalTagID)
}

// QueryPageByWorkIdByDTO 根据作品ID分页查询站点标签（基于 QueryDTO）
func (s *Service) QueryPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryPageByWorkId(ctx, page, pageSize, where, orderBy, workId, boundOnWorkId)
}

// QueryLocalRelateDTOPageByDTO 查询站点标签与本地标签关联DTO分页（基于 QueryDTO）
func (s *Service) QueryLocalRelateDTOPageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, orderBy, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkIdByDTO 根据作品ID分页查询站点标签选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64) (*model.Page[domain.SelectItem], error) {
	conditions := buildConditionsFromDTO(&queryDTO)
	where := combineConditions(conditions)
	orderBy := queryDTO.BuildOrderBy()
	return s.repo.QuerySelectItemPageByWorkId(ctx, page, pageSize, where, orderBy, workId)
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
func buildConditionsFromDTO(dto *SiteTagQueryDTO) []clause.Expression {
	var conditions []clause.Expression

	if dto.ID != nil {
		conditions = append(conditions, clause.Eq{Column: "id", Value: *dto.ID})
	}
	if dto.SiteID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_id", Value: *dto.SiteID})
	}
	if dto.SiteTagID != nil {
		conditions = append(conditions, clause.Eq{Column: "site_tag_id", Value: *dto.SiteTagID})
	}
	if dto.BaseSiteTagID != nil {
		conditions = append(conditions, clause.Eq{Column: "base_site_tag_id", Value: *dto.BaseSiteTagID})
	}
	if dto.LocalTagID != nil {
		conditions = append(conditions, clause.Eq{Column: "local_tag_id", Value: *dto.LocalTagID})
	}
	if dto.SiteTagNameLike != nil {
		conditions = append(conditions, clause.Like{Column: "site_tag_name", Value: *dto.SiteTagNameLike})
	}
	if dto.DescriptionLike != nil {
		conditions = append(conditions, clause.Like{Column: "description", Value: *dto.DescriptionLike})
	}

	return conditions
}

// ListByWorkId 查询作品的站点标签
func (s *Service) ListByWorkId(ctx context.Context, workId int64) ([]*domain.SiteTag, error) {
	return s.repo.ListByWorkId(ctx, workId)
}

// ListBySiteTagIds 根据站点标签ID列表查询
func (s *Service) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*domain.SiteTag, error) {
	return s.repo.ListBySiteTagIds(ctx, siteTagIds)
}

// UpdateBindLocalTag 绑定或解除本地标签绑定
func (s *Service) UpdateBindLocalTag(ctx context.Context, localTagId int64, siteTagIds []int64) (bool, error) {
	if len(siteTagIds) == 0 {
		return true, nil
	}
	affected, err := s.repo.UpdateBindLocalTag(ctx, localTagId, siteTagIds)
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (s *Service) QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO], error) {
	return s.repo.QueryPageByWorkId(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO], error) {
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem], error) {
	return s.repo.QuerySelectItemPageByWorkId(ctx, page, pageSize, where, order, workId)
}

// CreateAndBindSameNameLocalTag 创建并绑定同名本地标签
// 查找或创建同名本地标签，并绑定到站点标签
func (s *Service) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *domain.SiteTag) (*domain.LocalTag, error) {
	// 1. 查找同名本地标签
	localTag, err := s.localTagOperator.GetByName(ctx, siteTag.SiteTagName.String)
	if err != nil {
		return nil, err
	}

	// 2. 如果不存在，则创建
	if localTag == nil {
		localTag = domain.NewLocalTag()
		localTag.LocalTagName = siteTag.SiteTagName
		localTag.BaseLocalTagID = sql.NullInt64{Int64: 0, Valid: true}
		localTag.LastUse = sql.NullInt64{Int64: util.GetCurrentTimestamp(), Valid: true}
		if err := s.localTagOperator.Save(ctx, localTag); err != nil {
			return nil, err
		}
	}

	// 3. 绑定站点标签到本地标签
	if _, err := s.repo.UpdateBindLocalTag(ctx, localTag.ID, []int64{siteTag.ID}); err != nil {
		return nil, err
	}

	return localTag, nil
}

// 错误定义
var (
	ErrTagIdRequired = &BusinessError{Code: 400, Message: "更新站点标签失败，id不能为空"}
)

// BusinessError 业务错误
type BusinessError struct {
	Code    int
	Message string
}

func (e *BusinessError) Error() string {
	return e.Message
}
