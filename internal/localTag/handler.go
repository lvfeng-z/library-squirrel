package localTag

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 本地标签 Handler
// 用于 Wails Bind[] 参数，暴露给前端调用
type Handler struct {
	svc *Service
}

// NewHandler 创建本地标签 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存本地标签
func (h *Handler) Save(ctx context.Context, tag *LocalTagDTO) *model.ApiResponse[int64] {
	domainTag := &domain.LocalTag{
		BaseEntity: &model.BaseEntity{},
	}
	if tag.LocalTagName != nil {
		domainTag.LocalTagName.Valid = true
		domainTag.LocalTagName.String = *tag.LocalTagName
	}
	if tag.BaseLocalTagID != nil {
		domainTag.BaseLocalTagID.Valid = true
		domainTag.BaseLocalTagID.Int64 = *tag.BaseLocalTagID
	}

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTag.GetID())
}

// Delete 删除本地标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新本地标签
func (h *Handler) Update(ctx context.Context, tag *LocalTagDTO) *model.ApiResponse[any] {
	domainTag := &domain.LocalTag{
		BaseEntity: &model.BaseEntity{},
	}
	domainTag.SetID(tag.ID)
	if tag.LocalTagName != nil {
		domainTag.LocalTagName.Valid = true
		domainTag.LocalTagName.String = *tag.LocalTagName
	}
	if tag.BaseLocalTagID != nil {
		domainTag.BaseLocalTagID.Valid = true
		domainTag.BaseLocalTagID.Int64 = *tag.BaseLocalTagID
	}

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.LocalTag] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.LocalTag](err.Error())
	}
	return model.Success(tag)
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *LocalTagQueryDTO) *model.ApiResponse[*model.Page[domain.LocalTag]] {
	if queryDTO == nil {
		queryDTO = &LocalTagQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.LocalTag]](err.Error())
	}
	return model.Success(result)
}

// GetTree 获取标签树形结构
func (h *Handler) GetTree(ctx context.Context, rootId int64, depth int) *model.ApiResponse[[]*domain.LocalTag] {
	result, err := h.svc.GetTree(ctx, rootId, depth)
	if err != nil {
		return model.Error[[]*domain.LocalTag](err.Error())
	}
	return model.Success(result)
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalTagQueryDTO) *model.ApiResponse[[]*domain.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalTagQueryDTO{}
	}
	result, err := h.svc.ListSelectItemsByDTO(ctx, *queryDTO)
	if err != nil {
		return model.Error[[]*domain.SelectItem](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page, pageSize int, queryDTO *LocalTagQueryDTO, secondaryLabel string) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	if queryDTO == nil {
		queryDTO = &LocalTagQueryDTO{}
	}
	result, err := h.svc.QuerySelectItemPageByDTO(ctx, page, pageSize, *queryDTO, secondaryLabel)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*domain.LocalTag] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*domain.LocalTag](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page, pageSize int, queryDTO *LocalTagQueryDTO, workId int64) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	if queryDTO == nil {
		queryDTO = &LocalTagQueryDTO{}
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

// LocalTagDTO 本地标签数据传输对象
type LocalTagDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
}
