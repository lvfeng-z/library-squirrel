package work

import (
	"context"

	"github.com/library-squirrel/wails/backend/base/model"
	"github.com/library-squirrel/wails/backend/base/model/dto"
	"github.com/library-squirrel/wails/backend/base/model/entity"
)

// Handler 作品 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存作品
func (h *Handler) Save(ctx context.Context, work *dto.WorkDTO) *model.ApiResponse[int64] {
	domainWork := dto.ToWorkEntity(work)

	if err := h.svc.Save(ctx, domainWork); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainWork.GetID())
}

// Delete 删除作品
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新作品
func (h *Handler) Update(ctx context.Context, work *dto.WorkDTO) *model.ApiResponse[any] {
	domainWork := dto.ToWorkEntity(work)

	if err := h.svc.UpdateById(ctx, domainWork); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// DeleteWorkAndSurroundingData 删除作品及周边数据
func (h *Handler) DeleteWorkAndSurroundingData(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.DeleteWorkAndSurroundingData(ctx, id))
}

// ========== 查询操作 ==========

// GetById 根据ID获取作品
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.WorkDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*dto.WorkDTO](err)
	}
	return model.Success(dto.NewWorkDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto.WorkDTO], query WorkQueryDTO) *model.ApiResponse[*model.Page[dto.WorkDTO]] {
	if page == nil {
		page = &model.Page[dto.WorkDTO]{}
	}
	entityPage := &model.Page[entity.Work]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[dto.WorkDTO]](err)
	}
	// 转换为 DTO
	data := make([]*dto.WorkDTO, 0, len(result.Data))
	for _, work := range result.Data {
		data = append(data, dto.NewWorkDTO(work))
	}
	return model.Success(&model.Page[dto.WorkDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// GetFullWorkInfoById 获取完整作品信息
func (h *Handler) GetFullWorkInfoById(ctx context.Context, id int64) *model.ApiResponse[*dto.WorkFullDTO] {
	return model.HandleResult(h.svc.GetFullWorkInfoById(ctx, id))
}

// GetBySiteAndSiteWorkID 根据站点ID和站点作品ID获取作品
func (h *Handler) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) *model.ApiResponse[*dto.WorkDTO] {
	result, err := h.svc.GetBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
	if err != nil {
		return model.HandleError[*dto.WorkDTO](err)
	}
	return model.Success(dto.NewWorkDTO(result))
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的本地作者
func (h *Handler) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*dto.RankedLocalAuthor] {
	return model.HandleResult(h.svc.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds))
}

// UpdateLastUsed 批量更新作品最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateLastView(ctx, ids))
}
