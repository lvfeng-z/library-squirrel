package model

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// LocalTag 本地标签
type LocalTag struct {
	*model.BaseEntity                // 嵌入基础实体
	LocalTagName      sql.NullString `gorm:"column:local_tag_name" json:"localTagName"`
	BaseLocalTagID    sql.NullInt64  `gorm:"column:base_local_tag_id" json:"baseLocalTagId"`
	LastUse           sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
}

func NewLocalTag() *LocalTag {
	return &LocalTag{
		BaseEntity: &model.BaseEntity{},
	}
}

// TableName 指定表名
func (LocalTag) TableName() string {
	return "local_tag"
}
