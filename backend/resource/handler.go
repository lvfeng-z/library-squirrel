package resource

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	domain "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"
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
func (h *Handler) Save(ctx context.Context, resource *sdkdto.ResourceDTO) *model.ApiResponse[int64] {
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
		return model.HandleError[int64](err)
	}
	return model.Success(domainResource.GetID())
}

// Delete 删除资源
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新资源
func (h *Handler) Update(ctx context.Context, resource *sdkdto.ResourceDTO) *model.ApiResponse[any] {
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
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.ResourceDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.ResourceDTO](err)
	}
	return model.Success(dto2.NewResourceDTO(result))
}

// ListByWorkId 根据作品ID获取资源列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*sdkdto.ResourceDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.HandleError[[]*sdkdto.ResourceDTO](err)
	}
	resultDTOs := make([]*sdkdto.ResourceDTO, len(result))
	for i, resource := range result {
		resultDTOs[i] = dto2.NewResourceDTO(resource)
	}
	return model.Success(resultDTOs)
}

// DeleteByWorkId 根据作品ID删除资源
func (h *Handler) DeleteByWorkId(ctx context.Context, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteByWorkId(ctx, workId))
}

// ResourceDTO 资源数据传输对象（简化版，仅用于 Save/Update）
type ResourceDTO struct {
	ID       int64   `json:"id"`
	WorkID   int64   `json:"workId"`
	FilePath *string `json:"filePath"`
	FileName *string `json:"fileName"`
}
