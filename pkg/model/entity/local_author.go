package entity

import (
	"database/sql"

	"github.com/library-squirrel/wails/pkg/model"
)

// LocalAuthor 本地作者
type LocalAuthor struct {
	*model.BaseEntity
	AuthorName sql.NullString `gorm:"column:author_name" json:"authorName"`
	Introduce  sql.NullString `gorm:"column:introduce" json:"introduce"`
	LastUse    sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
}

func NewLocalAuthor() *LocalAuthor {
	return &LocalAuthor{
		BaseEntity: &model.BaseEntity{},
	}
}

func (LocalAuthor) TableName() string {
	return "local_author"
}
