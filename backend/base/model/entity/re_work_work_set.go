package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkWorkSet 作品与作品集关联
type ReWorkWorkSet struct {
	*model.BaseEntity
	WorkID    sql.NullInt64 `gorm:"column:work_id;uniqueIndex:idx_re_work_work_set_work_set" json:"workId"`
	WorkSetID sql.NullInt64 `gorm:"column:work_set_id;uniqueIndex:idx_re_work_work_set_work_set;index:idx_re_work_work_set_set_order" json:"workSetId"`
	IsCover   sql.NullInt64 `gorm:"column:is_cover;uniqueIndex:idx_re_work_work_set_set_cover" json:"isCover"`
	SortOrder sql.NullInt64 `gorm:"column:sort_order" json:"sortOrder"`
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
