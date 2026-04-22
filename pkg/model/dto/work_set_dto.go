package dto

import (
	"github.com/library-squirrel/wails/internal/util"
	entity2 "github.com/library-squirrel/wails/pkg/model/entity"
)

// WorkSetDTO 作品集数据传输对象（无 sql.Null* 版本）
type WorkSetDTO struct {
	ID                     int64   `json:"id"`
	SiteID                 *int64  `json:"siteId"`
	SiteWorkSetID          *string `json:"siteWorkSetId"`
	SiteWorkSetName        *string `json:"siteWorkSetName"`
	SiteAuthorID           *string `json:"siteAuthorId"`
	SiteWorkSetDescription *string `json:"siteWorkSetDescription"`
	SiteUploadTime         *int64  `json:"siteUploadTime"`
	SiteUpdateTime         *int64  `json:"siteUpdateTime"`
	NickName               *string `json:"nickName"`
	LastView               *int64  `json:"lastView"`
	CreateTime             int64   `json:"createTime"`
	UpdateTime             int64   `json:"updateTime"`
}

// NewWorkSetDTO 从 entity.WorkSet 创建 WorkSetDTO
func NewWorkSetDTO(workSet *entity2.WorkSet) *WorkSetDTO {
	if workSet == nil {
		return nil
	}
	return &WorkSetDTO{
		ID:                     workSet.GetID(),
		SiteID:                 util.NullInt64ToPointer(workSet.SiteID),
		SiteWorkSetID:          util.NullStringToPointer(workSet.SiteWorkSetID),
		SiteWorkSetName:        util.NullStringToPointer(workSet.SiteWorkSetName),
		SiteAuthorID:           util.NullStringToPointer(workSet.SiteAuthorID),
		SiteWorkSetDescription: util.NullStringToPointer(workSet.SiteWorkSetDescription),
		SiteUploadTime:         util.NullInt64ToPointer(workSet.SiteUploadTime),
		SiteUpdateTime:         util.NullInt64ToPointer(workSet.SiteUpdateTime),
		NickName:               util.NullStringToPointer(workSet.NickName),
		LastView:               util.NullInt64ToPointer(workSet.LastView),
		CreateTime:             workSet.GetCreateTime(),
		UpdateTime:             workSet.GetUpdateTime(),
	}
}
