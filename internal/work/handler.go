package work

import (
	"context"

	"github.com/library-squirrel/wails/pkg/model"
	"github.com/library-squirrel/wails/pkg/model/dto"
	domain "github.com/library-squirrel/wails/pkg/model/entity"
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
func (h *Handler) Save(ctx context.Context, work *dto.WorkParamDTO) *model.ApiResponse[int64] {
	domainWork := &domain.Work{
		BaseEntity: &model.BaseEntity{},
	}
	if work.SiteID != nil {
		domainWork.SiteID.Valid = true
		domainWork.SiteID.Int64 = *work.SiteID
	}
	if work.SiteWorkID != nil {
		domainWork.SiteWorkID.Valid = true
		domainWork.SiteWorkID.String = *work.SiteWorkID
	}
	if work.SiteWorkName != nil {
		domainWork.SiteWorkName.Valid = true
		domainWork.SiteWorkName.String = *work.SiteWorkName
	}

	if err := h.svc.Save(ctx, domainWork); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainWork.GetID())
}

// Delete 删除作品
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新作品
func (h *Handler) Update(ctx context.Context, work *dto.WorkParamDTO) *model.ApiResponse[any] {
	domainWork := &domain.Work{
		BaseEntity: &model.BaseEntity{},
	}
	domainWork.SetID(work.ID)
	if work.SiteID != nil {
		domainWork.SiteID.Valid = true
		domainWork.SiteID.Int64 = *work.SiteID
	}
	if work.SiteWorkID != nil {
		domainWork.SiteWorkID.Valid = true
		domainWork.SiteWorkID.String = *work.SiteWorkID
	}
	if work.SiteWorkName != nil {
		domainWork.SiteWorkName.Valid = true
		domainWork.SiteWorkName.String = *work.SiteWorkName
	}

	if err := h.svc.UpdateById(ctx, domainWork); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// DeleteWorkAndSurroundingData 删除作品及周边数据
func (h *Handler) DeleteWorkAndSurroundingData(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.DeleteWorkAndSurroundingData(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取作品
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*dto.WorkDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*dto.WorkDTO](err.Error())
	}
	return model.Success(dto.NewWorkDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *WorkQueryDTO) *model.ApiResponse[*model.Page[dto.WorkDTO, WorkQueryDTO]] {
	if queryDTO == nil {
		queryDTO = &WorkQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[dto.WorkDTO, WorkQueryDTO]](err.Error())
	}
	// 转换为 DTO
	data := make([]*dto.WorkDTO, 0, len(result.Data))
	for _, work := range result.Data {
		data = append(data, dto.NewWorkDTO(work))
	}
	return model.Success(&model.Page[dto.WorkDTO, WorkQueryDTO]{
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
	result, err := h.svc.GetFullWorkInfoById(ctx, id)
	if err != nil {
		return model.Error[*dto.WorkFullDTO](err.Error())
	}
	return model.Success(result)
}

// GetBySiteAndSiteWorkID 根据站点ID和站点作品ID获取作品
func (h *Handler) GetBySiteAndSiteWorkID(ctx context.Context, siteId int64, siteWorkId string) *model.ApiResponse[*dto.WorkDTO] {
	result, err := h.svc.GetBySiteAndSiteWorkID(ctx, siteId, siteWorkId)
	if err != nil {
		return model.Error[*dto.WorkDTO](err.Error())
	}
	return model.Success(dto.NewWorkDTO(result))
}

// ListRankedLocalAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的本地作者
func (h *Handler) ListRankedLocalAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*model.RankedLocalAuthor] {
	result, err := h.svc.ListRankedLocalAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*model.RankedLocalAuthor](err.Error())
	}
	return model.Success(result)
}

// UpdateLastUsed 批量更新作品最后使用时间
func (h *Handler) UpdateLastUsed(ctx context.Context, ids []int64) *model.ApiResponse[any] {
	if err := h.svc.UpdateLastView(ctx, ids); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}
