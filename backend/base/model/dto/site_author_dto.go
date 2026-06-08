package dto

import (
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// NewSiteAuthorDTO 从 entity.SiteAuthor 创建 SiteAuthorDTO
func NewSiteAuthorDTO(author *entity2.SiteAuthor) *sdkdto.SiteAuthorDTO {
	if author == nil {
		return nil
	}
	return &sdkdto.SiteAuthorDTO{
		ID:                   author.GetID(),
		SiteID:               util.NullInt64ToPointer(author.SiteID),
		SiteAuthorID:         util.NullStringToPointer(author.SiteAuthorID),
		AuthorName:           util.NullStringToPointer(author.AuthorName),
		FixedAuthorName:      util.NullStringToPointer(author.FixedAuthorName),
		SiteAuthorNameBefore: util.NullStringToPointer(author.SiteAuthorNameBefore),
		Introduce:            util.NullStringToPointer(author.Introduce),
		Homepage:             util.NullStringToPointer(author.Homepage),
		LocalAuthorID:        util.NullInt64ToPointer(author.LocalAuthorID),
		LastUse:              util.NullInt64ToPointer(author.LastUse),
		CreateTime:           author.GetCreateTime(),
		UpdateTime:           author.GetUpdateTime(),
	}
}

// RankedSiteAuthorWithWorkIdDTO 带作品ID的排名站点作者DTO
type RankedSiteAuthorWithWorkIdDTO struct {
	WorkId       int64   `json:"workId"`
	SiteAuthorID *string `json:"siteAuthorId"`
	AuthorName   *string `json:"authorName"`
	Rank         int     `json:"rank"`
}

// SiteAuthorFullDTO 站点作者完整DTO（包含绑定的本地作者和来源站点信息）
type SiteAuthorFullDTO struct {
	SiteAuthor *sdkdto.SiteAuthorDTO `json:"siteAuthor,omitempty"`
	// 关联的本地作者
	LocalAuthor *sdkdto.LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *sdkdto.SiteDTO `json:"site,omitempty"`
}

// NewSiteAuthorFullDTO 创建站点作者完整DTO
func NewSiteAuthorFullDTO(siteAuthor *entity2.SiteAuthor) *sdkdto.SiteAuthorFullDTO {
	if siteAuthor == nil {
		return nil
	}
	return &sdkdto.SiteAuthorFullDTO{
		SiteAuthor: NewSiteAuthorDTO(siteAuthor),
	}
}

// SiteAuthorLocalRelateDTO 站点作者与本地作者关联DTO
type SiteAuthorLocalRelateDTO struct {
	SiteAuthor *sdkdto.SiteAuthorDTO `json:"siteAuthor,omitempty"`
	// 关联的本地作者
	LocalAuthor *sdkdto.LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *sdkdto.SiteDTO `json:"site,omitempty"`
	// 是否有同名本地作者
	HasSameNameLocalAuthor bool `json:"hasSameNameLocalAuthor"`
}

// NewSiteAuthorLocalRelateDTO 创建站点作者与本地作者关联DTO
func NewSiteAuthorLocalRelateDTO(siteAuthor *entity2.SiteAuthor) *sdkdto.SiteAuthorLocalRelateDTO {
	if siteAuthor == nil {
		return nil
	}
	return &sdkdto.SiteAuthorLocalRelateDTO{
		SiteAuthor: NewSiteAuthorDTO(siteAuthor),
	}
}

// ToSiteAuthorEntity 将 SiteAuthorDTO 转换为 SiteAuthor 实体
func ToSiteAuthorEntity(dto *sdkdto.SiteAuthorDTO) *entity2.SiteAuthor {
	if dto == nil {
		return nil
	}

	entity := entity2.NewSiteAuthor()

	// 设置基础字段
	if dto.ID != 0 {
		entity.SetID(dto.ID)
	}

	// 设置业务字段
	if dto.SiteID != nil {
		entity.SiteID.Valid = true
		entity.SiteID.Int64 = *dto.SiteID
	} else {
		entity.SiteID.Valid = false
	}

	if dto.SiteAuthorID != nil {
		entity.SiteAuthorID.Valid = true
		entity.SiteAuthorID.String = *dto.SiteAuthorID
	} else {
		entity.SiteAuthorID.Valid = false
	}

	if dto.AuthorName != nil {
		entity.AuthorName.Valid = true
		entity.AuthorName.String = *dto.AuthorName
	} else {
		entity.AuthorName.Valid = false
	}

	if dto.FixedAuthorName != nil {
		entity.FixedAuthorName.Valid = true
		entity.FixedAuthorName.String = *dto.FixedAuthorName
	} else {
		entity.FixedAuthorName.Valid = false
	}

	if dto.SiteAuthorNameBefore != nil {
		entity.SiteAuthorNameBefore.Valid = true
		entity.SiteAuthorNameBefore.String = *dto.SiteAuthorNameBefore
	} else {
		entity.SiteAuthorNameBefore.Valid = false
	}

	if dto.Introduce != nil {
		entity.Introduce.Valid = true
		entity.Introduce.String = *dto.Introduce
	} else {
		entity.Introduce.Valid = false
	}

	if dto.Homepage != nil {
		entity.Homepage.Valid = true
		entity.Homepage.String = *dto.Homepage
	} else {
		entity.Homepage.Valid = false
	}

	if dto.LocalAuthorID != nil {
		entity.LocalAuthorID.Valid = true
		entity.LocalAuthorID.Int64 = *dto.LocalAuthorID
	} else {
		entity.LocalAuthorID.Valid = false
	}

	// 设置时间字段（如果DTO中有值则使用，否则让Repository自动处理）
	if dto.CreateTime != 0 {
		entity.SetCreateTime(dto.CreateTime)
	}
	if dto.UpdateTime != 0 {
		entity.SetUpdateTime(dto.UpdateTime)
	}

	return entity
}

// RankedSiteAuthor 带排名的站点作者
type RankedSiteAuthor struct {
	Author     sdkdto.SiteAuthorDTO `json:"author"`
	AuthorRank int                  `json:"authorRank"`
}

// RankedSiteAuthorWithWorkId 带作品ID的站点作者
type RankedSiteAuthorWithWorkId struct {
	Author     sdkdto.SiteAuthorDTO `json:"author"`
	AuthorRank int                  `json:"authorRank"`
	WorkId     int64                `json:"workId"`
}
