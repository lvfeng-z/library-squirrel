package dto

import (
	"database/sql"

	entity2 "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

// SiteAuthorDTO 站点作者数据传输对象
type SiteAuthorDTO struct {
	ID                   int64   `json:"id"`
	SiteID               *int64  `json:"siteId"`
	SiteAuthorID         *string `json:"siteAuthorId"`
	AuthorName           *string `json:"authorName"`
	FixedAuthorName      *string `json:"fixedAuthorName"`
	SiteAuthorNameBefore *string `json:"siteAuthorNameBefore"`
	Introduce            *string `json:"introduce"`
	Homepage             *string `json:"homepage"`
	LocalAuthorID        *int64  `json:"localAuthorId"`
	LastUse              *int64  `json:"lastUse"`
	CreateTime           int64   `json:"createTime"`
	UpdateTime           int64   `json:"updateTime"`
}

// NewSiteAuthorDTO 从 entity.SiteAuthor 创建 SiteAuthorDTO
func NewSiteAuthorDTO(author *entity2.SiteAuthor) *SiteAuthorDTO {
	if author == nil {
		return nil
	}
	return &SiteAuthorDTO{
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

// SiteAuthorFullDTO 站点作者完整DTO（包含绑定的本地作者和来源站点信息）
type SiteAuthorFullDTO struct {
	SiteAuthor *SiteAuthorDTO `json:"siteAuthor,omitempty"`
	// 关联的本地作者
	LocalAuthor *sdkdto.LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *sdkdto.SiteDTO `json:"site,omitempty"`
}

// NewSiteAuthorFullDTO 创建站点作者完整DTO
func NewSiteAuthorFullDTO(siteAuthor *entity2.SiteAuthor) *SiteAuthorFullDTO {
	if siteAuthor == nil {
		return nil
	}
	return &SiteAuthorFullDTO{
		SiteAuthor: NewSiteAuthorDTO(siteAuthor),
	}
}

// SiteAuthorLocalRelateDTO 站点作者与本地作者关联DTO
type SiteAuthorLocalRelateDTO struct {
	SiteAuthor *SiteAuthorDTO `json:"siteAuthor,omitempty"`
	// 关联的本地作者
	LocalAuthor *sdkdto.LocalAuthorDTO `json:"localAuthor,omitempty"`
	// 来源站点
	Site *sdkdto.SiteDTO `json:"site,omitempty"`
	// 是否有同名本地作者
	HasSameNameLocalAuthor bool `json:"hasSameNameLocalAuthor"`
}

// NewSiteAuthorLocalRelateDTO 创建站点作者与本地作者关联DTO
func NewSiteAuthorLocalRelateDTO(siteAuthor *entity2.SiteAuthor) *SiteAuthorLocalRelateDTO {
	if siteAuthor == nil {
		return nil
	}
	return &SiteAuthorLocalRelateDTO{
		SiteAuthor: NewSiteAuthorDTO(siteAuthor),
	}
}

// RankedSiteAuthor 带排序的站点作者
type RankedSiteAuthor struct {
	Author    SiteAuthorDTO `json:"author"`
	RoleName  string        `json:"roleName"`
	SortOrder int           `json:"sortOrder"`
}

// RankedSiteAuthorWithWorkId 带作品ID的站点作者
type RankedSiteAuthorWithWorkId struct {
	Author    SiteAuthorDTO `json:"author"`
	RoleName  string        `json:"roleName"`
	SortOrder int           `json:"sortOrder"`
	WorkId    int64         `json:"workId"`
}

// SiteAuthorFetchTarget 站点作者信息拉取目标行（site_author JOIN site 的窄投影）：
// 拉取编排据此路由（siteKey + 站点侧作者 id）并做头像变更检测（存量来源 URL 与 store 行引用）。
// 由按 DB id 批量反查产出，目标集含跨站引用作者（站点键按行各自解析）
type SiteAuthorFetchTarget struct {
	// ID site_author 行 DB id（在途去重键、暂存作用域键）
	ID int64 `json:"id"`
	// SiteID 站点行 id（元数据回写 upsert 的复合键段）
	SiteID int64 `json:"siteId"`
	// SiteKey 站点身份键（能力广播路由的归属判据；空=站点行缺失，该行不可路由）
	SiteKey string `json:"siteKey"`
	// SiteAuthorID 站点侧作者 id（拉取请求参数）
	SiteAuthorID string `json:"siteAuthorId"`
	// AvatarSourceURL 存量头像来源 URL（变更检测比对键；NULL=从未回报）
	AvatarSourceURL sql.NullString `json:"avatarSourceUrl"`
	// AvatarStoreID 存量头像 persistent_store 行引用（NULL=无头像文件）
	AvatarStoreID sql.NullInt64 `json:"avatarStoreId"`
}

// ToSiteAuthorEntity 将 SiteAuthorDTO 转换为 SiteAuthor 实体
func ToSiteAuthorEntity(dto *SiteAuthorDTO) *entity2.SiteAuthor {
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
