package recycleBin

import (
	domain "github.com/library-squirrel/backend/base/model/entity"
	"github.com/library-squirrel/backend/util"
)

// RecycleItemDTO 回收站条目 DTO（列表展示用，不含快照明细）
type RecycleItemDTO struct {
	ID         int64   `json:"id"`
	WorkID     *int64  `json:"workId"`
	SiteID     *int64  `json:"siteId"`
	SiteWorkID *string `json:"siteWorkId"`
	WorkName   *string `json:"workName"`
	Thumbnail  *string `json:"thumbnail"`
	DeleteTime int64   `json:"deleteTime"`
}

// NewRecycleItemDTO 从实体创建 DTO
func NewRecycleItemDTO(item *domain.RecycleItem) *RecycleItemDTO {
	if item == nil {
		return nil
	}
	return &RecycleItemDTO{
		ID:         item.GetID(),
		WorkID:     util.NullInt64ToPointer(item.WorkID),
		SiteID:     util.NullInt64ToPointer(item.SiteID),
		SiteWorkID: util.NullStringToPointer(item.SiteWorkID),
		WorkName:   util.NullStringToPointer(item.WorkName),
		Thumbnail:  util.NullStringToPointer(item.Thumbnail),
		DeleteTime: item.DeleteTime,
	}
}
