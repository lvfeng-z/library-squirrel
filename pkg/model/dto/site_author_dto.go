package dto

import (
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// SiteAuthorFullDTO 站点作者完整DTO（包含绑定的本地作者和来源站点信息）
// 注意：显式定义所有字段，不使用嵌入（embedding）来复现 TypeScript 的继承行为
type SiteAuthorFullDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点作者字段
	SiteID               int64  `json:"siteId"`
	SiteAuthorID         string `json:"siteAuthorId"`
	AuthorName           string `json:"authorName"`
	FixedAuthorName      string `json:"fixedAuthorName"`
	SiteAuthorNameBefore string `json:"siteAuthorNameBefore"`
	Introduce            string `json:"introduce"`
	LocalAuthorID        int64  `json:"localAuthorId"`
	LastUse              int64  `json:"lastUse"`
	// 关联的本地作者
	LocalAuthor *entity2.LocalAuthor `json:"localAuthor,omitempty"`
	// 来源站点
	Site *entity2.Site `json:"site,omitempty"`
}

// NewSiteAuthorFullDTO 创建站点作者完整DTO
func NewSiteAuthorFullDTO(siteAuthor *entity2.SiteAuthor) *SiteAuthorFullDTO {
	if siteAuthor == nil {
		return nil
	}
	dto := &SiteAuthorFullDTO{
		ID:                   siteAuthor.ID,
		CreateTime:           siteAuthor.CreateTime,
		UpdateTime:           siteAuthor.UpdateTime,
		SiteAuthorID:         siteAuthor.SiteAuthorID.String,
		AuthorName:           siteAuthor.AuthorName.String,
		FixedAuthorName:      siteAuthor.FixedAuthorName.String,
		SiteAuthorNameBefore: siteAuthor.SiteAuthorNameBefore.String,
		Introduce:            siteAuthor.Introduce.String,
	}
	if siteAuthor.SiteID.Valid {
		dto.SiteID = siteAuthor.SiteID.Int64
	}
	if siteAuthor.LocalAuthorID.Valid {
		dto.LocalAuthorID = siteAuthor.LocalAuthorID.Int64
	}
	if siteAuthor.LastUse.Valid {
		dto.LastUse = siteAuthor.LastUse.Int64
	}
	return dto
}

// SiteAuthorLocalRelateDTO 站点作者与本地作者关联DTO
// 注意：显式定义所有字段，不使用嵌入
type SiteAuthorLocalRelateDTO struct {
	// 基础实体字段
	ID         int64 `json:"id"`
	CreateTime int64 `json:"createTime"`
	UpdateTime int64 `json:"updateTime"`
	// 站点作者字段
	SiteID               int64  `json:"siteId"`
	SiteAuthorID         string `json:"siteAuthorId"`
	AuthorName           string `json:"authorName"`
	FixedAuthorName      string `json:"fixedAuthorName"`
	SiteAuthorNameBefore string `json:"siteAuthorNameBefore"`
	Introduce            string `json:"introduce"`
	LocalAuthorID        int64  `json:"localAuthorId"`
	LastUse              int64  `json:"lastUse"`
	// 关联的本地作者
	LocalAuthor *entity2.LocalAuthor `json:"localAuthor,omitempty"`
	// 来源站点
	Site *entity2.Site `json:"site,omitempty"`
	// 是否有同名本地作者
	HasSameNameLocalAuthor bool `json:"hasSameNameLocalAuthor"`
}

// NewSiteAuthorLocalRelateDTO 创建站点作者与本地作者关联DTO
func NewSiteAuthorLocalRelateDTO(siteAuthor *entity2.SiteAuthor) *SiteAuthorLocalRelateDTO {
	if siteAuthor == nil {
		return nil
	}
	dto := &SiteAuthorLocalRelateDTO{
		ID:                   siteAuthor.ID,
		CreateTime:           siteAuthor.CreateTime,
		UpdateTime:           siteAuthor.UpdateTime,
		SiteAuthorID:         siteAuthor.SiteAuthorID.String,
		AuthorName:           siteAuthor.AuthorName.String,
		FixedAuthorName:      siteAuthor.FixedAuthorName.String,
		SiteAuthorNameBefore: siteAuthor.SiteAuthorNameBefore.String,
		Introduce:            siteAuthor.Introduce.String,
	}
	if siteAuthor.SiteID.Valid {
		dto.SiteID = siteAuthor.SiteID.Int64
	}
	if siteAuthor.LocalAuthorID.Valid {
		dto.LocalAuthorID = siteAuthor.LocalAuthorID.Int64
	}
	if siteAuthor.LastUse.Valid {
		dto.LastUse = siteAuthor.LastUse.Int64
	}
	return dto
}
