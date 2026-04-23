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
func (h *Handler) Save(ctx context.Context, tag *dto.LocalTagParamDTO) *model.ApiResponse[int64] {
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
func (h *Handler) Update(ctx context.Context, tag *dto.LocalTagParamDTO) *model.ApiResponse[any] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalTagDTO] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto.LocalTagDTO](err.Error())
	}
	return model.Success(dto.NewLocalTagDTO(tag))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalTagDTO, LocalTagQueryDTO]) *model.ApiResponse[*model.Page[dto.LocalTagDTO, LocalTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.LocalTagDTO, LocalTagQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[dto.LocalTagDTO, LocalTagQueryDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.LocalTagDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, dto.NewLocalTagDTO(tag))
	}
	return model.Success(&model.Page[dto.LocalTagDTO, LocalTagQueryDTO]{
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
		return model.Error[[]*dto.LocalTagDTO](err.Error())
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
	result, err := h.svc.ListSelectItemsByDTO(ctx, *queryDTO)
	if err != nil {
		return model.Error[[]*dto.SelectItem](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem, LocalTagQueryDTO], secondaryLabel string) *model.ApiResponse[*model.Page[dto.SelectItem, LocalTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SelectItem, LocalTagQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPageByDTO(ctx, page.PageNumber, page.PageSize, page.Query, secondaryLabel)
	if err != nil {
		return model.Error[*model.Page[dto.SelectItem, LocalTagQueryDTO]](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.LocalTagDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*dto.LocalTagDTO](err.Error())
	}
	// 转换为 DTO
	resultDTOs := make([]*dto.LocalTagDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = dto.NewLocalTagDTO(tag)
	}
	return model.Success(resultDTOs)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[dto.SelectItem, LocalTagQueryDTO], workId int64) *model.ApiResponse[*model.Page[dto.SelectItem, LocalTagQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SelectItem, LocalTagQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPageByWorkIdByDTO(ctx, page.PageNumber, page.PageSize, page.Query, workId)
	if err != nil {
		return model.Error[*model.Page[dto.SelectItem, LocalTagQueryDTO]](err.Error())
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
