package localTag

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, tag *dto.LocalTagDTO) *model.ApiResponse[int64] {
	domainTag := dto.ToLocalTagEntity(tag)

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainTag.GetID())
}

// Delete 删除本地标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新本地标签
func (h *Handler) Update(ctx context.Context, tag *dto.LocalTagDTO) *model.ApiResponse[any] {
	domainTag := dto.ToLocalTagEntity(tag)

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalTagDTO] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*dto.LocalTagDTO](err)
	}
	return model.Success(dto.NewLocalTagDTO(tag))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalTagDTO], query LocalTagQueryDTO) *model.ApiResponse[*model.Page[dto.LocalTagDTO]] {
	if page == nil {
		page = &model.Page[dto.LocalTagDTO]{}
	}
	domainPage := &model.Page[domain.LocalTag]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, domainPage, query)
	if err != nil {
		return model.HandleError[*model.Page[dto.LocalTagDTO]](err)
	}
	// 转换为 DTO
	data := make([]*dto.LocalTagDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, dto.NewLocalTagDTO(tag))
	}
	return model.Success(&model.Page[dto.LocalTagDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// GetTree 获取标签树形结构
func (h *Handler) GetTree(ctx context.Context, rootId int64, depth int) *model.ApiResponse[[]*dto.LocalTagDTO] {
	result, err := h.svc.GetTree(ctx, rootId, depth)
	if err != nil {
		return model.HandleError[[]*dto.LocalTagDTO](err)
	}
	// 转换为 DTO
	resultDTOs := make([]*dto.LocalTagDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = dto.NewLocalTagDTO(tag)
	}
	return model.Success(resultDTOs)
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalTagQueryDTO) *model.ApiResponse[[]*dto.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalTagQueryDTO{}
	}
	return model.HandleResult(h.svc.ListSelectItems(ctx, *queryDTO))
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, reqPage *model.Page[dto.SelectItem], query LocalTagQueryDTO, secondaryLabel string) *model.ApiResponse[*model.Page[dto.SelectItem]] {
	if reqPage == nil {
		reqPage = &model.Page[dto.SelectItem]{}
	}
	domainPage := &model.Page[dto.SelectItem]{
		PageNumber: reqPage.PageNumber,
		PageSize:   reqPage.PageSize,
	}
	return model.HandleResult(h.svc.QuerySelectItemPage(ctx, domainPage, query, secondaryLabel))
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.LocalTagDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.HandleError[[]*dto.LocalTagDTO](err)
	}
	// 转换为 DTO
	resultDTOs := make([]*dto.LocalTagDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = dto.NewLocalTagDTO(tag)
	}
	return model.Success(resultDTOs)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[dto.SelectItem], query LocalTagQueryDTO) *model.ApiResponse[*model.Page[dto.SelectItem]] {
	if page == nil {
		page = &model.Page[dto.SelectItem]{}
	}
	domainPage := &model.Page[dto.SelectItem]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	return model.HandleResult(h.svc.QuerySelectItemPageByWorkId(ctx, domainPage, query))
}

// UpdateLastUse 更新最后使用时间
func (h *Handler) UpdateLastUse(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUse(ctx, ids))
}

// QueryWithBaseTagPage 分页查询包含基础标签信息的本地标签
func (h *Handler) QueryWithBaseTagPage(ctx context.Context, page *model.Page[dto.LocalTagWithBaseTagDTO], query LocalTagQueryDTO) *model.ApiResponse[*model.Page[dto.LocalTagWithBaseTagDTO]] {
	if page == nil {
		page = &model.Page[dto.LocalTagWithBaseTagDTO]{}
	}
	domainPage := &model.Page[dto.LocalTagWithBaseTagDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	return model.HandleResult(h.svc.QueryWithBaseTagPage(ctx, domainPage, query))
}
