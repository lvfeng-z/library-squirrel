package localAuthor

import (
	"context"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, author *LocalAuthorDTO) *model.ApiResponse[int64] {
	domainAuthor := &domain.LocalAuthor{
		BaseEntity: &model.BaseEntity{},
	}
	if author.AuthorName != nil {
		domainAuthor.AuthorName.Valid = true
		domainAuthor.AuthorName.String = *author.AuthorName
	}
	if author.Introduce != nil {
		domainAuthor.Introduce.Valid = true
		domainAuthor.Introduce.String = *author.Introduce
	}

	if err := h.svc.Save(ctx, domainAuthor); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainAuthor.GetID())
}

// Delete 删除作者
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新作者
func (h *Handler) Update(ctx context.Context, author *LocalAuthorDTO) *model.ApiResponse[any] {
	domainAuthor := &domain.LocalAuthor{
		BaseEntity: &model.BaseEntity{},
	}
	domainAuthor.SetID(author.ID)
	if author.AuthorName != nil {
		domainAuthor.AuthorName.Valid = true
		domainAuthor.AuthorName.String = *author.AuthorName
	}
	if author.Introduce != nil {
		domainAuthor.Introduce.Valid = true
		domainAuthor.Introduce.String = *author.Introduce
	}

	if err := h.svc.UpdateById(ctx, domainAuthor); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*domain.LocalAuthor] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*domain.LocalAuthor](err.Error())
	}
	return model.Success(author)
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[domain.LocalAuthor]] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.LocalAuthor]](err.Error())
	}
	return model.Success(result)
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[[]*domain.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.ListSelectItemsByDTO(ctx, *queryDTO)
	if err != nil {
		return model.Error[[]*domain.SelectItem](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page, pageSize int, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.QuerySelectItemPageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取作者列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*model.RankedLocalAuthor] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*model.RankedLocalAuthor](err.Error())
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

// LocalAuthorDTO 本地作者数据传输对象
type LocalAuthorDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
}
