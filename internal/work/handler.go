package work

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, work *WorkDTO) *model.ApiResponse[int64] {
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
func (h *Handler) Update(ctx context.Context, work *WorkDTO) *model.ApiResponse[any] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*WorkResultDTO] {
	result, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*WorkResultDTO](err.Error())
	}
	return model.Success(ToWorkResultDTO(result))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *WorkQueryDTO) *model.ApiResponse[*model.Page[WorkResultDTO]] {
	if queryDTO == nil {
		queryDTO = &WorkQueryDTO{}
	}
	result, err := h.svc.Page(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[WorkResultDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*WorkResultDTO, 0, len(result.Data))
	for _, work := range result.Data {
		data = append(data, ToWorkResultDTO(work))
	}
	return model.Success(&model.Page[WorkResultDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// GetFullWorkInfoById 获取完整作品信息
func (h *Handler) GetFullWorkInfoById(ctx context.Context, id int64) *model.ApiResponse[*domain.WorkFullDTO] {
	result, err := h.svc.GetFullWorkInfoById(ctx, id)
	if err != nil {
		return model.Error[*domain.WorkFullDTO](err.Error())
	}
	return model.Success(result)
}

// ========== DTO 定义 ==========

// WorkDTO 作品数据传输对象
type WorkDTO struct {
	ID           int64   `json:"id"`
	SiteID       *int64  `json:"siteId"`
	SiteWorkID   *string `json:"siteWorkId"`
	SiteWorkName *string `json:"siteWorkName"`
}

// WorkResultDTO 作品返回结果DTO（用于屏蔽sql.Null*类型）
type WorkResultDTO struct {
	ID                  int64   `json:"id"`
	SiteID             *int64  `json:"siteId"`
	SiteWorkID         *string `json:"siteWorkId"`
	SiteWorkName       *string `json:"siteWorkName"`
	SiteAuthorID       *string `json:"siteAuthorId"`
	SiteWorkDescription *string `json:"siteWorkDescription"`
	SiteUploadTime     *int64  `json:"siteUploadTime"`
	SiteUpdateTime     *int64  `json:"siteUpdateTime"`
	NickName           *string `json:"nickName"`
	LocalAuthorID      *int64  `json:"localAuthorId"`
	LastView           *int64  `json:"lastView"`
	CreateTime         int64   `json:"createTime"`
	UpdateTime         int64   `json:"updateTime"`
}

// ToWorkResultDTO 将 domain.Work 转换为 WorkResultDTO
func ToWorkResultDTO(work *domain.Work) *WorkResultDTO {
	if work == nil {
		return nil
	}
	return &WorkResultDTO{
		ID:                    work.GetID(),
		SiteID:                nullInt64ToPointer(work.SiteID),
		SiteWorkID:            nullStringToPointer(work.SiteWorkID),
		SiteWorkName:          nullStringToPointer(work.SiteWorkName),
		SiteAuthorID:          nullStringToPointer(work.SiteAuthorID),
		SiteWorkDescription:   nullStringToPointer(work.SiteWorkDescription),
		SiteUploadTime:        nullInt64ToPointer(work.SiteUploadTime),
		SiteUpdateTime:        nullInt64ToPointer(work.SiteUpdateTime),
		NickName:              nullStringToPointer(work.NickName),
		LocalAuthorID:         nullInt64ToPointer(work.LocalAuthorID),
		LastView:              nullInt64ToPointer(work.LastView),
		CreateTime:            work.GetCreateTime(),
		UpdateTime:            work.GetUpdateTime(),
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
