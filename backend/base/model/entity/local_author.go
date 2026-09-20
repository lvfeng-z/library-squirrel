package entity

import (
	"database/sql"

	"github.com/library-squirrel/backend/base/model"
)

// LocalAuthor 本地作者
type LocalAuthor struct {
	*model.BaseEntity
	AuthorName sql.NullString `gorm:"column:author_name" json:"authorName"`
	Introduce  sql.NullString `gorm:"column:introduce" json:"introduce"`
	LastUse    sql.NullInt64  `gorm:"column:last_use" json:"lastUse"`
	// AvatarStoreID 头像文件的 persistent_store 行内嵌引用（作者行发起入库，手动导入编排事务内写）；
	// 本地头像无站点来源，不设 source url 列
	AvatarStoreID sql.NullInt64 `gorm:"column:avatar_store_id" json:"avatarStoreId"`
}

func NewLocalAuthor() *LocalAuthor {
	return &LocalAuthor{
		BaseEntity: &model.BaseEntity{},
	}
}

func (LocalAuthor) TableName() string {
	return "local_author"
}
