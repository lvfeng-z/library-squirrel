package localAuthor

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
	"github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// Handler 本地作者 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建本地作者 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存作者
func (h *Handler) Save(ctx context.Context, author *sdkdto.LocalAuthorDTO) *model.ApiResponse[int64] {
	domainAuthor := dto.ToLocalAuthorEntity(author)

	if err := h.svc.Save(ctx, domainAuthor); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainAuthor.GetID())
}

// Delete 删除作者
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新作者
func (h *Handler) Update(ctx context.Context, author *sdkdto.LocalAuthorDTO) *model.ApiResponse[any] {
	domainAuthor := dto.ToLocalAuthorEntity(author)

	if err := h.svc.UpdateById(ctx, domainAuthor); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*sdkdto.LocalAuthorDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*sdkdto.LocalAuthorDTO](err)
	}
	return model.Success(dto.NewLocalAuthorDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[sdkdto.LocalAuthorDTO], query LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[sdkdto.LocalAuthorDTO]] {
	if page == nil {
		page = &model.Page[sdkdto.LocalAuthorDTO]{}
	}
	domainPage := &model.Page[entity.LocalAuthor]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, domainPage, query)
	if err != nil {
		return model.HandleError[*model.Page[sdkdto.LocalAuthorDTO]](err)
	}
	// 转换为 DTO
	data := make([]*sdkdto.LocalAuthorDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, dto.NewLocalAuthorDTO(author))
	}
	return model.Success(&model.Page[sdkdto.LocalAuthorDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[[]*sdkdto.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	return model.HandleResult(h.svc.ListSelectItems(ctx, *queryDTO))
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[sdkdto.SelectItem], query LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[sdkdto.SelectItem]] {
	if page == nil {
		page = &model.Page[sdkdto.SelectItem]{}
	}
	domainPage := &model.Page[sdkdto.SelectItem]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	return model.HandleResult(h.svc.QuerySelectItemPage(ctx, domainPage, query))
}

// ListByWorkId 根据作品ID获取作者列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*sdkdto.RankedLocalAuthor] {
	return model.HandleResult(h.svc.ListByWorkId(ctx, workId))
}

// UpdateLastUse 更新最后使用时间
func (h *Handler) UpdateLastUse(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUse(ctx, ids))
}
