package localAuthor

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, author *dto.LocalAuthorDTO) *model.ApiResponse[int64] {
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
func (h *Handler) Update(ctx context.Context, author *dto.LocalAuthorDTO) *model.ApiResponse[any] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.LocalAuthorDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto.LocalAuthorDTO](err.Error())
	}
	return model.Success(dto.NewLocalAuthorDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]](err.Error())
	}
	// 转换为 tDTO
	data := make([]*dto.LocalAuthorDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, dto.NewLocalAuthorDTO(author))
	}
	return model.Success(&model.Page[dto.LocalAuthorDTO, LocalAuthorQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[[]*dto.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.ListSelectItemsByDTO(ctx, *queryDTO)
	if err != nil {
		return model.Error[[]*dto.SelectItem](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page *model.Page[dto.SelectItem, LocalAuthorQueryDTO]) *model.ApiResponse[*model.Page[dto.SelectItem, LocalAuthorQueryDTO]] {
	if page == nil {
		page = &model.Page[dto.SelectItem, LocalAuthorQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[dto.SelectItem, LocalAuthorQueryDTO]](err.Error())
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
