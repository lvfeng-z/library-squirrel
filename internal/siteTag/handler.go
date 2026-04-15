package siteTag

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, tag *SiteTagDTO) *model.ApiResponse[int64] {
	domainTag := &domain.SiteTag{
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

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTag.GetID())
}

// Delete 删除站点标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点标签
func (h *Handler) Update(ctx context.Context, tag *SiteTagDTO) *model.ApiResponse[any] {
	domainTag := &domain.SiteTag{
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

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.SiteTag] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.SiteTag](err.Error())
	}
	return model.Success(tag)
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *SiteTagQueryDTO) *model.ApiResponse[*model.Page[domain.SiteTag]] {
	if queryDTO == nil {
		queryDTO = &SiteTagQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.SiteTag]](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*domain.SiteTag] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*domain.SiteTag](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, queryDTO *SiteTagQueryDTO, workId int64) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	if queryDTO == nil {
		queryDTO = &SiteTagQueryDTO{}
	}
	result, err := h.svc.QuerySelectItemPageByWorkIdByDTO(ctx, page, pageSize, *queryDTO, workId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
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

// SiteTagDTO 站点标签数据传输对象
type SiteTagDTO struct {
	ID         int64   `json:"id"`
	SiteID     *int64  `json:"siteId"`
	SiteTagID  *string `json:"siteTagId"`
	SiteTagName *string `json:"siteTagName"`
}
