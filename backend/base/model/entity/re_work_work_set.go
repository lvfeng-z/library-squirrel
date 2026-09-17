package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkWorkSet 作品与作品集关联
type ReWorkWorkSet struct {
	*model.BaseEntity
	WorkID        sql.NullInt64 `gorm:"column:work_id;uniqueIndex:idx_re_work_work_set_work_set;index:idx_re_work_work_set_set_order;" json:"workId"`
	WorkSetID     sql.NullInt64 `gorm:"column:work_set_id;uniqueIndex:idx_re_work_work_set_work_set;index:idx_re_work_work_set_set_order;" json:"workSetId"`
	SortOrder     sql.NullInt64 `gorm:"column:sort_order;index:idx_re_work_work_set_set_order;" json:"sortOrder"`
	SiteSortOrder sql.NullInt64 `gorm:"column:site_sort_order;index:idx_re_work_work_set_site_sort_order;" json:"siteSortOrder"`
	// Source 关联写入来源（constant.PLUGIN/MANUAL）：作品重拉只窄域重建插件来源关联（用户手动挂联/物理纳入
	// 复制的关联不动）；冲突 upsert 不写本列，先建行者的来源保持不变
	Source int64 `gorm:"column:source;not null;default:0" json:"source"`
}

func NewReWorkWorkSet() *ReWorkWorkSet {
	return &ReWorkWorkSet{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (ReWorkWorkSet) TableName() string {
	return "re_work_work_set"
}
