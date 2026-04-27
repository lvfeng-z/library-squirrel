package siteAuthor

import (
	"context"

	"github.com/library-squirrel/wails/internal/util"
	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// Handler 站点作者 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点作者 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存站点作者
func (h *Handler) Save(ctx context.Context, author *dto.SiteAuthorDTO) *model.ApiResponse[int64] {
	domainAuthor := &entity2.SiteAuthor{
		BaseEntity: &model.BaseEntity{},
	}
	if author.SiteID != nil {
		domainAuthor.SiteID.Valid = true
		domainAuthor.SiteID.Int64 = *author.SiteID
	}
	if author.SiteAuthorID != nil {
		domainAuthor.SiteAuthorID.Valid = true
		domainAuthor.SiteAuthorID.String = *author.SiteAuthorID
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

// SaveBatch 批量保存站点作者
func (h *Handler) SaveBatch(ctx context.Context, authors []*dto.SiteAuthorDTO) *model.ApiResponse[any] {
	domainAuthors := make([]*entity2.SiteAuthor, 0, len(authors))
	for _, author := range authors {
		domainAuthor := &entity2.SiteAuthor{
			BaseEntity: &model.BaseEntity{},
		}
		if author.SiteID != nil {
			domainAuthor.SiteID.Valid = true
			domainAuthor.SiteID.Int64 = *author.SiteID
		}
		if author.SiteAuthorID != nil {
			domainAuthor.SiteAuthorID.Valid = true
			domainAuthor.SiteAuthorID.String = *author.SiteAuthorID
		}
		if author.AuthorName != nil {
			domainAuthor.AuthorName.Valid = true
			domainAuthor.AuthorName.String = *author.AuthorName
		}
		if author.Introduce != nil {
			domainAuthor.Introduce.Valid = true
			domainAuthor.Introduce.String = *author.Introduce
		}
		domainAuthors = append(domainAuthors, domainAuthor)
	}

	if err := h.svc.SaveBatch(ctx, domainAuthors); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Delete 删除站点作者
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点作者
func (h *Handler) Update(ctx context.Context, author *dto.SiteAuthorDTO) *model.ApiResponse[any] {
	domainAuthor := &entity2.SiteAuthor{
		BaseEntity: &model.BaseEntity{},
	}
	domainAuthor.SetID(author.ID)
	if author.SiteID != nil {
		domainAuthor.SiteID.Valid = true
		domainAuthor.SiteID.Int64 = *author.SiteID
	}
	if author.SiteAuthorID != nil {
		domainAuthor.SiteAuthorID.Valid = true
		domainAuthor.SiteAuthorID.String = *author.SiteAuthorID
	}
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.SiteAuthorDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto.SiteAuthorDTO](err.Error())
	}
	return model.Success(dto.NewSiteAuthorDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.SiteAuthorDTO], query SiteAuthorQueryDTO) *model.ApiResponse[*model.Page[dto.SiteAuthorDTO]] {
	if page == nil {
		page = &model.Page[dto.SiteAuthorDTO]{}
	}
	entityPage := &model.Page[entity2.SiteAuthor]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto.SiteAuthorDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.SiteAuthorDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, dto.NewSiteAuthorDTO(author))
	}
	return model.Success(&model.Page[dto.SiteAuthorDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (h *Handler) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[dto.SiteAuthorFullDTO], query SiteAuthorQueryDTO) *model.ApiResponse[*model.Page[dto.SiteAuthorFullDTO]] {
	if page == nil {
		page = &model.Page[dto.SiteAuthorFullDTO]{}
	}
	entityPage := &model.Page[dto.SiteAuthorFullDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryBoundOrUnboundToLocalAuthorPage(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto.SiteAuthorFullDTO]](err.Error())
	}
	return model.Success(result)
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[dto.SiteAuthorLocalRelateDTO], query SiteAuthorQueryDTO) *model.ApiResponse[*model.Page[dto.SiteAuthorLocalRelateDTO]] {
	if page == nil {
		page = &model.Page[dto.SiteAuthorLocalRelateDTO]{}
	}
	entityPage := &model.Page[dto.SiteAuthorLocalRelateDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryLocalRelateDTOPage(ctx, entityPage, query)
	if err != nil {
		return model.Error[*model.Page[dto.SiteAuthorLocalRelateDTO]](err.Error())
	}
	return model.Success(result)
}

// ListBySiteAuthorIds 根据站点作者ID列表获取
func (h *Handler) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) *model.ApiResponse[[]*dto.SiteAuthorDTO] {
	result, err := h.svc.ListBySiteAuthorIds(ctx, siteAuthorIds)
	if err != nil {
		return model.Error[[]*dto.SiteAuthorDTO](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.SiteAuthorDTO, 0, len(result))
	for _, author := range result {
		data = append(data, dto.NewSiteAuthorDTO(author))
	}
	return model.Success(data)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的站点作者
func (h *Handler) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.RankedSiteAuthorWithWorkIdDTO] {
	result, err := h.svc.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*dto.RankedSiteAuthorWithWorkIdDTO](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.RankedSiteAuthorWithWorkIdDTO, 0, len(result))
	for _, author := range result {
		data = append(data, &dto.RankedSiteAuthorWithWorkIdDTO{
			WorkId:       author.WorkId,
			SiteAuthorID: util.StringPtrIfValid(author.SiteAuthorID),
			AuthorName:   util.StringPtrIfValid(author.AuthorName),
			Rank:         author.AuthorRank,
		})
	}
	return model.Success(data)
}

// UpdateBindLocalAuthor 更新绑定本地作者
func (h *Handler) UpdateBindLocalAuthor(ctx context.Context, localAuthorId int64, siteAuthorIds []int64) *model.ApiResponse[bool] {
	result, err := h.svc.UpdateBindLocalAuthor(ctx, localAuthorId, siteAuthorIds)
	if err != nil {
		return model.Error[bool](err.Error())
	}
	return model.Success(result)
}

// CreateAndBindSameNameLocalAuthor 创建并绑定同名本地作者
func (h *Handler) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *dto.SiteAuthorDTO) *model.ApiResponse[bool] {
	if siteAuthor.ID == 0 {
		return model.Error[bool]("创建同名本地作者失败，作者ID不能为空")
	}
	if siteAuthor.AuthorName == nil || *siteAuthor.AuthorName == "" {
		return model.Error[bool]("创建同名本地作者失败，作者名称不能为空")
	}

	domainAuthor := &entity2.SiteAuthor{
		BaseEntity: &model.BaseEntity{},
	}
	domainAuthor.SetID(siteAuthor.ID)
	if siteAuthor.AuthorName != nil {
		domainAuthor.AuthorName.Valid = true
		domainAuthor.AuthorName.String = *siteAuthor.AuthorName
	}
	if siteAuthor.Introduce != nil {
		domainAuthor.Introduce.Valid = true
		domainAuthor.Introduce.String = *siteAuthor.Introduce
	}

	result, err := h.svc.CreateAndBindSameNameLocalAuthor(ctx, domainAuthor)
	if err != nil {
		return model.Error[bool](err.Error())
	}
	return model.Success(result)
}

// ListByWorkId 根据作品ID获取作者列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto.RankedSiteAuthor] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*dto.RankedSiteAuthor](err.Error())
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
