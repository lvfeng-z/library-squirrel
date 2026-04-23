package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// SiteTagFullDTO 站点标签完整DTO（包含绑定的本地标签和来源站点信息）
type SiteTagFullDTO struct {
	SiteTag  *SiteTagDTO  `json:"siteTag,omitempty"`
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
}

// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *entity2.SiteTag) *SiteTagFullDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagFullDTO{
		SiteTag: &SiteTagDTO{
			ID:            siteTag.GetID(),
			SiteID:        util.NullInt64ToPointer(siteTag.SiteID),
			SiteTagID:     util.NullStringToPointer(siteTag.SiteTagID),
			SiteTagName:   util.NullStringToPointer(siteTag.SiteTagName),
			BaseSiteTagID: util.NullStringToPointer(siteTag.BaseSiteTagID),
			Description:   util.NullStringToPointer(siteTag.Description),
			LocalTagID:    util.NullInt64ToPointer(siteTag.LocalTagID),
			LastUse:       util.NullInt64ToPointer(siteTag.LastUse),
			CreateTime:    siteTag.GetCreateTime(),
			UpdateTime:    siteTag.GetUpdateTime(),
		},
	}
	return dto
}

// SiteTagLocalRelateDTO 站点标签与本地标签关联DTO
// 注意：显式定义所有字段，不使用嵌入（embedding）来复现 TypeScript 的继承行为
type SiteTagLocalRelateDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点标签字段
	SiteID        int64  `json:"siteId"`
	SiteTagID     string `json:"siteTagId"`
	SiteTagName   string `json:"siteTagName"`
	BaseSiteTagID string `json:"baseSiteTagId"`
	Description   string `json:"description"`
	LocalTagID    int64  `json:"localTagId"`
	LastUse       int64  `json:"lastUse"`
	// 关联的本地标签
	LocalTag *LocalTagDTO `json:"localTag,omitempty"`
	// 来源站点
	Site *SiteDTO `json:"site,omitempty"`
	// 是否有同名本地标签
	HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}

// NewSiteTagLocalRelateDTO 创建站点标签与本地标签关联DTO
func NewSiteTagLocalRelateDTO(siteTag *entity2.SiteTag) *SiteTagLocalRelateDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagLocalRelateDTO{
		ID:            siteTag.ID,
		CreateTime:    siteTag.CreateTime,
		UpdateTime:    siteTag.UpdateTime,
		SiteTagID:     siteTag.SiteTagID.String,
		SiteTagName:   siteTag.SiteTagName.String,
		BaseSiteTagID: siteTag.BaseSiteTagID.String,
		Description:   siteTag.Description.String,
	}
	if siteTag.SiteID.Valid {
		dto.SiteID = siteTag.SiteID.Int64
	}
	if siteTag.LocalTagID.Valid {
		dto.LocalTagID = siteTag.LocalTagID.Int64
	}
	if siteTag.LastUse.Valid {
		dto.LastUse = siteTag.LastUse.Int64
	}
	return dto
}

// SiteTagDTO 站点标签数据传输对象（无 sql.Null* 版本）
type SiteTagDTO struct {
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

// NewSiteTagDTO 从 entity.SiteTag 创建 SiteTagDTO
func NewSiteTagDTO(tag *entity2.SiteTag) *SiteTagDTO {
	if tag == nil {
		return nil
	}
	return &SiteTagDTO{
		ID:            tag.GetID(),
		SiteID:        util.NullInt64ToPointer(tag.SiteID),
		SiteTagID:     util.NullStringToPointer(tag.SiteTagID),
		SiteTagName:   util.NullStringToPointer(tag.SiteTagName),
		BaseSiteTagID: util.NullStringToPointer(tag.BaseSiteTagID),
		Description:   util.NullStringToPointer(tag.Description),
		LocalTagID:    util.NullInt64ToPointer(tag.LocalTagID),
		LastUse:       util.NullInt64ToPointer(tag.LastUse),
		CreateTime:    tag.GetCreateTime(),
		UpdateTime:    tag.GetUpdateTime(),
	}
}

// SiteTagParamDTO 站点标签数据传输对象（增删改参数）
type SiteTagParamDTO struct {
	ID          int64   `json:"id"`
	SiteID      *int64  `json:"siteId"`
	SiteTagID   *string `json:"siteTagId"`
	SiteTagName *string `json:"siteTagName"`
	Description *string `json:"description"`
}
