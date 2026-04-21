package site

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 站点 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存站点
func (h *Handler) Save(ctx context.Context, site *SiteDTO) *model.ApiResponse[int64] {
	domainSite := &domain.Site{
		BaseEntity: &model.BaseEntity{},
	}
	if site.SiteName != nil {
		domainSite.SiteName.Valid = true
		domainSite.SiteName.String = *site.SiteName
	}
	if site.SiteDescription != nil {
		domainSite.SiteDescription.Valid = true
		domainSite.SiteDescription.String = *site.SiteDescription
	}
	if site.Homepage != nil {
		domainSite.Homepage.Valid = true
		domainSite.Homepage.String = *site.Homepage
	}

	if err := h.svc.Save(ctx, domainSite); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainSite.GetID())
}

// Delete 删除站点
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点
func (h *Handler) Update(ctx context.Context, site *SiteDTO) *model.ApiResponse[any] {
	domainSite := &domain.Site{
		BaseEntity: &model.BaseEntity{},
	}
	domainSite.SetID(site.ID)
	if site.SiteName != nil {
		domainSite.SiteName.Valid = true
		domainSite.SiteName.String = *site.SiteName
	}
	if site.SiteDescription != nil {
		domainSite.SiteDescription.Valid = true
		domainSite.SiteDescription.String = *site.SiteDescription
	}
	if site.Homepage != nil {
		domainSite.Homepage.Valid = true
		domainSite.Homepage.String = *site.Homepage
	}

	if err := h.svc.UpdateById(ctx, domainSite); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*SiteResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*SiteResultDTO](err.Error())
	}
	return model.Success(ToSiteResultDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[SiteResultDTO, SiteQueryDTO]) *model.ApiResponse[*model.Page[SiteResultDTO, SiteQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteResultDTO, SiteQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[SiteResultDTO, SiteQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteResultDTO, 0, len(result.Data))
	for _, site := range result.Data {
		data = append(data, ToSiteResultDTO(site))
	}
	return model.Success(&model.Page[SiteResultDTO, SiteQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[domain.SelectItem, SiteQueryDTO]) *model.ApiResponse[*model.Page[domain.SelectItem, SiteQueryDTO]] {
	if page == nil {
		page = &model.Page[domain.SelectItem, SiteQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPage(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem, SiteQueryDTO]](err.Error())
	}
	return model.Success(result)
}

// GetByName 根据名称获取
func (h *Handler) GetByName(ctx context.Context, siteName string) *model.ApiResponse[*SiteResultDTO] {
	result, err := h.svc.GetByName(ctx, siteName)
	if err != nil {
		return model.Error[*SiteResultDTO](err.Error())
	}
	return model.Success(ToSiteResultDTO(result))
}

// ========== DTO 定义 ==========

// SiteDTO 站点数据传输对象
type SiteDTO struct {
	ID              int64   `json:"id"`
	SiteName        *string `json:"siteName"`
	SiteDescription *string `json:"siteDescription"`
	Homepage        *string `json:"homepage"`
}

// SiteResultDTO 站点返回结果DTO（用于屏蔽sql.Null*类型）
type SiteResultDTO struct {
	ID              int64   `json:"id"`
	SiteName        *string `json:"siteName"`
	SiteDescription *string `json:"siteDescription"`
	Homepage        *string `json:"homepage"`
	CreateTime      int64   `json:"createTime"`
	UpdateTime      int64   `json:"updateTime"`
}

// ToSiteResultDTO 将 domain.Site 转换为 SiteResultDTO
func ToSiteResultDTO(site *domain.Site) *SiteResultDTO {
	if site == nil {
		return nil
	}
	return &SiteResultDTO{
		ID:              site.GetID(),
		SiteName:        nullStringToPointer(site.SiteName),
		SiteDescription: nullStringToPointer(site.SiteDescription),
		Homepage:        nullStringToPointer(site.Homepage),
		CreateTime:      site.GetCreateTime(),
		UpdateTime:      site.GetUpdateTime(),
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}
