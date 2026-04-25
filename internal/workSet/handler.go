package workSet

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	dto2 "github.com/library-squirrel/wails/pkg/model/dto"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
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
		return model.Error[int64](err.Error())
	}
	return model.Success(domainWorkSet.GetID())
}

// Delete 删除作品集
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新作品集
func (h *Handler) Update(ctx context.Context, workSet *dto2.WorkSetDTO) *model.ApiResponse[any] {
	domainWorkSet := dto2.ToWorkSetEntity(workSet)

	if err := h.svc.Update(ctx, domainWorkSet); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取作品集
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto2.WorkSetDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto2.WorkSetDTO](err.Error())
	}
	return model.Success(dto2.NewWorkSetDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[dto2.WorkSetDTO, WorkSetQueryDTO]) *model.ApiResponse[*model.Page[dto2.WorkSetDTO, WorkSetQueryDTO]] {
	if page == nil {
		page = &model.Page[dto2.WorkSetDTO, WorkSetQueryDTO]{}
	}
	entityPage := &model.Page[entity2.WorkSet, WorkSetQueryDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
		Query:      page.Query,
	}
	result, err := h.svc.Page(ctx, entityPage)
	if err != nil {
		return model.Error[*model.Page[dto2.WorkSetDTO, WorkSetQueryDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto2.WorkSetDTO, 0, len(result.Data))
	for _, workSet := range result.Data {
		data = append(data, dto2.NewWorkSetDTO(workSet))
	}
	return model.Success(&model.Page[dto2.WorkSetDTO, WorkSetQueryDTO]{
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
		return model.Error[[]*dto2.WorkDTO](err.Error())
	}
	data := make([]*dto2.WorkDTO, 0, len(result))
	for _, work := range result {
		data = append(data, dto2.NewWorkDTO(work))
	}
	return model.Success(data)
}

// LinkWorkToWorkSet 关联作品到作品集
func (h *Handler) LinkWorkToWorkSet(ctx context.Context, workId, workSetId int64) *model.ApiResponse[any] {
	if err := h.svc.LinkWorkToWorkSet(ctx, workId, workSetId, 0); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// UnlinkWorkFromWorkSet 取消作品与作品集的关联
func (h *Handler) UnlinkWorkFromWorkSet(ctx context.Context, workId, workSetId int64) *model.ApiResponse[any] {
	if err := h.svc.UnlinkWorkFromWorkSet(ctx, workId, workSetId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// GetBySiteWorkSetIdAndSiteName 根据站点作品集ID和站点名称获取作品集
func (h *Handler) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) *model.ApiResponse[*dto2.WorkSetDTO] {
	result, err := h.svc.GetBySiteWorkSetIdAndSiteName(ctx, siteWorkSetId, siteName)
	if err != nil {
		return model.Error[*dto2.WorkSetDTO](err.Error())
	}
	return model.Success(dto2.NewWorkSetDTO(result))
}

// LinkBatchToWorkSet 批量关联作品到作品集
func (h *Handler) LinkBatchToWorkSet(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	if err := h.svc.LinkBatchToWorkSet(ctx, workSetId, workIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// RemoveBatchFromWorkSet 批量从作品集移除作品
func (h *Handler) RemoveBatchFromWorkSet(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	if err := h.svc.RemoveBatchFromWorkSet(ctx, workSetId, workIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// UpdateSortOrders 批量更新排序顺序
func (h *Handler) UpdateSortOrders(ctx context.Context, workSetId int64, workIds []int64) *model.ApiResponse[any] {
	if err := h.svc.UpdateSortOrders(ctx, workSetId, workIds); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// SetCover 设置作品集封面
func (h *Handler) SetCover(ctx context.Context, workSetId, workId int64) *model.ApiResponse[any] {
	if err := h.svc.SetCoverWork(ctx, workSetId, workId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// UnsetCover 取消作品集封面
func (h *Handler) UnsetCover(ctx context.Context, workSetId, workId int64) *model.ApiResponse[any] {
	if err := h.svc.UnsetCover(ctx, workSetId, workId); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// GetCoverWorkId 获取封面作品ID
func (h *Handler) GetCoverWorkId(ctx context.Context, workSetId int64) *model.ApiResponse[int64] {
	workId, err := h.svc.GetCoverWorkId(ctx, workSetId)
	if err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(workId)
}

// ListWorkSetWithWorkByIds 根据作品集ID列表获取作品集及作品
func (h *Handler) ListWorkSetWithWorkByIds(ctx context.Context, workSetIds []int64) *model.ApiResponse[[]*dto2.WorkSetWithWorksResultDTO] {
	result, err := h.svc.ListWorkSetWithWorkByIds(ctx, workSetIds)
	if err != nil {
		return model.Error[[]*dto2.WorkSetWithWorksResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	dtos := make([]*dto2.WorkSetWithWorksResultDTO, 0, len(result))
	for _, ws := range result {
		works := make([]*dto2.WorkDTO, 0, len(ws.Works))
		for _, w := range ws.Works {
			works = append(works, dto2.NewWorkDTO(w))
		}
		dtos = append(dtos, &dto2.WorkSetWithWorksResultDTO{
			WorkSet: dto2.NewWorkSetDTO(ws.WorkSet),
			Works:   works,
		})
	}
	return model.Success(dtos)
}

// QueryPageWithCover 分页查询作品集（带封面）
func (h *Handler) QueryPageWithCover(ctx context.Context, page *model.Page[dto2.WorkSetWithCoverResultDTO, WorkSetQueryDTO]) *model.ApiResponse[*model.Page[dto2.WorkSetWithCoverResultDTO, WorkSetQueryDTO]] {
	if page == nil {
		page = &model.Page[dto2.WorkSetWithCoverResultDTO, WorkSetQueryDTO]{}
	}
	workSetPage := &model.Page[WorkSetWithCoverDTO, WorkSetQueryDTO]{
		PageNumber: page.PageNumber,
		PageSize:   page.PageSize,
		Query:      page.Query,
	}
	result, err := h.svc.QueryPageWithCover(ctx, workSetPage)
	if err != nil {
		return model.Error[*model.Page[dto2.WorkSetWithCoverResultDTO, WorkSetQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*dto2.WorkSetWithCoverResultDTO, 0, len(result.Data))
	for _, ws := range result.Data {
		dto := &dto2.WorkSetWithCoverResultDTO{
			WorkSet: dto2.NewWorkSetDTO(ws.WorkSet),
		}
		if ws.CoverWork != nil {
			dto.CoverWork = dto2.NewWorkDTO(ws.CoverWork)
		}
		data = append(data, dto)
	}
	return model.Success(&model.Page[dto2.WorkSetWithCoverResultDTO, WorkSetQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

