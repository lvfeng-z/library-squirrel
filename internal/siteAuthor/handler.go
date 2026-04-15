package siteAuthor

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, author *SiteAuthorDTO) *model.ApiResponse[int64] {
	domainAuthor := &domain.SiteAuthor{
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

	if err := h.svc.Save(ctx, domainAuthor); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainAuthor.GetID())
}

// Delete 删除站点作者
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点作者
func (h *Handler) Update(ctx context.Context, author *SiteAuthorDTO) *model.ApiResponse[any] {
	domainAuthor := &domain.SiteAuthor{
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

	if err := h.svc.UpdateById(ctx, domainAuthor); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*SiteAuthorResultDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*SiteAuthorResultDTO](err.Error())
	}
	return model.Success(ToSiteAuthorResultDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *SiteAuthorQueryDTO) *model.ApiResponse[*model.Page[SiteAuthorResultDTO]] {
	if queryDTO == nil {
		queryDTO = &SiteAuthorQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[SiteAuthorResultDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteAuthorResultDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, ToSiteAuthorResultDTO(author))
	}
	return model.Success(&model.Page[SiteAuthorResultDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// ListByWorkId 根据作品ID获取作者列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*model.RankedSiteAuthor] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*model.RankedSiteAuthor](err.Error())
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

// SiteAuthorDTO 站点作者数据传输对象
type SiteAuthorDTO struct {
	ID           int64   `json:"id"`
	SiteID       *int64  `json:"siteId"`
	SiteAuthorID *string `json:"siteAuthorId"`
	AuthorName   *string `json:"authorName"`
}

// SiteAuthorResultDTO 站点作者返回结果DTO（用于屏蔽sql.Null*类型）
type SiteAuthorResultDTO struct {
	ID                 int64   `json:"id"`
	SiteID             *int64  `json:"siteId"`
	SiteAuthorID       *string `json:"siteAuthorId"`
	AuthorName         *string `json:"authorName"`
	FixedAuthorName    *string `json:"fixedAuthorName"`
	SiteAuthorNameBefore *string `json:"siteAuthorNameBefore"`
	Introduce          *string `json:"introduce"`
	LocalAuthorID      *int64  `json:"localAuthorId"`
	LastUse            *int64  `json:"lastUse"`
	CreateTime         int64   `json:"createTime"`
	UpdateTime         int64   `json:"updateTime"`
}

// ToSiteAuthorResultDTO 将 domain.SiteAuthor 转换为 SiteAuthorResultDTO
func ToSiteAuthorResultDTO(author *domain.SiteAuthor) *SiteAuthorResultDTO {
	if author == nil {
		return nil
	}
	return &SiteAuthorResultDTO{
		ID:                   author.GetID(),
		SiteID:               nullInt64ToPointer(author.SiteID),
		SiteAuthorID:         nullStringToPointer(author.SiteAuthorID),
		AuthorName:           nullStringToPointer(author.AuthorName),
		FixedAuthorName:      nullStringToPointer(author.FixedAuthorName),
		SiteAuthorNameBefore: nullStringToPointer(author.SiteAuthorNameBefore),
		Introduce:            nullStringToPointer(author.Introduce),
		LocalAuthorID:        nullInt64ToPointer(author.LocalAuthorID),
		LastUse:              nullInt64ToPointer(author.LastUse),
		CreateTime:           author.GetCreateTime(),
		UpdateTime:           author.GetUpdateTime(),
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
