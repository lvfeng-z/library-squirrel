package siteTag

import (
	"context"

	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	"github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, tag *dto.SiteTagDTO) *model.ApiResponse[int64] {
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

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTag.GetID())
}

// SaveBatch 批量保存站点标签
func (h *Handler) SaveBatch(ctx context.Context, tags []*dto.SiteTagDTO) *model.ApiResponse[any] {
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
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Delete 删除站点标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点标签
func (h *Handler) Update(ctx context.Context, tag *dto.SiteTagDTO) *model.ApiResponse[any] {
	domainTag := &entity.SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
	domainTag.SetID(tag.ID)
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

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.SiteTagDTO] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto.SiteTagDTO](err.Error())
	}
	return model.Success(dto.NewSiteTagDTO(tag))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.SiteTagDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteTagDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SiteTagDTO, SiteTagQueryDTO]{}
	}
	entityPage := &model.Page[entity.SiteTag, any]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
		Query:      page.Query,
	}
	result, err := h.svc.Page(ctx, entityPage)
	if err != nil {
		return model.Error[*model.Page[dto.SiteTagDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.SiteTagDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, dto.NewSiteTagDTO(tag))
	}
	return model.Success(&model.Page[dto.SiteTagDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (h *Handler) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, pageQuery model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]] {
	result, err := h.svc.QueryBoundOrUnboundToLocalTagPage(ctx, &pageQuery)
	if err != nil {
		return model.Error[*model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]](err.Error())
	}
	return model.Success(result)
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]{}
	}
	dtoPage := &model.Page[dto.SiteTagLocalRelateDTO, SiteTagQueryDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
		Query:      page.Query,
	}
	result, err := h.svc.QueryLocalRelateDTOPage(ctx, dtoPage, 0, nil)
	if err != nil {
		return model.Error[*model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagLocalRelateDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, ToSiteTagLocalRelateDTO(tag))
	}
	return model.Success(&model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (h *Handler) QueryPageByWorkId(ctx context.Context, page *model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO], workId int64) *model.ApiResponse[*model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]{}
	}
	result, err := h.svc.QueryPageByWorkId(ctx, page, workId, nil)
	if err != nil {
		return model.Error[*model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*dto.SiteTagFullDTO, 0, len(result.Data))
	return model.Success(&model.Page[dto.SiteTagFullDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// ListBySiteTagIds 根据站点标签ID列表获取
func (h *Handler) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) *model.ApiResponse[[]*dto.SiteTagDTO] {
	result, err := h.svc.ListBySiteTagIds(ctx, siteTagIds)
	if err != nil {
		return model.Error[[]*dto.SiteTagDTO](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.SiteTagDTO, 0, len(result))
	for _, tag := range result {
		data = append(data, dto.NewSiteTagDTO(tag))
	}
	return model.Success(data)
}

// UpdateBindLocalTag 更新绑定本地标签
func (h *Handler) UpdateBindLocalTag(ctx context.Context, localTagId *int64, siteTagIds []int64) *model.ApiResponse[bool] {
	result, err := h.svc.UpdateBindLocalTag(ctx, localTagId, siteTagIds)
	if err != nil {
		return model.Error[bool](err.Error())
	}
	return model.Success(result)
}

// CreateAndBindSameNameLocalTag 创建并绑定同名本地标签
func (h *Handler) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *dto.SiteTagDTO) *model.ApiResponse[*dto.LocalTagDTO] {
	if siteTag.ID == 0 {
		return model.Error[*dto.LocalTagDTO]("创建同名本地标签失败，标签ID不能为空")
	}
	if siteTag.SiteTagName == nil || *siteTag.SiteTagName == "" {
		return model.Error[*dto.LocalTagDTO]("创建同名本地标签失败，标签名称不能为空")
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
		return model.Error[*dto.LocalTagDTO](err.Error())
	}
	return model.Success(ToLocalTagDTO(result))
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.SiteTagDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*dto.SiteTagDTO](err.Error())
	}
	// 转换为 DTO
	resultDTOs := make([]*dto.SiteTagDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = dto.NewSiteTagDTO(tag)
	}
	return model.Success(resultDTOs)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[dto.SelectItem, SiteTagQueryDTO], workId int64) *model.ApiResponse[*model.Page[dto.SelectItem, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SelectItem, SiteTagQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPageByWorkId(ctx, page, workId)
	if err != nil {
		return model.Error[*model.Page[dto.SelectItem, SiteTagQueryDTO]](err.Error())
	}
	return model.Success(result)
}

// UpdateLastUse 更新最后使用时间
func (h *Handler) UpdateLastUse(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	if err := h.svc.UpdateLastUse(ctx, ids); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== DTO 定义 ==========

// SiteTagLocalRelateDTO 站点标签与本地标签关联DTO（Handler内部转换用）
type SiteTagLocalRelateDTO struct {
	dto.SiteTagDTO
	LocalTag *dto.LocalTagDTO `json:"localTag,omitempty"`
}

// ToSiteTagLocalRelateDTO 将 dto.SiteTagLocalRelateDTO 转换为 SiteTagLocalRelateDTO
func ToSiteTagLocalRelateDTO(fullDTO *dto.SiteTagLocalRelateDTO) *SiteTagLocalRelateDTO {
	if fullDTO == nil {
		return nil
	}
	return &SiteTagLocalRelateDTO{
		SiteTagDTO: dto.SiteTagDTO{
			ID:            fullDTO.ID,
			SiteID:        util.Int64PtrIfValid(fullDTO.SiteID),
			SiteTagID:     util.StringPtrIfValid(fullDTO.SiteTagID),
			SiteTagName:   util.StringPtrIfValid(fullDTO.SiteTagName),
			BaseSiteTagID: util.StringPtrIfValid(fullDTO.BaseSiteTagID),
			Description:   util.StringPtrIfValid(fullDTO.Description),
			LocalTagID:    util.Int64PtrIfValid(fullDTO.LocalTagID),
			LastUse:       util.Int64PtrIfValid(fullDTO.LastUse),
			CreateTime:    fullDTO.CreateTime,
			UpdateTime:    fullDTO.UpdateTime,
		},
		LocalTag: fullDTO.LocalTag,
	}
}

// ToLocalTagDTO 将 entity.LocalTag 转换为 dto.LocalTagDTO
func ToLocalTagDTO(tag *entity.LocalTag) *dto.LocalTagDTO {
	return dto.NewLocalTagDTO(tag)
}
