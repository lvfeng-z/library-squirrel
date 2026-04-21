package siteTag

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/internal/database"
	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/query"

	"gorm.io/gorm/clause"
)

// ========== 查询 DTO ==========

// SiteTagQueryDTO 站点标签查询条件
type SiteTagQueryDTO struct {
	ID                query.QueryAttribute `json:"-" query:"id"`                           // 站点标签ID（程序设置，不从JSON解析）
	SiteID            query.QueryAttribute `json:"siteId" query:"site_id"`                 // 站点ID
	SiteTagID         query.QueryAttribute `json:"siteTagId" query:"site_tag_id"`          // 站点标签ID（外部）
	BaseSiteTagID     query.QueryAttribute `json:"baseSiteTagId" query:"base_site_tag_id"` // 基础站点标签ID
	LocalTagID        query.QueryAttribute `json:"localTagId" query:"local_tag_id"`        // 本地标签ID
	BoundOnLocalTagId query.QueryAttribute `json:"boundOnLocalTagId" query:""`             // 是否绑定到指定本地标签（非数据库字段）
	SiteTagName       query.QueryAttribute `json:"siteTagName" query:"site_tag_name"`      // 站点标签名称（模糊匹配）
	Description       query.QueryAttribute `json:"description" query:"description"`        // 描述（模糊匹配）
	UpdateTime        query.QueryAttribute `json:"updateTime" query:"update_time"`         // 更新时间（可用于排序）
	CreateTime        query.QueryAttribute `json:"createTime" query:"create_time"`         // 创建时间（可用于排序）
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
	List(ctx context.Context, opt *database.QueryOption) ([]*domain.SiteTag, error)
	// Count 统计数量
	Count(ctx context.Context, opt *database.QueryOption) (int64, error)
	// Delete 删除
	Delete(ctx context.Context, id int64) error
	// Page 分页查询
	Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.SiteTag, any], error)
	// ListByWorkId 查询作品的站点标签
	ListByWorkId(ctx context.Context, workId int64) ([]*domain.SiteTag, error)
	// ListBySiteTagIds 根据站点标签ID列表查询
	ListBySiteTagIds(ctx context.Context, siteTagIds []int64) ([]*domain.SiteTag, error)
	// UpdateBindLocalTag 绑定本地标签
	UpdateBindLocalTag(ctx context.Context, localTagId int64, siteTagIds []int64) (int64, error)
	// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
	QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, boundOnLocalTagId *bool, localTagId *int64) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error)
	// QueryPageByWorkId 根据作品ID分页查询站点标签
	QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error)
	// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
	QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO, SiteTagQueryDTO], error)
	// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
	QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem, SiteTagQueryDTO], error)
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
func (s *Service) List(ctx context.Context, opt *database.QueryOption) ([]*domain.SiteTag, error) {
	return s.repo.List(ctx, opt)
}

// Count 统计数量
func (s *Service) Count(ctx context.Context, opt *database.QueryOption) (int64, error) {
	return s.repo.Count(ctx, opt)
}

// Delete 删除标签
func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

// Page 分页查询
func (s *Service) Page(ctx context.Context, opt *database.PageOption) (*model.Page[domain.SiteTag, any], error) {
	return s.repo.Page(ctx, opt)
}

// PageByDTO 分页查询（基于 QueryDTO）
func (s *Service) PageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO) (*model.Page[domain.SiteTag, any], error) {
	conv := query.NewConverter(domain.SiteTag{})
	opt, err := conv.ToPageOption(queryDTO, page, pageSize)
	if err != nil {
		return nil, err
	}
	return s.repo.Page(ctx, opt)
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页（基于 QueryDTO）
func (s *Service) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error) {
	conv := query.NewConverter(domain.SiteTag{})
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
	var boundOnLocalTagId *bool
	if queryDTO.BoundOnLocalTagId.Value != nil {
		if v, ok := queryDTO.BoundOnLocalTagId.Value.(bool); ok {
			boundOnLocalTagId = &v
		}
	}
	var localTagId *int64
	if queryDTO.LocalTagID.Value != nil {
		if v, ok := queryDTO.LocalTagID.Value.(int64); ok {
			localTagId = &v
		}
	}
	return s.repo.QueryBoundOrUnboundToLocalTagPage(ctx, page, pageSize, where, order, boundOnLocalTagId, localTagId)
}

// QueryPageByWorkIdByDTO 根据作品ID分页查询站点标签（基于 QueryDTO）
func (s *Service) QueryPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error) {
	conv := query.NewConverter(domain.SiteTag{})
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
	return s.repo.QueryPageByWorkId(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QueryLocalRelateDTOPageByDTO 查询站点标签与本地标签关联DTO分页（基于 QueryDTO）
func (s *Service) QueryLocalRelateDTOPageByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO, SiteTagQueryDTO], error) {
	conv := query.NewConverter(domain.SiteTag{})
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
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkIdByDTO 根据作品ID分页查询站点标签选择项（基于 QueryDTO）
func (s *Service) QuerySelectItemPageByWorkIdByDTO(ctx context.Context, page, pageSize int, queryDTO SiteTagQueryDTO, workId int64) (*model.Page[domain.SelectItem, SiteTagQueryDTO], error) {
	conv := query.NewConverter(domain.SiteTag{})
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
	return s.repo.QuerySelectItemPageByWorkId(ctx, page, pageSize, where, order, workId)
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
func (s *Service) QueryPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO], error) {
	return s.repo.QueryPageByWorkId(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (s *Service) QueryLocalRelateDTOPage(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64, boundOnWorkId *bool) (*model.Page[domain.SiteTagLocalRelateDTO, SiteTagQueryDTO], error) {
	return s.repo.QueryLocalRelateDTOPage(ctx, page, pageSize, where, order, workId, boundOnWorkId)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询站点标签选择项
func (s *Service) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, where clause.Expression, order clause.Expression, workId int64) (*model.Page[domain.SelectItem, SiteTagQueryDTO], error) {
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
