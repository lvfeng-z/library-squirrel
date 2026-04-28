package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// WorkDTO 作品数据传输对象（无 sql.Null* 版本）
type WorkDTO struct {
	ID                  int64   `json:"id"`
	SiteID              *int64  `json:"siteId"`
	SiteWorkID          *string `json:"siteWorkId"`
	SiteWorkName        *string `json:"siteWorkName"`
	SiteAuthorID        *string `json:"siteAuthorId"`
	SiteWorkDescription *string `json:"siteWorkDescription"`
	SiteUploadTime      *int64  `json:"siteUploadTime"`
	SiteUpdateTime      *int64  `json:"siteUpdateTime"`
	NickName            *string `json:"nickName"`
	LocalAuthorID       *int64  `json:"localAuthorId"`
	LastView            *int64  `json:"lastView"`
	CreateTime          int64   `json:"createTime"`
	UpdateTime          int64   `json:"updateTime"`
}

// NewWorkDTO 从 entity.Work 创建 WorkDTO
func NewWorkDTO(work *entity2.Work) *WorkDTO {
	if work == nil {
		return nil
	}
	return &WorkDTO{
		ID:                  work.GetID(),
		SiteID:              util.NullInt64ToPointer(work.SiteID),
		SiteWorkID:          util.NullStringToPointer(work.SiteWorkID),
		SiteWorkName:        util.NullStringToPointer(work.SiteWorkName),
		SiteAuthorID:        util.NullStringToPointer(work.SiteAuthorID),
		SiteWorkDescription: util.NullStringToPointer(work.SiteWorkDescription),
		SiteUploadTime:      util.NullInt64ToPointer(work.SiteUploadTime),
		SiteUpdateTime:      util.NullInt64ToPointer(work.SiteUpdateTime),
		NickName:            util.NullStringToPointer(work.NickName),
		LocalAuthorID:       util.NullInt64ToPointer(work.LocalAuthorID),
		LastView:            util.NullInt64ToPointer(work.LastView),
		CreateTime:          work.GetCreateTime(),
		UpdateTime:          work.GetUpdateTime(),
	}
}

// WorkFullDTO 作品完整信息DTO
type WorkFullDTO struct {
	// 基础作品信息（组合 WorkDTO，避免嵌入实体）
	Work *WorkDTO `json:"work,omitempty"`

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
	return &WorkFullDTO{
		Work: NewWorkDTO(work),
	}
}

// ToWorkEntity 将 WorkDTO 转换为 Work 实体
func ToWorkEntity(dto *WorkDTO) *entity2.Work {
	if dto == nil {
		return nil
	}

	entity := entity2.NewWork()

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

	if dto.SiteWorkID != nil {
		entity.SiteWorkID.Valid = true
		entity.SiteWorkID.String = *dto.SiteWorkID
	} else {
		entity.SiteWorkID.Valid = false
	}

	if dto.SiteWorkName != nil {
		entity.SiteWorkName.Valid = true
		entity.SiteWorkName.String = *dto.SiteWorkName
	} else {
		entity.SiteWorkName.Valid = false
	}

	if dto.SiteAuthorID != nil {
		entity.SiteAuthorID.Valid = true
		entity.SiteAuthorID.String = *dto.SiteAuthorID
	} else {
		entity.SiteAuthorID.Valid = false
	}

	if dto.SiteWorkDescription != nil {
		entity.SiteWorkDescription.Valid = true
		entity.SiteWorkDescription.String = *dto.SiteWorkDescription
	} else {
		entity.SiteWorkDescription.Valid = false
	}

	if dto.SiteUploadTime != nil {
		entity.SiteUploadTime.Valid = true
		entity.SiteUploadTime.Int64 = *dto.SiteUploadTime
	} else {
		entity.SiteUploadTime.Valid = false
	}

	if dto.SiteUpdateTime != nil {
		entity.SiteUpdateTime.Valid = true
		entity.SiteUpdateTime.Int64 = *dto.SiteUpdateTime
	} else {
		entity.SiteUpdateTime.Valid = false
	}

	if dto.NickName != nil {
		entity.NickName.Valid = true
		entity.NickName.String = *dto.NickName
	} else {
		entity.NickName.Valid = false
	}

	if dto.LocalAuthorID != nil {
		entity.LocalAuthorID.Valid = true
		entity.LocalAuthorID.Int64 = *dto.LocalAuthorID
	} else {
		entity.LocalAuthorID.Valid = false
	}

	if dto.LastView != nil {
		entity.LastView.Valid = true
		entity.LastView.Int64 = *dto.LastView
	} else {
		entity.LastView.Valid = false
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
