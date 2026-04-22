package siteAuthor

import (
	"context"
	"database/sql"

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
func (h *Handler) Save(ctx context.Context, author *SiteAuthorDTO) *model.ApiResponse[int64] {
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
func (h *Handler) SaveBatch(ctx context.Context, authors []*SiteAuthorDTO) *model.ApiResponse[any] {
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
func (h *Handler) Update(ctx context.Context, author *SiteAuthorDTO) *model.ApiResponse[any] {
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
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*SiteAuthorResultDTO] {
	author, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*SiteAuthorResultDTO](err.Error())
	}
	return model.Success(ToSiteAuthorResultDTO(author))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[SiteAuthorResultDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[SiteAuthorResultDTO, SiteAuthorQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteAuthorResultDTO, SiteAuthorQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[SiteAuthorResultDTO, SiteAuthorQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteAuthorResultDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, ToSiteAuthorResultDTO(author))
	}
	return model.Success(&model.Page[SiteAuthorResultDTO, SiteAuthorQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// QueryBoundOrUnboundToLocalAuthorPage 查询绑定或未绑定到本地作者的站点作者分页
func (h *Handler) QueryBoundOrUnboundToLocalAuthorPage(ctx context.Context, page *model.Page[SiteAuthorFullDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[SiteAuthorFullDTO, SiteAuthorQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteAuthorFullDTO, SiteAuthorQueryDTO]{}
	}
	result, err := h.svc.QueryBoundOrUnboundToLocalAuthorPageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[SiteAuthorFullDTO, SiteAuthorQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteAuthorFullDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, ToSiteAuthorFullDTO(author))
	}
	return model.Success(&model.Page[SiteAuthorFullDTO, SiteAuthorQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// QueryLocalRelateDTOPage 查询站点作者与本地作者关联DTO分页
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]) *model.ApiResponse[*model.Page[SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]{}
	}
	result, err := h.svc.QueryLocalRelateDTOPageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteAuthorLocalRelateDTO, 0, len(result.Data))
	for _, author := range result.Data {
		data = append(data, ToSiteAuthorLocalRelateDTO(author))
	}
	return model.Success(&model.Page[SiteAuthorLocalRelateDTO, SiteAuthorQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// ListBySiteAuthorIds 根据站点作者ID列表获取
func (h *Handler) ListBySiteAuthorIds(ctx context.Context, siteAuthorIds []int64) *model.ApiResponse[[]*SiteAuthorResultDTO] {
	result, err := h.svc.ListBySiteAuthorIds(ctx, siteAuthorIds)
	if err != nil {
		return model.Error[[]*SiteAuthorResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteAuthorResultDTO, 0, len(result))
	for _, author := range result {
		data = append(data, ToSiteAuthorResultDTO(author))
	}
	return model.Success(data)
}

// ListRankedSiteAuthorWithWorkIdByWorkIds 根据作品ID列表获取带排名的站点作者
func (h *Handler) ListRankedSiteAuthorWithWorkIdByWorkIds(ctx context.Context, workIds []int64) *model.ApiResponse[[]*RankedSiteAuthorWithWorkIdDTO] {
	result, err := h.svc.ListRankedSiteAuthorWithWorkIdByWorkIds(ctx, workIds)
	if err != nil {
		return model.Error[[]*RankedSiteAuthorWithWorkIdDTO](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*RankedSiteAuthorWithWorkIdDTO, 0, len(result))
	for _, author := range result {
		data = append(data, ToRankedSiteAuthorWithWorkIdDTO(author))
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
func (h *Handler) CreateAndBindSameNameLocalAuthor(ctx context.Context, siteAuthor *SiteAuthorDTO) *model.ApiResponse[bool] {
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
	Introduce    *string `json:"introduce"`
}

// SiteAuthorResultDTO 站点作者返回结果DTO（用于屏蔽sql.Null*类型）
type SiteAuthorResultDTO struct {
	ID                   int64   `json:"id"`
	SiteID               *int64  `json:"siteId"`
	SiteAuthorID         *string `json:"siteAuthorId"`
	AuthorName           *string `json:"authorName"`
	FixedAuthorName      *string `json:"fixedAuthorName"`
	SiteAuthorNameBefore *string `json:"siteAuthorNameBefore"`
	Introduce            *string `json:"introduce"`
	LocalAuthorID        *int64  `json:"localAuthorId"`
	LastUse              *int64  `json:"lastUse"`
	CreateTime           int64   `json:"createTime"`
	UpdateTime           int64   `json:"updateTime"`
}

// SiteAuthorFullDTO 站点作者完整信息DTO
type SiteAuthorFullDTO struct {
	SiteAuthorResultDTO
	LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
}

// SiteAuthorLocalRelateDTO 站点作者与本地作者关联DTO
type SiteAuthorLocalRelateDTO struct {
	SiteAuthorResultDTO
	LocalAuthor *LocalAuthorDTO `json:"localAuthor,omitempty"`
}

// RankedSiteAuthorWithWorkIdDTO 带作品ID的排名站点作者DTO
type RankedSiteAuthorWithWorkIdDTO struct {
	WorkId       int64   `json:"workId"`
	SiteAuthorID *string `json:"siteAuthorId"`
	AuthorName   *string `json:"authorName"`
	Rank         int     `json:"rank"`
}

// LocalAuthorDTO 本地作者数据传输对象
type LocalAuthorDTO struct {
	ID         int64   `json:"id"`
	AuthorName *string `json:"authorName"`
	Introduce  *string `json:"introduce"`
	CreateTime int64   `json:"createTime"`
	UpdateTime int64   `json:"updateTime"`
}

// ToSiteAuthorResultDTO 将 domain.SiteAuthor 转换为 SiteAuthorResultDTO
func ToSiteAuthorResultDTO(author *entity2.SiteAuthor) *SiteAuthorResultDTO {
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

// ToSiteAuthorFullDTO 将 domain.SiteAuthorFullDTO 转换为 SiteAuthorFullDTO
func ToSiteAuthorFullDTO(dto *dto.SiteAuthorFullDTO) *SiteAuthorFullDTO {
	if dto == nil {
		return nil
	}
	return &SiteAuthorFullDTO{
		SiteAuthorResultDTO: SiteAuthorResultDTO{
			ID:                   dto.ID,
			SiteAuthorID:         stringPtrIfValid(dto.SiteAuthorID),
			AuthorName:           stringPtrIfValid(dto.AuthorName),
			FixedAuthorName:      stringPtrIfValid(dto.FixedAuthorName),
			SiteAuthorNameBefore: stringPtrIfValid(dto.SiteAuthorNameBefore),
			Introduce:            stringPtrIfValid(dto.Introduce),
			LocalAuthorID:        int64PtrIfValid(dto.LocalAuthorID),
			LastUse:              int64PtrIfValid(dto.LastUse),
			CreateTime:           dto.CreateTime,
			UpdateTime:           dto.UpdateTime,
		},
		LocalAuthor: ToLocalAuthorDTO(dto.LocalAuthor),
	}
}

// ToSiteAuthorLocalRelateDTO 将 domain.SiteAuthorLocalRelateDTO 转换为 SiteAuthorLocalRelateDTO
func ToSiteAuthorLocalRelateDTO(dto *dto.SiteAuthorLocalRelateDTO) *SiteAuthorLocalRelateDTO {
	if dto == nil {
		return nil
	}
	return &SiteAuthorLocalRelateDTO{
		SiteAuthorResultDTO: SiteAuthorResultDTO{
			ID:                   dto.ID,
			SiteAuthorID:         stringPtrIfValid(dto.SiteAuthorID),
			AuthorName:           stringPtrIfValid(dto.AuthorName),
			FixedAuthorName:      stringPtrIfValid(dto.FixedAuthorName),
			SiteAuthorNameBefore: stringPtrIfValid(dto.SiteAuthorNameBefore),
			Introduce:            stringPtrIfValid(dto.Introduce),
			LocalAuthorID:        int64PtrIfValid(dto.LocalAuthorID),
			LastUse:              int64PtrIfValid(dto.LastUse),
			CreateTime:           dto.CreateTime,
			UpdateTime:           dto.UpdateTime,
		},
		LocalAuthor: ToLocalAuthorDTO(dto.LocalAuthor),
	}
}

// ToRankedSiteAuthorWithWorkIdDTO 将 model.RankedSiteAuthorWithWorkId 转换为 RankedSiteAuthorWithWorkIdDTO
func ToRankedSiteAuthorWithWorkIdDTO(dto *model.RankedSiteAuthorWithWorkId) *RankedSiteAuthorWithWorkIdDTO {
	if dto == nil {
		return nil
	}
	return &RankedSiteAuthorWithWorkIdDTO{
		WorkId:       dto.WorkId,
		SiteAuthorID: stringPtrIfValid(dto.SiteAuthorID),
		AuthorName:   stringPtrIfValid(dto.AuthorName),
		Rank:         dto.AuthorRank,
	}
}

// ToLocalAuthorDTO 将 domain.LocalAuthor 转换为 LocalAuthorDTO
func ToLocalAuthorDTO(author *entity2.LocalAuthor) *LocalAuthorDTO {
	if author == nil {
		return nil
	}
	return &LocalAuthorDTO{
		ID:         author.GetID(),
		AuthorName: nullStringToPointer(author.AuthorName),
		Introduce:  nullStringToPointer(author.Introduce),
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

// stringPtrIfValid 将 string 转换为 *string（非空时返回指针）
func stringPtrIfValid(s string) *string {
	if s != "" {
		return &s
	}
	return nil
}

// int64PtrIfValid 将 int64 转换为 *int64（非零时返回指针）
func int64PtrIfValid(i int64) *int64 {
	if i != 0 {
		return &i
	}
	return nil
}
