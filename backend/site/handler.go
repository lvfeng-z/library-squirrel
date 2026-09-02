package site

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// Handler 站点 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增改操作 ==========

// Update 更新站点
func (h *Handler) Update(ctx context.Context, site *sdkdto.SiteDTO) *model.ApiResponse[any] {
	domainSite := dto.ToSiteEntity(site)

	if err := h.svc.UpdateById(ctx, domainSite); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.SiteDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.SiteDTO](err)
	}
	return model.Success(dto.NewSiteDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[sdkdto.SiteDTO], query SiteQueryDTO) *model.ApiResponse[*model.Page[sdkdto.SiteDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.SiteDTO]{}
	}
	entityPage := &model.Page[entity.Site]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.SiteDTO]](err)
	}
	// 转换为 ResultDTO
	data := make([]*sdkdto.SiteDTO, 0, len(result.Data))
	for _, site := range result.Data {
		data = append(data, dto.NewSiteDTO(site))
	}
	return model.Success(&model.Page[sdkdto.SiteDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem], query SiteQueryDTO) *model.ApiResponse[*model.Page[dto.SelectItem]] {
	if page == nil {
		page = &model.Page[dto.SelectItem]{}
	}
	return model.HandleResult(h.svc.QuerySelectItemPage(ctx, page, query))
}
