package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// ReWorkSetWorkSet 作品集间父子关联（多父 DAG：一个子作品集可有多个父集，禁止成环）
// 逻辑关联 merge 的数据载体——B 作为 A 的子集 = 一行 (parent=A, child=B)
type ReWorkSetWorkSet struct {
	*model.BaseEntity
	ParentWorkSetID sql.NullInt64 `gorm:"column:parent_work_set_id;uniqueIndex:idx_re_work_set_work_set_pc;index:idx_re_work_set_work_set_parent;" json:"parentWorkSetId"`
	ChildWorkSetID  sql.NullInt64 `gorm:"column:child_work_set_id;uniqueIndex:idx_re_work_set_work_set_pc;index:idx_re_work_set_work_set_child;" json:"childWorkSetId"`
	SortOrder       sql.NullInt64 `gorm:"column:sort_order;index:idx_re_work_set_work_set_parent;" json:"sortOrder"`
}

func NewReWorkSetWorkSet() *ReWorkSetWorkSet {
	return &ReWorkSetWorkSet{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (ReWorkSetWorkSet) TableName() string {
	return "re_work_set_work_set"
}
