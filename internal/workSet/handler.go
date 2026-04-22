package workSet

import (
	"context"
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, workSet *WorkSetDTO) *model.ApiResponse[int64] {
	domainWorkSet := &entity2.WorkSet{
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
	domainWorkSet := &entity2.WorkSet{
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
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[WorkSetResultDTO, WorkSetQueryDTO]) *model.ApiResponse[*model.Page[WorkSetResultDTO, WorkSetQueryDTO]] {
	if page == nil {
		page = &model.Page[WorkSetResultDTO, WorkSetQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[WorkSetResultDTO, WorkSetQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*WorkSetResultDTO, 0, len(result.Data))
	for _, workSet := range result.Data {
		data = append(data, ToWorkSetResultDTO(workSet))
	}
	return model.Success(&model.Page[WorkSetResultDTO, WorkSetQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// GetWorksByWorkSetId 获取作品集下的作品列表
func (h *Handler) GetWorksByWorkSetId(ctx context.Context, workSetId int64) *model.ApiResponse[[]*entity2.Work] {
	result, err := h.svc.GetWorksByWorkSetId(ctx, workSetId)
	if err != nil {
		return model.Error[[]*entity2.Work](err.Error())
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
func (h *Handler) ListWorkSetWithWorkByIds(ctx context.Context, workSetIds []int64) *model.ApiResponse[[]*WorkSetWithWorksResultDTO] {
	result, err := h.svc.ListWorkSetWithWorkByIds(ctx, workSetIds)
	if err != nil {
		return model.Error[[]*WorkSetWithWorksResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	dtos := make([]*WorkSetWithWorksResultDTO, 0, len(result))
	for _, ws := range result {
		works := make([]*WorkResultDTO, 0, len(ws.Works))
		for _, w := range ws.Works {
			works = append(works, ToWorkResultDTO(w))
		}
		dtos = append(dtos, &WorkSetWithWorksResultDTO{
			WorkSet: ToWorkSetResultDTO(ws.WorkSet),
			Works:   works,
		})
	}
	return model.Success(dtos)
}

// QueryPageWithCover 分页查询作品集（带封面）
func (h *Handler) QueryPageWithCover(ctx context.Context, page *model.Page[WorkSetWithCoverResultDTO, WorkSetQueryDTO]) *model.ApiResponse[*model.Page[WorkSetWithCoverResultDTO, WorkSetQueryDTO]] {
	if page == nil {
		page = &model.Page[WorkSetWithCoverResultDTO, WorkSetQueryDTO]{}
	}
	result, err := h.svc.QueryPageWithCoverByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[WorkSetWithCoverResultDTO, WorkSetQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*WorkSetWithCoverResultDTO, 0, len(result.Data))
	for _, ws := range result.Data {
		dto := &WorkSetWithCoverResultDTO{
			WorkSet: ToWorkSetResultDTO(ws.WorkSet),
		}
		if ws.CoverWork != nil {
			dto.CoverWork = ToWorkResultDTO(ws.CoverWork)
		}
		data = append(data, dto)
	}
	return model.Success(&model.Page[WorkSetWithCoverResultDTO, WorkSetQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
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
	ID                     int64   `json:"id"`
	SiteID                 *int64  `json:"siteId"`
	SiteWorkSetID          *string `json:"siteWorkSetId"`
	SiteWorkSetName        *string `json:"siteWorkSetName"`
	SiteAuthorID           *string `json:"siteAuthorId"`
	SiteWorkSetDescription *string `json:"siteWorkSetDescription"`
	SiteUploadTime         *int64  `json:"siteUploadTime"`
	SiteUpdateTime         *int64  `json:"siteUpdateTime"`
	NickName               *string `json:"nickName"`
	LastView               *int64  `json:"lastView"`
	CreateTime             int64   `json:"createTime"`
	UpdateTime             int64   `json:"updateTime"`
}

// WorkResultDTO 作品返回结果DTO（用于屏蔽sql.Null*类型）
type WorkResultDTO struct {
	ID                  int64   `json:"id"`
	SiteID              *int64  `json:"siteId"`
	SiteWorkID          *string `json:"siteWorkId"`
	SiteWorkName        *string `json:"siteWorkName"`
	SiteAuthorID        *string `json:"siteAuthorId"`
	SiteWorkDescription *string `json:"siteWorkDescription"`
	SiteUploadTime      *int64  `json:"siteUploadTime"`
	SiteUpdateTime      *int64  `json:"siteUpdateTime"`
	NickName            *string `json:"nickName"`
	LocalAuthorID       *int64  `json:"localAuthorId"`
	LastView            *int64  `json:"lastView"`
	CreateTime          int64   `json:"createTime"`
	UpdateTime          int64   `json:"updateTime"`
}

// WorkSetWithWorksResultDTO 作品集及其作品信息
type WorkSetWithWorksResultDTO struct {
	WorkSet *WorkSetResultDTO `json:"workSet"`
	Works   []*WorkResultDTO  `json:"works"`
}

// WorkSetWithCoverResultDTO 作品集及其封面作品信息
type WorkSetWithCoverResultDTO struct {
	WorkSet   *WorkSetResultDTO `json:"workSet"`
	CoverWork *WorkResultDTO    `json:"coverWork,omitempty"`
}

// ToWorkSetResultDTO 将 domain.WorkSet 转换为 WorkSetResultDTO
func ToWorkSetResultDTO(workSet *entity2.WorkSet) *WorkSetResultDTO {
	if workSet == nil {
		return nil
	}
	return &WorkSetResultDTO{
		ID:                     workSet.GetID(),
		SiteID:                 nullInt64ToPointer(workSet.SiteID),
		SiteWorkSetID:          nullStringToPointer(workSet.SiteWorkSetID),
		SiteWorkSetName:        nullStringToPointer(workSet.SiteWorkSetName),
		SiteAuthorID:           nullStringToPointer(workSet.SiteAuthorID),
		SiteWorkSetDescription: nullStringToPointer(workSet.SiteWorkSetDescription),
		SiteUploadTime:         nullInt64ToPointer(workSet.SiteUploadTime),
		SiteUpdateTime:         nullInt64ToPointer(workSet.SiteUpdateTime),
		NickName:               nullStringToPointer(workSet.NickName),
		LastView:               nullInt64ToPointer(workSet.LastView),
		CreateTime:             workSet.GetCreateTime(),
		UpdateTime:             workSet.GetUpdateTime(),
	}
}

// ToWorkResultDTO 将 domain.Work 转换为 WorkResultDTO
func ToWorkResultDTO(work *entity2.Work) *WorkResultDTO {
	if work == nil {
		return nil
	}
	return &WorkResultDTO{
		ID:                  work.GetID(),
		SiteID:              nullInt64ToPointer(work.SiteID),
		SiteWorkID:          nullStringToPointer(work.SiteWorkID),
		SiteWorkName:        nullStringToPointer(work.SiteWorkName),
		SiteAuthorID:        nullStringToPointer(work.SiteAuthorID),
		SiteWorkDescription: nullStringToPointer(work.SiteWorkDescription),
		SiteUploadTime:      nullInt64ToPointer(work.SiteUploadTime),
		SiteUpdateTime:      nullInt64ToPointer(work.SiteUpdateTime),
		NickName:            nullStringToPointer(work.NickName),
		LocalAuthorID:       nullInt64ToPointer(work.LocalAuthorID),
		LastView:            nullInt64ToPointer(work.LastView),
		CreateTime:          work.GetCreateTime(),
		UpdateTime:          work.GetUpdateTime(),
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
