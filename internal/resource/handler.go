package resource

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 资源 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建资源 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存资源
func (h *Handler) Save(ctx context.Context, resource *ResourceDTO) *model.ApiResponse[int64] {
	domainResource := &domain.Resource{
		BaseEntity: &model.BaseEntity{},
		WorkID:     resource.WorkID,
	}
	if resource.FilePath != nil {
		domainResource.FilePath.Valid = true
		domainResource.FilePath.String = *resource.FilePath
	}
	if resource.FileName != nil {
		domainResource.FileName.Valid = true
		domainResource.FileName.String = *resource.FileName
	}

	if err := h.svc.Save(ctx, domainResource); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainResource.GetID())
}

// Delete 删除资源
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新资源
func (h *Handler) Update(ctx context.Context, resource *ResourceDTO) *model.ApiResponse[any] {
	domainResource := &domain.Resource{
		BaseEntity: &model.BaseEntity{},
	}
	domainResource.SetID(resource.ID)
	domainResource.WorkID = resource.WorkID
	if resource.FilePath != nil {
		domainResource.FilePath.Valid = true
		domainResource.FilePath.String = *resource.FilePath
	}
	if resource.FileName != nil {
		domainResource.FileName.Valid = true
		domainResource.FileName.String = *resource.FileName
	}

	if err := h.svc.Update(ctx, domainResource); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.Resource] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.Resource](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取资源列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*domain.Resource] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*domain.Resource](err.Error())
	}
	return model.Success(result)
}

// DeleteByWorkId 根据作品ID删除资源
func (h *Handler) DeleteByWorkId(ctx context.Context, workId int64) *model.ApiResponse[any] {
	if err := h.svc.DeleteByWorkId(ctx, workId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== DTO 定义 ==========

// ResourceDTO 资源数据传输对象
type ResourceDTO struct {
	ID       int64   `json:"id"`
	WorkID   int64   `json:"workId"`
	FilePath *string `json:"filePath"`
	FileName *string `json:"fileName"`
}
