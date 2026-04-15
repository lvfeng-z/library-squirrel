package localAuthor

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
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
func (h *Handler) Save(ctx context.Context, author *LocalAuthorDTO) *model.ApiResponse[int64] {
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
func (h *Handler) Update(ctx context.Context, author *LocalAuthorDTO) *model.ApiResponse[any] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*LocalAuthorResultDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*LocalAuthorResultDTO](err.Error())
	}
	return model.Success(ToLocalAuthorResultDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page, pageSize int, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[LocalAuthorResultDTO]] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.PageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[LocalAuthorResultDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*LocalAuthorResultDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, ToLocalAuthorResultDTO(author))
	}
	return model.Success(&model.Page[LocalAuthorResultDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// ListSelectItems 查询选择项列表
func (h *Handler) ListSelectItems(ctx context.Context, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[[]*domain.SelectItem] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.ListSelectItemsByDTO(ctx, *queryDTO)
	if err != nil {
		return model.Error[[]*domain.SelectItem](err.Error())
	}
	return model.Success(result)
}

// QuerySelectItemPage 分页查询选择项
func (h *Handler) QuerySelectItemPage(ctx context.Context, page, pageSize int, queryDTO *LocalAuthorQueryDTO) *model.ApiResponse[*model.Page[domain.SelectItem]] {
	if queryDTO == nil {
		queryDTO = &LocalAuthorQueryDTO{}
	}
	result, err := h.svc.QuerySelectItemPageByDTO(ctx, page, pageSize, *queryDTO)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem]](err.Error())
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

// ========== DTO 定义 ==========

// LocalAuthorDTO 本地作者数据传输对象
type LocalAuthorDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
}

// LocalAuthorResultDTO 本地作者返回结果DTO（用于屏蔽sql.Null*类型）
type LocalAuthorResultDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
	LastUse    *int64  `json:"lastUse"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// ToLocalAuthorResultDTO 将 domain.LocalAuthor 转换为 LocalAuthorResultDTO
func ToLocalAuthorResultDTO(author *domain.LocalAuthor) *LocalAuthorResultDTO {
	if author == nil {
		return nil
	}
	return &LocalAuthorResultDTO{
		ID:         author.GetID(),
		AuthorName: nullStringToPointer(author.AuthorName),
		Introduce:  nullStringToPointer(author.Introduce),
		LastUse:    nullInt64ToPointer(author.LastUse),
		CreateTime: author.GetCreateTime(),
		UpdateTime: author.GetUpdateTime(),
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
