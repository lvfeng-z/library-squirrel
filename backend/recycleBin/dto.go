package recycleBin

import (
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// RecycleItemDTO 回收站条目 DTO（列表展示用，不含快照明细）
type RecycleItemDTO struct {
	ID             int64   `json:"id"`
	WorkID         *int64  `json:"workId"`
	SiteID         *int64  `json:"siteId"`
	SiteName       string  `json:"siteName"` // 站点名（service 组装，列表展示）
	SiteWorkID     *string `json:"siteWorkId"`
	WorkName       *string `json:"workName"`
	AuthorNames    string  `json:"authorNames"`    // 作者名顿号拼接（service 组装；本地作者名优先，无本地关联回退站点作者名）
	WorkCreateTime *int64  `json:"workCreateTime"` // 原作品入库时间（快照采集该字段之前删除的条目为 null）
	Thumbnail      *string `json:"thumbnail"`
	DeleteTime     int64   `json:"deleteTime"`
}

// NewRecycleItemDTO 从实体创建 DTO（名字类字段由 service 组装填充）
func NewRecycleItemDTO(item *domain.RecycleItem) *RecycleItemDTO {
	if item == nil {
		return nil
	}
	return &RecycleItemDTO{
		ID:             item.GetID(),
		WorkID:         util.NullInt64ToPointer(item.WorkID),
		SiteID:         util.NullInt64ToPointer(item.SiteID),
		SiteWorkID:     util.NullStringToPointer(item.SiteWorkID),
		WorkName:       util.NullStringToPointer(item.WorkName),
		WorkCreateTime: util.NullInt64ToPointer(item.WorkCreateTime),
		Thumbnail:      util.NullStringToPointer(item.Thumbnail),
		DeleteTime:     item.DeleteTime,
	}
}
