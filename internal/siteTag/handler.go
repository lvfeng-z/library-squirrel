package siteTag

import (
	"context"
	"database/sql"

	domain "github.com/library-squirrel/wails/internal/model"
	"github.com/library-squirrel/wails/pkg/model"
)

// Handler 站点标签 Handler
type Handler struct {
	svc *Service
}

// NewHandler 创建站点标签 Handler
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// ========== 增删改操作 ==========

// Save 保存站点标签
func (h *Handler) Save(ctx context.Context, tag *SiteTagDTO) *model.ApiResponse[int64] {
	domainTag := &domain.SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
	if tag.SiteID != nil {
		domainTag.SiteID.Valid = true
		domainTag.SiteID.Int64 = *tag.SiteID
	}
	if tag.SiteTagID != nil {
		domainTag.SiteTagID.Valid = true
		domainTag.SiteTagID.String = *tag.SiteTagID
	}
	if tag.SiteTagName != nil {
		domainTag.SiteTagName.Valid = true
		domainTag.SiteTagName.String = *tag.SiteTagName
	}
	if tag.Description != nil {
		domainTag.Description.Valid = true
		domainTag.Description.String = *tag.Description
	}

	if err := h.svc.Save(ctx, domainTag); err != nil {
		return model.Error[int64](err.Error())
	}
	return model.Success(domainTag.GetID())
}

// SaveBatch 批量保存站点标签
func (h *Handler) SaveBatch(ctx context.Context, tags []*SiteTagDTO) *model.ApiResponse[any] {
	domainTags := make([]*domain.SiteTag, 0, len(tags))
	for _, tag := range tags {
		domainTag := &domain.SiteTag{
			BaseEntity: &model.BaseEntity{},
		}
		if tag.SiteID != nil {
			domainTag.SiteID.Valid = true
			domainTag.SiteID.Int64 = *tag.SiteID
		}
		if tag.SiteTagID != nil {
			domainTag.SiteTagID.Valid = true
			domainTag.SiteTagID.String = *tag.SiteTagID
		}
		if tag.SiteTagName != nil {
			domainTag.SiteTagName.Valid = true
			domainTag.SiteTagName.String = *tag.SiteTagName
		}
		if tag.Description != nil {
			domainTag.Description.Valid = true
			domainTag.Description.String = *tag.Description
		}
		domainTags = append(domainTags, domainTag)
	}

	if err := h.svc.SaveBatch(ctx, domainTags); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Delete 删除站点标签
func (h *Handler) Delete(ctx context.Context, id int64) *model.ApiResponse[any] {
	if err := h.svc.Delete(ctx, id); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// Update 更新站点标签
func (h *Handler) Update(ctx context.Context, tag *SiteTagDTO) *model.ApiResponse[any] {
	domainTag := &domain.SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
	domainTag.SetID(tag.ID)
	if tag.SiteID != nil {
		domainTag.SiteID.Valid = true
		domainTag.SiteID.Int64 = *tag.SiteID
	}
	if tag.SiteTagID != nil {
		domainTag.SiteTagID.Valid = true
		domainTag.SiteTagID.String = *tag.SiteTagID
	}
	if tag.SiteTagName != nil {
		domainTag.SiteTagName.Valid = true
		domainTag.SiteTagName.String = *tag.SiteTagName
	}
	if tag.Description != nil {
		domainTag.Description.Valid = true
		domainTag.Description.String = *tag.Description
	}

	if err := h.svc.UpdateById(ctx, domainTag); err != nil {
		return model.Error[any](err.Error())
	}
	return model.Success[any](nil)
}

// ========== 查询操作 ==========

// GetById 根据ID获取
func (h *Handler) GetById(ctx context.Context, id int64) *model.ApiResponse[*SiteTagResultDTO] {
	tag, err := h.svc.GetById(ctx, id)
	if err != nil {
		return model.Error[*SiteTagResultDTO](err.Error())
	}
	return model.Success(ToSiteTagResultDTO(tag))
}

// QueryPage 分页查询
func (h *Handler) QueryPage(ctx context.Context, page *model.Page[SiteTagResultDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[SiteTagResultDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteTagResultDTO, SiteTagQueryDTO]{}
	}
	result, err := h.svc.PageByDTO(ctx, page.PageNumber, page.PageSize, page.Query)
	if err != nil {
		return model.Error[*model.Page[SiteTagResultDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagResultDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, ToSiteTagResultDTO(tag))
	}
	return model.Success(&model.Page[SiteTagResultDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Data:         data,
	})
}

// QueryBoundOrUnboundToLocalTagPage 查询绑定或未绑定到本地标签的站点标签分页
func (h *Handler) QueryBoundOrUnboundToLocalTagPage(ctx context.Context, pageQuery model.Page[domain.SiteTagFullDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[SiteTagFullDTO, SiteTagQueryDTO]] {
	result, err := h.svc.QueryBoundOrUnboundToLocalTagPage(ctx, pageQuery)
	if err != nil {
		return model.Error[*model.Page[SiteTagFullDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagFullDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, ToSiteTagFullDTO(tag))
	}
	return model.Success(&model.Page[SiteTagFullDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// QueryLocalRelateDTOPage 查询站点标签与本地标签关联DTO分页
func (h *Handler) QueryLocalRelateDTOPage(ctx context.Context, page *model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]) *model.ApiResponse[*model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]{}
	}
	result, err := h.svc.QueryLocalRelateDTOPageByDTO(ctx, page.PageNumber, page.PageSize, page.Query, 0, nil)
	if err != nil {
		return model.Error[*model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagLocalRelateDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, ToSiteTagLocalRelateDTO(tag))
	}
	return model.Success(&model.Page[SiteTagLocalRelateDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// QueryPageByWorkId 根据作品ID分页查询站点标签
func (h *Handler) QueryPageByWorkId(ctx context.Context, page *model.Page[SiteTagFullDTO, SiteTagQueryDTO], workId int64) *model.ApiResponse[*model.Page[SiteTagFullDTO, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[SiteTagFullDTO, SiteTagQueryDTO]{}
	}
	result, err := h.svc.QueryPageByWorkIdByDTO(ctx, page.PageNumber, page.PageSize, page.Query, workId, nil)
	if err != nil {
		return model.Error[*model.Page[SiteTagFullDTO, SiteTagQueryDTO]](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagFullDTO, 0, len(result.Data))
	for _, tag := range result.Data {
		data = append(data, ToSiteTagFullDTO(tag))
	}
	return model.Success(&model.Page[SiteTagFullDTO, SiteTagQueryDTO]{
		PageNumber:   result.PageNumber,
		PageSize:     result.PageSize,
		PageCount:    result.PageCount,
		DataCount:    result.DataCount,
		CurrentCount: result.CurrentCount,
		Query:        result.Query,
		Data:         data,
	})
}

// ListBySiteTagIds 根据站点标签ID列表获取
func (h *Handler) ListBySiteTagIds(ctx context.Context, siteTagIds []int64) *model.ApiResponse[[]*SiteTagResultDTO] {
	result, err := h.svc.ListBySiteTagIds(ctx, siteTagIds)
	if err != nil {
		return model.Error[[]*SiteTagResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	data := make([]*SiteTagResultDTO, 0, len(result))
	for _, tag := range result {
		data = append(data, ToSiteTagResultDTO(tag))
	}
	return model.Success(data)
}

// UpdateBindLocalTag 更新绑定本地标签
func (h *Handler) UpdateBindLocalTag(ctx context.Context, localTagId int64, siteTagIds []int64) *model.ApiResponse[bool] {
	result, err := h.svc.UpdateBindLocalTag(ctx, localTagId, siteTagIds)
	if err != nil {
		return model.Error[bool](err.Error())
	}
	return model.Success(result)
}

// CreateAndBindSameNameLocalTag 创建并绑定同名本地标签
func (h *Handler) CreateAndBindSameNameLocalTag(ctx context.Context, siteTag *SiteTagDTO) *model.ApiResponse[*LocalTagDTO] {
	if siteTag.ID == 0 {
		return model.Error[*LocalTagDTO]("创建同名本地标签失败，标签ID不能为空")
	}
	if siteTag.SiteTagName == nil || *siteTag.SiteTagName == "" {
		return model.Error[*LocalTagDTO]("创建同名本地标签失败，标签名称不能为空")
	}

	domainTag := &domain.SiteTag{
		BaseEntity: &model.BaseEntity{},
	}
	domainTag.SetID(siteTag.ID)
	if siteTag.SiteTagName != nil {
		domainTag.SiteTagName.Valid = true
		domainTag.SiteTagName.String = *siteTag.SiteTagName
	}
	if siteTag.Description != nil {
		domainTag.Description.Valid = true
		domainTag.Description.String = *siteTag.Description
	}

	result, err := h.svc.CreateAndBindSameNameLocalTag(ctx, domainTag)
	if err != nil {
		return model.Error[*LocalTagDTO](err.Error())
	}
	return model.Success(ToLocalTagDTO(result))
}

// ListByWorkId 根据作品ID获取标签列表
func (h *Handler) ListByWorkId(ctx context.Context, workId int64) *model.ApiResponse[[]*SiteTagResultDTO] {
	result, err := h.svc.ListByWorkId(ctx, workId)
	if err != nil {
		return model.Error[[]*SiteTagResultDTO](err.Error())
	}
	// 转换为 ResultDTO
	resultDTOs := make([]*SiteTagResultDTO, len(result))
	for i, tag := range result {
		resultDTOs[i] = ToSiteTagResultDTO(tag)
	}
	return model.Success(resultDTOs)
}

// QuerySelectItemPageByWorkId 根据作品ID分页查询选择项
func (h *Handler) QuerySelectItemPageByWorkId(ctx context.Context, page *model.Page[domain.SelectItem, SiteTagQueryDTO], workId int64) *model.ApiResponse[*model.Page[domain.SelectItem, SiteTagQueryDTO]] {
	if page == nil {
		page = &model.Page[domain.SelectItem, SiteTagQueryDTO]{}
	}
	result, err := h.svc.QuerySelectItemPageByWorkIdByDTO(ctx, page.PageNumber, page.PageSize, page.Query, workId)
	if err != nil {
		return model.Error[*model.Page[domain.SelectItem, SiteTagQueryDTO]](err.Error())
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

// SiteTagDTO 站点标签数据传输对象
type SiteTagDTO struct {
	ID          int64   `json:"id"`
	SiteID      *int64  `json:"siteId"`
	SiteTagID   *string `json:"siteTagId"`
	SiteTagName *string `json:"siteTagName"`
	Description *string `json:"description"`
}

// SiteTagResultDTO 站点标签返回结果DTO（用于屏蔽sql.Null*类型）
type SiteTagResultDTO struct {
	ID            int64   `json:"id"`
	SiteID        *int64  `json:"siteId"`
	SiteTagID     *string `json:"siteTagId"`
	SiteTagName   *string `json:"siteTagName"`
	BaseSiteTagID *string `json:"baseSiteTagId"`
	Description   *string `json:"description"`
	LocalTagID    *int64  `json:"localTagId"`
	LastUse       *int64  `json:"lastUse"`
	CreateTime    int64   `json:"createTime"`
	UpdateTime    int64   `json:"updateTime"`
}

// SiteTagFullDTO 站点标签完整信息DTO
type SiteTagFullDTO struct {
	SiteTagResultDTO
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
}

// SiteTagLocalRelateDTO 站点标签与本地标签关联DTO
type SiteTagLocalRelateDTO struct {
	SiteTagResultDTO
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
}

// LocalTagDTO 本地标签数据传输对象
type LocalTagDTO struct {
	ID             int64   `json:"id"`
	LocalTagName   *string `json:"localTagName"`
	BaseLocalTagID *int64  `json:"baseLocalTagId"`
	Description    *string `json:"description"`
	CreateTime     int64   `json:"createTime"`
	UpdateTime     int64   `json:"updateTime"`
}

// ToSiteTagResultDTO 将 domain.SiteTag 转换为 SiteTagResultDTO
func ToSiteTagResultDTO(tag *domain.SiteTag) *SiteTagResultDTO {
	if tag == nil {
		return nil
	}
	return &SiteTagResultDTO{
		ID:            tag.GetID(),
		SiteID:        nullInt64ToPointer(tag.SiteID),
		SiteTagID:     nullStringToPointer(tag.SiteTagID),
		SiteTagName:   nullStringToPointer(tag.SiteTagName),
		BaseSiteTagID: nullStringToPointer(tag.BaseSiteTagID),
		Description:   nullStringToPointer(tag.Description),
		LocalTagID:    nullInt64ToPointer(tag.LocalTagID),
		LastUse:       nullInt64ToPointer(tag.LastUse),
		CreateTime:    tag.GetCreateTime(),
		UpdateTime:    tag.GetUpdateTime(),
	}
}

// ToSiteTagFullDTO 将 domain.SiteTagFullDTO 转换为 SiteTagFullDTO
func ToSiteTagFullDTO(dto *domain.SiteTagFullDTO) *SiteTagFullDTO {
	if dto == nil {
		return nil
	}
	return &SiteTagFullDTO{
		SiteTagResultDTO: SiteTagResultDTO{
			ID:            dto.ID,
			SiteID:        int64PtrIfValid(dto.SiteID),
			SiteTagID:     stringPtrIfValid(dto.SiteTagID),
			SiteTagName:   stringPtrIfValid(dto.SiteTagName),
			BaseSiteTagID: stringPtrIfValid(dto.BaseSiteTagID),
			Description:   stringPtrIfValid(dto.Description),
			LocalTagID:    int64PtrIfValid(dto.LocalTagID),
			LastUse:       int64PtrIfValid(dto.LastUse),
			CreateTime:    dto.CreateTime,
			UpdateTime:    dto.UpdateTime,
		},
		LocalTag: ToLocalTagDTO(dto.LocalTag),
	}
}

// ToSiteTagLocalRelateDTO 将 domain.SiteTagLocalRelateDTO 转换为 SiteTagLocalRelateDTO
func ToSiteTagLocalRelateDTO(dto *domain.SiteTagLocalRelateDTO) *SiteTagLocalRelateDTO {
	if dto == nil {
		return nil
	}
	return &SiteTagLocalRelateDTO{
		SiteTagResultDTO: SiteTagResultDTO{
			ID:            dto.ID,
			SiteID:        int64PtrIfValid(dto.SiteID),
			SiteTagID:     stringPtrIfValid(dto.SiteTagID),
			SiteTagName:   stringPtrIfValid(dto.SiteTagName),
			BaseSiteTagID: stringPtrIfValid(dto.BaseSiteTagID),
			Description:   stringPtrIfValid(dto.Description),
			LocalTagID:    int64PtrIfValid(dto.LocalTagID),
			LastUse:       int64PtrIfValid(dto.LastUse),
			CreateTime:    dto.CreateTime,
			UpdateTime:    dto.UpdateTime,
		},
		LocalTag: ToLocalTagDTO(dto.LocalTag),
	}
}

// ToLocalTagDTO 将 domain.LocalTag 转换为 LocalTagDTO
func ToLocalTagDTO(tag *domain.LocalTag) *LocalTagDTO {
	if tag == nil {
		return nil
	}
	return &LocalTagDTO{
		ID:             tag.GetID(),
		LocalTagName:   nullStringToPointer(tag.LocalTagName),
		BaseLocalTagID: nullInt64ToPointer(tag.BaseLocalTagID),
		Description:    nil,
		CreateTime:     tag.GetCreateTime(),
		UpdateTime:     tag.GetUpdateTime(),
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
