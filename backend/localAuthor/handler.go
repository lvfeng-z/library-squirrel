package localAuthor

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	"github.com/library-squirrel/backend/base/model/dto"
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

// GetById 根据ID获取（宿主侧展示 DTO：SDK 实体 DTO + 头像展示路径）
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalAuthorFullDTO] {
	result, err := h.svc.GetFullById(ctx, id)
	if err != nil {
		return model.HandleError[*dto.LocalAuthorFullDTO](err)
	}
	return model.Success(result)
}

// QueryPage 分页查询（宿主侧展示 DTO：SDK 实体 DTO + 头像展示路径）
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalAuthorFullDTO], query LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[dto.LocalAuthorFullDTO]] {
	if page == nil {
		page = &model.Page[dto.LocalAuthorFullDTO]{}
	}
	result, err := h.svc.QueryFullPage(ctx, page, query)
	if err != nil {
		return model.HandleError[*model.Page[dto.LocalAuthorFullDTO]](err)
	}
	return model.Success(result)
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[[]*dto.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	return model.HandleResult(h.svc.ListSelectItems(ctx, *queryDTO))
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem], query LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[dto.SelectItem]] {
	if page == nil {
		page = &model.Page[dto.SelectItem]{}
	}
	domainPage := &model.Page[dto.SelectItem]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	return model.HandleResult(h.svc.QuerySelectItemPage(ctx, domainPage, query))
}

// ListByWorkId 根据作品ID获取作者列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.RankedLocalAuthor] {
	return model.HandleResult(h.svc.ListByWorkId(ctx, workId))
}

// UpdateLastUse 更新最后使用时间
func (h *Handler) UpdateLastUse(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastUse(ctx, ids))
}
