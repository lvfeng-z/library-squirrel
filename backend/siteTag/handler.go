package siteTag

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// Handler 站点标签 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点标签 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存站点标签
func (h *Handler) Save(ctx context.Context, tag *sdkdto.SiteTagDTO) *model.ApiResponse[int64] {
	domainTag := dto.ToSiteTagEntity(tag)

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainTag.GetID())
}

// SaveBatch 批量保存站点标签
func (h *Handler) SaveBatch(ctx context.Context, tags []*sdkdto.SiteTagDTO) *model.ApiResponse[any] {
	domainTags := make([]*entity.SiteTag, 0, len(tags))
	for _, tag := range tags {
		domainTag := &entity.SiteTag{
			BaseEntity: &model.BaseEntity{},
		}
		if tag.SiteID != nil {
			domainTag.SiteID.Valid = true
			domainTag.SiteID.Int64 = *tag.SiteID
		}
		if tag.SiteTagID != nil {
			domainTag.SiteTagID.Valid = true
			domainTag.SiteTagID.String = *tag.SiteTagID
		}
		if tag.SiteTagName != nil {
			domainTag.SiteTagName.Valid = true
			domainTag.SiteTagName.String = *tag.SiteTagName
		}
		if tag.Description != nil {
			domainTag.Description.Valid = true
			domainTag.Description.String = *tag.Description
		}
		domainTags = append(domainTags, domainTag)
	}

	if err := h.svc.SaveBatch(ctx, domainTags); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// Delete 删除站点标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新站点标签
func (h *Handler) Update(ctx context.Context, tag *sdkdto.SiteTagDTO) *model.ApiResponse[any] {
	domainTag := dto.ToSiteTagEntity(tag)

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.SiteTagDTO] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.SiteTagDTO](err)
	}
	return model.Success(dto.NewSiteTagDTO(tag))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[sdkdto.SiteTagDTO], query SiteTagQueryDTO) *model.ApiResponse[*model.Page[sdkdto.SiteTagDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.SiteTagDTO]{}
	}
	entityPage := &model.Page[entity.SiteTag]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.SiteTagDTO]](err)
	}
	// 转换为 DTO
	data := make([]*sdkdto.SiteTagDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, dto.NewSiteTagDTO(tag))
	}
	return model.Success(&model.Page[sdkdto.SiteTagDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (h *Handler) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, page *model.Page[sdkdto.SiteTagFullDTO], query SiteTagQueryDTO) *model.ApiResponse[*model.Page[sdkdto.SiteTagFullDTO]] {
	return model.HandleResult(h.svc.QueryBoundOrUnboundToLocalTagPage(ctx, page, query))
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[sdkdto.SiteTagLocalRelateDTO], query SiteTagQueryDTO) *model.ApiResponse[*model.Page[sdkdto.SiteTagLocalRelateDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.SiteTagLocalRelateDTO]{}
	}
	return model.HandleResult(h.svc.QueryLocalRelateDTOPage(ctx, page, query, 0, nil))
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (h *Handler) QueryPageByWorkId(ctx context.Context, page *model.Page[sdkdto.SiteTagFullDTO], query SiteTagQueryDTO, workId int64) *model.ApiResponse[*model.Page[sdkdto.SiteTagFullDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.SiteTagFullDTO]{}
	}
	result, err := h.svc.QueryPageByWorkId(ctx, page, query, workId, nil)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.SiteTagFullDTO]](err)
	}
	// 转换为 ResultDTO
	data := make([]*sdkdto.SiteTagFullDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, tag)
	}
	return model.Success(&model.Page[sdkdto.SiteTagFullDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// ListBySiteTagIds 根据站点标签ID列表获取
func (h *Handler) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) *model.ApiResponse[[]*sdkdto.SiteTagDTO] {
	result, err := h.svc.ListBySiteTagIds(ctx, siteTagIds)
	if err != nil {
		return model.HandleError[[]*sdkdto.SiteTagDTO](err)
	}
	// 转换为 DTO
	data := make([]*sdkdto.SiteTagDTO, 0, len(result))
	for _, tag := range result {
		data = append(data, dto.NewSiteTagDTO(tag))
	}
	return model.Success(data)
}

// UpdateBindLocalTag 更新绑定本地标签
func (h *Handler) UpdateBindLocalTag(ctx context.Context, localTagId *int64, siteTagIds []int64) *model.ApiResponse[bool] {
	return model.HandleResult(h.svc.UpdateBindLocalTag(ctx, localTagId, siteTagIds))
}

// CreateAndBindSameNameLocalTag 创建并绑定同名本地标签
func (h *Handler) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *sdkdto.SiteTagDTO) *model.ApiResponse[*sdkdto.LocalTagDTO] {
	if siteTag.ID == 0 {
		return model.Error[*sdkdto.LocalTagDTO]("创建同名本地标签失败，标签ID不能为空")
	}
	if siteTag.SiteTagName == nil || *siteTag.SiteTagName == "" {
		return model.Error[*sdkdto.LocalTagDTO]("创建同名本地标签失败，标签名称不能为空")
	}

	domainTag := &entity.SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
	domainTag.SetID(siteTag.ID)
	if siteTag.SiteTagName != nil {
		domainTag.SiteTagName.Valid = true
		domainTag.SiteTagName.String = *siteTag.SiteTagName
	}
	if siteTag.Description != nil {
		domainTag.Description.Valid = true
		domainTag.Description.String = *siteTag.Description
	}

	result, err := h.svc.CreateAndBindSameNameLocalTag(ctx, domainTag)
	if err != nil {
		return model.HandleError[*sdkdto.LocalTagDTO](err)
	}
	return model.Success(ToLocalTagDTO(result))
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*sdkdto.SiteTagDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.HandleError[[]*sdkdto.SiteTagDTO](err)
	}
	// 转换为 DTO
	resultDTOs := make([]*sdkdto.SiteTagDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = dto.NewSiteTagDTO(tag)
	}
	return model.Success(resultDTOs)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[sdkdto.SelectItem], query SiteTagQueryDTO, workId int64, boundOnWorkId *bool) *model.ApiResponse[*model.Page[sdkdto.SelectItem]] {
	if page == nil {
		page = &model.Page[sdkdto.SelectItem]{}
	}
	return model.HandleResult(h.svc.QuerySelectItemPageByWorkId(ctx, page, query, workId, boundOnWorkId))
}

// UpdateLastUse 更新最后使用时间
func (h *Handler) UpdateLastUse(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUse(ctx, ids))
}

// ========== DTO 定义 ==========

// ToLocalTagDTO 将 entity.LocalTag 转换为 sdkdto.LocalTagDTO
func ToLocalTagDTO(tag *entity.LocalTag) *sdkdto.LocalTagDTO {
	return dto.NewLocalTagDTO(tag)
}
