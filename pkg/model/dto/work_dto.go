package dto

import (
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// WorkFullDTO 作品完整信息DTO
type WorkFullDTO struct {
	// 基础字段
	ID                  int64  `json:"id"`
	CreateTime          int64  `json:"createTime"`
	UpdateTime          int64  `json:"updateTime"`
	SiteID              int64  `json:"siteId"`
	SiteWorkID          string `json:"siteWorkId"`
	SiteWorkName        string `json:"siteWorkName"`
	SiteAuthorID        string `json:"siteAuthorId"`
	SiteWorkDescription string `json:"siteWorkDescription"`
	SiteUploadTime      int64  `json:"siteUploadTime"`
	SiteUpdateTime      int64  `json:"siteUpdateTime"`
	NickName            string `json:"nickName"`
	LocalAuthorID       int64  `json:"localAuthorId"`
	LastView            int64  `json:"lastView"`

	// 关联的本地作者列表
	LocalAuthors []*LocalAuthorDTO `json:"localAuthors,omitempty"`

	// 关联的站点作者列表
	SiteAuthors []*SiteAuthorFullDTO `json:"siteAuthors,omitempty"`

	// 关联的站点信息
	Site *SiteDTO `json:"site,omitempty"`

	// 关联的本地标签列表
	LocalTags []*LocalTagDTO `json:"localTags,omitempty"`

	// 关联的站点标签列表
	SiteTags []*SiteTagFullDTO `json:"siteTags,omitempty"`

	// 关联的资源列表
	Resources []*ResourceDTO `json:"resources,omitempty"`
}

// NewWorkFullDTO 创建WorkFullDTO
func NewWorkFullDTO(work *entity2.Work) *WorkFullDTO {
	if work == nil {
		return nil
	}
	dto := &WorkFullDTO{
		ID:                  work.ID,
		CreateTime:          work.CreateTime,
		UpdateTime:          work.UpdateTime,
		SiteWorkID:          work.SiteWorkID.String,
		SiteWorkName:        work.SiteWorkName.String,
		SiteAuthorID:        work.SiteAuthorID.String,
		SiteWorkDescription: work.SiteWorkDescription.String,
		NickName:            work.NickName.String,
	}
	if work.SiteID.Valid {
		dto.SiteID = work.SiteID.Int64
	}
	if work.SiteUploadTime.Valid {
		dto.SiteUploadTime = work.SiteUploadTime.Int64
	}
	if work.SiteUpdateTime.Valid {
		dto.SiteUpdateTime = work.SiteUpdateTime.Int64
	}
	if work.LocalAuthorID.Valid {
		dto.LocalAuthorID = work.LocalAuthorID.Int64
	}
	if work.LastView.Valid {
		dto.LastView = work.LastView.Int64
	}
	return dto
}
