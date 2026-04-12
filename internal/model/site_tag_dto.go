package model

// SiteTagFullDTO 站点标签完整DTO（包含绑定的本地标签和来源站点信息）
// 注意：显式定义所有字段，不使用嵌入（embedding）来复现 TypeScript 的继承行为
type SiteTagFullDTO struct {
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
	LocalTag *LocalTag `json:"localTag,omitempty"`
	// 来源站点
	Site *Site `json:"site,omitempty"`
}

// NewSiteTagFullDTO 创建站点标签完整DTO
func NewSiteTagFullDTO(siteTag *SiteTag) *SiteTagFullDTO {
	if siteTag == nil {
		return nil
	}
	dto := &SiteTagFullDTO{
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
	LocalTag *LocalTag `json:"localTag,omitempty"`
	// 来源站点
	Site *Site `json:"site,omitempty"`
	// 是否有同名本地标签
	HasSameNameLocalTag bool `json:"hasSameNameLocalTag"`
}

// NewSiteTagLocalRelateDTO 创建站点标签与本地标签关联DTO
func NewSiteTagLocalRelateDTO(siteTag *SiteTag) *SiteTagLocalRelateDTO {
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
