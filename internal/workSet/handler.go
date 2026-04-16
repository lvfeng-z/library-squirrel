package workSet

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, workSet *WorkSetDTO) *model.ApiResponse[int64] {
	domainWorkSet := &domain.WorkSet{
		BaseEntity: &model.BaseEntity{},
	}
	if workSet.SiteID != nil {
		domainWorkSet.SiteID.Valid = true
		domainWorkSet.SiteID.Int64 = *workSet.SiteID
	}
	if workSet.SiteWorkSetName != nil {
		domainWorkSet.SiteWorkSetName.Valid = true
		domainWorkSet.SiteWorkSetName.String = *workSet.SiteWorkSetName
	}

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
func (h *Handler) Update(ctx context.Context, workSet *WorkSetDTO) *model.ApiResponse[any] {
	domainWorkSet := &domain.WorkSet{
		BaseEntity: &model.BaseEntity{},
	}
	domainWorkSet.SetID(workSet.ID)
	if workSet.SiteID != nil {
		domainWorkSet.SiteID.Valid = true
		domainWorkSet.SiteID.Int64 = *workSet.SiteID
	}
	if workSet.SiteWorkSetName != nil {
		domainWorkSet.SiteWorkSetName.Valid = true
		domainWorkSet.SiteWorkSetName.String = *workSet.SiteWorkSetName
	}

	if err := h.svc.Update(ctx, domainWorkSet); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取作品集
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*WorkSetResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*WorkSetResultDTO](err.Error())
	}
	return model.Success(ToWorkSetResultDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *WorkSetQueryDTO) *model.ApiResponse[*model.Page[WorkSetResultDTO]] {
	if queryDTO == nil {
		queryDTO = &WorkSetQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[WorkSetResultDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*WorkSetResultDTO, 0, len(result.Data))
	for _, workSet := range result.Data {
		data = append(data, ToWorkSetResultDTO(workSet))
	}
	return model.Success(&model.Page[WorkSetResultDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// GetWorksByWorkSetId 获取作品集下的作品列表
func (h *Handler) GetWorksByWorkSetId(ctx context.Context, workSetId int64) *model.ApiResponse[[]*domain.Work] {
	result, err := h.svc.GetWorksByWorkSetId(ctx, workSetId)
	if err != nil {
		return model.Error[[]*domain.Work](err.Error())
	}
	return model.Success(result)
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
func (h *Handler) GetBySiteWorkSetIdAndSiteName(ctx context.Context, siteWorkSetId string, siteName string) *model.ApiResponse[*WorkSetResultDTO] {
	result, err := h.svc.GetBySiteWorkSetIdAndSiteName(ctx, siteWorkSetId, siteName)
	if err != nil {
		return model.Error[*WorkSetResultDTO](err.Error())
	}
	return model.Success(ToWorkSetResultDTO(result))
}

// ========== DTO 定义 ==========

// WorkSetDTO 作品集数据传输对象
type WorkSetDTO struct {
	ID              int64   `json:"id"`
	SiteID          *int64  `json:"siteId"`
	SiteWorkSetName *string `json:"siteWorkSetName"`
}

// WorkSetResultDTO 作品集返回结果DTO（用于屏蔽sql.Null*类型）
type WorkSetResultDTO struct {
	ID                  int64   `json:"id"`
	SiteID             *int64  `json:"siteId"`
	SiteWorkSetID      *string `json:"siteWorkSetId"`
	SiteWorkSetName    *string `json:"siteWorkSetName"`
	SiteAuthorID       *string `json:"siteAuthorId"`
	SiteWorkSetDescription *string `json:"siteWorkSetDescription"`
	SiteUploadTime     *int64  `json:"siteUploadTime"`
	SiteUpdateTime     *int64  `json:"siteUpdateTime"`
	NickName           *string `json:"nickName"`
	LastView           *int64  `json:"lastView"`
	CreateTime         int64   `json:"createTime"`
	UpdateTime         int64   `json:"updateTime"`
}

// ToWorkSetResultDTO 将 domain.WorkSet 转换为 WorkSetResultDTO
func ToWorkSetResultDTO(workSet *domain.WorkSet) *WorkSetResultDTO {
	if workSet == nil {
		return nil
	}
	return &WorkSetResultDTO{
		ID:                    workSet.GetID(),
		SiteID:                nullInt64ToPointer(workSet.SiteID),
		SiteWorkSetID:         nullStringToPointer(workSet.SiteWorkSetID),
		SiteWorkSetName:       nullStringToPointer(workSet.SiteWorkSetName),
		SiteAuthorID:          nullStringToPointer(workSet.SiteAuthorID),
		SiteWorkSetDescription: nullStringToPointer(workSet.SiteWorkSetDescription),
		SiteUploadTime:        nullInt64ToPointer(workSet.SiteUploadTime),
		SiteUpdateTime:        nullInt64ToPointer(workSet.SiteUpdateTime),
		NickName:              nullStringToPointer(workSet.NickName),
		LastView:              nullInt64ToPointer(workSet.LastView),
		CreateTime:            workSet.GetCreateTime(),
		UpdateTime:            workSet.GetUpdateTime(),
	}
}

// nullStringToPointer 将 sql.NullString 转换为 *string
func nullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// nullInt64ToPointer 将 sql.NullInt64 转换为 *int64
func nullInt64ToPointer(ns sql.NullInt64) *int64 {
	if ns.Valid {
		return &ns.Int64
	}
	return nil
}
