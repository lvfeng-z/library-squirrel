package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// SiteTagFullDTO 站点标签完整DTO（包含绑定的本地标签和来源站点信息）
type SiteTagFullDTO struct {
	SiteTag  SiteTagResultDTO `json:"siteTag,omitempty"`
	LocalTag *LocalTagDTO     `json:"localTag,omitempty"`
	// 来源站点
	Site *entity2.Site `json:"site,omitempty"`
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

// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *entity2.SiteTag) *SiteTagFullDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagFullDTO{
		SiteTag: SiteTagResultDTO{
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
// 注意：显式定义所有字段，不使用嵌入
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
	LocalTag *entity2.LocalTag `json:"localTag,omitempty"`
	// 来源站点
	Site *entity2.Site `json:"site,omitempty"`
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
