package workSet

import (
	"context"

	"github.com/library-squirrel/backend/base/model"
	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
)

// Handler 作品集 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建作品集 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存作品集
func (h *Handler) Save(ctx context.Context, workSet *dto2.WorkSetDTO) *model.ApiResponse[int64] {
	domainWorkSet := dto2.ToWorkSetEntity(workSet)

	if err := h.svc.Save(ctx, domainWorkSet); err != nil {
		return model.HandleError[int64](err)
	}
	return model.Success(domainWorkSet.GetID())
}

// Delete 删除作品集
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.Delete(ctx, id))
}

// Update 更新作品集
func (h *Handler) Update(ctx context.Context, workSet *dto2.WorkSetDTO) *model.ApiResponse[any] {
	domainWorkSet := dto2.ToWorkSetEntity(workSet)

	if err := h.svc.Update(ctx, domainWorkSet); err != nil {
		return model.HandleError[any](err)
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取作品集
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto2.WorkSetDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.HandleError[*dto2.WorkSetDTO](err)
	}
	return model.Success(dto2.NewWorkSetDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto2.WorkSetDTO], query WorkSetQueryDTO) *model.ApiResponse[*model.Page[dto2.WorkSetDTO]] {
	if page == nil {
		page = &model.Page[dto2.WorkSetDTO]{}
	}
	entityPage := &model.Page[entity2.WorkSet]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.Page(ctx, entityPage, query)
	if err != nil {
		return model.HandleError[*model.Page[dto2.WorkSetDTO]](err)
	}
	// 转换为 DTO
	data := make([]*dto2.WorkSetDTO, 0, len(result.Data))
	for _, workSet := range result.Data {
		data = append(data, dto2.NewWorkSetDTO(workSet))
	}
	return model.Success(&model.Page[dto2.WorkSetDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// GetWorksByWorkSetId 获取作品集下的作品列表
func (h *Handler) GetWorksByWorkSetId(ctx context.Context, workSetId int64) *model.ApiResponse[[]*dto2.WorkDTO] {
	result, err := h.svc.GetWorksByWorkSetId(ctx, workSetId)
	if err != nil {
		return model.HandleError[[]*dto2.WorkDTO](err)
	}
	data := make([]*dto2.WorkDTO, 0, len(result))
	for _, work := range result {
		data = append(data, dto2.NewWorkDTO(work))
	}
	return model.Success(data)
}

// ListWorkSetsByWorkId 获取作品关联的作品集列表
func (h *Handler) ListWorkSetsByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*dto2.WorkSetDTO] {
	result, err := h.svc.ListWorkSetsByWorkId(ctx, workId)
	if err != nil {
		return model.HandleError[[]*dto2.WorkSetDTO](err)
	}
	data := make([]*dto2.WorkSetDTO, 0, len(result))
	for _, ws := range result {
		data = append(data, dto2.NewWorkSetDTO(ws))
	}
	return model.Success(data)
}

// LinkWorkToWorkSet 关联作品到作品集
func (h *Handler) LinkWorkToWorkSet(ctx context.Context, workId, workSetId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.LinkWorkToWorkSet(ctx, workId, workSetId, 0))
}

// UnlinkWorkFromWorkSet 取消作品与作品集的关联
func (h *Handler) UnlinkWorkFromWorkSet(ctx context.Context, workId, workSetId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UnlinkWorkFromWorkSet(ctx, workId, workSetId))
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称获取作品集
func (h *Handler) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) *model.ApiResponse[*dto2.WorkSetDTO] {
	result, err := h.svc.GetBySiteWorkSetIdAndSiteName(ctx, siteWorkSetId, siteName)
	if err != nil {
		return model.HandleError[*dto2.WorkSetDTO](err)
	}
	return model.Success(dto2.NewWorkSetDTO(result))
}

// LinkBatchToWorkSet 批量关联作品到作品集
func (h *Handler) LinkBatchToWorkSet(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.LinkBatchToWorkSet(ctx, workSetId, workIds))
}

// RemoveBatchFromWorkSet 批量从作品集移除作品
func (h *Handler) RemoveBatchFromWorkSet(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.RemoveBatchFromWorkSet(ctx, workSetId, workIds))
}

// UpdateSortOrders 批量更新排序顺序
func (h *Handler) UpdateSortOrders(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UpdateSortOrders(ctx, workSetId, workIds))
}

// SetCover 设置作品集封面
func (h *Handler) SetCover(ctx context.Context, workSetId, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.SetCoverWork(ctx, workSetId, workId))
}

// UnsetCover 取消作品集封面
func (h *Handler) UnsetCover(ctx context.Context, workSetId, workId int64) *model.ApiResponse[any] {
	return model.HandleVoid(h.svc.UnsetCover(ctx, workSetId, workId))
}

// GetCoverWorkId 获取封面作品ID
func (h *Handler) GetCoverWorkId(ctx context.Context, workSetId int64) *model.ApiResponse[int64] {
	return model.HandleResult(h.svc.GetCoverWorkId(ctx, workSetId))
}

// ListWorkSetWithWorkByIds 根据作品集ID列表获取作品集及作品完整信息
func (h *Handler) ListWorkSetWithWorkByIds(ctx context.Context, workSetIds []int64) *model.ApiResponse[[]*dto2.WorkSetWithWorksResultDTO] {
	result, err := h.svc.ListWorkSetWithWorkByIds(ctx, workSetIds)
	if err != nil {
		return model.HandleError[[]*dto2.WorkSetWithWorksResultDTO](err)
	}
	return model.Success(result)
}

// QueryPageWithCover 分页查询作品集（带封面）
func (h *Handler) QueryPageWithCover(ctx context.Context, page *model.Page[dto2.WorkSetWithCoverDTO], query WorkSetQueryDTO) *model.ApiResponse[*model.Page[dto2.WorkSetWithCoverDTO]] {
	if page == nil {
		page = &model.Page[dto2.WorkSetWithCoverDTO]{}
	}
	workSetPage := &model.Page[WorkSetWithCoverDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
	}
	result, err := h.svc.QueryPageWithCover(ctx, workSetPage, query)
	if err != nil {
		return model.HandleError[*model.Page[dto2.WorkSetWithCoverDTO]](err)
	}
	// 转换为 ResultDTO
	data := make([]*dto2.WorkSetWithCoverDTO, 0, len(result.Data))
	for _, ws := range result.Data {
		dto := &dto2.WorkSetWithCoverDTO{
			WorkSet: dto2.NewWorkSetDTO(ws.WorkSet),
		}
		if ws.CoverWork != nil {
			dto.CoverWork = dto2.NewWorkDTO(ws.CoverWork)
		}
		data = append(data, dto)
	}
	return model.Success(&model.Page[dto2.WorkSetWithCoverDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}
